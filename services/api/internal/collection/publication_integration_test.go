//go:build integration

package collection

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func TestDurableCatalogForward(t *testing.T) {
	ctx := t.Context()
	cfg, _ := fixtureDB(t, "r1")
	const migrations = "../../migrations"
	const revision = "202611130001_durable_catalog.sql"
	prior := t.TempDir()
	files, err := filepath.Glob(filepath.Join(migrations, "*.sql"))
	must(t, err)
	for _, file := range files {
		name := filepath.Base(file)
		if name >= revision {
			continue
		}
		data, err := os.ReadFile(file)
		must(t, err)
		must(t, os.WriteFile(filepath.Join(prior, name), data, 0600))
	}
	must(t, database.Migrate(ctx, cfg, prior, "up", io.Discard))
	pool, err := database.Open(ctx, cfg)
	must(t, err)
	defer pool.Close()

	seedGame := func(publicID, key, expiry string, available bool) {
		t.Helper()
		work, source := uuid.NewV7(), uuid.NewV7()
		execSQL(t, pool, "INSERT INTO games(id,public_id,slug,status,intake_managed,intake_display_source,published_at) VALUES($1,$2,'legacy-fable','published',true,$3,now()-interval '365 days')",
			work, publicID, source)
		if !available {
			return
		}
		execSQL(t, pool, "INSERT INTO game_sources(id,game_id,platform_code,external_key,source_url,active) VALUES($1,$2,'steam',$3,'https://store.steampowered.com/app/'||$3||'/',false)",
			source, work, key)
		execSQL(t, pool, `INSERT INTO game_intake_candidates(id,platform_code,external_key,discovered_via,state,game_id,game_source_id,
 profile,profile_at,profile_observation_id,decision_version,review,review_until,next_at)
 VALUES ($1,'steam',$2,'operator','accepted',$3,$4,'{"locale":"en","name":"Legacy Fable","summary":"The approved story."}',
 now()-interval '365 days',$5,1,'{"policy":"furry-and-sfw-1","furry_ref":"fixture.synopsis","sfw_ref":"fixture.review"}',
 now()-$6::interval,NULL)`, uuid.NewV7(), key, work, source, uuid.NewV7(), nullableExpiry(expiry))
	}
	const expired = "000000000000000a"
	const revoked = "000000000000000b"
	seedGame(expired, "50101", "181 days", true)
	seedGame(revoked, "50102", "", true)
	seedGame("000000000000000c", "50103", "", false)
	known, unknown := uuid.NewV7(), uuid.NewV7()
	execSQL(t, pool, `INSERT INTO sites(id,public_id,slug,status,revision,published_at,legacy_detail) VALUES
 ($1,'000000000000000d','known-site','published',2,now(),true),($2,'000000000000000e','unknown-site','published',1,now(),true)`, known, unknown)
	execSQL(t, pool, "INSERT INTO site_localizations(site_id,locale,name,summary) VALUES ($1,'en','Known Site','Reference site.'),($2,'en','Unknown Site','Historical profile.')", known, unknown)
	execSQL(t, pool, `INSERT INTO site_navigation_reviews(id,site_id,site_revision,previous_rating,rating,actor,reason)
 VALUES (uuidv7(),$1,2,'unreviewed','sfw','fixture','Prior documented rating')`, known)
	must(t, database.Migrate(ctx, cfg, migrations, "up", io.Discard))
	must(t, database.Migrate(ctx, cfg, migrations, "up", io.Discard))

	editor := catalog.New(pool)
	first, err := editor.PublishedGame(ctx, expired)
	must(t, err)
	if len(first.Localizations) != 1 || first.Localizations[0].Name != "Legacy Fable" || first.Localizations[0].Summary != "The approved story." {
		t.Fatalf("forward migration did not recover the approved presentation: %+v", first)
	}
	count(t, pool, 1, "SELECT count(*) FROM catalog_publication_decisions WHERE entity_kind='game' AND action='block' AND actor='r1_migration'")
	count(t, pool, 2, "SELECT count(*) FROM game_approved_presentations")
	count(t, pool, 0, "SELECT count(*) FROM games WHERE public_id=$1 AND NOT publication_blocked", revoked)
	execSQL(t, pool, "DELETE FROM game_intake_candidates WHERE external_key='50101'")
	after, err := editor.PublishedGame(ctx, expired)
	must(t, err)
	if !reflect.DeepEqual(first, after) {
		t.Fatal("removing the historical candidate changed the public response")
	}
	decision := catalog.PublicationDecision{Actor: "fixture", Reason: "Explicit disposition fixture"}
	work, err := dbgen.New(pool).LockGamePublicID(ctx, expired)
	must(t, err)
	archived, err := editor.ChangeGamePublication(ctx, expired, work.Revision, "archive", decision)
	must(t, err)
	_, err = editor.PublishedGame(ctx, expired)
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Fatal("explicit Game archive left the Game public")
	}
	_, err = editor.ChangeGamePublication(ctx, expired, work.Revision, "restore", decision)
	if !errors.Is(err, catalog.ErrConflict) {
		t.Fatal("stale revision restored an archived Game")
	}
	restored, err := editor.ChangeGamePublication(ctx, expired, archived.Revision, "restore", decision)
	must(t, err)
	after, err = editor.PublishedGame(ctx, expired)
	must(t, err)
	if !reflect.DeepEqual(first, after) || !restored.PublishedAt.Time.Equal(work.PublishedAt.Time) {
		t.Fatal("restoring an archived Game changed its original identity or publication date")
	}
	count(t, pool, 2, "SELECT count(*) FROM catalog_publication_decisions WHERE entity_id=$1 AND actor=$2 AND reason=$3", work.ID, decision.Actor, decision.Reason)

	out, err := editor.DiagnosePublication(ctx, true)
	must(t, err)
	if out.LegacyPromoted != 1 || out.LegacyUnknown != 1 || out.GamesMissing != 1 {
		t.Fatalf("wrong legacy diagnosis: %+v", out)
	}
	out, err = editor.DiagnosePublication(ctx, true)
	must(t, err)
	if out.LegacyPromoted != 0 || out.LegacyUnknown != 1 || out.GamesMissing != 1 {
		t.Fatalf("legacy upgrade was not repeatable: %+v", out)
	}
	count(t, pool, 1, "SELECT count(*) FROM site_public_navigation WHERE id=$1 AND primary_category IS NULL", known)
	count(t, pool, 0, "SELECT count(*) FROM site_public_navigation WHERE id=$1", unknown)
	legacy, err := editor.PublishedSite(ctx, "000000000000000e")
	must(t, err)
	if legacy.Reviewed || legacy.Localizations[0].Name != "Unknown Site" {
		t.Fatal("unreviewed legacy Site lost compatible detail or acquired a fabricated rating")
	}
	count(t, pool, 2, "SELECT count(*) FROM site_navigation_reviews WHERE site_id=$1 AND actor='fixture' AND reason='Prior documented rating'", known)
}

func nullableExpiry(expiry string) *string {
	if expiry == "" {
		return nil
	}
	return &expiry
}
