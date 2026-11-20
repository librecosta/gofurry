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
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
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
	approve := func(editor *Service, region, id string, rev int64) int64 {
		t.Helper()
		profile, err := editor.ReviewSiteNavigation(ctx, region, id, rev,
			NavigationEdit{Rating: "sfw", Primary: "community", Actor: "fixture", Reason: "Reviewed Site fixture"})
		ok(t, err)
		return profile.Revision
	}

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
	_, err = c.SetSiteStatus(ctx, site.ID, site.Revision, Published)
	want(t, err, ErrInvalid)
	site, err = c.SetSiteStatus(ctx, site.ID, approve(c, "CN", site.PublicID, site.Revision), Published)
	ok(t, err)
	if !site.PublishedAt.Valid {
		t.Fatal("missing publication timestamp")
	}
	zhBody := strings.Repeat("站点正文与示例。", 300)
	revision, err := c.SetSiteDescription(ctx, fixtureSite, site.Revision, "zh", zhBody)
	ok(t, err)
	_, err = c.SetSiteDescription(ctx, fixtureSite, site.Revision, "en", "stale")
	want(t, err, ErrConflict)
	_, err = c.SetSiteDescription(ctx, fixtureSite, revision, "en", "<script>invalid</script>")
	want(t, err, ErrInvalid)
	_, err = c.SetSiteDescription(ctx, fixtureSite, revision, "en", strings.Repeat("文", SiteDescriptionLimit/3+1))
	want(t, err, ErrInvalid)
	site.Revision, err = c.SetSiteDescription(ctx, fixtureSite, revision, "en", "First paragraph.\r\n\r\nSecond paragraph.")
	ok(t, err)

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
	if read.Localizations[0].Body != "First paragraph.\n\nSecond paragraph." || read.Localizations[1].Body != zhBody {
		t.Fatal("ordinary Site edit lost reviewed localized body")
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
	_, err = c.SetSiteStatus(ctx, second.ID, approve(c, "CN", second.PublicID, second.Revision), Published)
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
	_, err = c.SetSiteStatus(ctx, decimalSite.ID, approve(c, "CN", decimalSite.PublicID, decimalSite.Revision), Published)
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
	preview, err := c.ManagementGame(ctx, game.PublicID)
	ok(t, err)
	operation := GamePublicationReview{Revision: game.Revision, ConfirmRelevance: true, ContentRating: "sfw", PreviewToken: preview.PreviewToken}
	_, err = c.PublishManagementGame(ctx, game.PublicID, operation)
	want(t, err, ErrGamePublicationIdentity)
	_, err = c.Manage(ctx, dbgen.RecordManagementOperationParams{
		RequestID: uuid.NewV7(), ActorID: uuid.NewV7(), ActorName: "Developer fixture", ActorRole: "developer", Action: "game.publish",
	}, func(ctx context.Context, tx *Service, _ *dbgen.Queries) (ManagementResult, error) {
		return tx.PublishManagementGame(ctx, game.PublicID, operation)
	})
	want(t, err, ErrGamePublicationIdentity)

	game, err = publishCatalogGame(t, ctx, c, game)
	ok(t, err)
	gameRead, err := c.PublishedGame(ctx, fixtureGame)
	ok(t, err)
	if len(gameRead.Sources) != 2 || gameRead.Sources[1].ExternalKey != "42" {
		t.Fatal("game source normalization failed")
	}
	manualOnly := gameInput
	manualOnly.Sources = gameInput.Sources[1:]
	_, err = c.EditGame(ctx, game.ID, game.Revision, manualOnly)
	want(t, err, ErrConflict)
	game, err = changeCatalogGame(t, ctx, c, game, "unpublish", GamePublicationReview{})
	ok(t, err)
	game, err = c.EditGame(ctx, game.ID, game.Revision, manualOnly)
	ok(t, err)
	game, err = publishCatalogGame(t, ctx, c, game)
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
	_, err = publishCatalogGame(t, ctx, c, noSource)
	ok(t, err)
	game, err = changeCatalogGame(t, ctx, c, game, "unpublish", GamePublicationReview{})
	ok(t, err)
	game, err = c.EditGame(ctx, game.ID, game.Revision, gameInput)
	ok(t, err)
	game, err = publishCatalogGame(t, ctx, c, game)
	ok(t, err)

	c.publicID = func() string { return archiveGame }
	archived, err := c.CreateGame(ctx, GameInput{Editorial: gameInput.Editorial})
	ok(t, err)
	archived, err = publishCatalogGame(t, ctx, c, archived)
	ok(t, err)
	archived, err = changeCatalogGame(t, ctx, c, archived, "archive", GamePublicationReview{})
	ok(t, err)
	if !archived.PublishedAt.Valid || !archived.ArchivedAt.Valid || archived.PublicID != archiveGame {
		t.Fatal("archival lost published history or identity")
	}
	_, err = c.PublishedGame(ctx, archiveGame)
	want(t, err, ErrNotFound)
	_, err = c.EditGame(ctx, archived.ID, archived.Revision, gameInput)
	want(t, err, ErrConflict)
	testGameDraftPublication(t, c)

	// Explicit fixture assignment represents a future approved projection.
	projected.publicID = func() string { return fixtureSite }
	intlSite, err := projected.CreateSite(ctx, updatedInput)
	ok(t, err)
	intlSite, err = projected.SetSiteStatus(ctx, intlSite.ID, approve(projected, "INTL", intlSite.PublicID, intlSite.Revision), Published)
	ok(t, err)
	projected.publicID = func() string { return fixtureGame }
	intlGame, err := projected.CreateGame(ctx, gameInput)
	ok(t, err)
	intlGame, err = publishCatalogGame(t, ctx, projected, intlGame)
	ok(t, err)
	if intlSite.ID == site.ID || intlGame.ID == game.ID || intlSite.PublicID != site.PublicID || intlGame.PublicID != game.PublicID {
		t.Fatal("separate regional keys and shared public identity were not retained")
	}
	_, err = projected.PublishedGame(ctx, otherGame)
	want(t, err, ErrNotFound)

	for _, editor := range []*Service{c, projected} {
		seedIntakeBrowserGame(t, ctx, editor, "000000000000000g", "40001", "Fixture Starlight", false)
		seedIntakeBrowserGame(t, ctx, editor, "000000000000000h", "40002", "Fixture Hidden", true)
		editor.publicID = func() string { return "000000000000000j" }
		input := SiteInput{
			Editorial: Editorial{Localizations: []Localization{
				{Locale: "en", Name: "Unsorted Almanac", Summary: "A bilingual index."},
				{Locale: "zh", Name: "无类手册", Summary: "双语索引。"},
			}},
			Addresses: []Address{{Kind: "primary", URL: "https://unsorted.example.com"}},
		}
		pending, err := editor.CreateSite(ctx, input)
		ok(t, err)
		decision := PublicationDecision{Actor: "fixture", Reason: "Reviewed ordinary reference material"}
		_, err = editor.ChangeSitePublication(ctx, pending.PublicID, pending.Revision, "publish", false, decision)
		want(t, err, ErrInvalid)
		published, err := editor.ChangeSitePublication(ctx, pending.PublicID, pending.Revision, "publish", true, decision)
		ok(t, err)
		if published.PrimaryCategory.Valid || len(published.NavigationTags) != 0 {
			t.Fatal("publishing invented optional taxonomy")
		}
		_, err = editor.SetNavigationFeature(ctx, "secondary_2", published.PublicID, 0, decision.Actor, decision.Reason)
		ok(t, err)
		archived, err := editor.ChangeSitePublication(ctx, published.PublicID, published.Revision, "archive", false, decision)
		ok(t, err)
		_, err = editor.PublishedSite(ctx, published.PublicID)
		want(t, err, ErrNotFound)
		restored, err := editor.ChangeSitePublication(ctx, published.PublicID, archived.Revision, "restore", false, decision)
		ok(t, err)
		if restored.PublicID != published.PublicID || !restored.FirstPublishedAt.Time.Equal(published.FirstPublishedAt.Time) {
			t.Fatal("Site restore changed its identity or first publication date")
		}
	}
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

func testGameDraftPublication(t *testing.T, s *Service) {
	t.Helper()
	ctx := t.Context()
	s.publicID = func() string { return "00000000000000b1" }
	text := []Localization{{Locale: "en", Name: "Old copy", Summary: "Old summary"}, {Locale: "zh", Name: "旧版中文"}}
	g, err := s.CreateGame(ctx, GameInput{Editorial: Editorial{Slug: "old-slug", Localizations: text},
		Sources: []Source{{PlatformCode: "steam", ExternalKey: "40101"}}})
	ok(t, err)
	var source uuid.UUID
	ok(t, s.pool.QueryRow(ctx, "SELECT id FROM game_sources WHERE game_id=$1", g.ID).Scan(&source))
	oldMedia, newMedia, observed := uuid.NewV7(), uuid.NewV7(), uuid.NewV7()
	const oldCover = "https://cdn.steamstatic.com/fixture/old.jpg"
	const newCover = "https://cdn.steamstatic.com/fixture/new.jpg"
	putFact := func(id uuid.UUID, section, payload string) {
		t.Helper()
		_, err := s.pool.Exec(ctx, `INSERT INTO game_source_metadata(game_source_id,locale,store_country,egress_region,section,
 binding_id,scope_key,epoch,position,event_at,state,output_schema,ruleset,observation_id,content_hash,observed_at,confirmed_at,payload)
 VALUES($1,'en','us','cn',$2,uuidv7(),repeat('a',64),uuidv7(),1,now(),'present',1,'gofurry-steam-1',$3,repeat('b',64),now(),now(),$4)
 ON CONFLICT(game_source_id,locale,store_country,egress_region,section) DO UPDATE SET
 observation_id=excluded.observation_id,payload=excluded.payload,observed_at=excluded.observed_at`, source, section, id, payload)
		ok(t, err)
	}
	putFact(oldMedia, "media", `{"header":"`+oldCover+`","screenshots":["https://untrusted.example/test.jpg"]}`)
	putFact(observed, "details", `{"type":"game","name":"Collected replacement","description":"Collected summary"}`)
	media := func(url string, observation uuid.UUID) []GameMediaSelection {
		return []GameMediaSelection{{Locale: "en", Media: SteamMedia{Header: url}, CoverURL: url, MediaSourceID: &source, MediaObservationID: &observation}}
	}
	before, err := s.SaveManagementGame(ctx, g.PublicID, GameManagementEdit{
		Slug: g.Slug, Revision: g.Revision, Localizations: text, DraftMedia: media(oldCover, oldMedia)})
	ok(t, err)
	g.Revision = before.Revision
	g, err = publishCatalogGame(t, ctx, s, g)
	ok(t, err)
	firstPublished := g.FirstPublishedAt
	assertPublic := func(slug, name, cover string, locales int) {
		t.Helper()
		p, err := s.PublicGameProfile(ctx, ChannelMarket{Region: "CN", Egress: "cn"}, g.PublicID, "en")
		ok(t, err)
		if p.Summary.Slug != slug || len(p.Summary.Localizations) != locales || p.Summary.Localizations[0].Name != name || p.CoverURL != cover {
			t.Fatalf("unexpected public game: %+v", p)
		}
	}
	assertPublic("old-slug", "Old copy", oldCover, 2)
	putFact(newMedia, "media", `{"header":"`+newCover+`"}`)
	published, err := s.ManagementGame(ctx, g.PublicID)
	ok(t, err)
	if published.HasUnpublishedChanges || len(published.DraftMedia) != 1 || published.DraftMedia[0].CoverURL != oldCover {
		t.Fatal("source refresh changed the draft or its review status")
	}
	edit := GameManagementEdit{Slug: "new-slug", Revision: g.Revision, Localizations: text[:1],
		TextAdoptions: []GameTextAdoption{{Locale: "en", SourceID: source, ObservationID: observed, UseName: true}}, DraftMedia: media(newCover, newMedia)}
	for _, reject := range []GameManagementEdit{
		{Slug: edit.Slug, Revision: edit.Revision, Localizations: text, DraftMedia: media("https://untrusted.example/test.jpg", newMedia)},
		{Slug: edit.Slug, Revision: edit.Revision, Localizations: text, DraftMedia: media(newCover, uuid.NewV7())},
		{Slug: edit.Slug, Revision: edit.Revision, Localizations: text, TextAdoptions: []GameTextAdoption{{Locale: "en", SourceID: uuid.NewV7(), ObservationID: observed, UseName: true}}},
	} {
		_, err := s.SaveManagementGame(ctx, g.PublicID, reject)
		want(t, err, ErrInvalid)
	}
	draft, err := s.SaveManagementGame(ctx, g.PublicID, edit)
	ok(t, err)
	reloaded, err := New(s.pool).ManagementGame(ctx, g.PublicID)
	ok(t, err)
	if reloaded.DraftSlug == nil || *reloaded.DraftSlug != edit.Slug || reloaded.Slug != "old-slug" ||
		!reloaded.HasUnpublishedChanges || len(reloaded.Localizations) != 1 || reloaded.Localizations[0].TextSourceID != nil ||
		reloaded.Localizations[0].Name != "Collected replacement" || reloaded.Localizations[0].Summary != "Old summary" {
		t.Fatalf("partial source adoption lost draft ownership: %+v", reloaded)
	}
	assertPublic("old-slug", "Old copy", oldCover, 2)
	noChange, err := s.SaveManagementGame(ctx, g.PublicID, GameManagementEdit{Slug: edit.Slug, Revision: draft.Revision,
		Localizations: []Localization{{Locale: "en", Name: "Collected replacement", Summary: "Old summary"}}})
	ok(t, err)
	if noChange.Revision != draft.Revision {
		t.Fatal("no-op changed the revision")
	}
	g.Revision = draft.Revision
	intent := GamePublicationReview{Revision: g.Revision, ConfirmRelevance: true, ContentRating: "sfw", PreviewToken: reloaded.PreviewToken}
	stale := intent
	stale.PreviewToken = published.PreviewToken
	_, err = changeCatalogGame(t, ctx, s, g, "publish_changes", stale)
	want(t, err, ErrGamePreviewChanged)

	receipt := dbgen.RecordManagementOperationParams{RequestID: uuid.NewV7(), ActorID: uuid.NewV7(), ActorName: "Fixture", ActorRole: "owner", Action: "game.publish_changes", Target: g.PublicID, Revision: g.Revision}
	ok(t, dbgen.New(s.pool).RecordManagementOperation(ctx, receipt))
	publish := func(receipt dbgen.RecordManagementOperationParams) (ManagementResult, error) {
		return s.Manage(ctx, receipt, func(ctx context.Context, tx *Service, _ *dbgen.Queries) (ManagementResult, error) {
			return tx.PublishManagementGameChanges(ctx, g.PublicID, intent)
		})
	}
	_, err = publish(receipt)
	want(t, err, ErrConflict)
	assertPublic("old-slug", "Old copy", oldCover, 2)
	var count int
	ok(t, s.pool.QueryRow(ctx, "SELECT count(*) FROM catalog_publication_decisions WHERE entity_id=$1 AND action='publish_changes'", g.ID).Scan(&count))
	if count != 0 {
		t.Fatal("receipt conflict did not roll back the publication decision")
	}
	receipt.RequestID = uuid.NewV7()
	result, err := publish(receipt)
	ok(t, err)
	assertPublic("new-slug", "Collected replacement", newCover, 1)
	next, err := dbgen.New(s.pool).LockGame(ctx, g.ID)
	ok(t, err)
	if next.FirstPublishedAt != firstPublished || next.PublicID != g.PublicID || next.DraftSlug.Valid || next.Revision != result.Revision {
		t.Fatal("published change altered identity or first publication")
	}
	var audited bool
	ok(t, s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM catalog_publication_decisions d
 JOIN management_operations m ON m.request_id=d.management_request_id
 WHERE d.entity_id=$1 AND d.action='publish_changes' AND d.actor=$2 AND d.content_rating='sfw' AND d.revision=m.revision)`,
		g.ID, receipt.ActorID.String()).Scan(&audited))
	if !audited {
		t.Fatal("published change is missing its correlated actor and audit")
	}
	cleared, err := s.SaveManagementGame(ctx, g.PublicID, GameManagementEdit{Slug: next.Slug, Revision: next.Revision,
		Localizations: []Localization{{Locale: "en", Name: "Collected replacement", Summary: "Old summary"}}, DraftMedia: []GameMediaSelection{}})
	ok(t, err)
	empty, err := s.ManagementGame(ctx, g.PublicID)
	ok(t, err)
	if len(empty.DraftMedia) != 0 || empty.DraftPresentation[0].CoverURL != "" || !empty.HasUnpublishedChanges {
		t.Fatal("empty draft media fell back to the latest source")
	}
	assertPublic("new-slug", "Collected replacement", newCover, 1)
	next.Revision = cleared.Revision
	next, err = changeCatalogGame(t, ctx, s, next, "publish_changes", GamePublicationReview{
		Revision: next.Revision, ConfirmRelevance: true, ContentRating: "sfw", PreviewToken: empty.PreviewToken})
	ok(t, err)
	assertPublic("new-slug", "Collected replacement", "", 1)
	pending, err := s.SaveManagementGame(ctx, g.PublicID, GameManagementEdit{Slug: "restore-slug", Revision: next.Revision,
		Localizations: []Localization{{Locale: "en", Name: "Collected replacement", Summary: "Old summary"}}})
	ok(t, err)
	next.Revision = pending.Revision
	next, err = changeCatalogGame(t, ctx, s, next, "archive", GamePublicationReview{})
	ok(t, err)
	_, err = changeCatalogGame(t, ctx, s, next, "publish_changes", GamePublicationReview{
		Revision: next.Revision, ConfirmRelevance: true, ContentRating: "sfw", PreviewToken: empty.PreviewToken})
	want(t, err, ErrConflict)
	next, err = changeCatalogGame(t, ctx, s, next, "restore", GamePublicationReview{})
	ok(t, err)
	if next.Status != Draft || next.Slug != "restore-slug" || next.DraftSlug.Valid {
		t.Fatal("archive and restore did not preserve the edited slug in Draft")
	}
	_, err = s.PublishedGame(ctx, next.PublicID)
	want(t, err, ErrNotFound)
}

// Seed only the final local projection for the independent CN and INTL SSR
// fixtures. The real preflight-to-public path runs in TestGameIntakeHarvestD.
func seedIntakeBrowserGame(t *testing.T, ctx context.Context, s *Service, publicID, key, name string, revoked bool) {
	t.Helper()
	s.publicID = func() string { return publicID }
	game, err := write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Game, error) {
		return s.CreateIntakeGame(ctx, q, name)
	})
	ok(t, err)
	source := uuid.NewV7()
	observation := uuid.NewV7()
	_, err = s.pool.Exec(ctx, "INSERT INTO game_sources (id,game_id,platform_code,external_key,source_url) VALUES ($1,$2,'steam',$3,$4)",
		source, game.ID, key, "https://store.steampowered.com/app/"+key+"/")
	ok(t, err)
	_, err = s.pool.Exec(ctx, `INSERT INTO game_intake_candidates (id,platform_code,external_key,discovered_via,state,game_id,game_source_id,
 profile,profile_at,profile_observation_id,next_at)
 VALUES ($1,'steam',$2,'operator','accepted',$3,$4,jsonb_build_object('locale','en','name',$5::text,'summary','Collected English synopsis.'),
 now(),$6,NULL)`,
		uuid.NewV7(), key, game.ID, source, name, observation)
	ok(t, err)
	ok(t, InitializeIntakeGameText(ctx, dbgen.New(s.pool), game, source, observation,
		Localization{Locale: "en", Name: name, Summary: "Collected English synopsis."}))
	if !revoked {
		_, err = publishCatalogGame(t, ctx, s, game)
		ok(t, err)
	}
}

func publishCatalogGame(t *testing.T, ctx context.Context, s *Service, g dbgen.Game) (dbgen.Game, error) {
	t.Helper()
	preview, err := s.ManagementGame(ctx, g.PublicID)
	if err != nil {
		return g, err
	}
	return changeCatalogGame(t, ctx, s, g, "publish", GamePublicationReview{Revision: g.Revision, ConfirmRelevance: true, ContentRating: "sfw", PreviewToken: preview.PreviewToken})
}

func changeCatalogGame(t *testing.T, ctx context.Context, s *Service, g dbgen.Game, action string, review GamePublicationReview) (dbgen.Game, error) {
	t.Helper()
	_, err := s.Manage(ctx, dbgen.RecordManagementOperationParams{
		RequestID: uuid.NewV7(), ActorID: uuid.NewV7(), ActorName: "Catalog fixture operator", ActorRole: "operator", Action: "game." + action,
	}, func(ctx context.Context, tx *Service, _ *dbgen.Queries) (ManagementResult, error) {
		if action == "publish" {
			return tx.PublishManagementGame(ctx, g.PublicID, review)
		}
		if action == "publish_changes" {
			return tx.PublishManagementGameChanges(ctx, g.PublicID, review)
		}
		return tx.ChangeManagementGameVisibility(ctx, g.PublicID, g.Revision, action, "Catalog fixture lifecycle")
	})
	if err != nil {
		return g, err
	}
	return dbgen.New(s.pool).LockGame(ctx, g.ID)
}

func openFixture(t *testing.T, ctx context.Context, path, region string) *pgxpool.Pool {
	t.Helper()
	cfg, err := platform.LoadConfig(path)
	ok(t, err)
	d := cfg.Database
	if cfg.Deployment.Region != region || d == nil || d.Host != "127.0.0.1" ||
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
