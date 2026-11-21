package collection

import (
	"context"
	"flag"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var nativeCollectionConfig = flag.String("native-collection-config", "", "absolute path to the development YAML")

func nativeCollectionPostgres(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()
	path := *nativeCollectionConfig
	if path == "" {
		t.Skip("pass -native-collection-config with an explicit development YAML to run")
	}
	if !filepath.IsAbs(path) {
		t.Fatal("expected an absolute configuration path")
	}
	cfg, err := platform.LoadConfig(path)
	if err != nil || cfg.Database == nil || cfg.Database.Name != "gofurry_cn_dev" {
		t.Fatal("expected a valid gofurry_cn_dev configuration")
	}
	pc, err := database.PoolConfig(*cfg.Database)
	if err != nil {
		t.Fatal("could not configure PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := pgx.ConnectConfig(ctx, pc.ConnConfig)
	if err != nil {
		t.Fatal("could not connect to the development PostgreSQL")
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	var name string
	if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil || name != cfg.Database.Name {
		t.Fatal("unexpected development database")
	}
	return ctx, conn
}

func nativeCollectionTables(t *testing.T, ctx context.Context, conn *pgx.Conn, tables ...string) pgx.Tx {
	t.Helper()
	tx, err := conn.Begin(ctx)
	requirePlanTest(t, err)
	t.Cleanup(func() { tx.Rollback(context.Background()) })
	for _, table := range tables {
		_, err := tx.Exec(ctx, "CREATE TEMP TABLE "+table+" (LIKE public."+table+" INCLUDING ALL) ON COMMIT DROP")
		requirePlanTest(t, err)
	}
	// Only the temporary copies are visible to application queries.
	_, err = tx.Exec(ctx, "SET LOCAL search_path = pg_temp, pg_catalog")
	requirePlanTest(t, err)
	return tx
}

func TestIntakePlansPostgres(t *testing.T) {
	ctx, conn := nativeCollectionPostgres(t)
	for _, test := range []struct {
		name, existing string
	}{
		{name: "empty"},
		{name: "existing English", existing: "en"},
		{name: "existing Chinese", existing: "zh-cn"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := nativeCollectionTables(t, ctx, conn, "collection_sources", "collection_bindings", "collection_plans", "collection_plan_bindings")
			q := dbgen.New(tx)
			targetID := uuid.NewV7()
			candidate := dbgen.GameIntakeCandidate{Namespace: "test", ExternalKey: "123", EgressRegion: "cn", TargetID: &targetID}
			gameSource := dbgen.GameSource{ID: uuid.NewV7(), GameID: uuid.NewV7()}
			var existing uuid.UUID
			if test.existing == "" {
				attempt, err := tx.Begin(ctx)
				requirePlanTest(t, err)
				_, err = attempt.Exec(ctx, "ALTER TABLE collection_plans ADD CHECK (scope->>'locale' <> 'zh-cn')")
				requirePlanTest(t, err)
				if err := intakeSchedule(ctx, dbgen.New(attempt), candidate, gameSource); err == nil {
					t.Fatal("expected the Chinese plan to fail")
				}
				requirePlanTest(t, attempt.Rollback(ctx))
				var empty bool
				requirePlanTest(t, tx.QueryRow(ctx, `
SELECT NOT EXISTS (SELECT 1 FROM collection_sources)
   AND NOT EXISTS (SELECT 1 FROM collection_bindings)
   AND NOT EXISTS (SELECT 1 FROM collection_plans)
   AND NOT EXISTS (SELECT 1 FROM collection_plan_bindings)
`).Scan(&empty))
				if !empty {
					t.Fatal("the failed plan left partial work behind")
				}
			} else {
				source, err := q.SaveCollectionSource(ctx, dbgen.SaveCollectionSourceParams{
					ID: uuid.NewV7(), Namespace: candidate.Namespace, Provider: "steam", ResourceType: "app", ExternalKey: candidate.ExternalKey,
				})
				requirePlanTest(t, err)
				binding, err := q.AddCollectionGameBinding(ctx, dbgen.AddCollectionGameBindingParams{
					ID: uuid.NewV7(), SourceID: source.ID, GameID: &gameSource.GameID, GameSourceID: &gameSource.ID,
				})
				requirePlanTest(t, err)
				for _, plan := range intakePlans(candidate.EgressRegion) {
					if plan.Locale == test.existing {
						plan.Interval = 48 * time.Hour
						requirePlanTest(t, attachIntakePlan(ctx, q, source.ID, binding.ID, candidate.ExternalKey, plan))
					}
				}
				requirePlanTest(t, tx.QueryRow(ctx, "SELECT id FROM collection_plans").Scan(&existing))
			}

			for range 2 {
				requirePlanTest(t, intakeSchedule(ctx, q, candidate, gameSource))
			}
			var bindingOK, plansOK bool
			requirePlanTest(t, tx.QueryRow(ctx, `
SELECT (SELECT count(*)=1 FROM collection_sources WHERE target_id=$1)
   AND (SELECT count(*)=1 FROM collection_bindings WHERE game_id=$2 AND game_source_id=$3)
   AND (SELECT count(*)=2 FROM collection_plan_bindings),
       (SELECT array_agg((scope->>'locale') || '/' || (scope->>'region') ORDER BY scope->>'locale')
               = ARRAY['en/us','zh-cn/cn']
        FROM collection_plans WHERE scope->>'egress_region'='cn' AND capability='steam.details')
`, targetID, gameSource.GameID, gameSource.ID).Scan(&bindingOK, &plansOK))
			if !bindingOK || !plansOK {
				t.Fatal("expected one verified target, one binding and two correctly scoped plans")
			}
			if test.existing != "" {
				var id uuid.UUID
				var interval int
				requirePlanTest(t, tx.QueryRow(ctx, "SELECT id, interval_seconds FROM collection_plans WHERE scope->>'locale'=$1", test.existing).Scan(&id, &interval))
				if id != existing || interval != int((48*time.Hour)/time.Second) {
					t.Fatal("original plan identity or interval changed")
				}
			}
		})
	}
}

func TestSteamProjectionPostgres(t *testing.T) {
	ctx, conn := nativeCollectionPostgres(t)
	for _, status := range []string{"draft", "published", "archived"} {
		t.Run(status, func(t *testing.T) {
			tx := nativeCollectionTables(t, ctx, conn, "collection_sources", "collection_bindings", "collection_plans",
				"collection_plan_bindings", "collection_current", "game_source_metadata", "games", "game_sources",
				"game_localizations", "game_draft_media", "game_published_localizations")
			exec := func(query string, args ...any) {
				t.Helper()
				_, err := tx.Exec(ctx, query, args...)
				requirePlanTest(t, err)
			}
			gameID, gameSourceID, targetID := uuid.NewV7(), uuid.NewV7(), uuid.NewV7()
			exec(`INSERT INTO games(id,public_id,slug,status,published_at,archived_at,content_rating)
				VALUES ($1,'0000000000000001','curated',$2,CASE WHEN $2='published' THEN now() END,
				        CASE WHEN $2='archived' THEN now() END,'sfw')`, gameID, status)
			exec(`INSERT INTO game_localizations(game_id,locale,name,summary) VALUES ($1,'zh','人工文案','人工简介')`, gameID)
			exec(`INSERT INTO game_draft_media(game_id,locale,media,cover_url,media_store_country) VALUES
				($1,'zh','{}','https://cdn.steamstatic.com/curated.jpg','')`, gameID)
			if status == "published" {
				exec(`INSERT INTO game_published_localizations(game_id,locale,name,summary,revision)
					VALUES ($1,'zh','公开文案','公开简介',1)`, gameID)
			}
			exec(`INSERT INTO game_sources(id,game_id,platform_code,external_key,source_url)
				VALUES ($1,$2,'steam','123','https://store.steampowered.com/app/123/')`, gameSourceID, gameID)
			q := dbgen.New(tx)
			candidate := dbgen.GameIntakeCandidate{Namespace: "test", ExternalKey: "123", EgressRegion: "cn", TargetID: &targetID}
			requirePlanTest(t, intakeSchedule(ctx, q, candidate, dbgen.GameSource{ID: gameSourceID, GameID: gameID}))
			generation, epoch, observation := uuid.NewV7(), uuid.NewV7(), uuid.NewV7()
			exec(`INSERT INTO collection_current(source_id,generation,capability,output_schema,scope_key,scope,position,
			       state,observation_id,observed_at,received_at,content_hash,evidence,ruleset,data,facts)
			       SELECT id,$1,'steam.details',1,repeat('a',64),
			       '{"namespace":"test","locale":"zh-cn","region":"cn","egress_region":"cn","dimensions":{"section":"media"}}',
			       1,'present',$2,now(),now(),repeat('b',64),'fixture','gofurry-steam-1',
			       '{"app_id":123,"tier":"observed_storefront","source_url":"https://store.steampowered.com/api/appdetails","header":"https://cdn.steamstatic.com/source.jpg"}','{}'
			       FROM collection_sources`, generation, observation)
			const editorial = `SELECT jsonb_build_array((SELECT to_jsonb(g) FROM games g),
			       (SELECT to_jsonb(l) FROM game_localizations l), (SELECT to_jsonb(m) FROM game_draft_media m),
			       (SELECT to_jsonb(p) FROM game_published_localizations p))::text`
			var before, after string
			requirePlanTest(t, tx.QueryRow(ctx, editorial).Scan(&before))
			requirePlanTest(t, saveSteamMetadata(ctx, q, generation, epoch, nil, ""))
			exec(`UPDATE collection_current SET state='confirmed_empty',observation_id=NULL,data='{}',position=2`)
			requirePlanTest(t, saveSteamMetadata(ctx, q, generation, epoch, nil, ""))
			var sourceObservation uuid.UUID
			var cover string
			requirePlanTest(t, tx.QueryRow(ctx, `SELECT observation_id,payload->>'header' FROM game_source_metadata
				WHERE locale='zh-cn' AND store_country='cn' AND section='media'`).Scan(&sourceObservation, &cover))
			requirePlanTest(t, tx.QueryRow(ctx, editorial).Scan(&after))
			if sourceObservation != observation || cover != "https://cdn.steamstatic.com/source.jpg" || after != before {
				t.Fatal("projection lost valid media or changed editorial data")
			}
		})
	}
}

func requirePlanTest(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
