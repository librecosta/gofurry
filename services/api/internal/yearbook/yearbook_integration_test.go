//go:build integration

package yearbook

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var (
	yearbookCN      = flag.String("yearbook-cn-config", "", "Explicit disposable D1 CN YAML")
	yearbookINTL    = flag.String("yearbook-intl-config", "", "Explicit disposable D1 INTL YAML")
	yearbookForward = flag.String("yearbook-forward-config", "", "Explicit empty disposable B2 upgrade YAML")
	yearbookMeasure = flag.Bool("yearbook-measure", false, "Measure synthetic isolated Site-years")
)

const (
	publishedSite = "000000000000000a"
	annualSite    = "0000000000000010"
	siteScope     = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	otherScope    = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func checked(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func moment(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}

func execFixture(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	_, err := pool.Exec(t.Context(), sql, args...)
	checked(t, err)
}

func scalarFixture[T any](t *testing.T, pool *pgxpool.Pool, sql string, args ...any) T {
	t.Helper()
	var value T
	checked(t, pool.QueryRow(t.Context(), sql, args...).Scan(&value))
	return value
}

func fixture(t *testing.T, yaml, region, name string) (platform.DatabaseConfig, *pgxpool.Pool) {
	t.Helper()
	cfg, err := platform.LoadConfig(yaml)
	checked(t, err)
	d := cfg.Database
	if cfg.Deployment.Region != region || cfg.Server.Environment != "development" || d == nil ||
		d.Host != "127.0.0.1" || d.Name != name || d.Port == 5432 || d.User != "postgres" ||
		cfg.Storage != nil || cfg.Redis != nil || cfg.Harvest != nil || cfg.Management != nil {
		t.Fatal("not a disposable yearbook fixture")
	}
	pool, err := database.Open(t.Context(), *d)
	checked(t, err)
	t.Cleanup(pool.Close)
	if n := scalarFixture[int](t, pool, "SELECT current_setting('server_version_num')::integer"); n < 180000 || n >= 190000 {
		t.Fatal("PostgreSQL 18 is required")
	}
	return *d, pool
}

type bindingFixture struct {
	site, source, binding, target uuid.UUID
	host                          string
}

func newSiteFixture(t *testing.T, pool *pgxpool.Pool, id, status, locale string) uuid.UUID {
	t.Helper()
	site := uuid.NewV7()
	execFixture(t, pool, `INSERT INTO sites (id,public_id,slug,status,published_at,archived_at) VALUES
 ($1,$2,'evidence-site',$3,CASE WHEN $3='published' THEN timestamptz '2024-01-01 UTC' END,
 CASE WHEN $3='archived' THEN timestamptz '2026-10-01 UTC' END)`, site, id, status)
	execFixture(t, pool, "INSERT INTO site_localizations VALUES ($1,$2,'Evidence Site','Editorial summary')", site, locale)
	return site
}

func bindFixture(t *testing.T, pool *pgxpool.Pool, site uuid.UUID, hostname, capability string, from time.Time, until *time.Time) bindingFixture {
	t.Helper()
	f := bindingFixture{site, uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), hostname}
	address := uuid.NewV7()
	execFixture(t, pool, "INSERT INTO site_addresses (id,site_id,url,kind,active) VALUES ($1,$2,$3,'primary',false)", address, site, "https://"+hostname)
	resource, purpose, key := "origin", "primary", "https://"+hostname
	if capability == "web.dns" {
		resource, purpose, key = "domain", "domain", hostname
	}
	execFixture(t, pool, "INSERT INTO collection_sources (id,namespace,provider,resource_type,external_key,target_id) VALUES ($1,'fixture','web',$2,$3,$4)",
		f.source, resource, key, f.target)
	execFixture(t, pool, `INSERT INTO collection_bindings (id,source_id,resource_type,site_id,address_id,purpose,effective_from,effective_until)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, f.binding, f.source, resource, site, address, purpose, from, until)
	return f
}

func confirmFixture(t *testing.T, pool *pgxpool.Pool, f bindingFixture, capability, scope string, when time.Time) {
	t.Helper()
	err := dbgen.New(pool).PutSiteDailyCoverage(t.Context(), dbgen.PutSiteDailyCoverageParams{
		SiteID: f.site, SourceID: f.source, BindingID: f.binding, Capability: capability, OutputSchema: 1, ScopeKey: scope,
		ConfirmedAt: stamp(when), EvidenceState: "present", ObservationID: uuid.NewV7(),
	})
	checked(t, err)
}

func factFixture(t *testing.T, pool *pgxpool.Pool, f bindingFixture, capability, scope, section, key, kind string, when time.Time, value, prior, rules, priorRules string) {
	t.Helper()
	scopeJSON := fmt.Sprintf(`{"namespace":"fixture","egress_region":"cn","dimensions":{"section":%q}}`, section)
	if section == "dns" {
		scopeJSON = `{"namespace":"fixture","egress_region":"cn","dimensions":{"section":"dns","rrtype":"txt","resolver":"google-public-dns"}}`
	}
	var raw, before []byte
	if value != "" {
		raw = []byte(value)
	}
	if prior != "" {
		before = []byte(prior)
	}
	id := uuid.NewV7()
	sourceID, evidence := &id, "fixture_verified"
	if kind == "withdrawn" {
		sourceID, evidence = nil, "scope_withdrawn"
	}
	err := dbgen.New(pool).AddSiteEvidenceChange(t.Context(), dbgen.AddSiteEvidenceChangeParams{
		ID: uuid.NewV7(), SiteID: f.site, BindingID: f.binding, TargetID: f.target, Epoch: uuid.NewV7(),
		Position: 1, Capability: capability, OutputSchema: 1, ScopeKey: scope, Scope: []byte(scopeJSON), FactKey: key,
		EventKind: kind, PriorValue: before, Value: raw, Ruleset: rules, PriorRuleset: priorRules,
		SourceObservationID: sourceID, ContentHash: siteScope, Evidence: evidence,
		FirstObservedAt: stamp(when), SourceObservedAt: stamp(when), ReceivedAt: stamp(when.Add(time.Minute)),
	})
	checked(t, err)
}

func httpFixture(t *testing.T, pool *pgxpool.Pool, f bindingFixture, when time.Time) {
	t.Helper()
	factFixture(t, pool, f, "web.http", siteScope, "http", "http.response", "baseline", when,
		fmt.Sprintf(`{"origin":"https://%s","final_url":"https://%s/path?token=PRIVATE-FIXTURE","status":200,"raw":"<script>FIXTURE</script>"}`, f.host, f.host),
		"", "web_rules_1", "")
}

func TestYearbookPostgreSQL(t *testing.T) {
	if *yearbookCN == "" || *yearbookINTL == "" || *yearbookForward == "" {
		t.Skip("explicit disposable catalog and upgrade fixtures required")
	}
	ctx := t.Context()
	cfg, pool := fixture(t, *yearbookForward, "CN", "gofurry_d1_forward_test")
	if n := scalarFixture[int](t, pool, "SELECT count(*) FROM pg_tables WHERE schemaname='public'"); n != 0 {
		t.Fatal("upgrade fixture must be empty")
	}
	const migrations = "../../migrations"
	b1 := t.TempDir()
	for _, name := range []string{"202611050001_wallpapers.sql", "202611050002_official_wallpaper_format.sql",
		"202611110001_site_game_identity.sql", "202611110002_harvest_evidence.sql"} {
		raw, err := os.ReadFile(filepath.Join(migrations, name))
		checked(t, err)
		checked(t, os.WriteFile(filepath.Join(b1, name), raw, 0600))
	}
	checked(t, database.Migrate(ctx, cfg, b1, "up", io.Discard))
	if n := scalarFixture[int](t, pool, fkCountSQL); n != 17 {
		t.Fatalf("B1 reference inventory changed: %d", n)
	}
	site := newSiteFixture(t, pool, annualSite, "published", "en")
	execFixture(t, pool, "INSERT INTO site_localizations VALUES ($1,'zh','证据站点','编辑简介')", site)
	start := moment("2026-11-11T02:00:00Z")
	move := start.AddDate(0, 0, 13)
	secondMove := start.AddDate(0, 0, 23)
	one := bindFixture(t, pool, site, "old-yearbook.example.com", "web.http", start.Add(-time.Hour), &move)
	two := bindFixture(t, pool, site, "middle-yearbook.example.com", "web.http", move, &secondMove)
	three := bindFixture(t, pool, site, "new-yearbook.example.com", "web.http", secondMove, nil)
	confirmFixture(t, pool, one, "web.http", siteScope, start)
	httpFixture(t, pool, one, start)
	checked(t, database.Migrate(ctx, cfg, migrations, "up", io.Discard))
	checked(t, database.Migrate(ctx, cfg, migrations, "up", io.Discard))
	if n := scalarFixture[int](t, pool, fkCountSQL); n != 0 {
		t.Fatalf("B1/B2 foreign keys remaining: %d", n)
	}
	if n := scalarFixture[int](t, pool, `SELECT count(*) FROM pg_constraint WHERE contype='f' AND conrelid::regclass::text IN
 ('site_localizations','site_addresses','game_localizations','game_sources') AND confdeltype IN ('a','r')`); n != 4 {
		t.Fatal("editorial aggregate foreign keys changed")
	}
	if n := scalarFixture[int](t, pool, "SELECT (SELECT count(*) FROM site_evidence_changes)+(SELECT count(*) FROM site_daily_coverage)"); n != 2 {
		t.Fatal("forward migration did not preserve B1 records")
	}
	svc := New(pool)
	svc.now = func() time.Time { return start }
	checkEligibility(t, svc, annualSite, 2026, false, "grace_period_open")
	if _, err := svc.Generate(ctx, annualSite, 2026); !errors.Is(err, ErrEarly) {
		t.Fatal("2026 was writable before closure")
	}
	for i := range 14 {
		when := start.AddDate(0, 0, 2*i)
		f := one
		if !when.Before(secondMove) {
			f = three
		} else if !when.Before(move) {
			f = two
		}
		confirmFixture(t, pool, f, "web.http", siteScope, when)
		confirmFixture(t, pool, f, "web.http", otherScope, when.Add(time.Minute))
	}
	httpFixture(t, pool, two, move)
	httpFixture(t, pool, three, secondMove)
	svc.now = func() time.Time { return moment("2027-01-07T23:59:59Z") }
	checkEligibility(t, svc, annualSite, 2026, false, "grace_period_open")
	svc.now = func() time.Time { return moment("2027-01-08T00:00:00Z") }
	checkEligibility(t, svc, annualSite, 2026, false, "coverage_span_too_short")
	confirmFixture(t, pool, three, "web.http", siteScope, start.AddDate(0, 0, 27))
	// Keep fourteen days and make the first-to-last inclusive span exactly 28.
	execFixture(t, pool, "DELETE FROM site_daily_coverage WHERE site_id=$1 AND utc_day=$2", site, date(start.AddDate(0, 0, 26)))
	checkEligibility(t, svc, annualSite, 2026, true, "")
	var group sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		group.Go(func() { _, err := svc.reconcile(ctx, 2026, true, true); errs <- err })
	}
	group.Wait()
	close(errs)
	for err := range errs {
		checked(t, err)
	}
	pub, err := svc.Public(ctx, annualSite, 2026)
	checked(t, err)
	if pub.Revision != 1 || pub.Document.Coverage.Days != 14 || !pub.Document.Coverage.PartialYear ||
		pub.Document.Coverage.RecordedFrom != "2026-11-11" || pub.Document.Coverage.RecordedTo != "2026-12-08" || len(pub.Document.Timeline) != 2 {
		t.Fatalf("wrong closure: rev=%d coverage=%+v timeline=%+v", pub.Revision, pub.Document.Coverage, pub.Document.Timeline)
	}
	november := New(pool)
	november.now = func() time.Time { return start }
	if _, err := november.Public(ctx, annualSite, 2026); !errors.Is(err, ErrMissing) {
		t.Fatal("a snapshot was publicly readable before its grace window")
	}
	if n := scalarFixture[int](t, pool, "SELECT count(*) FROM site_yearbook_revisions WHERE site_id=$1", site); n != 1 {
		t.Fatal("concurrent workers created duplicate revisions")
	}
	before, err := svc.Preview(ctx, annualSite, 2026)
	checked(t, err)
	svc.now = func() time.Time { return moment("2027-03-10T00:00:00Z") }
	tx, err := pool.Begin(ctx)
	checked(t, err)
	testRollup, err := dbgen.New(tx).CompactSiteCoverage(ctx, dbgen.CompactSiteCoverageParams{BeforeDay: date(svc.now()), AfterDay: date(time.Time{}), BatchSize: 3})
	checked(t, err)
	if testRollup.Selected != 3 || testRollup.Pruned != 3 {
		t.Fatal("bounded rollup did not select/verify the same rows")
	}
	checked(t, tx.Rollback(ctx))
	if n := scalarFixture[int](t, pool, "SELECT count(*) FROM site_monthly_coverage"); n != 0 {
		t.Fatal("rollback left partial rollup")
	}
	pruned, err := svc.Compact(ctx)
	checked(t, err)
	if pruned != 27 {
		t.Fatalf("compacted %d raw confirmations, expected 27", pruned)
	}
	after, err := svc.Preview(ctx, annualSite, 2026)
	checked(t, err)
	if before.Digest != after.Digest || before.Document.Coverage.Months != after.Document.Coverage.Months {
		t.Fatal("compaction changed semantic digest or day set")
	}
	same, err := svc.Generate(ctx, annualSite, 2026)
	checked(t, err)
	if !same.Published || same.Revision != 1 || same.Digest != before.Digest {
		t.Fatal("an identical regeneration did not preserve the first revision")
	}
	// Replay an already compacted confirmation and deliver a genuinely late date.
	confirmFixture(t, pool, one, "web.http", siteScope, start)
	confirmFixture(t, pool, three, "web.http", siteScope, start.AddDate(0, 0, 29))
	_, err = svc.Compact(ctx)
	checked(t, err)
	execFixture(t, pool, "DELETE FROM collection_bindings WHERE id=$1", one.binding)
	execFixture(t, pool, "DELETE FROM collection_sources WHERE id=$1", one.source)
	execFixture(t, pool, "UPDATE sites SET slug='renamed-evidence-site' WHERE id=$1", site)
	execFixture(t, pool, "UPDATE site_localizations SET name='Current name' WHERE site_id=$1 AND locale='en'", site)
	corrected, err := svc.Generate(ctx, annualSite, 2026)
	checked(t, err)
	if corrected.Revision != 2 || corrected.Published || corrected.Document.Coverage.Days != 15 || len(corrected.Document.Timeline) != 2 {
		t.Fatal("late data, archived provenance or immutable correction failed")
	}
	still, err := svc.Public(ctx, annualSite, 2026)
	checked(t, err)
	if still.Revision != 1 || still.Slug != "renamed-evidence-site" || still.Document.Names[0].Name == still.CurrentNames[0].Name {
		t.Fatal("draft correction overwrote the public snapshot or canonical identity")
	}
	if err := svc.Publish(ctx, annualSite, 2026, 2, strings.Repeat("f", 64)); !errors.Is(err, ErrConflict) {
		t.Fatal("stale digest could be published")
	}
	checked(t, svc.Publish(ctx, annualSite, 2026, 2, corrected.Digest))
	if row, err := NewAt(pool, svc.now).Public(ctx, annualSite, 2026); err != nil || row.Revision != 2 {
		t.Fatalf("reviewed revision unavailable after restart: %v", err)
	}
	execFixture(t, pool, "UPDATE sites SET status='archived',archived_at=now() WHERE id=$1", site)
	archived, err := svc.Public(ctx, annualSite, 2026)
	checked(t, err)
	if archived.SiteAvailable {
		t.Fatal("archived Site remained linked")
	}
	if *yearbookMeasure {
		measurePreviewCPU(t, svc, annualSite, 2026)
	}
	checked(t, svc.Hide(ctx, annualSite, 2026))
	if _, err := svc.Public(ctx, annualSite, 2026); !errors.Is(err, ErrMissing) {
		t.Fatal("hidden revision remained publicly readable")
	}
	if n := scalarFixture[int](t, pool, "SELECT count(*) FROM site_yearbook_revisions WHERE site_id=$1 AND first_published_at IS NOT NULL", site); n != 2 {
		t.Fatal("historical public revisions were lost")
	}
	audit, err := svc.Audit(ctx)
	checked(t, err)
	for _, row := range audit {
		if row.Mismatches != 0 {
			t.Fatalf("orphan audit %s=%d", row.Kind, row.Mismatches)
		}
	}
	checkOtherEvidence(t, pool, svc)

	for _, target := range []struct{ path, region, name string }{
		{*yearbookCN, "CN", "gofurry_d1_cn_test"}, {*yearbookINTL, "INTL", "gofurry_d1_intl_test"},
	} {
		_, db := fixture(t, target.path, target.region, target.name)
		seedPublicYearbook(t, db)
	}
	t.Logf("mock closure 2027-01-08 UTC: Site=%s year=2026 rev=1, %d/%d days, %d host changes, %d raw confirmations compacted, correction rev=2 with %d days",
		annualSite, pub.Document.Coverage.Days, pub.Document.Coverage.PeriodDays, len(pub.Document.Timeline), pruned, corrected.Document.Coverage.Days)
	if *yearbookMeasure {
		measureYearbooks(t, pool)
	}
}

func NewAt(pool *pgxpool.Pool, now func() time.Time) *Service {
	s := New(pool)
	s.now = now
	return s
}

const fkCountSQL = `SELECT count(*) FROM pg_constraint WHERE contype='f' AND
 (conrelid::regclass::text LIKE 'collection_%' OR conrelid::regclass::text LIKE 'site_evidence_%' OR
  conrelid::regclass::text LIKE 'site_%_coverage' OR conrelid::regclass::text LIKE 'game_evidence_%' OR conrelid::regclass::text LIKE 'site_yearbook%')`

func checkEligibility(t *testing.T, svc *Service, id string, year int32, want bool, reason string) {
	t.Helper()
	result, err := svc.Preview(t.Context(), id, year)
	checked(t, err)
	if result.Eligible != want || (reason != "" && !slices.Contains(result.Reasons, reason)) {
		t.Fatalf("eligibility %s %d: %v %v", id, year, result.Eligible, result.Reasons)
	}
}

func checkOtherEvidence(t *testing.T, pool *pgxpool.Pool, svc *Service) {
	t.Helper()
	ctx := t.Context()
	start := moment("2024-02-02T23:30:00Z")
	s := newSiteFixture(t, pool, "0000000000000011", "published", "en")
	web := bindFixture(t, pool, s, "leap-yearbook.example.com", "web.http", start.Add(-time.Hour), nil)
	checkEligibility(t, svc, "0000000000000011", 2024, false, "insufficient_evidence")
	httpFixture(t, pool, web, start)
	for i := range 13 {
		confirmFixture(t, pool, web, "web.http", siteScope, start.AddDate(0, 0, 2*i))
	}
	checkEligibility(t, svc, "0000000000000011", 2024, false, "insufficient_evidence")
	confirmFixture(t, pool, web, "web.http", siteScope, start.AddDate(0, 0, 27).In(time.FixedZone("UTC+8", 8*3600)))
	confirmFixture(t, pool, web, "web.http", siteScope, start.AddDate(0, 0, -1)) // before approved binding
	checkEligibility(t, svc, "0000000000000011", 2024, true, "")
	dns := bindFixture(t, pool, s, "dns-yearbook.example.com", "web.dns", start, nil)
	factFixture(t, pool, dns, "web.dns", otherScope, "dns", "dns.rrset", "baseline", start,
		`{"rrset_sha256":"`+siteScope+`","vantage":"recursive_doh","records":[{"value":"DNS-SECRET-FIXTURE"}]}`, "", "gofurry-evidence-1", "")
	factFixture(t, pool, dns, "web.dns", otherScope, "dns", "dns.rrset", "change", start.Add(time.Hour),
		`{"rrset_sha256":"`+siteScope+`","vantage":"recursive_doh","ttl":30}`,
		`{"rrset_sha256":"`+siteScope+`","vantage":"recursive_doh","ttl":300}`, "gofurry-evidence-1", "gofurry-evidence-1")
	tech := `{"id":"ghost","confidence":"high","evidence":["generator_meta"]}`
	factFixture(t, pool, web, "web.http", otherScope, "html", "tech.detected.ghost", "change", start.Add(time.Hour), tech, "", "web_rules_2", "web_rules_1")
	factFixture(t, pool, web, "web.http", otherScope, "html", "tech.detected.ghost", "withdrawn", start.Add(2*time.Hour), "", tech, "web_rules_2", "web_rules_2")
	tls := func(hash string) string {
		return `{"verified":true,"certificate_sha256":"` + hash + `","spki_sha256":"` + siteScope + `"}`
	}
	factFixture(t, pool, web, "web.tls", otherScope, "tls", "tls.certificate", "baseline", start, tls(siteScope), "", "gofurry-evidence-1", "")
	factFixture(t, pool, web, "web.tls", otherScope, "tls", "tls.certificate", "change", start.Add(3*time.Hour), tls(otherScope), tls(siteScope), "gofurry-evidence-1", "gofurry-evidence-1")
	result, err := svc.Generate(ctx, "0000000000000011", 2024)
	checked(t, err)
	if result.Document.Coverage.PeriodDays != 366 || result.Document.Coverage.Days != 14 || result.Document.Coverage.Months[1]&(1<<28) == 0 ||
		len(result.Document.Timeline) != 1 || result.Document.Timeline[0].Kind != "tls" ||
		slices.ContainsFunc(result.Document.YearEnd, func(f Fact) bool { return f.Kind == "technology" }) {
		t.Fatal("UTC/leap coverage, TTL jitter, detector revision or scoped tombstone failed")
	}
	raw, err := json.Marshal(result.Document)
	checked(t, err)
	for _, token := range []string{"DNS-SECRET-FIXTURE", "PRIVATE-FIXTURE", "<script>", web.binding.String(), dns.target.String()} {
		if strings.Contains(string(raw), token) {
			t.Fatal("public document includes private source data")
		}
	}
	missing := newSiteFixture(t, pool, "0000000000000012", "published", "en")
	b := bindFixture(t, pool, missing, "no-baseline.example.com", "web.http", start, nil)
	for i := range 14 {
		confirmFixture(t, pool, b, "web.http", siteScope, start.AddDate(0, 0, 3*i))
	}
	checkEligibility(t, svc, "0000000000000012", 2024, false, "verified_baseline_missing")
	// A prior-year correction remains last known, and its verified older
	// baseline can qualify a later year without an annual change event.
	priorID := "0000000000000013"
	priorSite := newSiteFixture(t, pool, priorID, "published", "en")
	prior := bindFixture(t, pool, priorSite, "prior.example.com", "web.http", moment("2023-01-01T00:00:00Z"), nil)
	httpFixture(t, pool, prior, moment("2023-10-01T00:00:00Z"))
	old := `{"origin":"https://prior.example.com","final_url":"https://prior.example.com"}`
	revised := `{"origin":"https://prior.example.com","final_url":"https://revised.example.com"}`
	factFixture(t, pool, prior, "web.http", siteScope, "http", "http.response", "change",
		moment("2023-12-01T00:00:00Z"), revised, old, "web_rules_1", "web_rules_1")
	for i := range 14 {
		confirmFixture(t, pool, prior, "web.http", siteScope, start.AddDate(0, 0, 3*i))
	}
	factFixture(t, pool, prior, "web.http", siteScope, "http", "http.response", "change",
		moment("2025-01-01T00:00:00Z"), old, revised, "web_rules_1", "web_rules_1")
	carried, err := svc.Preview(ctx, priorID, 2024)
	checked(t, err)
	if !carried.Eligible || len(carried.Document.Timeline) != 0 || len(carried.Document.YearEnd) != 1 ||
		!carried.Document.YearEnd[0].PriorKnown || carried.Document.YearEnd[0].Observed != "2023-12-01T00:00:00Z" ||
		carried.Document.YearEnd[0].Value != "revised.example.com" {
		t.Fatal("prior-known observation lost its real cutoff, observation date or baseline")
	}
	// Read-only audit reports invalid cross-owner references without deleting data.
	execFixture(t, pool, "UPDATE site_daily_coverage SET binding_id=$1 WHERE site_id=$2", uuid.NewV7(), missing)
	audit, err := svc.Audit(ctx)
	checked(t, err)
	if !slices.ContainsFunc(audit, func(row dbgen.AuditYearbookReferencesRow) bool {
		return row.Kind == "daily_binding" && row.Mismatches == 14
	}) {
		t.Fatal("ownership audit did not identify orphan coverage")
	}
	execFixture(t, pool, "UPDATE site_daily_coverage SET binding_id=$1 WHERE site_id=$2", b.binding, missing)
	rollup, err := pool.Begin(ctx)
	checked(t, err)
	defer rollup.Rollback(ctx)
	_, err = dbgen.New(rollup).CompactSiteCoverage(ctx, dbgen.CompactSiteCoverageParams{
		BeforeDay: date(svc.now()), AfterDay: date(time.Time{}), BatchSize: 2048,
	})
	checked(t, err)
	closeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	closed := make(chan error, 1)
	go func() {
		closed <- pgx.BeginFunc(closeCtx, pool, func(tx pgx.Tx) error {
			q := dbgen.New(tx)
			if err := q.CloseCollectionBinding(closeCtx, dbgen.CloseCollectionBindingParams{ID: prior.binding, Namespace: "fixture"}); err != nil {
				return err
			}
			return q.CloseCollectionBindingCoverage(closeCtx, dbgen.CloseCollectionBindingCoverageParams{ID: prior.binding, Namespace: "fixture"})
		})
	}()
	for scalarFixture[int](t, pool, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database()
 AND wait_event_type='Lock' AND query LIKE '%name: CloseCollectionBinding :exec%'`) == 0 {
		select {
		case err := <-closed:
			t.Fatalf("binding close did not wait for in-flight compaction: %v", err)
		case <-closeCtx.Done():
			t.Fatal("binding close did not reach the compaction lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
	checked(t, rollup.Commit(ctx))
	checked(t, <-closed)
	if n := scalarFixture[int](t, pool, `SELECT count(*) FROM site_monthly_coverage m JOIN collection_bindings b ON m.binding_id=b.id
 WHERE m.binding_id=$1 AND m.binding_until=b.effective_until`, prior.binding); n == 0 {
		t.Fatal("closing a previously compacted binding did not preserve the closing date in history")
	}
}

func seedPublicYearbook(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	site := scalarFixture[uuid.UUID](t, pool, "SELECT id FROM sites WHERE public_id=$1", publishedSite)
	start := moment("2025-11-11T10:00:00Z")
	b := bindFixture(t, pool, site, "public-yearbook.example.com", "web.http", start, nil)
	httpFixture(t, pool, b, start)
	for i := range 14 {
		confirmFixture(t, pool, b, "web.http", siteScope, start.AddDate(0, 0, 3*i))
	}
	// The running Fiber process performs first publication in this fixture.
}
