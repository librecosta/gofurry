//go:build integration

package collection

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
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
	execSQL(t, pool, "INSERT INTO games(id,public_id,slug,status,archived_at) VALUES(uuidv7(),'000000000000000f','archived-game','archived',now())")
	known, unknown := uuid.NewV7(), uuid.NewV7()
	execSQL(t, pool, `INSERT INTO sites(id,public_id,slug,status,revision,published_at,legacy_detail) VALUES
 ($1,'000000000000000d','known-site','published',2,now(),true),($2,'000000000000000e','unknown-site','published',1,now(),true)`, known, unknown)
	execSQL(t, pool, "INSERT INTO site_localizations(site_id,locale,name,summary) VALUES ($1,'en','Known Site','Reference site.'),($2,'en','Unknown Site','Historical profile.')", known, unknown)
	execSQL(t, pool, `INSERT INTO site_navigation_reviews(id,site_id,site_revision,previous_rating,rating,actor,reason)
 VALUES (uuidv7(),$1,2,'unreviewed','sfw','fixture','Prior documented rating')`, known)
	must(t, database.Migrate(ctx, cfg, migrations, "up", io.Discard))
	must(t, database.Migrate(ctx, cfg, migrations, "up", io.Discard))

	editor := catalog.New(pool)
	for _, id := range []string{expired, revoked, "000000000000000c"} {
		_, err := editor.PublishedGame(ctx, id)
		if !errors.Is(err, catalog.ErrNotFound) {
			t.Fatalf("unverified pre-cutover publication %s is visible: %v", id, err)
		}
	}
	count(t, pool, 3, "SELECT count(*) FROM games WHERE status='draft' AND content_rating='unreviewed' AND published_at IS NOT NULL")
	count(t, pool, 1, "SELECT count(*) FROM games WHERE public_id='000000000000000f' AND status='archived'")
	count(t, pool, 2, "SELECT count(*) FROM game_sources")
	count(t, pool, 2, "SELECT count(*) FROM game_intake_candidates WHERE state='accepted'")
	count(t, pool, 2, "SELECT count(*) FROM game_source_text_history WHERE name='Legacy Fable'")
	count(t, pool, 0, "SELECT count(*) FROM game_published_localizations")
	count(t, pool, 3, "SELECT count(*) FROM catalog_publication_decisions WHERE entity_kind='game' AND action='unpublish' AND actor='schema-migration'")
	count(t, pool, 1, "SELECT count(*) FROM catalog_publication_decisions WHERE entity_kind='game' AND action='block' AND actor='r1_migration'")
	count(t, pool, 1, "SELECT count(*) FROM catalog_publication_decisions WHERE entity_kind='game' AND action='unpublish' AND reason LIKE 'Legacy restricted%'")
	count(t, pool, 0, "SELECT count(*) FROM information_schema.columns WHERE table_name='game_intake_candidates' AND column_name IN ('review','review_until','review_revoked','automation','published_at','decision_version')")

	out, err := editor.DiagnosePublication(ctx, true)
	must(t, err)
	if out.LegacyPromoted != 1 || out.LegacyUnknown != 1 {
		t.Fatalf("wrong legacy diagnosis: %+v", out)
	}
	out, err = editor.DiagnosePublication(ctx, true)
	must(t, err)
	if out.LegacyPromoted != 0 || out.LegacyUnknown != 1 {
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
	t.Run("EditorialDraft", testGameEditorialForward)
}

func testGameEditorialForward(t *testing.T) {
	ctx := t.Context()
	cfg, _ := fixtureDB(t, "r1b1")
	const migrations = "../../migrations"
	prior := t.TempDir()
	files, err := filepath.Glob(filepath.Join(migrations, "*.sql"))
	must(t, err)
	for _, file := range files {
		if filepath.Base(file) >= "202611200002" {
			continue
		}
		data, err := os.ReadFile(file)
		must(t, err)
		must(t, os.WriteFile(filepath.Join(prior, filepath.Base(file)), data, 0600))
	}
	must(t, database.Migrate(ctx, cfg, prior, "up", io.Discard))
	pool, err := database.Open(ctx, cfg)
	must(t, err)
	defer pool.Close()
	pub, draft, manual, source, observation := uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7()
	execSQL(t, pool, `INSERT INTO games(id,public_id,slug,status,content_rating,published_at) VALUES
 ($1,'abcd000000000001','public-slug','published','sfw',now()),
 ($2,'abcd000000000002','first-intake','draft','unreviewed',NULL),
 ($3,'abcd000000000003','manual','draft','unreviewed',NULL)`, pub, draft, manual)
	execSQL(t, pool, "INSERT INTO game_localizations VALUES($1,'en','Human edit',''),($2,'zh','手工草稿','')", pub, manual)
	execSQL(t, pool, `INSERT INTO game_published_localizations(game_id,locale,name,summary,media,media_source_id,media_observed_at,media_store_country,cover_url,revision)
VALUES($1,'en','Already public','','{}',NULL,NULL,'','',1),
 ($1,'zh','公开译名','','{"header":"https://cdn.steamstatic.com/header.jpg","screenshots":["https://cdn.steamstatic.com/one.jpg"]}',$2,now(),'cn','https://cdn.steamstatic.com/header.jpg',1)`, pub, uuid.NewV7())
	execSQL(t, pool, `INSERT INTO game_sources (id,game_id,platform_code,external_key,source_url) VALUES
 ($1,$2,'steam','949480','https://store.steampowered.com/app/949480/'),(uuidv7(),$3,'steam','42','https://store.steampowered.com/app/42/')`, source, draft, manual)
	execSQL(t, pool, `INSERT INTO game_intake_candidates(id,platform_code,external_key,discovered_via,state,game_id,game_source_id,profile,profile_at,profile_observation_id,next_at)
 SELECT uuidv7(),platform_code,external_key,'operator','accepted',game_id,id,'{"locale":"en","name":"Source draft","summary":"Source synopsis"}',now(),$1,NULL FROM game_sources`, observation)
	must(t, database.Migrate(ctx, cfg, migrations, "up", io.Discard))
	must(t, database.Migrate(ctx, cfg, migrations, "up", io.Discard))
	count(t, pool, 1, "SELECT count(*) FROM games WHERE id=$1 AND status='published' AND content_rating='sfw' AND slug='public-slug' AND draft_slug IS NULL", pub)
	count(t, pool, 1, "SELECT count(*) FROM game_localizations WHERE game_id=$1 AND locale='en' AND name='Human edit'", pub)
	count(t, pool, 1, "SELECT count(*) FROM game_localizations WHERE game_id=$1 AND locale='zh' AND name='公开译名'", pub)
	count(t, pool, 1, "SELECT count(*) FROM game_draft_media WHERE game_id=$1 AND cover_url='https://cdn.steamstatic.com/header.jpg' AND media_observation_id IS NULL", pub)
	count(t, pool, 1, "SELECT count(*) FROM game_localizations WHERE game_id=$1 AND name='Source draft' AND text_source_id=$2 AND text_observation_id=$3", draft, source, observation)
	count(t, pool, 1, "SELECT count(*) FROM game_localizations WHERE game_id=$1 AND locale='zh'", manual)
	count(t, pool, 2, "SELECT count(*) FROM game_published_localizations WHERE game_id=$1", pub)
	count(t, pool, 0, "SELECT count(*) FROM catalog_publication_decisions")
}

func nullableExpiry(expiry string) *string {
	if expiry == "" {
		return nil
	}
	return &expiry
}
