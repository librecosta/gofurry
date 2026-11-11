//go:build integration

package collection

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func measureSiteYears(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	sizes := func() (heap, indexes, toast int64) {
		err := pool.QueryRow(ctx, `SELECT sum(pg_relation_size(oid))::bigint,sum(pg_indexes_size(oid))::bigint,
 sum(CASE WHEN reltoastrelid=0 THEN 0 ELSE pg_total_relation_size(reltoastrelid) END)::bigint
 FROM pg_class WHERE relname IN ('site_evidence_changes','site_daily_coverage')`).Scan(&heap, &indexes, &toast)
		must(t, err)
		return
	}
	baseHeap, baseIndexes, baseToast := sizes()
	wal := scalar[string](t, pool, "SELECT pg_current_wal_insert_lsn()::text")
	var total time.Duration
	from := 1
	for _, sites := range []int{100, 1000} {
		start := time.Now()
		tx, err := pool.Begin(ctx)
		must(t, err)
		_, err = tx.Exec(ctx, `CREATE TEMP TABLE IF NOT EXISTS b1_site_year (
 site_id uuid, address_id uuid, source_id uuid, binding_id uuid, target_id uuid, ordinal integer
) ON COMMIT DELETE ROWS`)
		must(t, err)
		_, err = tx.Exec(ctx, `INSERT INTO b1_site_year SELECT uuidv7(),uuidv7(),uuidv7(),uuidv7(),uuidv7(),i FROM generate_series($1::integer,$2::integer) i`, from, sites)
		must(t, err)
		for _, sql := range []string{
			`INSERT INTO sites (id,public_id,slug) SELECT site_id,lpad(ordinal::text,16,'0'),'measurement' FROM b1_site_year`,
			`INSERT INTO site_addresses (id,site_id,url,kind) SELECT address_id,site_id,'https://s'||ordinal||'.example.com','primary' FROM b1_site_year`,
			`INSERT INTO collection_sources (id,namespace,provider,resource_type,external_key,target_id)
 SELECT source_id,'storage_fixture','web','origin','https://s'||ordinal||'.example.com',target_id FROM b1_site_year`,
			`INSERT INTO collection_bindings (id,source_id,resource_type,site_id,address_id,purpose,effective_from)
 SELECT binding_id,source_id,'origin',site_id,address_id,'primary','2025-01-01 UTC' FROM b1_site_year`,
			`INSERT INTO site_daily_coverage (site_id,source_id,binding_id,capability,output_schema,scope_key,utc_day,confirmed_at,evidence_state,observation_id)
 SELECT site_id,source_id,binding_id,'web.http',1,repeat('a',64),date '2025-01-01'+d,'2025-01-01 UTC'::timestamptz+d*interval '1 day','present',uuidv7()
 FROM b1_site_year CROSS JOIN generate_series(0,364) d`,
			`INSERT INTO site_evidence_changes (id,site_id,binding_id,target_id,epoch,position,capability,output_schema,scope_key,scope,
 fact_key,event_kind,prior_value,value,ruleset,prior_ruleset,source_observation_id,content_hash,evidence,first_observed_at,source_observed_at,received_at)
 SELECT uuidv7(),site_id,binding_id,target_id,'0193c812-c600-7000-8000-000000000001',ordinal*4+d,'web.http',1,repeat('a',64),
 '{"namespace":"storage_fixture","egress_region":"cn","dimensions":{"section":"html"}}','tech.detected.ghost','change',
 '{"id":"ghost","category":"cms","confidence":"medium","evidence":["asset_path"]}',
 '{"id":"ghost","category":"cms","confidence":"high","evidence":["generator_meta","asset_path"]}',
 'web_rules_1','web_rules_1',uuidv7(),repeat('b',64),'html_parsed',date '2025-01-01'+d,date '2025-01-01'+d,date '2025-01-01'+d
 FROM b1_site_year CROSS JOIN generate_series(1,4) d`,
		} {
			_, err = tx.Exec(ctx, sql)
			if err != nil {
				_ = tx.Rollback(ctx)
				t.Fatal(err)
			}
		}
		must(t, tx.Commit(ctx))
		total += time.Since(start)
		from = sites + 1
		heap, indexes, toast := sizes()
		walBytes := scalar[int64](t, pool, "SELECT pg_wal_lsn_diff(pg_current_wal_insert_lsn(),$1::text::pg_lsn)::bigint", wal)
		count(t, pool, int64(sites)*365, "SELECT count(*) FROM site_daily_coverage WHERE utc_day>=date '2025-01-01' AND utc_day<date '2026-01-01'")
		count(t, pool, int64(sites)*4, "SELECT count(*) FROM site_evidence_changes WHERE first_observed_at>=date '2025-01-01' AND first_observed_at<date '2026-01-01'")
		t.Logf("synthetic site-years=%d coverage=%d transitions=%d SQL_wall=%s heap=%d indexes=%d toast=%d bytes_per_site_year=%d WAL=%d",
			sites, sites*365, sites*4, total, heap-baseHeap, indexes-baseIndexes, toast-baseToast,
			(heap+indexes+toast-baseHeap-baseIndexes-baseToast)/int64(sites), walBytes)
	}
}
