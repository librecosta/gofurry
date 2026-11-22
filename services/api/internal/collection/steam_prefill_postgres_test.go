package collection

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestSteamPrefillRetentionPostgres(t *testing.T) {
	ctx, conn := nativeCollectionPostgres(t)
	tx := nativeCollectionTables(t, ctx, conn, "steam_editorial_prefills", "game_sources")
	exec := func(sql string) {
		t.Helper()
		_, err := tx.Exec(ctx, sql)
		requirePlanTest(t, err)
	}
	exec(`INSERT INTO steam_editorial_prefills (namespace,app_id,locale,generation,started_at)
SELECT 'local', id::text, lang, uuidv7(), now()-interval '31 days'
FROM generate_series(1, 103) ids(id) CROSS JOIN (VALUES ('en'), ('zh-cn')) langs(lang)`)
	exec(`UPDATE steam_editorial_prefills SET started_at=now(), job_id=uuidv7(), target_id=uuidv7()
WHERE app_id='102' AND locale='zh-cn'`)
	exec(`UPDATE steam_editorial_prefills SET namespace='unrelated' WHERE app_id='103'`)
	exec(`INSERT INTO steam_editorial_prefills (namespace,app_id,locale,generation,started_at)
VALUES ('local','104','zh-cn',uuidv7(),now()-interval '30 days 1 hour')`)
	exec(`INSERT INTO game_sources (id,game_id,platform_code,external_key,source_url,active)
VALUES (uuidv7(),uuidv7(),'steam','101','https://store.steampowered.com/app/101/',false)`)
	q := dbgen.New(tx)
	requirePlanTest(t, q.LockSteamPrefillAdmission(ctx, "local"))
	for _, want := range []int64{100, 1, 0} {
		got, err := q.PruneExpiredSteamPrefills(ctx, dbgen.PruneExpiredSteamPrefillsParams{Namespace: "local", BatchSize: 100})
		requirePlanTest(t, err)
		if got != want {
			t.Fatalf("pruned %d AppIDs, want %d", got, want)
		}
	}
	for _, record := range []struct{ namespace, app string }{
		{"local", "101"}, {"local", "102"}, {"unrelated", "103"},
	} {
		rows, err := q.ListSteamEditorialPrefills(ctx, dbgen.ListSteamEditorialPrefillsParams{Namespace: record.namespace, AppID: record.app})
		requirePlanTest(t, err)
		if len(rows) != 2 {
			t.Fatalf("lost a protected language scope for %s", record.app)
		}
	}
	var remaining int64
	requirePlanTest(t, tx.QueryRow(ctx, "SELECT count(*) FROM steam_editorial_prefills").Scan(&remaining))
	if remaining != 6 {
		t.Fatalf("expected only the six protected rows, got %d", remaining)
	}
}

func TestSteamPrefillLostSubmissionPostgres(t *testing.T) {
	ctx, conn := nativeCollectionPostgres(t)
	tx := nativeCollectionTables(t, ctx, conn, "steam_editorial_prefills", "game_sources", "games")
	const namespace, app = "prefill-recovery", "42"
	target, jobID := uuid.NewV7(), uuid.NewV7()
	var posts atomic.Int32
	var firstKey atomic.Pointer[string]
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(status int, payload any) {
			w.WriteHeader(status)
			if err := json.MarshalWrite(w, payload); err != nil {
				t.Error(err)
			}
		}
		switch r.URL.Path {
		case "/api/v1/targets":
			reply(201, targetView{targetKey: targetKey{namespace, "steam", "app", app}, ID: target, State: "active"})
		case "/api/v1/observations/history":
			reply(200, struct{ Items []observation }{})
		case "/api/v1/jobs":
			key := r.Header.Get("Idempotency-Key")
			var body struct {
				Request request `json:"request"`
			}
			if err := json.UnmarshalRead(r.Body, &body); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if posts.Add(1) == 1 {
				firstKey.Store(&key)
				// The remote side accepted the job, but the reply did not arrive.
				w.WriteHeader(502)
				return
			}
			if key == "" || key != *firstKey.Load() {
				t.Error("resend changed the Harvest idempotency key")
			}
			now := time.Now().UTC()
			reply(202, intakeJob{ID: jobID, Namespace: namespace, Request: body.Request, Cause: "manual",
				State: "queued", Deadline: now.Add(5 * time.Minute), Updated: now})
		case "/api/v1/jobs/" + jobID.String():
			req := (&Service{}).prefillRequest(dbgen.SteamEditorialPrefill{AppID: app, Locale: "en"}, "cn", target)
			now := time.Now().UTC()
			reply(200, intakeJobDetail{Job: intakeJob{ID: jobID, Namespace: namespace, Request: req, Cause: "manual",
				State: "queued", Deadline: now.Add(5 * time.Minute), Updated: now}})
		default:
			w.WriteHeader(503)
		}
	}))
	t.Cleanup(remote.Close)
	cfg := platform.HarvestConfig{URL: remote.URL, Namespace: namespace, WriterToken: "writer", ReaderToken: "reader"}
	s := New(nil, cfg)
	t.Cleanup(s.Close)
	q := dbgen.New(tx)
	requirePlanTest(t, s.AdmitSteamPrefill(ctx, q, app))
	rows, err := s.prefillRows(ctx, q, app)
	requirePlanTest(t, err)
	if len(rows) != 2 {
		t.Fatal("expected both languages")
	}
	s.kickPrefillScope(ctx, q, rows[0], "cn", false)
	restarted := New(nil, cfg)
	t.Cleanup(restarted.Close)
	rows, err = restarted.prefillRows(ctx, q, app)
	requirePlanTest(t, err)
	if rows[0].JobID != nil || rows[0].SubmissionCode == "" ||
		restarted.readPrefillScope(ctx, q, rows[0], "cn").State != "unavailable" {
		t.Fatal("lost submission was not available for a retry")
	}
	requirePlanTest(t, restarted.AdmitSteamPrefill(ctx, q, app))
	restarted.kickPrefillScope(ctx, q, rows[0], "cn", true)
	rows, err = restarted.prefillRows(ctx, q, app)
	requirePlanTest(t, err)
	if rows[0].JobID == nil || *rows[0].JobID != jobID || rows[0].SubmissionCode != "" || posts.Load() != 2 {
		t.Fatal("replay did not recover the original Job")
	}
	if result := restarted.readPrefillScope(ctx, q, rows[0], "cn"); result.State != "queued" {
		t.Fatalf("recovered job is not queryable: %+v", result)
	}
	restarted.kickPrefillScope(ctx, q, rows[0], "cn", false)
	var games int
	requirePlanTest(t, tx.QueryRow(ctx, "SELECT count(*) FROM games").Scan(&games))
	if games != 0 || posts.Load() != 2 {
		t.Fatal("prefill created a game or resubmitted a known Job")
	}
	_, err = tx.Exec(ctx, "UPDATE steam_editorial_prefills SET started_at=now()-interval '15 minutes' WHERE locale='zh-cn'")
	requirePlanTest(t, err)
	renewed, err := q.ReplaceSteamPrefill(ctx, dbgen.ReplaceSteamPrefillParams{Namespace: namespace, AppID: app, Locale: "zh-cn",
		ExpectedGeneration: rows[1].Generation, NewGeneration: uuid.NewV7()})
	requirePlanTest(t, err)
	if renewed.Generation == rows[1].Generation || renewed.JobID != nil {
		t.Fatal("expired submission could not start a fresh generation")
	}
}
