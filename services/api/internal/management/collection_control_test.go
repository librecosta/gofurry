package management

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/collection"
)

type tempCollectionJobs struct {
	api                               *http.ServeMux
	jobID                             uuid.UUID
	runCalls, cancelCalls, retryCalls atomic.Int32
}

// The fixture supplies only Harvest's network replies. Plan authorization,
// binding intent, and Management receipts use native PostgreSQL temporary tables.
func tempCollectionExecution(t *testing.T, ctx context.Context, pool *pgxpool.Pool, plan collection.CollectionPlan) *tempCollectionJobs {
	t.Helper()
	target, schedule := uuid.NewV7(), uuid.NewV7()
	_, err := pool.Exec(ctx, "UPDATE collection_sources SET target_id=$1 WHERE id=$2", target, plan.SourceID)
	siteMust(t, err)
	_, err = pool.Exec(ctx, "UPDATE collection_plans SET schedule_id=$1, remote_enabled=true WHERE id=$2", schedule, plan.ID)
	siteMust(t, err)

	var params, scope jsontext.Value
	var interval int32
	var anchor time.Time
	var policy string
	siteMust(t, pool.QueryRow(ctx, "SELECT params,scope,interval_seconds,anchor_at,policy FROM collection_plans WHERE id=$1", plan.ID).Scan(&params, &scope, &interval, &anchor, &policy))
	const namespace = "test_editorial"
	request := map[string]any{"target_id": target, "capability": plan.Capability, "request_schema": 1, "params": params, "scope": scope}
	fixture := &tempCollectionJobs{api: http.NewServeMux(), jobID: uuid.NewV7()}
	var created time.Time
	siteMust(t, pool.QueryRow(ctx, "SELECT statement_timestamp()").Scan(&created))
	send := func(w http.ResponseWriter, status int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.MarshalWrite(w, value); err != nil {
			t.Errorf("encode isolated response: %v", err)
		}
	}
	activeJob := func(id uuid.UUID, state string) map[string]any {
		return map[string]any{"id": id, "namespace": namespace, "request": request, "schedule_id": schedule,
			"state": state, "cause": "manual", "attempt_count": 0, "created_at": created, "updated_at": created, "deadline_at": created.Add(5 * time.Minute)}
	}
	fixture.api.HandleFunc("GET /api/v1/schedules/"+schedule.String(), func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]any{"id": schedule, "namespace": namespace, "request": request, "enabled": true, "revision": 1,
			"recurrence": map[string]any{"interval_seconds": interval, "anchor_at": anchor, "policy": policy}})
	})
	fixture.api.HandleFunc("POST /api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ScheduleID uuid.UUID `json:"schedule_id"`
		}
		if json.UnmarshalRead(r.Body, &in) != nil || in.ScheduleID != schedule || r.Header.Get("Idempotency-Key") == "" {
			t.Error("Run Now used an unverified schedule or lacked an idempotency key")
			w.WriteHeader(400)
			return
		}
		fixture.runCalls.Add(1)
		send(w, 202, activeJob(fixture.jobID, "queued"))
	})
	path := "/api/v1/jobs/" + fixture.jobID.String()
	fixture.api.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
		state := "queued"
		if fixture.cancelCalls.Load() != 0 {
			state = "cancelled"
		}
		send(w, 200, map[string]any{"job": activeJob(fixture.jobID, state), "attempts": []any{}})
	})
	fixture.api.HandleFunc("POST "+path+"/cancel", func(w http.ResponseWriter, r *http.Request) {
		fixture.cancelCalls.Add(1)
		send(w, 202, activeJob(fixture.jobID, "cancelled"))
	})
	fixture.api.HandleFunc("POST "+path+"/retry", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "gofurry-retry-"+fixture.jobID.String() {
			t.Error("retry idempotency lost")
		}
		fixture.retryCalls.Add(1)
		job := activeJob(uuid.NewV7(), "queued")
		job["origin_job_id"] = fixture.jobID
		send(w, 202, job)
	})
	return fixture
}
