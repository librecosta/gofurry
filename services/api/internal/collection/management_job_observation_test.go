package collection

import (
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func TestWebJobObservationsUseExactAttempt(t *testing.T) {
	id := uuid.NewV7
	start := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	req := request{TargetID: id(), Capability: "web.dns", Schema: 1, Scope: scope{EgressRegion: "cn"}}
	detail := intakeJobDetail{
		Job: intakeJob{ID: id(), Namespace: "public", Request: req},
		Attempts: []intakeAttempt{
			{ID: id(), Claimed: start, Ended: start.Add(time.Second)},
			{ID: id(), Claimed: start.Add(2 * time.Second), Ended: start.Add(3 * time.Second)},
		},
	}
	attempt := detail.Attempts[1]
	segment := func(rr, state string) observation {
		s := segmentScope{Namespace: "public", scope: req.Scope,
			Dimensions: map[string]string{"section": "dns", "rrtype": rr, "resolver": "google-public-dns"}}
		raw, _ := json.Marshal(s, json.Deterministic(true))
		key := sha256.Sum256(raw)
		return observation{identity: identity{TargetID: req.TargetID, Capability: req.Capability, Schema: 1, ScopeKey: hex.EncodeToString(key[:])},
			ID: id(), Scope: s, State: state, ObservedAt: attempt.Ended, ReceivedAt: attempt.Ended,
			Source: "web", JobID: detail.Job.ID, AttemptID: attempt.ID}
	}
	answer := segment("a", "present")
	failure := segment("mx", "unavailable")
	failure.Evidence = "resolver_unavailable"
	unrelated := segment("txt", "unavailable")
	unrelated.JobID = id()
	previous := segment("ns", "unavailable")
	previous.AttemptID = detail.Attempts[0].ID
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/v1/observations/history" || q.Get("stream") != "public" || q.Get("target") != req.TargetID.String() ||
			q.Get("capability") != "web.dns" || q.Get("since") != attempt.Ended.Format(time.RFC3339Nano) || q.Get("until") != q.Get("since") {
			t.Error("unexpected history filter")
		}
		b, err := json.Marshal(map[string]any{"items": []observation{answer, failure, unrelated, previous}})
		if err != nil {
			t.Error(err)
		}
		_, _ = w.Write(b)
	}))
	defer ts.Close()
	s := Service{remote: &client{base: ts.URL, reader: "fixture", http: ts.Client()}}
	got := s.webJobObservations(t.Context(), detail, dbgen.ManagementJobOwnersRow{Provider: "web"})
	if len(got) != 2 || got[0].ID != answer.ID || got[1].ID != failure.ID {
		t.Fatalf("expected only this attempt's answer and diagnostic, got %d records", len(got))
	}
}
