//go:build integration

package collection

import (
	"io"
	"testing"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func TestSteamMetadataScale(t *testing.T) {
	ctx := t.Context()
	cfg, _ := fixtureDB(t, "steam")
	must(t, database.Migrate(ctx, cfg, "../../migrations", "up", io.Discard))
	pool, err := database.Open(ctx, cfg)
	must(t, err)
	defer pool.Close()
	generation, epoch := uuid.NewV7(), uuid.NewV7()
	q := dbgen.New(pool)
	editor := catalog.New(pool)
	execSQL(t, pool, `INSERT INTO collection_consumer(namespace,consumer_id,generation,epoch,cursor)
VALUES ('steam_scale',uuidv7(),$1,$2,'scale')`, generation, epoch)
	previous := 0
	for _, size := range []int{100, 1000, 10000} {
		execSQL(t, pool, `WITH input AS MATERIALIZED (
 SELECT i,uuidv7() AS game_id,uuidv7() AS game_source_id,uuidv7() AS source_id,uuidv7() AS binding_id,uuidv7() AS plan_id
 FROM generate_series($1::integer,$2::integer) i
), works AS (
 INSERT INTO games(id,public_id,slug,status,published_at)
 SELECT game_id,lpad(i::text,16,'0'),'scale','published',now()-interval '1 day' FROM input
), game_source AS (
 INSERT INTO game_sources(id,game_id,platform_code,external_key,source_url)
 SELECT game_source_id,game_id,'steam',i::text,'https://store.steampowered.com/app/'||i||'/' FROM input
), source AS (
 INSERT INTO collection_sources(id,namespace,provider,resource_type,external_key,target_id)
 SELECT source_id,'steam_scale','steam','app',i::text,uuidv7() FROM input
), binding AS (
 INSERT INTO collection_bindings(id,source_id,resource_type,game_id,game_source_id,purpose,effective_from)
 SELECT binding_id,source_id,'app',game_id,game_source_id,'steam',now()-interval '1 day' FROM input
), plan AS (
 INSERT INTO collection_plans(id,source_id,capability,request_schema,params,scope,interval_seconds,anchor_at,policy)
 SELECT plan_id,source_id,'steam.details',1,'{"language":"english","store_country":"US"}','{"locale":"en","region":"us","egress_region":"cn"}',86400,now(),'state_refresh' FROM input
), attached AS (
 INSERT INTO collection_plan_bindings(plan_id,binding_id,source_id,attached_at)
 SELECT plan_id,binding_id,source_id,now()-interval '1 day' FROM input
)
INSERT INTO collection_current(source_id,generation,capability,output_schema,scope_key,scope,position,state,observation_id,
 observed_at,received_at,confirmed_at,content_hash,evidence,ruleset,data,facts)
SELECT source_id,$3,'steam.details',1,repeat('a',64),
 '{"namespace":"steam_scale","locale":"en","region":"us","egress_region":"cn","dimensions":{"section":"details"}}',
 i,'present',uuidv7(),now(),now(),now(),repeat('b',64),'','gofurry-steam-1',
 jsonb_build_object('app_id',i,'tier','observed_storefront','source_url','https://store.steampowered.com/api/appdetails',
 'name','Fox Library','type','game','description','Foxes restore a library.','developers',jsonb_build_array('Studio')), '{}'
FROM input`, previous+1, size, generation)
		start := time.Now()
		changed, err := q.SaveSteamMetadata(ctx, dbgen.SaveSteamMetadataParams{Generation: generation, Epoch: epoch})
		must(t, err)
		if len(changed) != size-previous {
			t.Fatalf("wrong material writes at %d: %d", size, len(changed))
		}
		elapsed := time.Since(start)
		replayStart := time.Now()
		replayed, err := q.SaveSteamMetadata(ctx, dbgen.SaveSteamMetadataParams{Generation: generation, Epoch: epoch})
		must(t, err)
		if len(replayed) != 0 {
			t.Fatal("replay generated material writes")
		}
		replayTime := time.Since(replayStart)
		readStart := time.Now()
		for range 20 {
			result, err := editor.ReadGamePresentation(ctx, "0000000000000001", "cn")
			must(t, err)
			if len(result.Sources) != 1 || len(result.Sources[0].Metadata) != 1 || result.Sources[0].Metadata[0].Details.Name != "Fox Library" {
				t.Fatal("single-game read was not source-scoped")
			}
		}
		t.Logf("Steam source rows=%d inserted=%d SQL_write=%s replay=%s avg_read=%s table_and_indexes=%d B",
			size, len(changed), elapsed, replayTime, time.Since(readStart)/20,
			scalar[int64](t, pool, "SELECT pg_total_relation_size('game_source_metadata')"))
		previous = size
	}
}
