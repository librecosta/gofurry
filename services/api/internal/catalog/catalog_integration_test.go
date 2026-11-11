//go:build integration

package catalog

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var cnFixture = flag.String("catalog-cn-config", "", "Explicit empty disposable CN database YAML")
var intlFixture = flag.String("catalog-intl-config", "", "Explicit empty disposable INTL database YAML")

const (
	fixtureSite = "000000000000000a"
	fixtureGame = "000000000000000b"
	draftSite   = "000000000000000c"
	otherSite   = "000000000000000d"
	otherGame   = "000000000000000e"
	archiveGame = "000000000000000f"
)

func TestCatalogPostgreSQL(t *testing.T) {
	if *cnFixture == "" || *intlFixture == "" {
		t.Skip("explicit independent empty catalog fixtures required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	cn := openFixture(t, ctx, *cnFixture, "CN")
	intl := openFixture(t, ctx, *intlFixture, "INTL")
	c := New(cn)
	projected := New(intl)

	editorial := Editorial{Slug: "original-slug", Localizations: []Localization{
		{Locale: "zh", Name: "示例社区", Summary: "中文编辑简介。"},
		{Locale: "en", Name: "Example Community", Summary: "A curated English summary."},
	}}
	siteInput := SiteInput{Editorial: editorial, Addresses: []Address{
		{Kind: "primary", URL: "https://EXAMPLE.COM:443/communities/Main?view=all&sort=A"},
		{Kind: "alternate", URL: "https://mirror.example.com/archive"},
	}}

	c.publicID = func() string { return fixtureSite }
	site, err := c.CreateSite(ctx, siteInput)
	ok(t, err)
	if site.Status != Draft || site.Revision != 1 || site.ID.String()[14] != '7' {
		t.Fatal("wrong initial Site identity/lifecycle")
	}
	_, err = c.PublishedSite(ctx, fixtureSite)
	want(t, err, ErrNotFound)
	site, err = c.SetSiteStatus(ctx, site.ID, site.Revision, Published)
	ok(t, err)
	if !site.PublishedAt.Valid {
		t.Fatal("missing publication timestamp")
	}

	updatedInput := siteInput
	updatedInput.Slug = "example-community"
	updatedInput.Addresses = slices.Clone(siteInput.Addresses)
	updatedInput.Addresses[0].URL = "https://example.com/new/Community?tag=art&tag=games"
	oldRevision := site.Revision
	site, err = c.EditSite(ctx, site.ID, oldRevision, updatedInput)
	ok(t, err)
	if site.PublicID != fixtureSite || site.Slug != "example-community" {
		t.Fatal("address or slug edit changed identity")
	}
	_, err = c.EditSite(ctx, site.ID, oldRevision, siteInput)
	want(t, err, ErrConflict)
	broken := updatedInput
	broken.Addresses = nil
	_, err = c.EditSite(ctx, site.ID, site.Revision, broken)
	want(t, err, ErrInvalid)
	read, err := c.PublishedSite(ctx, fixtureSite)
	ok(t, err)
	if len(read.Addresses) != 2 || read.Addresses[0].URL != updatedInput.Addresses[0].URL || len(read.Localizations) != 2 {
		t.Fatal("site edits, primary and alternate addresses failed")
	}
	var originalAddress string
	ok(t, cn.QueryRow(ctx, "SELECT url FROM site_addresses WHERE site_id=$1 AND NOT active", site.ID).Scan(&originalAddress))
	if originalAddress != "https://example.com/communities/Main?view=all&sort=A" {
		t.Fatal("meaningful path/query was lost")
	}

	calls := 0
	c.publicID = func() string {
		calls++
		if calls == 1 {
			return fixtureSite
		}
		return otherSite
	}
	second, err := c.CreateSite(ctx, updatedInput)
	ok(t, err)
	if calls != 2 || second.ID == site.ID || second.PublicID != otherSite || second.Slug != site.Slug {
		t.Fatal("Public ID retry or nonunique slug failed")
	}
	_, err = c.SetSiteStatus(ctx, second.ID, second.Revision, Published)
	ok(t, err)

	c.publicID = func() string { return draftSite }
	hidden, err := c.CreateSite(ctx, SiteInput{Editorial: Editorial{Slug: "private-draft", Localizations: []Localization{{Locale: "en", Name: "Hidden Site"}}}})
	ok(t, err)
	_, err = c.SetSiteStatus(ctx, hidden.ID, hidden.Revision, Published)
	want(t, err, ErrInvalid)
	firstPage, err := c.SitePage(ctx, "", 1)
	ok(t, err)
	nextPage, err := c.SitePage(ctx, firstPage.After, 1)
	ok(t, err)
	if len(firstPage.Items) != 1 || firstPage.Items[0].PublicID != fixtureSite || firstPage.After != fixtureSite ||
		len(nextPage.Items) != 1 || nextPage.Items[0].PublicID != otherSite || nextPage.After != "" {
		t.Fatal("published page cursor missed/duplicated content or exposed a draft")
	}

	// A second writer for the same revision cannot overwrite the first.
	outcomes := make(chan error, 2)
	var writers sync.WaitGroup
	for range 2 {
		writers.Go(func() {
			_, editErr := c.EditSite(ctx, site.ID, site.Revision, updatedInput)
			outcomes <- editErr
		})
	}
	writers.Wait()
	close(outcomes)
	wins, conflicts := 0, 0
	for result := range outcomes {
		switch {
		case result == nil:
			wins++
		case errors.Is(result, ErrConflict):
			conflicts++
		default:
			t.Fatal(result)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal("concurrent revision check failed")
	}

	c.publicID = func() string { return "1234567890123456" }
	decimalSite, err := c.CreateSite(ctx, updatedInput)
	ok(t, err)
	_, err = c.SetSiteStatus(ctx, decimalSite.ID, decimalSite.Revision, Published)
	ok(t, err)

	gameInput := GameInput{Editorial: Editorial{Slug: "orbit-game", Localizations: []Localization{
		{Locale: "en", Name: "Orbit Game", Summary: "The original English description."},
	}}, Sources: []Source{
		{PlatformCode: "steam", ExternalKey: "000042"},
		{PlatformCode: "itch_io", ExternalKey: "orbit/demo", SourceURL: "https://orbit.itch.io/demo"},
	}}
	c.publicID = func() string { return fixtureGame }
	game, err := c.CreateGame(ctx, gameInput)
	ok(t, err)
	game, err = c.SetGameStatus(ctx, game.ID, game.Revision, Published)
	ok(t, err)
	gameRead, err := c.PublishedGame(ctx, fixtureGame)
	ok(t, err)
	if len(gameRead.Sources) != 2 || gameRead.Sources[1].ExternalKey != "42" {
		t.Fatal("game source normalization failed")
	}
	manualOnly := gameInput
	manualOnly.Sources = gameInput.Sources[1:]
	game, err = c.EditGame(ctx, game.ID, game.Revision, manualOnly)
	ok(t, err)
	gameRead, err = c.PublishedGame(ctx, fixtureGame)
	ok(t, err)
	if game.PublicID != fixtureGame || len(gameRead.Sources) != 1 {
		t.Fatal("Steam removal changed identity or remained public")
	}

	c.publicID = func() string { return otherGame }
	_, err = c.CreateGame(ctx, gameInput)
	want(t, err, ErrConflict)
	noSource, err := c.CreateGame(ctx, GameInput{Editorial: Editorial{Localizations: []Localization{{Locale: "en", Name: "Offline Game"}}}})
	ok(t, err)
	_, err = c.SetGameStatus(ctx, noSource.ID, noSource.Revision, Published)
	ok(t, err)
	game, err = c.EditGame(ctx, game.ID, game.Revision, gameInput)
	ok(t, err)

	c.publicID = func() string { return archiveGame }
	archived, err := c.CreateGame(ctx, GameInput{Editorial: gameInput.Editorial})
	ok(t, err)
	archived, err = c.SetGameStatus(ctx, archived.ID, archived.Revision, Published)
	ok(t, err)
	archived, err = c.SetGameStatus(ctx, archived.ID, archived.Revision, Archived)
	ok(t, err)
	if !archived.PublishedAt.Valid || !archived.ArchivedAt.Valid || archived.PublicID != archiveGame {
		t.Fatal("archival lost published history or identity")
	}
	_, err = c.PublishedGame(ctx, archiveGame)
	want(t, err, ErrNotFound)
	_, err = c.EditGame(ctx, archived.ID, archived.Revision, gameInput)
	want(t, err, ErrConflict)

	// Explicit fixture assignment represents a future approved projection.
	projected.publicID = func() string { return fixtureSite }
	intlSite, err := projected.CreateSite(ctx, updatedInput)
	ok(t, err)
	intlSite, err = projected.SetSiteStatus(ctx, intlSite.ID, intlSite.Revision, Published)
	ok(t, err)
	projected.publicID = func() string { return fixtureGame }
	intlGame, err := projected.CreateGame(ctx, gameInput)
	ok(t, err)
	intlGame, err = projected.SetGameStatus(ctx, intlGame.ID, intlGame.Revision, Published)
	ok(t, err)
	if intlSite.ID == site.ID || intlGame.ID == game.ID || intlSite.PublicID != site.PublicID || intlGame.PublicID != game.PublicID {
		t.Fatal("separate regional keys and shared public identity were not retained")
	}
	_, err = projected.PublishedGame(ctx, otherGame)
	want(t, err, ErrNotFound)

	_, err = cn.Exec(ctx, "UPDATE sites SET public_id='000000000000000z' WHERE id=$1", site.ID)
	pgCode(t, err, "23514")
	_, err = cn.Exec(ctx, "UPDATE games SET id=$2 WHERE id=$1", game.ID, uuid.NewV7())
	pgCode(t, err, "23514")
	_, err = cn.Exec(ctx, "INSERT INTO site_addresses (id,site_id,url,kind) VALUES ($1,$2,'https://another.example.com','primary')", uuid.NewV7(), site.ID)
	pgCode(t, err, "23505")
	_, err = cn.Exec(ctx, "INSERT INTO game_localizations VALUES ($1,'en','orphan','')", uuid.NewV7())
	pgCode(t, err, "23503")
	_, err = cn.Exec(ctx, "DELETE FROM games WHERE id=$1", game.ID)
	pgCode(t, err, "23001")
}

func openFixture(t *testing.T, ctx context.Context, path, region string) *pgxpool.Pool {
	t.Helper()
	cfg, err := platform.LoadConfig(path)
	ok(t, err)
	d := cfg.Database
	if cfg.Deployment.Region != region || cfg.Server.Environment != "development" || d == nil || d.Host != "127.0.0.1" ||
		d.Name != "gofurry_d1_"+strings.ToLower(region)+"_test" || d.User != "postgres" || d.Port == 5432 ||
		cfg.Storage != nil || cfg.Redis != nil || cfg.Management != nil {
		t.Fatal("not an isolated catalog fixture")
	}
	pool, err := database.Open(ctx, *d)
	ok(t, err)
	t.Cleanup(pool.Close)
	var version, tables int
	ok(t, pool.QueryRow(ctx, "SELECT current_setting('server_version_num')::integer, count(*)::integer FROM information_schema.tables WHERE table_schema='public'").Scan(&version, &tables))
	if version < 180000 || version >= 190000 || tables != 0 {
		t.Fatal("fixture must be an empty PostgreSQL 18 database")
	}
	const migrations = "../../migrations"
	ok(t, database.Migrate(ctx, *d, migrations, "status", io.Discard))
	ok(t, pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&tables))
	if tables != 0 {
		t.Fatal("migration status wrote schema")
	}
	wallpapers := t.TempDir()
	for _, name := range []string{"202611050001_wallpapers.sql", "202611050002_official_wallpaper_format.sql"} {
		data, err := os.ReadFile(filepath.Join(migrations, name))
		ok(t, err)
		ok(t, os.WriteFile(filepath.Join(wallpapers, name), data, 0600))
	}
	ok(t, database.Migrate(ctx, *d, wallpapers, "up", io.Discard))
	wallpaper := uuid.NewV7()
	_, err = pool.Exec(ctx, `INSERT INTO start_wallpapers (
		id,title,status,image_key,image_sha256,image_type,image_bytes,width,height,
		preview_key,preview_sha256,preview_bytes,preview_width,preview_height,published_at)
		VALUES ($1,'Existing wallpaper','published','start/backgrounds/'||repeat('a',64)||'.avif',
		repeat('a',64),'image/avif',1024,1920,1080,'start/backgrounds/'||repeat('b',64)||'.webp',
		repeat('b',64),128,480,270,clock_timestamp())`, wallpaper)
	ok(t, err)
	ok(t, database.Migrate(ctx, *d, migrations, "up", io.Discard))
	ok(t, database.Migrate(ctx, *d, migrations, "up", io.Discard))
	var title string
	ok(t, pool.QueryRow(ctx, "SELECT title FROM start_wallpapers WHERE id=$1 AND revision=1", wallpaper).Scan(&title))
	if title != "Existing wallpaper" {
		t.Fatal("forward migration changed existing wallpaper")
	}
	return pool
}

func ok(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func want(t *testing.T, got, expected error) {
	t.Helper()
	if !errors.Is(got, expected) {
		t.Fatalf("wanted %v, got %v", expected, got)
	}
}

func pgCode(t *testing.T, err error, expected string) {
	t.Helper()
	var postgres *pgconn.PgError
	if !errors.As(err, &postgres) || postgres.Code != expected {
		t.Fatalf("wanted SQLSTATE %s, got %v", expected, err)
	}
}
