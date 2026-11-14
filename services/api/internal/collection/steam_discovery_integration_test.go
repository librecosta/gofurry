//go:build integration

package collection

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func (l *lab) catalogFeed(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if r.Header.Get("X-WebAPI-Key") != fixtureSteamKey || q.Get("key") != "" || q.Get("include_games") != "true" || q.Get("include_dlc") != "false" {
		l.t.Error("unexpected catalog request")
	}
	w.Header().Set("Content-Type", "application/json")
	after, _ := strconv.ParseUint(q.Get("last_appid"), 10, 32)
	limit, _ := strconv.Atoi(q.Get("max_results"))
	since, _ := strconv.ParseInt(q.Get("if_modified_since"), 10, 64)
	mode := l.discoveryMode.Load()
	if mode == 4 {
		w.WriteHeader(401)
		return
	}
	if mode == 1 && after > 0 {
		_, _ = fmt.Fprintf(w, `{"response":{"apps":[{"appid":%d,"name":"Duplicate","last_modified":%d}],"have_more_results":true,"last_appid":%d}}`, after, time.Now().Unix(), after)
		return
	}
	names := []string{"Fox Library", "Furry Tax Filing", "Furry Night", "Furry Mystery", "Wolf Demo", "Fox Library", "Otter Library", "Badger Borough"}
	if mode >= 2 {
		names = append(names, "Quiet Orchard")
	}
	rows := make([]catalogRow, 0)
	// The timestamps are immutable across unchanged scans.
	for i, name := range names {
		id := 3100000001 + uint32(i)
		modified := l.created - 7200 + int64(i)
		priceChange := uint64(i + 1)
		if mode >= 2 && (i == 0 || i == 3 || i == 8) {
			modified = l.created + int64(i)
			if i == 0 {
				priceChange++
			}
		}
		if uint64(id) > after && modified >= since {
			rows = append(rows, catalogRow{ID: id, Name: name, Modified: modified, PriceChange: priceChange})
		}
	}
	more := len(rows) > limit
	rows = rows[:min(len(rows), limit)]
	last := uint32(after)
	if len(rows) > 0 {
		last = rows[len(rows)-1].ID
	}
	raw, _ := json.Marshal(map[string]any{"response": map[string]any{"apps": rows, "have_more_results": more, "last_appid": last}})
	_, _ = w.Write(raw)
}

func (l *lab) eligibilityFixture(app, name, kind, description, extra string) (string, string, string, string) {
	name = "Fixture " + app
	descs := `,"required_age":"0","ratings":{"esrb":{"rating":"E10+"}},"content_descriptors":{"ids":[]}`
	description = "Play as anthropomorphic foxes restoring a library."
	switch app {
	case "3100000002":
		description = "A spreadsheet tutorial for tax filing."
	case "3100000003":
		descs = `,"content_descriptors":{"ids":[3]}`
	case "3100000004":
		if l.discoveryMode.Load() < 2 {
			descs = `,"required_age":0,"content_descriptors":{"ids":[]}`
		}
	case "3100000005":
		kind = "demo"
	case "3100000006":
		name = "Fixture 3100000001"
	case "3100000007":
		description = "Help the talking otters run the library with their animal villagers."
	case "3100000008":
		description = "Meet the talking badgers and build their borough."
	case "3100000001":
		if l.discoveryMode.Load() == 3 {
			descs = `,"content_descriptors":{"ids":[3]}`
		}
	}
	return name, kind, description, extra + descs
}

func TestSteamDiscoveryHarvest(t *testing.T) {
	ctx := t.Context()
	goDB, _ := fixtureDB(t, "discovery")
	remoteDB, _ := fixtureDB(t, "discovery_harvest")
	redisID, redis := intakeRedis(t)
	must(t, database.Migrate(ctx, goDB, "../../migrations", "up", io.Discard))
	pool, err := database.Open(ctx, goDB)
	must(t, err)
	defer pool.Close()
	l := newLab(t, remoteDB)
	remote, err := database.Open(ctx, remoteDB)
	must(t, err)
	defer remote.Close()
	cut := proxyLab(t, l)
	h := newClient(platform.HarvestConfig{URL: cut.server.URL, WriterToken: l.writer, ReaderToken: l.reader})
	var enrolled ackView
	must(t, h.call(ctx, l.operator, "POST", "consumers", nil, map[string]string{"name": "discovery_intake", "stream": fixtureStream, "principal": "b1_reader"}, &enrolled, 200))
	var aiCalls atomic.Int32
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aiCalls.Add(1)
		if r.URL.Path != "/v1/chat/completions" || r.Method != "POST" || r.Header.Get("Authorization") == "" {
			t.Error("bad AI Gateway request")
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
		if strings.Contains(string(body), "talking otters") {
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"relevant\":true,\"excerpt\":\"talking otters\"}"}}]}`)
		} else {
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"relevant\":true,\"excerpt\":\"manufactured animal protagonist\"}"}}]}`)
		}
	}))
	defer ai.Close()
	cfg := platform.HarvestConfig{URL: cut.server.URL, Namespace: fixtureStream, WriterToken: l.writer, ReaderToken: l.reader, ConsumerID: enrolled.ID.String(),
		Consume: true, PollInterval: time.Second, PageSize: 4, IntakeEnabled: true, IntakeEgressRegion: "cn",
		Discovery: &platform.DiscoveryConfig{Enabled: true, Namespace: discoveryStream, WriterToken: l.discoveryWriter, ReaderToken: l.discoveryReader,
			PollInterval: time.Second, ScanInterval: time.Minute, PageSize: 2, JobsPerDay: 32, CandidatesPerDay: 16,
			Seeds: []uint32{3100000009}, AI: &platform.DiscoveryAIConfig{URL: ai.URL, Token: "synthetic-fixture-authorization-token", Model: "fixture", CallsPerDay: 2}}}
	intake := NewIntake(pool, &cfg)
	defer intake.Close()
	d := NewDiscovery(pool, &cfg, intake)
	defer d.Close()
	d2 := NewDiscovery(pool, &cfg, intake)
	defer d2.Close()
	l.discoveryMode.Store(1)
	must(t, d.Configure(ctx, false, true))
	cut.jobDrop.Store(true)
	tick := func() {
		var wg sync.WaitGroup
		for _, service := range []*Discovery{d, d2} {
			wg.Go(func() {
				e := service.Tick(ctx)
				if e != nil && !errors.Is(e, errNetwork) {
					t.Errorf("Discovery tick: %v", e)
				}
			})
		}
		wg.Wait()
	}
	expectEventually(t, 40*time.Second, "first page did not commit or corrupt page skipped", func() bool {
		tick()
		s, e := d.Status(ctx)
		must(t, e)
		return s.Pages == 1 && s.LastCode == "catalog_failed" && s.Watermark == 0 && s.AppID == 3100000002 && s.Backfill
	})
	count(t, remote, 1, "SELECT count(*) FROM harvest.jobs WHERE namespace=$1 AND params->>'last_appid'='0'", discoveryStream)
	must(t, d.Configure(ctx, true, false))
	tick()
	paused, e := d.Status(ctx)
	must(t, e)
	if !paused.Paused || paused.Pages != 1 {
		t.Fatal("pause changed a window")
	}
	l.discoveryMode.Store(0)
	must(t, d2.Configure(ctx, false, false))
	expectEventually(t, 50*time.Second, "backfill window did not resume", func() bool {
		tick()
		s, e := d2.Status(ctx)
		must(t, e)
		return s.Windows == 1 && s.Pages == 4 && s.Rows == 8 && s.Watermark > 0 && s.Pending == 0
	})
	dir, e := filepath.Abs("../..")
	must(t, e)
	bin := filepath.Join(t.TempDir(), "api")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	run(t, dir, nil, "go", "build", "-o", bin, "./cmd/api")
	must(t, d.Configure(ctx, true, false))
	address := localAddress(t)
	yaml := filepath.Join(t.TempDir(), "api.yaml")
	writeIntakeYAML(t, yaml, address, redis, goDB, cfg)
	buf, e := os.ReadFile(yaml)
	must(t, e)
	buf = fmt.Appendf(buf, `  discovery:
    enabled: true
    namespace: %s
    writer_token: %q
    reader_token: %q
    poll_interval: 1s
    scan_interval: 1m
    page_size: 2
    jobs_per_day: 32
    candidates_per_day: 16
    seeds: [3100000009]
    ai:
      url: %q
      token: %q
      model: fixture
      calls_per_day: 2
`, discoveryStream, l.discoveryWriter, l.discoveryReader, ai.URL, cfg.Discovery.AI.Token)
	must(t, os.WriteFile(yaml, buf, 0600))
	api := launch(t, filepath.Dir(yaml), nil, bin, "--config", yaml)
	defer api.stop()
	base := "http://" + address
	expectEventually(t, 15*time.Second, "API readiness", func() bool {
		resp, err := l.http.Get(base + "/readyz")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == 200
	})
	complete := func() bool {
		return scalar[int64](t, pool, "SELECT count(*) FROM game_intake_candidates WHERE state='accepted' OR reason IN ('eligibility_uncertain','adult_game','irrelevant_game','not_full_game','weak_identity_match')") == 8
	}
	for deadline := time.Now().Add(140 * time.Second); !complete(); {
		if time.Now().After(deadline) {
			states, e := intake.Status(ctx)
			must(t, e)
			t.Fatalf("automatic eligibility did not converge, candidates: %+v, AI calls: %d", states, aiCalls.Load())
		}
		// Advance only the disposable queue's planned poll times. The Go
		// services still claim their own leases and verify real Job results.
		execSQL(t, pool, "UPDATE game_intake_candidates SET next_at=now() WHERE state='deferred' AND reason IN ('target_verified','job_submitted','job_pending') AND next_at>now()")
		_, e := intake.Tick(ctx)
		must(t, e)
		time.Sleep(200 * time.Millisecond)
	}
	for _, want := range []struct{ key, state, reason string }{
		{"3100000001", "accepted", "created_game"}, {"3100000002", "rejected", "irrelevant_game"}, {"3100000003", "rejected", "adult_game"},
		{"3100000004", "deferred", "eligibility_uncertain"}, {"3100000005", "rejected", "not_full_game"}, {"3100000006", "deferred", "weak_identity_match"},
	} {
		assertIntakeReason(t, intake, want.key, want.state, want.reason)
	}
	first, err := intake.Inspect(ctx, "steam", "3100000001")
	must(t, err)
	assertIntakeHTTP(t, l.http, base, first.PublicID, 200, "anthropomorphic foxes")
	count(t, pool, 2, "SELECT count(*) FROM games WHERE status='published'")
	count(t, pool, 2, "SELECT count(*) FROM game_approved_presentations WHERE approval->>'policy'='steam-eligibility-1'")
	if aiCalls.Load() != 2 {
		t.Fatalf("AI daily budget: %d", aiCalls.Load())
	}
	t.Logf("catalog 8 rows => 2 automatically published, 1 irrelevant, 1 adult, 2 uncertain, 1 Demo and 1 weak identity; first public ID %s", first.PublicID)

	editor := catalog.New(pool)
	game, e := dbgen.New(pool).LockGame(ctx, *first.Game)
	must(t, e)
	_, e = editor.EditGame(ctx, game.ID, game.Revision, catalog.GameInput{Editorial: catalog.Editorial{Localizations: []catalog.Localization{{Locale: "en", Name: "Edited Fox Library", Summary: "Editorial wins."}}}})
	must(t, e)
	execSQL(t, remote, "UPDATE harvest.consumers SET reset_required=true,reset_reason='fixture' WHERE id=$1", enrolled.ID)
	expectEventually(t, 20*time.Second, "business consumer did not recover", func() bool {
		return scalar[int64](t, pool, "SELECT count(*) FROM collection_gaps WHERE namespace=$1", fixtureStream) > 0
	})
	count(t, pool, 0, "SELECT count(*) FROM collection_sources WHERE namespace=$1", discoveryStream)
	l.discoveryMode.Store(2)
	oldCandidate, e := intake.Inspect(ctx, "steam", "3100000004")
	must(t, e)
	expectEventually(t, 10*time.Second, "resume incremental worker", func() bool { return d2.Configure(ctx, false, false) == nil })
	for deadline := time.Now().Add(65 * time.Second); ; {
		status, e := d.Status(ctx)
		must(t, e)
		changed, e := intake.Inspect(ctx, "steam", "3100000004")
		must(t, e)
		seeded := scalar[bool](t, pool, "SELECT EXISTS (SELECT 1 FROM game_intake_candidates c JOIN games g ON g.id=c.game_id WHERE c.external_key='3100000009' AND g.status='published')")
		if status.Windows >= 2 && changed.GameState == "published" && seeded {
			if changed.Job == nil || oldCandidate.Job == nil || *changed.Job == *oldCandidate.Job {
				t.Fatal("incremental change reused an old Job")
			}
			t.Logf("incremental publication %s from new Job %s; trusted seed with a neutral title also published", changed.PublicID, *changed.Job)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("incremental change not re-evaluated: cursor %+v; candidate %+v", status, changed)
		}
		execSQL(t, pool, "UPDATE game_intake_candidates SET next_at=now() WHERE state='deferred' AND reason IN ('target_verified','job_submitted','job_pending') AND next_at>now()")
		tick()
		_, e = intake.Tick(ctx)
		must(t, e)
		time.Sleep(200 * time.Millisecond)
	}
	must(t, d2.Configure(ctx, true, false))
	current, e := intake.Inspect(ctx, "steam", "3100000001")
	must(t, e)
	if current.Job == nil || first.Job == nil || *current.Job != *first.Job || current.PublicID != first.PublicID {
		t.Fatal("a price-only change restarted Intake for an already bound game")
	}
	ai.Close()
	run(t, "", nil, "docker", "stop", redisID)
	assertIntakeHTTP(t, l.http, base, first.PublicID, 200, "Edited Fox Library", "Editorial wins")
	l.discoveryMode.Store(4)
	execSQL(t, pool, "UPDATE steam_discovery_state SET paused=false,next_at=now() WHERE namespace=$1", discoveryStream)
	expectEventually(t, 25*time.Second, "Steam 401 did not pause progress", func() bool {
		tick()
		s, e := d.Status(ctx)
		must(t, e)
		return s.LastCode == "upstream_auth" && s.Windows == 2 && s.Pages == 6 && s.WindowStarted > 0
	})
	count(t, pool, 1, "SELECT count(*) FROM steam_discovery_state WHERE namespace=$1 AND next_at>now()+interval '59 minutes'", discoveryStream)
	assertIntakeHTTP(t, l.http, base, first.PublicID, 200, "Edited Fox Library", "Editorial wins")
	must(t, d2.Configure(ctx, true, false))
	execSQL(t, remote, "UPDATE harvest.quota_buckets SET cooldown_until=now() WHERE cooldown_until>now()")
	// Recover a confirmed adult-only observation through a Consumer reset.
	cut.offline.Store(true)
	l.discoveryMode.Store(3)
	row, e := dbgen.New(pool).IntakeCandidate(ctx, dbgen.IntakeCandidateParams{PlatformCode: "steam", ExternalKey: "3100000001"})
	must(t, e)
	req, _, _ := intakeRequest(row)
	l.job(req)
	execSQL(t, remote, "UPDATE harvest.consumers SET reset_required=true,reset_reason='fixture' WHERE id=$1", enrolled.ID)
	cut.offline.Store(false)
	expectEventually(t, 25*time.Second, "confirmed adult content did not block publication", func() bool {
		return scalar[bool](t, pool, "SELECT publication_blocked FROM games WHERE id=$1", *first.Game)
	})
	assertIntakeHTTP(t, l.http, base, first.PublicID, 404)
	count(t, pool, 1, "SELECT count(*) FROM catalog_publication_decisions WHERE entity_id=$1 AND action='block' AND actor='steam-eligibility-1'", *first.Game)
	// Replay cannot clear a block or undo an explicit disposition.
	count(t, pool, 2, "SELECT count(*) FROM collection_gaps WHERE namespace=$1", fixtureStream)
	assertIntakeHTTP(t, l.http, base, first.PublicID, 404)
	game, e = dbgen.New(pool).LockGame(ctx, *first.Game)
	must(t, e)
	_, e = editor.ChangeGamePublication(ctx, first.PublicID, game.Revision, "clear_block", catalog.PublicationDecision{Actor: "fixture", Reason: "Verified editorial correction"})
	must(t, e)
	assertIntakeHTTP(t, l.http, base, first.PublicID, 200, "Edited Fox Library", "Editorial wins")
	execSQL(t, remote, "UPDATE harvest.consumers SET reset_required=true,reset_reason='fixture' WHERE id=$1", enrolled.ID)
	expectEventually(t, 20*time.Second, "clear disposition replay did not settle", func() bool {
		return scalar[int64](t, pool, "SELECT count(*) FROM collection_gaps WHERE namespace=$1", fixtureStream) == 3
	})
	l.worker.stop()
	l.plane.stop()
	for _, days := range []int32{31, 181, 365} {
		execSQL(t, pool, "UPDATE game_approved_presentations SET observed_at=now()-make_interval(days=>$1)", days)
		assertIntakeHTTP(t, l.http, base, first.PublicID, 200, "Edited Fox Library", "Editorial wins")
	}
}

func BenchmarkSteamCatalogSelection(b *testing.B) {
	d := Discovery{config: platform.DiscoveryConfig{ExplorePerDay: 1}}
	for _, size := range []int{100, 1000, 10000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			rows := make([]catalogRow, size)
			for i := range rows {
				rows[i] = catalogRow{ID: uint32(i + 1), Name: "Index fixture title", Modified: 1794614400}
				if i%11 == 0 {
					rows[i].Name = "Fox Library"
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				for start := 0; start < len(rows); start += 64 {
					d.picks(rows[start:min(start+64, len(rows))], nil, 20000)
				}
			}
		})
	}
}
