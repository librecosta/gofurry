//go:build integration

package collection

import (
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"io"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestCollectionHarvestD(t *testing.T) {
	if *harvestSource == "" {
		t.Skip("explicit read-only Harvest V0.1-D source required")
	}
	ctx := t.Context()
	goDB, goContainer := fixtureDB(t, "gofurry")
	harvestDB, _ := fixtureDB(t, "harvest")
	l := newLab(t, harvestDB)
	cut := proxyLab(t, l)
	must(t, database.Migrate(ctx, goDB, "../../migrations", "up", io.Discard))
	pool, err := database.Open(ctx, goDB)
	must(t, err)
	t.Cleanup(pool.Close)
	remotePool, err := database.Open(ctx, harvestDB)
	must(t, err)
	t.Cleanup(remotePool.Close)
	unauthorized := harvestDB
	unauthorized.User = goDB.User
	wrong, err := database.Open(ctx, unauthorized)
	must(t, err)
	if wrong.Ping(ctx) == nil {
		t.Fatal("GoFurry database login connected to Harvest")
	}
	wrong.Close()
	var version string
	must(t, pool.QueryRow(ctx, "SHOW server_version").Scan(&version))
	if !strings.HasPrefix(version, "18.") {
		t.Fatal("fixture requires PostgreSQL 18")
	}

	var enrolled ackView
	client := &client{base: "http://" + l.address, http: l.http}
	enrollment := struct {
		Stream    string `json:"stream"`
		Name      string `json:"name"`
		Principal string `json:"principal"`
	}{fixtureStream, "gofurry", "b1_reader"}
	must(t, client.call(ctx, l.operator, "POST", "consumers", nil, enrollment, &enrolled, 200))
	if !v7(enrolled.ID) {
		t.Fatal("invalid test Consumer enrollment")
	}
	if err := client.call(ctx, l.reader, "POST", "consumers", nil, enrollment, &ackView{}, 200); !errors.Is(err, remoteStatus(403)) {
		t.Fatal("reader could enroll a Consumer")
	}
	if err := client.call(ctx, l.writer, "POST", "consumers/"+enrolled.ID.String()+"/reset", nil, struct {
		Stream string `json:"stream"`
	}{fixtureStream}, &resetView{}, 200); !errors.Is(err, remoteStatus(404)) {
		t.Fatal("writer could reset reader Consumer")
	}

	cfg := platform.HarvestConfig{URL: cut.server.URL, Namespace: fixtureStream, WriterToken: l.writer, ReaderToken: l.reader,
		ConsumerID: enrolled.ID.String(), Consume: true, PollInterval: time.Second, PageSize: 2}
	svc := New(pool, cfg)
	t.Cleanup(svc.Close)
	q := dbgen.New(pool)
	editor := catalog.New(pool)
	text := catalog.Editorial{Slug: "curated", Localizations: []catalog.Localization{{Locale: "en", Name: "Curated name", Summary: "Editorial text"}}}
	oldAddress := "https://content.example.com/business?view=all"
	makeSite := func() (dbgen.Site, []dbgen.CollectionBinding) {
		site, err := editor.CreateSite(ctx, catalog.SiteInput{Editorial: text, Addresses: []catalog.Address{{Kind: "primary", URL: oldAddress}}})
		must(t, err)
		site, err = editor.SetSiteStatus(ctx, site.ID, site.Revision, catalog.Published)
		must(t, err)
		bindings, err := svc.BindSite(ctx, site.PublicID, oldAddress)
		must(t, err)
		return site, bindings
	}
	siteA, bindA := makeSite()
	siteB, bindB := makeSite()
	if bindA[0].SourceID != bindB[0].SourceID || bindA[1].SourceID != bindB[1].SourceID {
		t.Fatal("shared source was cloned")
	}
	game, err := editor.CreateGame(ctx, catalog.GameInput{Editorial: text, Sources: []catalog.Source{{PlatformCode: "steam", ExternalKey: "42"}}})
	must(t, err)
	game, err = editor.SetGameStatus(ctx, game.ID, game.Revision, catalog.Published)
	must(t, err)
	gameBind, err := svc.BindGame(ctx, game.PublicID, "42")
	must(t, err)
	without, err := editor.CreateGame(ctx, catalog.GameInput{Editorial: text})
	must(t, err)
	without, err = editor.SetGameStatus(ctx, without.ID, without.Revision, catalog.Published)
	must(t, err)
	if _, err = svc.BindGame(ctx, without.PublicID, "42"); !errors.Is(err, ErrIntent) {
		t.Fatal("Game without Steam accepted a binding")
	}
	anchor := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	plan := func(binding uuid.UUID, cap, locale, region string, rr ...string) uuid.UUID {
		id, err := svc.AddPlan(ctx, binding, Plan{Capability: cap, Locale: locale, Region: region, EgressRegion: "cn", Interval: 24 * time.Hour, Anchor: anchor, RRTypes: rr})
		must(t, err)
		return id
	}
	webPlan := plan(bindA[0].ID, "web.http", "", "")
	if webPlan != plan(bindB[0].ID, "web.http", "", "") {
		t.Fatal("equivalent schedules did not share intent")
	}
	cut.post.Store(true)
	if err := svc.Reconcile(ctx); !errors.Is(err, errNetwork) {
		t.Fatalf("expected lost schedule response: %v", err)
	}
	count(t, remotePool, 1, "SELECT count(*) FROM harvest.schedules WHERE NOT enabled")
	count(t, pool, 1, "SELECT count(*) FROM collection_plans WHERE post_started AND schedule_id IS NULL")
	if err := svc.RetryAbsent(ctx, webPlan); !errors.Is(err, errRemoteConflict) {
		t.Fatalf("an existing Schedule must prevent clearing the uncertain POST: %v", err)
	}
	execSQL(t, pool, "UPDATE collection_plans SET checked_at=NULL WHERE id=$1", webPlan)
	must(t, svc.Reconcile(ctx))
	if cut.schedulePosts.Load() != 1 {
		t.Fatal("schedule creation was repeated")
	}
	count(t, remotePool, 1, "SELECT count(*) FROM harvest.schedules WHERE enabled")
	dnsPlan := plan(bindA[1].ID, "web.dns", "", "", "a")
	tlsPlan := plan(bindA[0].ID, "web.tls", "", "")
	priceUS := plan(gameBind.ID, "steam.details", "en", "us")
	priceCN := plan(gameBind.ID, "steam.details", "zh-cn", "cn")
	priceHK := plan(gameBind.ID, "steam.details", "en", "hk")
	newsPlan := plan(gameBind.ID, "steam.news", "", "")
	playersPlan := plan(gameBind.ID, "steam.players", "", "")
	must(t, svc.Reconcile(ctx))
	count(t, remotePool, 8, "SELECT count(*) FROM harvest.schedules WHERE enabled")
	count(t, remotePool, 3, "SELECT count(*) FROM harvest.targets")
	req := func(id uuid.UUID) request {
		var r request
		var rawScope []byte
		must(t, pool.QueryRow(ctx, `SELECT s.target_id,p.capability,p.request_schema,p.params,p.scope
 FROM collection_plans p JOIN collection_sources s ON s.id=p.source_id WHERE p.id=$1`, id).
			Scan(&r.TargetID, &r.Capability, &r.Schema, &r.Params, &rawScope))
		must(t, platform.UnmarshalJSON(rawScope, &r.Scope))
		return r
	}
	web, dns, tlsReq := req(webPlan), req(dnsPlan), req(tlsPlan)
	steamUS, steamCN, news, players := req(priceUS), req(priceCN), req(newsPlan), req(playersPlan)
	for _, r := range []request{web, dns, tlsReq, steamUS} {
		l.job(r)
	}
	deferred := scalar[int64](t, remotePool, "SELECT deferred FROM harvest.quota_buckets WHERE key='steam_store'")
	execSQL(t, remotePool, "UPDATE harvest.quota_buckets SET cooldown_until=now()+interval '5 seconds' WHERE key='steam_store'")
	l.job(steamCN)
	if scalar[int64](t, remotePool, "SELECT deferred FROM harvest.quota_buckets WHERE key='steam_store'") <= deferred {
		t.Fatal("Storefront did not pass through the persisted Harvest quota cooldown")
	}
	l.job(news)
	l.job(players)
	l.job(req(priceHK))
	count(t, pool, 0, "SELECT count(*) FROM collection_current")
	var memBefore, memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)
	walBefore := scalar[string](t, pool, "SELECT pg_current_wal_insert_lsn()::text")
	start := time.Now()
	cut.ack.Store(true)
	if err := svc.Tick(ctx); !errors.Is(err, errNetwork) {
		t.Fatalf("expected lost reset ACK: %v", err)
	}
	state, err := q.GetCollectionConsumer(ctx, fixtureStream)
	must(t, err)
	if state.Generation == nil || !state.AckPending || state.ResetAck == nil {
		t.Fatal("projection and reset ACK token were not committed atomically")
	}
	count(t, pool, 2, "SELECT count(*) FROM collection_current WHERE generation=$1 AND capability='steam.details' AND data->>'state' IN ('free','zero_price')", state.Generation)
	count(t, pool, 1, "SELECT count(*) FROM collection_current WHERE generation=$1 AND capability='steam.details' AND scope->>'region'='hk' AND scope->'dimensions'->>'section'='details'", state.Generation)
	count(t, pool, 0, "SELECT count(*) FROM collection_current WHERE generation=$1 AND capability='steam.details' AND scope->>'region'='hk' AND scope->'dimensions'->>'section'='price'", state.Generation)
	count(t, pool, 1, "SELECT count(*) FROM game_evidence_players WHERE players=0")
	count(t, pool, 1, "SELECT count(*) FROM game_evidence_news WHERE gid='123456'")
	runtime.ReadMemStats(&memAfter)
	t.Logf("paged snapshot wall=%s allocated=%d bytes retained_heap_delta=%d bytes fixture_requests=%d",
		time.Since(start), memAfter.TotalAlloc-memBefore.TotalAlloc, int64(memAfter.HeapAlloc)-int64(memBefore.HeapAlloc), l.calls.Load())
	// Release all consumer transport and pool state before retrying its durable ACK.
	svc.Close()
	restartedPool, err := database.Open(ctx, goDB)
	must(t, err)
	t.Cleanup(restartedPool.Close)
	svc = New(restartedPool, cfg)
	t.Cleanup(svc.Close)
	must(t, svc.Tick(ctx))
	state, err = q.GetCollectionConsumer(ctx, fixtureStream)
	must(t, err)
	if state.AckPending || state.ResetAck != nil {
		t.Fatal("lost ACK did not converge on restart")
	}

	initial := state
	l.dns.Store(1)
	l.job(dns)
	live, err := svc.changes(ctx, initial.Cursor, "", nil, *initial.Epoch)
	must(t, err)
	if len(live.Items) != 0 || live.Next != initial.Cursor {
		t.Fatal("TTL-only confirmation became a Change Event")
	}
	execSQL(t, pool, "UPDATE collection_consumer SET sweep_at=now() WHERE namespace=$1", fixtureStream)
	oldCoverage := scalar[time.Time](t, pool, "SELECT max(confirmed_at) FROM site_daily_coverage WHERE capability='web.dns'")
	for range 8 {
		must(t, svc.Tick(ctx))
	}
	newCoverage := scalar[time.Time](t, pool, "SELECT max(confirmed_at) FROM site_daily_coverage WHERE capability='web.dns'")
	if !newCoverage.After(oldCoverage) {
		t.Fatal("same-value confirmation did not refresh daily coverage")
	}
	count(t, pool, 0, "SELECT count(*) FROM site_daily_coverage WHERE utc_day<(now() AT TIME ZONE 'UTC')::date")
	count(t, pool, 0, "SELECT count(*) FROM site_evidence_changes WHERE event_kind<>'baseline'")
	count(t, pool, 2, "SELECT count(*) FROM site_evidence_changes WHERE event_kind='baseline' AND fact_key='tech.detected.wordpress'")

	l.mode.Store(1)
	l.dns.Store(2)
	l.news.Store(1)
	l.certificate.Store(l.sign())
	for _, r := range []request{web, dns, tlsReq, news, players} {
		l.job(r)
	}
	beforeRollback, err := q.GetCollectionConsumer(ctx, fixtureStream)
	must(t, err)
	execSQL(t, pool, "ALTER TABLE collection_current ADD CONSTRAINT b1_fixture_rollback CHECK (position<0) NOT VALID")
	if err := svc.Tick(ctx); err == nil {
		t.Fatal("rejected projection unexpectedly committed")
	}
	afterRollback, err := q.GetCollectionConsumer(ctx, fixtureStream)
	must(t, err)
	if afterRollback.Cursor != beforeRollback.Cursor || afterRollback.AckPending {
		t.Fatal("projection rollback advanced the cursor or marked it ready for ACK")
	}
	count(t, pool, 0, "SELECT count(*) FROM site_evidence_changes WHERE event_kind<>'baseline'")
	execSQL(t, pool, "ALTER TABLE collection_current DROP CONSTRAINT b1_fixture_rollback")
	must(t, svc.Tick(ctx))
	count(t, pool, 1, "SELECT count(*) FROM site_evidence_changes WHERE site_id=$1 AND fact_key='dns.rrset' AND value::text LIKE '%8.8.8.9%'", siteA.ID)
	aged := scalar[uuid.UUID](t, pool, "SELECT source_observation_id FROM site_evidence_changes WHERE site_id=$1 AND fact_key='dns.rrset' AND value::text LIKE '%8.8.8.9%'", siteA.ID)
	count(t, pool, 2, "SELECT count(*) FROM site_evidence_changes WHERE fact_key='tech.detected.ghost'")
	count(t, pool, 0, "SELECT count(*) FROM site_evidence_changes WHERE fact_key LIKE '%wordpress%' AND event_kind<>'baseline'")
	if got := scalar[int64](t, pool, "SELECT count(*) FROM site_evidence_changes WHERE fact_key='tls.certificate'"); got < 1 {
		t.Fatal("renewed certificate not retained")
	}
	count(t, pool, 1, "SELECT count(*) FROM game_evidence_news WHERE gid='123456'")
	count(t, pool, 1, "SELECT count(*) FROM collection_current c JOIN collection_consumer n USING(generation) WHERE capability='steam.news' AND state='confirmed_empty'")
	count(t, pool, 1, "SELECT count(*) FROM game_evidence_players WHERE players=0")
	count(t, pool, 0, `SELECT count(*) FROM site_evidence_changes WHERE
 coalesce(value::text,'')||coalesce(prior_value::text,'') LIKE '%discard-%'`)
	detail, err := editor.PublishedGame(ctx, game.PublicID)
	must(t, err)
	if detail.Localizations[0].Summary != "Editorial text" {
		t.Fatal("source data overwrote editorial content")
	}

	// Replay real pages in reverse order, including a duplicate. The durable
	// version checks must prevent an old row replacing a later projection.
	cursor := initial.Cursor
	var replay []event
	for range 15 {
		page, err := svc.changes(ctx, cursor, "", nil, *initial.Epoch)
		must(t, err)
		replay = append(replay, page.Items...)
		cursor = page.Next
		if !page.More {
			break
		}
	}
	slices.Reverse(replay)
	priorFacts := scalar[int64](t, pool, "SELECT count(*) FROM site_evidence_changes")
	must(t, svc.locked(ctx, func(c *pgx.Conn, _ *dbgen.Queries) error {
		return write(ctx, c, func(tx *dbgen.Queries) error {
			return svc.applyPage(ctx, tx, *initial.Epoch, *initial.Generation, append(replay, replay...), false)
		})
	}))
	count(t, pool, priorFacts, "SELECT count(*) FROM site_evidence_changes")
	for _, ev := range replay {
		if ev.Observation != nil && ev.Observation.Scope.Dimensions["section"] == "html" {
			checkRulesAndWithdrawal(t, pool, svc, ev, *initial.Epoch, *initial.Generation, priorFacts)
			break
		}
	}

	l.plane.stop()
	if err := svc.Tick(ctx); !errors.Is(err, errNetwork) {
		t.Fatalf("Harvest shutdown not detected: %v", err)
	}
	detail, err = editor.PublishedGame(ctx, game.PublicID)
	must(t, err)
	if detail.PublicID != game.PublicID {
		t.Fatal("local catalog depended on Harvest")
	}
	l.worker.stop()
	l.start()
	must(t, svc.Tick(ctx))
	state, err = q.GetCollectionConsumer(ctx, fixtureStream)
	must(t, err)
	run(t, "", nil, "docker", "pause", goContainer)
	outage := runWithTimeout(ctx, 1200*time.Millisecond, svc.Tick)
	run(t, "", nil, "docker", "unpause", goContainer)
	if outage == nil {
		t.Fatal("consumer unexpectedly wrote during GoFurry database outage")
	}
	recovered, err := q.GetCollectionConsumer(ctx, fixtureStream)
	must(t, err)
	if recovered.Cursor != state.Cursor {
		t.Fatal("database outage changed the durable cursor")
	}
	must(t, svc.Tick(ctx))

	siteA, err = editor.EditSite(ctx, siteA.ID, siteA.Revision, catalog.SiteInput{Editorial: text,
		Addresses: []catalog.Address{{Kind: "primary", URL: "https://new.example.com/catalog"}}})
	must(t, err)
	moved, err := svc.BindSite(ctx, siteA.PublicID, "https://new.example.com/catalog")
	must(t, err)
	movedPlan := plan(moved[0].ID, "web.http", "", "")
	movedDNS := plan(moved[1].ID, "web.dns", "", "", "A")
	must(t, svc.Reconcile(ctx))
	count(t, pool, 2, "SELECT count(*) FROM collection_bindings WHERE site_id=$1 AND effective_until IS NOT NULL", siteA.ID)
	count(t, remotePool, 1, "SELECT count(*) FROM harvest.schedules WHERE target_id=$1 AND capability='web.http' AND enabled", web.TargetID)
	l.job(req(movedPlan))
	must(t, svc.Tick(ctx))
	count(t, pool, 1, "SELECT count(*) FROM site_evidence_changes WHERE site_id=$1 AND binding_id=$2 AND fact_key='http.response'", siteA.ID, moved[0].ID)
	count(t, pool, 0, "SELECT count(*) FROM site_evidence_changes WHERE site_id=$1 AND binding_id=$2", siteB.ID, moved[0].ID)

	// Force the real 14/30-day retention paths to age only disposable data.
	l.dns.Store(0)
	l.job(dns)
	must(t, svc.Tick(ctx))
	state, err = q.GetCollectionConsumer(ctx, fixtureStream)
	must(t, err)
	l.job(players)
	l.job(players)
	l.job(players)
	execSQL(t, remotePool, "UPDATE harvest.change_events SET committed_at=now()-interval '15 days' WHERE stream=$1 AND position<(SELECT last_position FROM harvest.change_heads WHERE stream=$1)", fixtureStream)
	execSQL(t, remotePool, `UPDATE harvest.observations o SET observed_at=now()-interval '31 days',received_at=now()-interval '31 days'
 WHERE o.stream=$1 AND NOT EXISTS(SELECT 1 FROM harvest.current_results c WHERE c.observation_id=o.id)`, fixtureStream)
	execSQL(t, remotePool, "UPDATE harvest.change_heads SET last_pruned_at='epoch' WHERE stream=$1", fixtureStream)
	expectEventually(t, 40*time.Second, "Harvest cursor did not expire after pruning", func() bool {
		_, err := svc.changes(ctx, state.Cursor, "", nil, *state.Epoch)
		return errors.Is(err, remoteStatus(410))
	})
	ledger := scalar[int64](t, pool, "SELECT count(*) FROM site_evidence_changes WHERE event_kind<>'baseline'")
	cut.pauseSnapshot.Store(true)
	defer func() {
		select {
		case <-cut.resumeSnapshot:
		default:
			close(cut.resumeSnapshot)
		}
	}()
	resetResult := make(chan error, 1)
	go func() { resetResult <- svc.Tick(ctx) }()
	select {
	case <-cut.snapshotRead:
	case err := <-resetResult:
		t.Fatalf("snapshot did not start: %v", err)
	case <-time.After(12 * time.Second):
		t.Fatal("snapshot did not reach its first page")
	}
	l.mode.Store(0)
	l.job(req(movedPlan))
	l.job(req(movedDNS))
	close(cut.resumeSnapshot)
	must(t, <-resetResult)
	reset, err := q.GetCollectionConsumer(ctx, fixtureStream)
	must(t, err)
	if reset.Generation == nil || *reset.Generation == *state.Generation || reset.AckPending {
		t.Fatal("expired cursor did not activate fresh state")
	}
	count(t, pool, ledger, "SELECT count(*) FROM site_evidence_changes WHERE event_kind<>'baseline'")
	count(t, pool, 1, "SELECT count(*) FROM collection_gaps WHERE namespace=$1", fixtureStream)
	count(t, pool, 1, "SELECT count(*) FROM game_evidence_news WHERE gid='123456'")
	count(t, pool, 1, "SELECT count(*) FROM collection_current c JOIN collection_consumer n USING(generation) WHERE source_id=$1 AND facts ? 'tech.detected.wordpress'", moved[0].SourceID)
	count(t, pool, 1, "SELECT count(*) FROM collection_current c JOIN collection_consumer n USING(generation) WHERE source_id=$1 AND facts ? 'dns.rrset'", moved[1].SourceID)
	expectEventually(t, 15*time.Second, "Harvest did not prune the replaced Observation", func() bool {
		return scalar[int64](t, remotePool, "SELECT count(*) FROM harvest.observations WHERE id=$1", aged) == 0
	})
	count(t, pool, 1, "SELECT count(*) FROM site_evidence_changes WHERE site_id=$1 AND fact_key='dns.rrset' AND value::text LIKE '%8.8.8.9%'", siteA.ID)

	l.mode.Store(1)
	l.job(req(movedPlan))
	checkConsumerLifecycle(t, l, cut, goDB, pool, enrolled.ID, siteA.PublicID, reset.Cursor)

	game, err = editor.EditGame(ctx, game.ID, game.Revision, catalog.GameInput{Editorial: text})
	must(t, err)
	must(t, svc.Reconcile(ctx))
	count(t, remotePool, 0, "SELECT count(*) FROM harvest.schedules WHERE target_id=$1 AND enabled", players.TargetID)
	count(t, remotePool, 1, "SELECT count(*) FROM harvest.targets WHERE id=$1 AND state='active'", players.TargetID)
	count(t, pool, 1, "SELECT count(*) FROM collection_bindings WHERE game_id=$1 AND effective_until IS NOT NULL", game.ID)

	t.Logf("B1 measured evidence heap=%d indexes=%d toast=%d WAL=%d bytes, real fixture HTTP requests=%d",
		scalar[int64](t, pool, "SELECT sum(pg_relation_size(oid))::bigint FROM pg_class WHERE relname IN ('site_evidence_changes','site_daily_coverage','collection_current','game_evidence_news','game_evidence_players')"),
		scalar[int64](t, pool, "SELECT sum(pg_indexes_size(oid))::bigint FROM pg_class WHERE relname IN ('site_evidence_changes','site_daily_coverage','collection_current','game_evidence_news','game_evidence_players')"),
		scalar[int64](t, pool, "SELECT sum(CASE WHEN reltoastrelid=0 THEN 0 ELSE pg_total_relation_size(reltoastrelid) END)::bigint FROM pg_class WHERE relname IN ('site_evidence_changes','site_daily_coverage','collection_current','game_evidence_news','game_evidence_players')"),
		scalar[int64](t, pool, "SELECT pg_wal_lsn_diff(pg_current_wal_insert_lsn(),$1::text::pg_lsn)::bigint", walBefore), l.calls.Load())
	measureSiteYears(t, pool)
}

func checkRulesAndWithdrawal(t *testing.T, pool *pgxpool.Pool, svc *Service, current event, epoch, generation uuid.UUID, priorFacts int64) {
	t.Helper()
	ctx := t.Context()
	tx, err := pool.Begin(ctx)
	must(t, err)
	defer tx.Rollback(ctx)
	var data map[string]any
	must(t, json.Unmarshal(current.Observation.Data, &data))
	data["ruleset_version"] = "fixture_rule_revision"
	data["technology"] = []technology{{ID: "fixture_new_detector", Category: "cms", Confidence: "high", Evidence: []string{"generator_meta"}}}
	revised := current
	revised.Position++
	revised.Committed = time.Now().UTC()
	o := *current.Observation
	o.ID, o.ObservedAt, o.ReceivedAt = uuid.NewV7(), revised.Committed, revised.Committed
	o.Data, err = json.Marshal(data)
	must(t, err)
	sum := sha256.Sum256(o.Data)
	o.Hash = hex.EncodeToString(sum[:])
	revised.Observation = &o
	q := dbgen.New(tx)
	must(t, svc.applyPage(ctx, q, epoch, generation, []event{revised}, false))
	var n int64
	must(t, tx.QueryRow(ctx, "SELECT count(*) FROM site_evidence_changes").Scan(&n))
	if n != priorFacts {
		t.Fatal("a detector revision created a false transition")
	}
	withdrawn := event{identity: revised.identity, Position: revised.Position + 1, Action: "withdraw", Committed: revised.Committed, Tombstone: o.Scope}
	must(t, svc.applyPage(ctx, q, epoch, generation, []event{withdrawn}, false))
	must(t, svc.applyPage(ctx, q, epoch, generation, []event{revised, withdrawn}, false))
	must(t, tx.QueryRow(ctx, "SELECT count(*) FROM collection_current WHERE generation=$1 AND scope_key=$2 AND state='withdrawn'", generation, o.ScopeKey).Scan(&n))
	if n != 1 {
		t.Fatal("a late upsert overwrote the scoped withdrawal")
	}
	must(t, tx.QueryRow(ctx, "SELECT count(*) FROM site_evidence_changes WHERE event_kind='withdrawn' AND fact_key='tech.detected.fixture_new_detector'").Scan(&n))
	if n != 2 {
		t.Fatal("the explicit withdrawal lost attribution to either Site")
	}
}

func count(t *testing.T, pool *pgxpool.Pool, want int64, sql string, args ...any) {
	t.Helper()
	if got := scalar[int64](t, pool, sql, args...); got != want {
		t.Fatalf("count %d, want %d: %s", got, want, sql)
	}
}

func scalar[T any](t *testing.T, pool *pgxpool.Pool, sql string, args ...any) T {
	t.Helper()
	var v T
	must(t, pool.QueryRow(t.Context(), sql, args...).Scan(&v))
	return v
}

func execSQL(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	_, err := pool.Exec(t.Context(), sql, args...)
	must(t, err)
}
