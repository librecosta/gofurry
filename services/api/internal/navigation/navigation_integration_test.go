//go:build integration

package navigation

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var fixtureConfig = flag.String("navigation-config", "", "Disposable navigation PostgreSQL YAML")
var fixtureRedis = flag.String("navigation-redis", "", "Disposable Redis loopback address")
var measure = flag.Bool("navigation-measure", false, "Measure directory and heat across representative Site counts")

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustError(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("got %v; expected %v", err, want)
	}
}

func execute(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	_, err := pool.Exec(t.Context(), sql, args...)
	must(t, err)
}

func value[T any](t *testing.T, pool *pgxpool.Pool, sql string, args ...any) (v T) {
	t.Helper()
	must(t, pool.QueryRow(t.Context(), sql, args...).Scan(&v))
	return v
}

func TestNavigationPostgreSQLRedis(t *testing.T) {
	if *fixtureConfig == "" || *fixtureRedis == "" {
		t.Skip("explicit disposable PostgreSQL and Redis fixtures required")
	}
	cfg, err := platform.LoadConfig(*fixtureConfig)
	must(t, err)
	d := cfg.Database
	host, port, err := net.SplitHostPort(*fixtureRedis)
	must(t, err)
	if d == nil || d.Name != "gofurry_d2_navigation_test" || d.Host != "127.0.0.1" || d.Port == 5432 ||
		d.User != "postgres" || cfg.Server.Environment != "development" || cfg.Deployment.Region != "CN" ||
		host != "127.0.0.1" || port == "6379" || cfg.Harvest != nil || cfg.Redis != nil {
		t.Fatal("isolated local fixtures required")
	}
	ctx := t.Context()
	pool, err := database.Open(ctx, *d)
	must(t, err)
	defer pool.Close()
	if v := value[int](t, pool, "SELECT current_setting('server_version_num')::int"); v < 180000 || v >= 190000 {
		t.Fatal("PostgreSQL 18 required")
	}
	if value[int](t, pool, "SELECT count(*) FROM pg_tables WHERE schemaname='public'") != 0 {
		t.Fatal("navigation fixture must be initially empty")
	}
	const migrations = "../../migrations"
	old := t.TempDir()
	for _, name := range []string{"202611050001_wallpapers.sql", "202611050002_official_wallpaper_format.sql",
		"202611110001_site_game_identity.sql", "202611110002_harvest_evidence.sql", "202611110003_site_yearbooks.sql", "202611110004_site_coverage_scan.sql"} {
		data, err := os.ReadFile(filepath.Join(migrations, name))
		must(t, err)
		must(t, os.WriteFile(filepath.Join(old, name), data, 0600))
	}
	must(t, database.Migrate(ctx, *d, old, "up", io.Discard))
	const oldID = "000000000000001v"
	legacy := uuid.NewV7()
	execute(t, pool, "INSERT INTO sites(id,public_id,slug,status,published_at) VALUES ($1,$2,'legacy-site','published','2025-11-01 UTC')", legacy, oldID)
	execute(t, pool, "INSERT INTO site_localizations VALUES ($1,'en','Legacy publication','Before the taxonomy')", legacy)
	execute(t, pool, "INSERT INTO site_addresses(id,site_id,url,kind) VALUES ($1,$2,'https://legacy.example.com','primary')", uuid.NewV7(), legacy)
	execute(t, pool, "INSERT INTO site_yearbooks(site_id,year,public_id,stored_slug,checked_at) VALUES ($1,2025,$2,'legacy-site',now())", legacy, oldID)
	execute(t, pool, "INSERT INTO collection_sources(id,namespace,provider,resource_type,external_key) VALUES ($1,'fixture','web','origin','https://legacy.example.com')", uuid.NewV7())
	execute(t, pool, `INSERT INTO start_wallpapers(id,title,status,image_key,image_sha256,image_type,image_bytes,width,height,
 preview_key,preview_sha256,preview_bytes,preview_width,preview_height,published_at)
 VALUES (uuidv7(),'Preserved fixture','published','start/backgrounds/'||repeat('a',64)||'.avif',repeat('a',64),
 'image/avif',1000,1920,1080,'start/backgrounds/'||repeat('b',64)||'.webp',repeat('b',64),100,480,270,now())`)
	must(t, database.Migrate(ctx, *d, migrations, "up", io.Discard))
	must(t, database.Migrate(ctx, *d, migrations, "up", io.Discard))
	if value[int](t, pool, "SELECT count(*) FROM site_yearbooks WHERE site_id=$1", legacy) != 1 ||
		value[int](t, pool, "SELECT count(*) FROM collection_sources WHERE namespace='fixture'") != 1 ||
		value[int](t, pool, "SELECT count(*) FROM start_wallpapers WHERE title='Preserved fixture' AND status='published'") != 1 ||
		value[int](t, pool, "SELECT count(*) FROM pg_constraint WHERE contype='f' AND conrelid::regclass::text IN ('site_navigation_reviews','site_navigation_features','site_interaction_days','site_navigation_heat','site_navigation_heat_state')") != 0 {
		t.Fatal("migration lost history or added foreign keys")
	}
	if !value[bool](t, pool, "SELECT legacy_detail AND content_rating='unreviewed' AND first_published_at IS NULL AND primary_category IS NULL FROM sites WHERE id=$1", legacy) {
		t.Fatal("migration invented editorial facts for a legacy Site")
	}
	editor := catalog.New(pool)
	oldDetail, err := editor.PublishedSite(ctx, oldID)
	must(t, err)
	if oldDetail.InteractionEligible {
		t.Fatal("unreviewed legacy detail enabled interaction collection")
	}
	svc := New(pool)
	now := time.Date(2027, 1, 8, 14, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	f := Filters{Locale: "en"}
	empty, err := svc.Directory(ctx, f, "")
	must(t, err)
	if empty.Total != 0 || len(empty.Items) != 0 || empty.Heat.Status != "cold" {
		t.Fatal("legacy publication was discovered or cold state was hidden")
	}
	_, err = svc.Stats(ctx, oldID)
	mustError(t, err, catalog.ErrNotFound)

	input := catalog.NavigationEdit{Rating: "sfw", Primary: "gallery", Secondary: []string{"community", "resources"},
		Tags: []string{"portfolio", "fursuit"}, Languages: []string{"ja"}, Actor: "fixture", Reason: "Verified classification"}
	add := func(name, zh, summary string, intent catalog.NavigationEdit) dbgen.Site {
		t.Helper()
		localizations := []catalog.Localization{{Locale: "en", Name: name, Summary: summary}}
		if zh != "" {
			localizations = append(localizations, catalog.Localization{Locale: "zh", Name: zh})
		}
		row, err := editor.CreateSite(ctx, catalog.SiteInput{Editorial: catalog.Editorial{Slug: "navigation-fixture", Localizations: localizations},
			Addresses: []catalog.Address{{Kind: "primary", URL: "https://site.example.com/entry?type=fixture"}}})
		must(t, err)
		profile, err := editor.ReviewSiteNavigation(ctx, "CN", row.PublicID, row.Revision, intent)
		must(t, err)
		if intent.Rating != "sfw" {
			return row
		}
		row, err = editor.SetSiteStatus(ctx, row.ID, profile.Revision, catalog.Published)
		must(t, err)
		if !row.FirstPublishedAt.Valid {
			t.Fatal("first publication timestamp missing")
		}
		return row
	}
	a := add("Art %_Hub", "兽装展示", "Bilingual fixture", input)
	b := add("Art %_Hub", "", "Another fixture", catalog.NavigationEdit{Rating: "sfw", Primary: "community",
		Tags: []string{"forum"}, Languages: []string{"en", "zh"}, Actor: "fixture", Reason: "Verified classification"})
	c := add("Different catalogue", "", "", input)
	draft := add("Hidden Secret", "", "", catalog.NavigationEdit{Rating: "unreviewed", Actor: "fixture", Reason: "Draft remains unreviewed"})
	for _, invalid := range []catalog.NavigationEdit{
		{Rating: "sfw", Primary: "other", Actor: "fixture", Reason: "invalid"},
		{Rating: "sfw", Primary: "gallery", Secondary: []string{"gallery"}, Actor: "fixture", Reason: "invalid"},
		{Rating: "sfw", Primary: "gallery", Secondary: []string{"resources", "events", "gaming"}, Actor: "fixture", Reason: "invalid"},
		{Rating: "sfw", Primary: "gallery", Tags: []string{"forum", "forum"}, Actor: "fixture", Reason: "invalid"},
		{Rating: "sfw", Primary: "gallery", Tags: []string{"unknown"}, Actor: "fixture", Reason: "invalid"},
		{Rating: "sfw", Primary: "gallery", Languages: []string{"zh-CN"}, Actor: "fixture", Reason: "invalid"},
		{Rating: "adult", Primary: "gallery", Actor: "fixture", Reason: "invalid CN"},
		{Rating: "mixed", Primary: "gallery", Actor: "fixture", Reason: "invalid CN"},
	} {
		_, err := editor.ReviewSiteNavigation(ctx, "CN", a.PublicID, a.Revision, invalid)
		mustError(t, err, catalog.ErrInvalid)
	}
	_, err = editor.ReviewSiteNavigation(ctx, "CN", a.PublicID, a.Revision-1, input)
	mustError(t, err, catalog.ErrConflict)
	if value[int](t, pool, "SELECT count(*) FROM site_navigation_reviews WHERE site_id=$1", a.ID) != 1 {
		t.Fatal("failed reviews were not atomic")
	}
	for _, tc := range []struct {
		query Filters
		want  int64
	}{
		{Filters{Locale: "en", Keyword: "ART %_hUB"}, 2}, {Filters{Locale: "zh", Keyword: "兽装展示"}, 1},
		{Filters{Locale: "zh", Keyword: "兽装"}, 2}, {Filters{Locale: "en", Keyword: "%_"}, 2},
		{Filters{Locale: "en", Keyword: "' OR 1=1 --"}, 0}, {Filters{Locale: "en", Keyword: "  "}, 3},
		{Filters{Locale: "en", Categories: []string{"community", "resources"}, Tags: []string{"portfolio", "forum"}, Languages: []string{"ja"}}, 2},
		{Filters{Locale: "en", Categories: []string{"marketplace"}, Languages: []string{"ja"}}, 0},
		{Filters{Locale: "en", Keyword: "Hidden Secret"}, 0},
	} {
		out, err := svc.Directory(ctx, tc.query, "")
		must(t, err)
		if out.Total != tc.want || int64(len(out.Items)) != tc.want {
			t.Fatalf("filter %+v matched %d (%d shown), expected %d", tc.query, out.Total, len(out.Items), tc.want)
		}
	}
	f.Sort, f.PageSize, f.Keyword = "name_asc", 1, "%_"
	page1, err := svc.Directory(ctx, f, "")
	must(t, err)
	page2, err := svc.Directory(ctx, f, page1.Next)
	must(t, err)
	if page1.Total != 2 || page2.Total != 2 || len(page1.Items) != 1 || len(page2.Items) != 1 ||
		page1.Items[0].PublicID >= page2.Items[0].PublicID || page2.Next != "" {
		t.Fatal("keyset did not preserve C-collation Public ID tie-breaking")
	}
	f.Keyword = "changed"
	_, err = svc.Directory(ctx, f, page1.Next)
	mustError(t, err, catalog.ErrInvalid)
	_, err = editor.SetNavigationFeature(ctx, "other", a.PublicID, 0, "fixture", "Invalid")
	mustError(t, err, catalog.ErrInvalid)
	_, err = editor.SetNavigationFeature(ctx, "secondary_2", draft.PublicID, 0, "fixture", "Invalid draft")
	mustError(t, err, catalog.ErrInvalid)
	rev, err := editor.SetNavigationFeature(ctx, "primary", a.PublicID, 0, "fixture", "Feature")
	must(t, err)
	_, err = editor.SetNavigationFeature(ctx, "secondary_1", a.PublicID, 0, "fixture", "Duplicate")
	mustError(t, err, catalog.ErrConflict)
	_, err = editor.SetNavigationFeature(ctx, "primary", b.PublicID, 0, "fixture", "Stale")
	mustError(t, err, catalog.ErrConflict)
	for _, row := range []struct {
		site               uuid.UUID
		ago, views, clicks int
	}{{a.ID, 0, 5, 2}, {a.ID, 1, 2, 1}, {a.ID, 6, 1, 0}, {a.ID, 7, 99, 0},
		{b.ID, 0, 6, 0}, {b.ID, 1, 4, 0}, {c.ID, 0, 99, 0}} {
		execute(t, pool, "INSERT INTO site_interaction_days VALUES ($1,$2,$3,$4)", row.site, pgDay(now.AddDate(0, 0, -row.ago)), row.views, row.clicks)
	}
	var wg sync.WaitGroup
	results := make(chan RefreshResult, 2)
	failures := make(chan error, 2)
	for range 2 {
		wg.Go(func() { r, err := svc.RefreshHeat(ctx, false); results <- r; failures <- err })
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		must(t, err)
	}
	published := 0
	for r := range results {
		if r.Refreshed && r.Ranked == 2 {
			published++
		}
	}
	if published != 1 || value[int64](t, pool, "SELECT score FROM site_navigation_heat WHERE site_id=$1", a.ID) != 1000137569 {
		t.Fatal("concurrent refresh, seven-day decay, or sample threshold was incorrect")
	}
	out, err := svc.Home(ctx)
	must(t, err)
	if out.Heat.Status != "ready" || out.Featured.Primary == nil || out.Featured.Primary.PublicID != a.PublicID ||
		out.Featured.Secondary1 != nil || len(out.Trending) != 1 || out.Trending[0].PublicID != b.PublicID ||
		len(out.Recent) != 1 || out.Recent[0].PublicID != c.PublicID || len(out.Categories) != 8 || out.Categories[0].Count != 3 {
		t.Fatalf("unexpected fixed discovery response %+v", out)
	}
	rank, err := svc.Directory(ctx, Filters{Locale: "en", Sort: "heat_7d", PageSize: 1}, "")
	must(t, err)
	if rank.Items[0].PublicID != a.PublicID || !rank.Items[0].HeatQualified {
		t.Fatal("featured selection altered the natural heat ranking")
	}
	low, err := svc.Stats(ctx, c.PublicID)
	must(t, err)
	if low.DetailViews != nil || low.OutboundClicks != nil {
		t.Fatal("low-sample stats were disclosed")
	}
	stats, err := svc.Stats(ctx, a.PublicID)
	must(t, err)
	if stats.DetailViews == nil || *stats.DetailViews != 8 || *stats.OutboundClicks != 3 {
		t.Fatal("stats did not use the seven-day UTC window")
	}
	now = now.AddDate(0, 0, 1)
	stale, err := svc.Directory(ctx, Filters{Locale: "en", Sort: "heat_7d", PageSize: 1}, "")
	must(t, err)
	if stale.Heat.Status != "stale" || stale.EffectiveSort != "published_desc" {
		t.Fatal("stale heat did not disclose its fallback")
	}
	_, err = svc.Directory(ctx, Filters{Locale: "en", Sort: "heat_7d", PageSize: 1}, rank.Next)
	mustError(t, err, catalog.ErrInvalid)
	// Two active days are also insufficient when there are only nine acts.
	execute(t, pool, "UPDATE site_interaction_days SET detail_views=8 WHERE site_id=$1", c.ID)
	execute(t, pool, "INSERT INTO site_interaction_days VALUES ($1,$2,1,0)", c.ID, pgDay(now.AddDate(0, 0, -2)))
	refresh, err := svc.RefreshHeat(ctx, false)
	must(t, err)
	if refresh.Ranked != 2 || value[int64](t, pool, "SELECT score FROM site_navigation_heat WHERE site_id=$1", a.ID) != 829600000 {
		t.Fatal("midnight decay or two-day low sample protection failed")
	}
	checkRedis(t, pool, svc, c.PublicID, c.ID)
	restricted := input
	restricted.Rating = "mixed"
	profile, err := editor.ReviewSiteNavigation(ctx, "INTL", a.PublicID, a.Revision, restricted)
	must(t, err)
	if profile.Rating != "mixed" {
		t.Fatal("INTL quarantine rating was not recorded")
	}
	_, err = editor.PublishedSite(ctx, a.PublicID)
	mustError(t, err, catalog.ErrNotFound)
	_, err = svc.Stats(ctx, a.PublicID)
	mustError(t, err, catalog.ErrNotFound)
	out, err = svc.Home(ctx)
	must(t, err)
	if out.Featured.Primary != nil || slices.ContainsFunc(out.Trending, func(card Card) bool { return card.PublicID == a.PublicID }) {
		t.Fatal("withdrawn rating leaked through saved feature or score")
	}
	slots, err := editor.NavigationFeatures(ctx)
	must(t, err)
	if slots[0].PublicID != a.PublicID || slots[0].Visible || slots[0].Revision != rev {
		t.Fatal("withdrawal lost internal feature assignment")
	}
	_, err = editor.SetNavigationFeature(ctx, "primary", "", rev, "fixture", "Clear")
	must(t, err)
	t.Log("forward migration and shared Redis; fixed discovery and heat v1 1000137569 -> 829600000; historic rows preserved")
	if *measure {
		measureNavigation(t, pool, svc)
	}
}

func checkRedis(t *testing.T, pool *pgxpool.Pool, svc *Service, id string, site uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	options := &platform.RedisConfig{Address: *fixtureRedis}
	client, other := platform.NewRedis(options), platform.NewRedis(options)
	defer client.Close()
	defer other.Close()
	must(t, client.Ping(ctx).Err())
	key := make([]byte, 32)
	key[0] = 42
	one, two := NewCounter(svc, client, key, "CN"), NewCounter(svc, other, key, "CN")
	now := time.Date(2027, 1, 9, 23, 55, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	count := func(counter *Counter, ip netip.Addr, kind string, want bool, wantErr error) {
		t.Helper()
		got, err := counter.Count(ctx, ip, id, kind)
		mustError(t, err, wantErr)
		if got != want {
			t.Fatalf("counter got %v, expected %v", got, want)
		}
	}
	ip := netip.MustParseAddr("192.0.2.50")
	count(one, ip, DetailView, true, nil)
	count(two, ip, DetailView, false, nil)
	count(two, ip, OutboundClick, true, nil)
	now = now.Add(10 * time.Minute)
	count(two, ip, DetailView, false, nil)
	now = now.Add(20 * time.Minute)
	count(two, ip, DetailView, true, nil)
	if value[int](t, pool, "SELECT detail_views FROM site_interaction_days WHERE site_id=$1 AND utc_day='2027-01-10'", site) != 1 ||
		value[int](t, pool, "SELECT outbound_clicks FROM site_interaction_days WHERE site_id=$1 AND utc_day='2027-01-09'", site) != 1 {
		t.Fatal("UTC interaction aggregation was incorrect")
	}
	ip = netip.MustParseAddr("192.0.2.51")
	for range 8 {
		count(one, ip, DetailView, true, nil)
		now = now.Add(30 * time.Minute)
	}
	count(two, ip, DetailView, false, ErrLimited)
	ip = netip.MustParseAddr("192.0.2.52")
	count(one, ip, DetailView, true, nil)
	for range 29 {
		count(two, ip, DetailView, false, nil)
	}
	count(one, ip, DetailView, false, ErrLimited)
	count(NewCounter(svc, nil, key, "CN"), ip, DetailView, false, catalog.ErrUnavailable)
	count(NewCounter(svc, other, nil, "CN"), ip, DetailView, false, catalog.ErrUnavailable)
	broken := NewCounter(New(nil), other, key, "CN")
	count(broken, ip, DetailView, false, catalog.ErrUnavailable)
	// Separate clients agree on the atomic reservation when called concurrently.
	ip = netip.MustParseAddr("192.0.2.53")
	ch := make(chan bool, 16)
	errCh := make(chan error, 16)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			counter := one
			if i%2 == 0 {
				counter = two
			}
			result, err := counter.Count(context.Background(), ip, id, OutboundClick)
			ch <- result
			errCh <- err
		})
	}
	wg.Wait()
	close(ch)
	close(errCh)
	for err := range errCh {
		must(t, err)
	}
	accepted := 0
	for result := range ch {
		if result {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("concurrent shared reservation counted %d times", accepted)
	}
	minute := one.prefix + "minute:" + strconv.FormatInt(now.Unix()/60, 10)
	must(t, client.Set(ctx, minute, 600, 2*time.Minute).Err())
	count(two, netip.MustParseAddr("192.0.2.54"), DetailView, false, ErrLimited)
	keys := client.Scan(ctx, 0, one.prefix+"*", 50).Iterator()
	for keys.Next(ctx) {
		key := keys.Val()
		ttl, err := client.TTL(ctx, key).Result()
		must(t, err)
		if ttl <= 0 || ttl > 48*time.Hour || strings.Contains(key, "192.0.2.") || strings.Contains(key, id) {
			t.Fatal("Redis kept unbounded or clear-text identity state")
		}
	}
	must(t, keys.Err())
}
