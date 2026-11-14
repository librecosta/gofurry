//go:build integration

package collection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestGameIntakeHarvestD(t *testing.T) {
	if *harvestSource == "" {
		t.Skip("explicit read-only Harvest V0.1-D source required")
	}
	ctx := t.Context()
	goDB, pg := fixtureDB(t, "intake")
	harvestDB, _ := fixtureDB(t, "preflight")
	l := newLab(t, harvestDB)
	cut := proxyLab(t, l)
	redis, redisAddr := intakeRedis(t)
	must(t, database.Migrate(ctx, goDB, "../../migrations", "up", io.Discard))
	must(t, database.Migrate(ctx, goDB, "../../migrations", "up", io.Discard))
	pool, err := database.Open(ctx, goDB)
	must(t, err)
	t.Cleanup(pool.Close)
	remote, err := database.Open(ctx, harvestDB)
	must(t, err)
	t.Cleanup(remote.Close)
	count(t, pool, 4, "SELECT count(*) FROM pg_constraint WHERE contype='f' AND connamespace='public'::regnamespace")

	h := &client{base: "http://" + l.address, http: l.http}
	var consumer ackView
	must(t, h.call(ctx, l.operator, "POST", "consumers", nil, map[string]string{
		"stream": fixtureStream, "name": "intake", "principal": "b1_reader",
	}, &consumer, 200))
	cfg := platform.HarvestConfig{URL: cut.server.URL, Namespace: fixtureStream, WriterToken: l.writer, ReaderToken: l.reader,
		ConsumerID: consumer.ID.String(), Consume: true, PollInterval: time.Second, PageSize: 4, IntakeEgressRegion: "cn"}
	s := NewIntake(pool, &cfg)
	t.Cleanup(s.Close)
	noHarvest := NewIntake(pool, nil)
	catalogDB := catalog.New(pool)
	first := "3000000001"
	intakeStarted := time.Now()
	dir := t.TempDir()
	bin := filepath.Join(dir, "gofurry-api")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	run(t, "../..", nil, "go", "build", "-o", bin, "./cmd/api")
	addr := localAddress(t)
	yamlPath := filepath.Join(dir, "api.yaml")
	writeIntakeYAML(t, yamlPath, addr, redisAddr, goDB, cfg)
	run(t, dir, nil, bin, "--config", yamlPath, "game-intake", "submit", "StEaM", "000"+first)
	run(t, dir, nil, bin, "--config", yamlPath, "game-intake", "review", first, "fixture.work-synopsis", "fixture.content-assessment", "--confirm-furry", "--confirm-sfw")
	var concurrent sync.WaitGroup
	for i := range 16 {
		concurrent.Go(func() {
			via := "operator"
			if i%2 == 0 {
				via = "discovery"
			}
			c, err := s.SubmitSteam(ctx, "00"+first, via)
			if err != nil || c.Key != first {
				t.Errorf("concurrent normalized submission failed: %v", err)
			}
		})
	}
	concurrent.Wait()
	count(t, pool, 1, "SELECT count(*) FROM game_intake_candidates WHERE external_key=$1", first)
	for _, bad := range []string{"0", "4294967296", "+1", "steam://123"} {
		if _, err := s.SubmitSteam(ctx, bad, "operator"); !errors.Is(err, ErrIntakeInput) {
			t.Fatalf("accepted invalid Steam key %q", bad)
		}
	}
	if err := s.ReviewSteam(ctx, first, "same.ref", "same.ref"); !errors.Is(err, ErrIntakeInput) {
		t.Fatal("one evidence record counted for two different reviews")
	}
	_, err = noHarvest.Tick(ctx)
	must(t, err)
	assertIntakeReason(t, s, first, "deferred", "harvest_unconfigured")
	wakeIntake(t, pool, first)

	lease, err := s.claim(ctx)
	must(t, err)
	for range 16 {
		concurrent.Go(func() {
			c, err := s.claim(ctx)
			if err != nil || c.LeaseID != nil {
				t.Errorf("duplicate Intake lease: %v", err)
			}
		})
	}
	concurrent.Wait()
	execSQL(t, pool, "UPDATE game_intake_candidates SET lease_until=now()-interval '1s',next_at=now() WHERE id=$1", lease.ID)
	recovered, err := s.claim(ctx)
	must(t, err)
	if lease.LeaseID == nil || recovered.LeaseID == nil || *lease.LeaseID == *recovered.LeaseID {
		t.Fatal("expired lease was not recovered")
	}
	if err := releaseIntake(ctx, dbgen.New(pool), lease, "deferred", "stale", 0, 0); !errors.Is(err, ErrIntakeState) {
		t.Fatal("old executor changed the candidate after fencing")
	}
	must(t, s.process(ctx, recovered))
	assertIntakeReason(t, s, first, "deferred", "target_verified")
	count(t, pool, 0, "SELECT count(*) FROM collection_bindings")
	count(t, remote, 0, "SELECT count(*) FROM harvest.schedules")
	cut.jobDrop.Store(true)
	s.collection.remote.http.CloseIdleConnections()
	wakeIntake(t, pool, first)
	_, err = s.Tick(ctx)
	must(t, err)
	lost := assertIntakeReason(t, s, first, "deferred", "harvest_unreachable")
	if lost.Job != nil {
		t.Fatal("lost response made an unverified Job ID durable")
	}
	onlyJob := scalar[uuid.UUID](t, remote, "SELECT id FROM harvest.jobs WHERE target_id=$1", lost.Target)
	count(t, remote, 1, "SELECT count(*) FROM harvest.submissions WHERE job_id=$1", onlyJob)
	count(t, pool, 0, "SELECT count(*) FROM collection_current")
	wakeIntake(t, pool, first)
	_, err = s.Tick(ctx)
	must(t, err)
	got := assertIntakeReason(t, s, first, "deferred", "job_submitted")
	if *got.Job != onlyJob || cut.jobPosts.Load() != 2 {
		t.Fatal("same-key replay did not recover the original Job")
	}

	api := launch(t, dir, nil, bin, "--config", yamlPath)
	base := "http://" + addr
	probe(t, base)
	var accepted CandidateStatus
	expectEventually(t, 35*time.Second, "first Intake failed to publish through Fiber lifecycle", func() bool {
		accepted, err = s.Inspect(ctx, "steam", first)
		return err == nil && accepted.GameState == catalog.Published
	})
	if accepted.Target == nil || accepted.Job == nil || accepted.Attempt == nil || accepted.Observation == nil ||
		accepted.Game == nil || accepted.Source == nil || !catalog.ValidPublicID(accepted.PublicID) ||
		!v7(*accepted.Game) || !v7(*accepted.Source) || *accepted.Job != onlyJob {
		t.Fatalf("incomplete accepted Intake identity: %+v", accepted)
	}
	t.Logf("intake trace in %s: %s Target %s Job %s Attempt %s Observation %s Game %s Public %s",
		time.Since(intakeStarted).Round(time.Millisecond), accepted.ID, accepted.Target, accepted.Job, accepted.Attempt, accepted.Observation, accepted.Game, accepted.PublicID)
	count(t, remote, 1, "SELECT count(*) FROM harvest.observations WHERE id=$1 AND job_id=$2 AND attempt_id=$3", accepted.Observation, accepted.Job, accepted.Attempt)
	count(t, pool, 1, "SELECT count(*) FROM games")
	count(t, pool, 1, "SELECT count(*) FROM game_sources")
	count(t, pool, 0, "SELECT count(*) FROM game_localizations WHERE game_id=$1", accepted.Game)
	if scalar[string](t, pool, "SELECT profile->>'summary' FROM game_intake_candidates WHERE id=$1", accepted.ID) != "An anthropomorphic fox cast restores a library." {
		t.Fatal("first intake did not retain only the normalized source synopsis")
	}
	expectEventually(t, 15*time.Second, "formal Schedule was not reconciled", func() bool {
		return scalar[int64](t, remote, "SELECT count(*) FROM harvest.schedules WHERE target_id=$1 AND enabled", accepted.Target) == 1
	})
	count(t, pool, 1, "SELECT count(*) FROM collection_bindings WHERE game_id=$1 AND effective_until IS NULL", accepted.Game)
	assertIntakeHTTP(t, l.http, base, accepted.PublicID, 200, "Fable of the Foxes")

	l.intakeVersion.Store(1)
	execSQL(t, remote, "UPDATE harvest.schedules SET next_due_at=now()-interval '1s' WHERE target_id=$1", accepted.Target)
	expectEventually(t, 30*time.Second, "scheduled Steam update did not reach the local Consumer", func() bool {
		return scalar[string](t, pool, "SELECT profile->>'name' FROM game_intake_candidates WHERE id=$1", accepted.ID) == "Fable of the Foxes Revised"
	})
	count(t, remote, 1, "SELECT count(*) FROM harvest.jobs WHERE target_id=$1 AND schedule_id IS NOT NULL AND state IN ('succeeded','partial')", accepted.Target)
	assertIntakeHTTP(t, l.http, base, accepted.PublicID, 200, "Fable of the Foxes Revised")
	steam := New(pool, cfg)
	t.Cleanup(steam.Close)
	binding, err := steam.BindGame(ctx, accepted.PublicID, first)
	must(t, err)
	anchor := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	for _, market := range []string{"cn", "hk"} {
		plan := Plan{Capability: intakeCapability, Locale: "zh-cn", Region: market, EgressRegion: "cn", Anchor: anchor, Interval: 24 * time.Hour}
		_, err = steam.AddPlan(ctx, binding.ID, plan)
		must(t, err)
		params, _, _, err := plan.params("app", first)
		must(t, err)
		l.job(request{TargetID: *accepted.Target, Capability: intakeCapability, Schema: 1, Params: params,
			Scope: scope{Locale: plan.Locale, Region: market, EgressRegion: "cn"}})
		if market == "cn" {
			expectEventually(t, 20*time.Second, "verified Chinese metadata did not extend the approved source", func() bool {
				return scalar[int64](t, pool, "SELECT count(*) FROM game_approved_presentations WHERE game_id=$1 AND locale='zh' AND steam_store_country='cn'", accepted.Game) == 1
			})
		}
	}
	assertIntakeHTTP(t, l.http, base, accepted.PublicID, 200, "Fable of the Foxes Revised", "狐狸寓言")
	game, err := dbgen.New(pool).LockGame(ctx, *accepted.Game)
	must(t, err)
	_, err = catalogDB.EditGame(ctx, game.ID, game.Revision, catalog.GameInput{Editorial: catalog.Editorial{Localizations: []catalog.Localization{
		{Locale: "en", Name: "Editorial English", Summary: "English review"},
		{Locale: "zh", Name: "编辑译名", Summary: "中文介绍"},
	}}})
	must(t, err)
	execSQL(t, remote, "UPDATE harvest.consumers SET reset_required=true,reset_reason='fixture' WHERE id=$1", consumer.ID)
	expectEventually(t, 25*time.Second, "Consumer did not finish real 410 recovery", func() bool {
		return scalar[int64](t, pool, "SELECT count(*) FROM collection_gaps WHERE namespace=$1", fixtureStream) > 0 &&
			!scalar[bool](t, remote, "SELECT reset_required FROM harvest.consumers WHERE id=$1", consumer.ID)
	})
	l.intakeVersion.Store(0)
	execSQL(t, remote, `UPDATE harvest.schedules SET next_due_at=greatest(anchor_at,now()-interval '1s')
 WHERE target_id=$1 AND scope->>'locale'='en' AND scope->>'region'='us'`, accepted.Target)
	expectEventually(t, 30*time.Second, "automatic refresh stopped after the Consumer reset", func() bool {
		return scalar[string](t, pool, "SELECT profile->>'name' FROM game_intake_candidates WHERE id=$1", accepted.ID) == "Fable of the Foxes"
	})
	assertIntakeHTTP(t, l.http, base, accepted.PublicID, 200, "Editorial English", "编辑译名", "中文介绍")
	decision := catalog.PublicationDecision{Actor: "fixture", Reason: "Fixture policy decision"}
	must(t, s.RevokeSteamReview(ctx, first, decision))
	assertIntakeHTTP(t, l.http, base, accepted.PublicID, 404)
	assertNotPublic(t, ctx, catalogDB, accepted.PublicID)
	must(t, s.ReviewSteam(ctx, first, "fixture.work-synopsis", "fixture.content-assessment"))
	assertIntakeHTTP(t, l.http, base, accepted.PublicID, 404)
	locked, err := dbgen.New(pool).LockGame(ctx, *accepted.Game)
	must(t, err)
	_, err = catalogDB.ChangeGamePublication(ctx, accepted.PublicID, locked.Revision, "clear_block", decision)
	must(t, err)
	for _, days := range []int32{31, 181, 365} {
		execSQL(t, pool, "UPDATE game_intake_candidates SET profile_at=now()-make_interval(days=>$2),review_until=now()-make_interval(days=>$2) WHERE id=$1", accepted.ID, days)
		assertIntakeHTTP(t, l.http, base, accepted.PublicID, 200, "Editorial English", "编辑译名")
	}

	run(t, dir, nil, "docker", "pause", redis)
	t.Cleanup(func() { _ = exec.Command("docker", "unpause", redis).Run() })
	assertIntakeHTTP(t, l.http, base, accepted.PublicID, 200, "Editorial English")
	run(t, dir, nil, "docker", "pause", pg)
	t.Cleanup(func() { _ = exec.Command("docker", "unpause", pg).Run() })
	assertIntakeHTTP(t, l.http, base, accepted.PublicID, 503)
	short, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	_, err = s.SubmitSteam(short, "3000000007", "operator")
	cancel()
	if err == nil {
		t.Fatal("unreachable PostgreSQL acknowledged a submission")
	}
	run(t, dir, nil, "docker", "unpause", pg, redis)
	expectEventually(t, 15*time.Second, "GoFurry DB did not recover", func() bool { return pool.Ping(ctx) == nil })
	assertIntakeHTTP(t, l.http, base, accepted.PublicID, 200, "Editorial English")
	cut.offline.Store(true)
	_, err = s.SubmitSteam(ctx, "3000000007", "operator")
	must(t, err)
	expectEventually(t, 15*time.Second, "Harvest outage did not defer candidate", func() bool {
		c, err := s.Inspect(ctx, "steam", "3000000007")
		return err == nil && c.Reason == "harvest_unreachable"
	})
	assertIntakeHTTP(t, l.http, base, accepted.PublicID, 200, "Editorial English")
	l.worker.stop()
	cut.offline.Store(false)
	expectEventually(t, 25*time.Second, "a stopped Worker did not leave the durable Job pending", func() bool {
		c, err := s.Inspect(ctx, "steam", "3000000007")
		return err == nil && c.Reason == "job_pending" && c.Job != nil
	})
	l.worker = launch(t, l.dir, l.env, l.bin, "fixture", "worker", "--config", l.workerConfig)
	waitForIntake(t, s, pool, "3000000007", "accepted", "created_game", false)
	for _, scenario := range []struct{ key, state, reason string }{
		{"3000000002", "accepted", "created_game"}, {"3000000003", "rejected", "not_full_game"},
		{"3000000004", "deferred", "unknown_app_type"}, {"3000000005", "deferred", "weak_identity_match"},
	} {
		_, err = s.SubmitSteam(ctx, scenario.key, "discovery")
		must(t, err)
		row := waitForIntake(t, s, pool, scenario.key, scenario.state, scenario.reason, true)
		if scenario.key == "3000000002" && (row.GameState != "draft" || row.Observation == nil) {
			t.Fatal("missing positive evidence should allow only a sourced draft")
		}
		if scenario.key == "3000000002" {
			assertNotPublic(t, ctx, catalogDB, row.PublicID)
		}
		if scenario.state != "accepted" && row.Game != nil {
			t.Fatal("ambiguous or unsupported source acquired a Game")
		}
	}
	count(t, remote, 0, "SELECT count(*) FROM harvest.schedules s JOIN harvest.targets t ON t.id=s.target_id WHERE t.external_key IN ('3000000003','3000000004','3000000005')")

	// Confirmation links the original immutable Observation without
	// replacing its creation Job and Attempt.
	key := targetKey{fixtureStream, "steam", "app", "3000000006"}
	var target targetView
	must(t, h.call(ctx, l.writer, "POST", "targets", nil, key, &target, 201))
	pre := dbgen.GameIntakeCandidate{ExternalKey: key.ExternalKey, Namespace: key.Namespace, EgressRegion: "cn", TargetID: &target.ID}
	req, _, _ := intakeRequest(pre)
	l.job(req)
	_, err = s.SubmitSteam(ctx, key.ExternalKey, "operator")
	must(t, err)
	unchanged := waitForIntake(t, s, pool, key.ExternalKey, "accepted", "created_game", true)
	if unchanged.Observation == nil || unchanged.Game == nil || unchanged.Attempt == nil {
		t.Fatal("Harvest confirmation did not attribute the result")
	}
	count(t, remote, 1, "SELECT count(*) FROM harvest.observations WHERE id=$1 AND job_id<>$2 AND attempt_id<>$3", *unchanged.Observation, *unchanged.Job, *unchanged.Attempt)

	testIntakeRelations(t, s, pool, catalogDB, accepted)
	count(t, pool, 3, "SELECT count(*) FROM catalog_publication_decisions WHERE entity_id=$1 AND action IN ('publish','block','clear_block')", *accepted.Game)
	l.worker.stop()
	l.plane.stop()
	readAgedSteamGame(t, pool, catalogDB, accepted.PublicID, func() {
		assertIntakeHTTP(t, l.http, base, accepted.PublicID, 200, "Editorial English", "编辑译名")
	})
	if runtime.GOOS == "windows" {
		api.stop()
	} else {
		must(t, api.cmd.Process.Signal(syscall.SIGTERM))
		select {
		case <-api.done:
			if !api.cmd.ProcessState.Success() {
				t.Fatal("Fiber failed to drain Intake on SIGTERM")
			}
		case <-time.After(12 * time.Second):
			t.Fatal("Fiber did not drain Intake on SIGTERM")
		}
	}
	t.Logf("Harvest simulated network calls: %d", l.calls.Load())
}

func testIntakeRelations(t *testing.T, s *Intake, pool *pgxpool.Pool, editor *catalog.Service, first CandidateStatus) {
	t.Helper()
	ctx := t.Context()
	if err := s.RecordRelation(ctx, "itch", "fox-studio/library", "steam", first.Key, "one.record", "one.record"); !errors.Is(err, ErrIntakeInput) {
		t.Fatal("single-source relation accepted")
	}
	must(t, s.RecordRelation(ctx, "itch", "FOX-STUDIO/Library", "steam", first.Key, "simulated.itch-product", "simulated.steam-product"))
	attached, err := s.Tick(ctx)
	must(t, err)
	if attached.State != "accepted" || attached.Reason != "related_source" || *attached.Game != *first.Game {
		t.Fatalf("reciprocal product links did not attach to the same work: %+v", attached)
	}
	must(t, s.RevokeRelation(ctx, "itch", "fox-studio/library"))
	count(t, pool, 1, "SELECT count(*) FROM game_sources WHERE id=$1 AND game_id=$2 AND NOT active", attached.Source, first.Game)
	count(t, pool, 1, "SELECT count(*) FROM game_intake_candidates WHERE id=$1 AND state='rejected' AND relation_attached AND relation<>'{}'", attached.ID)

	curated := catalog.Editorial{Localizations: []catalog.Localization{{Locale: "en", Name: "Another curated work"}}}
	g, err := editor.CreateGame(ctx, catalog.GameInput{Editorial: curated, Sources: []catalog.Source{
		{PlatformCode: "itch_io", ExternalKey: "sample/curated", SourceURL: "https://sample.itch.io/curated"},
		{PlatformCode: "steam", ExternalKey: "42"},
	}})
	must(t, err)
	g, err = editor.SetGameStatus(ctx, g.ID, g.Revision, catalog.Published)
	must(t, err)
	_, err = s.SubmitSteam(ctx, "42", "operator")
	must(t, err)
	existing := waitForIntake(t, s, pool, "42", "accepted", "existing_source", true)
	if *existing.Game != g.ID || existing.PublicID != g.PublicID || existing.Job != nil {
		t.Fatal("existing source was reprovisioned")
	}
	must(t, s.RecordRelation(ctx, "itch", "sample/curated", "steam", first.Key, "simulated.itch-curated", "simulated.steam-curated"))
	suggestion, err := s.Tick(ctx)
	must(t, err)
	if suggestion.Reason != "merge_suggested" || suggestion.PublicID != g.PublicID {
		t.Fatalf("two published Games did not preserve their identities: %+v", suggestion)
	}
	_, err = editor.CreateGame(ctx, catalog.GameInput{Editorial: curated, Sources: []catalog.Source{{PlatformCode: "steam", ExternalKey: first.Key}}})
	if !errors.Is(err, catalog.ErrConflict) {
		t.Fatal("a different Game took an existing Steam identity")
	}
}

func wakeIntake(t *testing.T, pool *pgxpool.Pool, key string) {
	t.Helper()
	execSQL(t, pool, "UPDATE game_intake_candidates SET next_at=now() WHERE platform_code='steam' AND external_key=$1 AND next_at IS NOT NULL AND state='deferred'", key)
}

func assertIntakeReason(t *testing.T, s *Intake, key, state, reason string) CandidateStatus {
	t.Helper()
	row, err := s.Inspect(t.Context(), "steam", key)
	must(t, err)
	if row.State != state || row.Reason != reason {
		t.Fatalf("%s wanted %s/%s, got %+v", key, state, reason, row)
	}
	return row
}

func waitForIntake(t *testing.T, s *Intake, pool *pgxpool.Pool, key, state, reason string, manual bool) CandidateStatus {
	t.Helper()
	var row CandidateStatus
	until := time.Now().Add(35 * time.Second)
	for time.Now().Before(until) {
		if manual {
			wakeIntake(t, pool, key)
			_, err := s.Tick(t.Context())
			must(t, err)
		}
		var err error
		row, err = s.Inspect(t.Context(), "steam", key)
		must(t, err)
		if row.State == state && row.Reason == reason {
			return row
		}
		if row.State == "accepted" || row.State == "rejected" || row.State == "deferred" && row.NextAt == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("%s wanted %s/%s, got %+v", key, state, reason, row)
	return row
}

func assertIntakeHTTP(t *testing.T, client *http.Client, base, id string, code int, contains ...string) {
	t.Helper()
	resp, err := client.Get(base + "/api/v1/games/" + id)
	must(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	must(t, err)
	if resp.StatusCode != code {
		t.Fatalf("Game API HTTP %d, wanted %d", resp.StatusCode, code)
	}
	if resp.StatusCode == 200 && resp.Header.Get("Cache-Control") != "public, max-age=0, must-revalidate" {
		t.Fatal("Game detail could remain cached after publication review is revoked")
	}
	for _, part := range contains {
		if !strings.Contains(string(raw), part) {
			t.Fatalf("Game API is missing %q", part)
		}
	}
}

func intakeRedis(t *testing.T) (string, string) {
	t.Helper()
	container := run(t, "", nil, "docker", "run", "--detach", "--label", "gofurry-test=d3-a1", "--publish", "127.0.0.1::6379",
		"redis:8.2.1-alpine@sha256:987c376c727652f99625c7d205a1cba3cb2c53b92b0b62aade2bd48ee1593232",
		"redis-server", "--protected-mode", "no", "--save", "", "--appendonly", "no")
	t.Cleanup(func() {
		if exec.Command("docker", "rm", "--force", "--volumes", container).Run() != nil {
			t.Error("disposable Redis cleanup failed")
		}
	})
	expectEventually(t, 20*time.Second, "Redis fixture not ready", func() bool {
		return exec.Command("docker", "exec", container, "redis-cli", "ping").Run() == nil
	})
	return container, run(t, "", nil, "docker", "port", container, "6379/tcp")
}

func writeIntakeYAML(t *testing.T, path, addr, redis string, db platform.DatabaseConfig, cfg platform.HarvestConfig) {
	t.Helper()
	yaml := fmt.Sprintf(`server:
  address: %q
  environment: development
deployment:
  region: CN
logging:
  slow_request: 1s
redis:
  address: %q
database:
  host: 127.0.0.1
  port: %d
  name: %q
  user: %q
  password: %q
  ssl_mode: disable
  connect_timeout: 1s
  max_connections: 5
  min_connections: 0
  max_connection_lifetime: 30m
  max_connection_idle_time: 5m
harvest:
  url: %q
  namespace: %q
  consume: true
  writer_token: %q
  reader_token: %q
  consumer_id: %q
  poll_interval: 1s
  page_size: 4
  intake_enabled: true
  intake_egress_region: cn
`, addr, redis, db.Port, db.Name, db.User, db.Password, cfg.URL, cfg.Namespace, cfg.WriterToken, cfg.ReaderToken, cfg.ConsumerID)
	must(t, os.WriteFile(path, []byte(yaml), 0600))
}
