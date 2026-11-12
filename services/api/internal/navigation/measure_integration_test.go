//go:build integration

package navigation

import (
	"runtime"
	"runtime/metrics"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func measureNavigation(t *testing.T, pool *pgxpool.Pool, svc *Service) {
	t.Helper()
	ctx := t.Context()
	start := time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return start }
	for _, n := range []int{100, 1000, 10000} {
		// The caller verified the explicitly named, disposable D2 database.
		execute(t, pool, "TRUNCATE site_localizations,site_addresses,sites,site_interaction_days,site_navigation_heat,site_navigation_heat_state")
		execute(t, pool, "TRUNCATE collection_current,collection_plan_bindings,collection_plans,collection_bindings,collection_sources,collection_consumer,collection_gaps,site_evidence_changes")
		begin := time.Now()
		execute(t, pool, `WITH fixture AS MATERIALIZED (
 SELECT i,uuidv7() AS id FROM generate_series(1,$1::integer) i
), published AS (
 INSERT INTO sites(id,public_id,slug,status,published_at,first_published_at,content_rating,primary_category,secondary_categories,navigation_tags,supported_languages,logo_asset_key)
 SELECT id,lpad(to_hex(i),16,'0'),'measure-site','published',timestamptz '2026-01-01 UTC'+i*interval '1 second',
  timestamptz '2026-01-01 UTC'+i*interval '1 second','sfw',
  CASE WHEN i%2=0 THEN 'gallery' ELSE 'community' END,ARRAY['resources'],ARRAY['tutorial'],ARRAY['en','zh'],
  CASE WHEN i%4=0 THEN 'sites/logos/'||repeat('a',64)||'.webp' END FROM fixture
), names AS (
 INSERT INTO site_localizations SELECT id,'en','Fixture community '||i,'A measured directory with resources and galleries' FROM fixture
 UNION ALL SELECT id,'zh','测量社区 '||i,'真实数据库基准' FROM fixture
), addresses AS (
 INSERT INTO site_addresses(id,site_id,url,kind)
 SELECT uuidv7(),id,'https://site-'||i||'.example.test/guide?language=en','primary' FROM fixture
)
INSERT INTO site_interaction_days SELECT id,$2::date-d,4+i%19,1+i%3 FROM fixture CROSS JOIN generate_series(0,6) d`, n, pgDay(start))
		seed := time.Since(begin)
		for _, table := range []string{"sites", "site_localizations", "site_interaction_days"} {
			execute(t, pool, "ANALYZE "+table)
		}
		begin = time.Now()
		cold, err := svc.Directory(ctx, Filters{Locale: "en", Sort: "heat_7d"}, "")
		must(t, err)
		if cold.Total != int64(n) || cold.Heat.Status != "cold" {
			t.Fatal("cold directory is incorrect")
		}
		coldRead := time.Since(begin)
		lsn := value[string](t, pool, "SELECT pg_current_wal_insert_lsn()::text")
		begin = time.Now()
		refresh, err := svc.RefreshHeat(ctx, false)
		must(t, err)
		refreshWall := time.Since(begin)
		if !refresh.Refreshed || refresh.Ranked != int64(n) {
			t.Fatal("heat did not cover every eligible fixture")
		}
		wal := value[int64](t, pool, "SELECT pg_wal_lsn_diff(pg_current_wal_insert_lsn(),$1::text::pg_lsn)::bigint", lsn)
		size := func(table string) (heap, index, toast int64) {
			t.Helper()
			must(t, pool.QueryRow(ctx, `SELECT pg_table_size(c.oid),pg_indexes_size(c.oid),
 CASE WHEN c.reltoastrelid=0 THEN 0 ELSE pg_total_relation_size(c.reltoastrelid) END
 FROM pg_class c WHERE c.oid=$1::text::regclass`, table).Scan(&heap, &index, &toast))
			return
		}
		dh, di, dt := size("site_interaction_days")
		hh, hi, ht := size("site_navigation_heat")
		const iterations = 20
		stats := []metrics.Sample{{Name: "/cpu/classes/user:cpu-seconds"}}
		hits := func() int64 {
			return value[int64](t, pool, "SELECT blks_hit FROM pg_stat_database WHERE datname=current_database()")
		}
		beforeHits := hits()
		runtime.GC()
		metrics.Read(stats)
		cpu := stats[0].Value.Float64()
		begin = time.Now()
		for range iterations {
			out, err := svc.Directory(ctx, Filters{Locale: "zh", Keyword: "测量", Categories: []string{"resources"}, Sort: "heat_7d"}, "")
			must(t, err)
			if out.Total != int64(n) || len(out.Items) != 24 || !out.Items[0].HeatQualified {
				t.Fatal("warm filtered ranking is incorrect")
			}
		}
		warm := time.Since(begin) / iterations
		runtime.GC()
		metrics.Read(stats)
		goCPU := time.Duration((stats[0].Value.Float64() - cpu) * float64(time.Second) / iterations)
		execute(t, pool, "SELECT pg_stat_force_next_flush()")
		hitDelta := hits() - beforeHits
		begin = time.Now()
		home, err := svc.Home(ctx)
		must(t, err)
		if len(home.Trending) != 8 || len(home.Recent) != 8 {
			t.Fatal("home candidate bounds are wrong")
		}
		t.Logf("sites=%d days=%d seed=%s cold_directory=%s warm_filtered_rank=%s go_user_cpu=%s buffer_hits_delta=%d home=%s heat_refresh=%s heat_wal=%d daily_heap=%d daily_index=%d daily_toast=%d heat_heap=%d heat_index=%d heat_toast=%d",
			n, 7*n, seed, coldRead, warm, goCPU, hitDelta, time.Since(begin), refreshWall, wal, dh, di, dt, hh, hi, ht)
		measureProfiles(t, pool, svc, n)
	}
}

func measureProfiles(t *testing.T, pool *pgxpool.Pool, svc *Service, n int) {
	t.Helper()
	ctx := t.Context()
	execute(t, pool, `
UPDATE site_localizations l SET body=repeat('中',3413) FROM sites s WHERE s.id=l.site_id AND s.public_id='0000000000000004' AND l.locale='zh';
WITH input AS MATERIALIZED (
 SELECT uuidv7() AS source_id,s.public_id,a.id AS address_id,s.id AS site_id,split_part(a.url,'/guide',1) AS origin
 FROM sites s JOIN site_addresses a ON a.site_id=s.id AND a.active AND a.kind='primary'
), src AS (
 INSERT INTO collection_sources (id,namespace,provider,resource_type,external_key,target_id)
 SELECT source_id,'profile-measure','web','origin',origin,uuidv7() FROM input
), binding AS (
 INSERT INTO collection_bindings(id,source_id,site_id,address_id,resource_type,purpose,effective_from)
 SELECT uuidv7(),source_id,site_id,address_id,'origin','primary',now()-interval '3 days' FROM input RETURNING *
), plans AS (
 INSERT INTO collection_plans(id,source_id,capability,request_schema,params,scope,interval_seconds,anchor_at,policy)
 SELECT uuidv7(),source_id,'web.http',1,'{}','{"egress_region":"cn"}',86400,now(),'state_refresh' FROM input RETURNING *
), members AS (
 INSERT INTO collection_plan_bindings(plan_id,binding_id,source_id,attached_at)
 SELECT p.id,b.id,p.source_id,now()-interval '3 days' FROM plans p JOIN binding b USING(source_id)
), consumer AS (
 INSERT INTO collection_consumer(namespace,consumer_id,generation,epoch,cursor)
 VALUES('profile-measure',uuidv7(),uuidv7(),uuidv7(),'measure') RETURNING *
)
INSERT INTO collection_current (source_id,generation,capability,output_schema,scope_key,scope,position,state,observation_id,
 observed_at,received_at,confirmed_at,content_hash,evidence,ruleset,data,facts)
SELECT i.source_id,c.generation,'web.http',1,repeat('a',64),'{"namespace":"profile-measure","egress_region":"cn","dimensions":{"section":"http"}}',
 1,'present',uuidv7(),now()-interval '1 day',now()-interval '1 day',now()-interval '1 hour',repeat('b',64),'','web-passive-1','{}',
 jsonb_build_object('http.response',jsonb_build_object('origin',i.origin,'status',200)) ||
 CASE WHEN i.public_id='0000000000000002' THEN '{}'::jsonb ELSE
 '{"tech.detected.astro":{"id":"astro","confidence":"high","evidence":["generator:astro"]},
 "tech.detected.wordpress":{"id":"wordpress","confidence":"medium","evidence":["generator:wordpress"]}}'::jsonb END
FROM input i CROSS JOIN consumer c WHERE i.public_id<>'0000000000000001';
`)
	for _, table := range []string{"collection_sources", "collection_bindings", "collection_current", "collection_plan_bindings", "collection_plans", "site_addresses"} {
		execute(t, pool, "ANALYZE "+table)
	}
	editor := catalog.New(pool)
	for id, want := range map[string]int{"0000000000000001": 0, "0000000000000002": 1, "0000000000000004": 3} {
		profile, err := editor.PublishedSite(ctx, id)
		must(t, err)
		if len(profile.Technology) != want || (want == 3 && len(profile.Localizations[1].Body) != 10239) {
			t.Fatal("profile fixture was not projected as measured")
		}
	}
	const id = "0000000000000004"
	q := dbgen.New(pool)
	reads := []struct {
		name string
		read func() error
	}{
		{"detail", func() error { _, err := editor.PublishedSite(ctx, id); return err }},
		{"related", func() error { _, err := svc.Related(ctx, id); return err }},
		{"tech", func() error {
			_, err := q.SiteTechnology(ctx, dbgen.SiteTechnologyParams{PublicID: id, FactKeys: []string{"http.response", "tech.detected.astro", "tech.detected.wordpress"}})
			return err
		}},
	}
	for _, read := range reads {
		hits := func() int64 {
			return value[int64](t, pool, "SELECT blks_hit FROM pg_stat_database WHERE datname=current_database()")
		}
		execute(t, pool, "SELECT pg_stat_force_next_flush()")
		before := hits()
		sample := []metrics.Sample{{Name: "/cpu/classes/user:cpu-seconds"}}
		runtime.GC()
		metrics.Read(sample)
		cpu := sample[0].Value.Float64()
		const iterations = 30
		start := time.Now()
		for range iterations {
			must(t, read.read())
		}
		wall := time.Since(start) / iterations
		runtime.GC()
		metrics.Read(sample)
		execute(t, pool, "SELECT pg_stat_force_next_flush()")
		t.Logf("profile_sites=%d query=%s mean=%s go_user_cpu=%s pg_buffer_hits_total=%d",
			n, read.name, wall, time.Duration((sample[0].Value.Float64()-cpu)*float64(time.Second)/iterations), hits()-before)
	}
}
