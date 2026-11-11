//go:build integration

package yearbook

import (
	"fmt"
	"runtime"
	"runtime/metrics"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type relationSize struct{ heap, index int64 }

func sizeFixture(t *testing.T, pool *pgxpool.Pool, name string) (size relationSize) {
	t.Helper()
	checked(t, pool.QueryRow(t.Context(), "SELECT pg_table_size($1::text::regclass),pg_indexes_size($1::text::regclass)", name).Scan(&size.heap, &size.index))
	return size
}

func measurePreviewCPU(t *testing.T, svc *Service, id string, year int32) {
	t.Helper()
	const n = 100
	sample := []metrics.Sample{{Name: "/cpu/classes/user:cpu-seconds"}}
	runtime.GC()
	metrics.Read(sample)
	before := sample[0].Value.Float64()
	start := time.Now()
	for range n {
		_, err := svc.Preview(t.Context(), id, year)
		checked(t, err)
	}
	wall := time.Since(start) / n
	runtime.GC()
	metrics.Read(sample)
	t.Logf("generation preview: mean wall=%s, approximate Go user CPU=%s (%d iterations, excludes PostgreSQL CPU)",
		wall, time.Duration((sample[0].Value.Float64()-before)*float64(time.Second)/n), n)
}

func measureYearbooks(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	svc := NewAt(pool, func() time.Time { return moment("2027-03-15T00:00:00Z") })
	_, err := svc.Compact(ctx)
	checked(t, err)
	execFixture(t, pool, "VACUUM (FULL, ANALYZE) site_daily_coverage")
	for i, sample := range []struct{ sites, scopes int }{{100, 1}, {1000, 1}, {100, 3}, {1000, 3}} {
		execFixture(t, pool, "VACUUM (FULL, ANALYZE) site_monthly_coverage")
		baseDaily := sizeFixture(t, pool, "site_daily_coverage")
		baseMonthly := sizeFixture(t, pool, "site_monthly_coverage")
		baseEvidence := sizeFixture(t, pool, "site_evidence_changes")
		ns := fmt.Sprintf("yearbook_measure_%d", i)
		first := 100000 + 10000*i
		begin := time.Now()
		execFixture(t, pool, `WITH fixture AS MATERIALIZED (
 SELECT i,uuidv7() AS site,uuidv7() AS source,uuidv7() AS binding,uuidv7() AS address,uuidv7() AS target,
  'https://s'||i||'.example.com' AS origin FROM generate_series($1::integer,$1::integer+$2::integer-1) i
), owners AS (
 INSERT INTO sites (id,public_id,slug,status,published_at)
 SELECT site,lpad(i::text,16,'0'),'measurement','published','2025-01-01 UTC' FROM fixture
), names AS (
 INSERT INTO site_localizations (site_id,locale,name,summary) SELECT site,'en','Measurement Site','' FROM fixture
), addresses AS (
 INSERT INTO site_addresses (id,site_id,url,kind) SELECT address,site,origin,'primary' FROM fixture
), sources AS (
 INSERT INTO collection_sources (id,namespace,provider,resource_type,external_key,target_id)
 SELECT source,$3,'web','origin',origin,target FROM fixture
), bindings AS (
 INSERT INTO collection_bindings (id,source_id,resource_type,site_id,address_id,purpose,effective_from)
 SELECT binding,source,'origin',site,address,'primary','2025-01-01 UTC' FROM fixture
)
INSERT INTO site_daily_coverage (site_id,source_id,binding_id,capability,output_schema,scope_key,utc_day,confirmed_at,evidence_state,observation_id)
SELECT site,source,binding,'web.http',1,lpad(to_hex(j),64,'0'),date '2025-01-01'+d,'2025-01-01 UTC'::timestamptz+d*interval '1 day','present',uuidv7()
FROM fixture CROSS JOIN generate_series(0,364) d CROSS JOIN generate_series(1,$4::integer) j`, first, sample.sites, ns, sample.scopes)
		execFixture(t, pool, `INSERT INTO site_evidence_changes (id,site_id,binding_id,target_id,epoch,position,capability,output_schema,scope_key,scope,
 fact_key,event_kind,prior_value,value,ruleset,prior_ruleset,source_observation_id,content_hash,evidence,first_observed_at,source_observed_at,received_at)
 SELECT uuidv7(),b.site_id,b.id,s.target_id,'0193c812-c600-7000-8000-000000000001',d,'web.http',1,lpad('1',64,'0'),
 '{"namespace":"measurement","egress_region":"cn","dimensions":{"section":"http"}}','http.response',
 CASE WHEN d=1 THEN 'baseline' ELSE 'change' END,
 jsonb_build_object('origin',s.external_key,'final_url','https://a'||(d-1)||'.example.com'),
 jsonb_build_object('origin',s.external_key,'final_url','https://a'||d||'.example.com'),
 'web_rules_1','web_rules_1',uuidv7(),repeat('b',64),'fixture_verified',
 date '2025-01-01'+d,date '2025-01-01'+d,date '2025-01-01'+d
 FROM collection_sources s JOIN collection_bindings b ON b.source_id=s.id CROSS JOIN generate_series(1,5) d WHERE s.namespace=$1`, ns)
		daily, evidence := sizeFixture(t, pool, "site_daily_coverage"), sizeFixture(t, pool, "site_evidence_changes")
		seed := time.Since(begin)
		for _, table := range []string{"site_daily_coverage", "collection_bindings", "collection_sources"} {
			execFixture(t, pool, "ANALYZE "+table)
		}
		wal := scalarFixture[string](t, pool, "SELECT pg_current_wal_insert_lsn()::text")
		begin = time.Now()
		var compacted int32
		for {
			n, err := svc.compact(ctx)
			if err != nil {
				t.Fatalf("sample %d sites x %d scopes: %d committed rows, last call compacted %d: %v", sample.sites, sample.scopes, compacted, n, err)
			}
			compacted += n
			if n == 0 {
				break
			}
		}
		compactWall := time.Since(begin)
		if compacted != int32(sample.sites*sample.scopes*365) {
			t.Fatalf("wrong measurement row count %d", compacted)
		}
		walBytes := scalarFixture[int64](t, pool, "SELECT pg_wal_lsn_diff(pg_current_wal_insert_lsn(),$1::text::pg_lsn)::bigint", wal)
		execFixture(t, pool, "VACUUM (FULL, ANALYZE) site_daily_coverage")
		monthly := sizeFixture(t, pool, "site_monthly_coverage")
		if count := scalarFixture[int](t, pool, `SELECT count(*) FROM site_monthly_coverage m JOIN collection_sources s ON s.id=m.source_id WHERE s.namespace=$1`, ns); count != sample.sites*sample.scopes*12 {
			t.Fatal("wrong monthly sample count")
		}
		publicID := fmt.Sprintf("%016d", first)
		var memBefore, memAfter runtime.MemStats
		runtime.ReadMemStats(&memBefore)
		begin = time.Now()
		result, err := svc.Generate(ctx, publicID, 2025)
		checked(t, err)
		generate := time.Since(begin)
		runtime.ReadMemStats(&memAfter)
		if !result.Published || result.Document.Coverage.Days != 365 {
			t.Fatal("measurement did not produce a full-year publication")
		}
		begin = time.Now()
		for range 20 {
			_, err = svc.Public(ctx, publicID, 2025)
			checked(t, err)
		}
		read := time.Since(begin) / 20
		payload := scalarFixture[int](t, pool, "SELECT octet_length(document::text) FROM site_yearbook_revisions r JOIN sites s ON s.id=r.site_id WHERE s.public_id=$1", publicID)
		rawBytes := daily.heap - baseDaily.heap + daily.index - baseDaily.index
		monthlyBytes := monthly.heap - baseMonthly.heap + monthly.index - baseMonthly.index
		ledgerBytes := evidence.heap - baseEvidence.heap + evidence.index - baseEvidence.index
		saving := 100 * float64(rawBytes-monthlyBytes) / float64(rawBytes+ledgerBytes)
		t.Logf("sample sites=%d scopes=%d daily_heap=%d daily_index=%d monthly_heap=%d monthly_index=%d ledger=%d combined_saving=%.1f%% compaction_WAL=%d seed=%s compact=%s generate=%s allocated=%d snapshot=%d public_read_avg=%s",
			sample.sites, sample.scopes, daily.heap-baseDaily.heap, daily.index-baseDaily.index, monthly.heap-baseMonthly.heap,
			monthly.index-baseMonthly.index, ledgerBytes, saving, walBytes, seed, compactWall, generate, memAfter.TotalAlloc-memBefore.TotalAlloc, payload, read)
	}
}
