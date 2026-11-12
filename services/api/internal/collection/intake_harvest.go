package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

const intakeCapability = "steam.details"

type intakeJob struct {
	ID              uuid.UUID  `json:"id"`
	Namespace       string     `json:"namespace"`
	Request         request    `json:"request"`
	State           string     `json:"state"`
	Cause           string     `json:"cause"`
	Code            string     `json:"code"`
	Deadline        time.Time  `json:"deadline_at"`
	Updated         time.Time  `json:"updated_at"`
	AcceptedAttempt *uuid.UUID `json:"accepted_attempt_id"`
	ScheduleID      *uuid.UUID `json:"schedule_id"`
}

type intakeAttempt struct {
	ID      uuid.UUID `json:"id"`
	State   string    `json:"state"`
	Claimed time.Time `json:"claimed_at"`
	Ended   time.Time `json:"ended_at"`
}

type intakeJobDetail struct {
	Job      intakeJob       `json:"job"`
	Attempts []intakeAttempt `json:"attempts"`
}

type intakeObservation struct {
	observation
	detail steamDetail
}

func intakePlan(region string) Plan {
	return Plan{Capability: intakeCapability, Locale: "en", Region: "us", EgressRegion: region,
		Interval: 24 * time.Hour, Anchor: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func intakeRequest(c dbgen.GameIntakeCandidate) (request, segmentScope, string) {
	plan := intakePlan(c.EgressRegion)
	params, _, _, _ := plan.params("app", c.ExternalKey)
	semantic := scope{Locale: plan.Locale, Region: plan.Region, EgressRegion: plan.EgressRegion}
	output := segmentScope{Namespace: c.Namespace, scope: semantic, Dimensions: map[string]string{"section": "details"}}
	raw, _ := json.Marshal(output, json.Deterministic(true))
	digest := sha256.Sum256(raw)
	var target uuid.UUID
	if c.TargetID != nil {
		target = *c.TargetID
	}
	return request{target, plan.Capability, 1, params, semantic}, output, hex.EncodeToString(digest[:])
}

func (s *Intake) target(ctx context.Context, c dbgen.GameIntakeCandidate) error {
	key := targetKey{c.Namespace, "steam", "app", c.ExternalKey}
	var v targetView
	r := s.collection.remote
	if err := r.call(ctx, r.writer, http.MethodPost, "targets", nil, key, &v, http.StatusCreated); err != nil {
		return err
	}
	if v.targetKey != key || !v7(v.ID) || v.State != "active" {
		return errRemoteConflict
	}
	if err := s.collection.verifyTarget(ctx, v.ID, key); err != nil {
		return err
	}
	return affected(s.queries.SaveIntakeTarget(ctx, dbgen.SaveIntakeTargetParams{ID: c.ID, LeaseID: c.LeaseID, TargetID: &v.ID}))
}

func sameIntakeJob(j intakeJob, req request, namespace string) bool {
	return v7(j.ID) && j.Namespace == namespace && j.Cause == "manual" && j.ScheduleID == nil &&
		j.Request.TargetID == req.TargetID && j.Request.Capability == req.Capability &&
		j.Request.Schema == req.Schema && j.Request.Scope == req.Scope && sameParams(j.Request.Params, req.Params) &&
		!j.Deadline.IsZero() && !j.Updated.IsZero()
}

func (s *Intake) submitJob(ctx context.Context, c dbgen.GameIntakeCandidate, req request) error {
	r := s.collection.remote
	sum := sha256.Sum256([]byte(r.base + "\x00" + r.writer))
	binding := hex.EncodeToString(sum[:])
	if c.JobWriterBinding != "" && c.JobWriterBinding != binding {
		return errIntakeWriterChanged
	}
	start, err := s.queries.StartIntakeJob(ctx, dbgen.StartIntakeJobParams{ID: c.ID, LeaseID: c.LeaseID, WriterBinding: binding})
	if err != nil {
		return err
	}
	if time.Since(start.Time) > 15*time.Minute {
		return errIntakeUncertain
	}
	// Harvest has no read by idempotency key. Replay the same POST until
	// its ID is known, then use read-only GETs. The writer and origin fence
	// above keeps replay in the same Harvest idempotency namespace.
	key := "gofurry-intake-" + c.ID.String() + "-" + strconv.Itoa(int(c.JobRound))
	in := struct {
		Request request `json:"request"`
	}{req}
	var j intakeJob
	if err := r.callWithKey(ctx, r.writer, http.MethodPost, "jobs", nil, in, &j, http.StatusAccepted, key); err != nil {
		return err
	}
	if !sameIntakeJob(j, req, c.Namespace) {
		return errRemoteConflict
	}
	return affected(s.queries.SaveIntakeJob(ctx, dbgen.SaveIntakeJobParams{ID: c.ID, LeaseID: c.LeaseID, JobID: &j.ID}))
}

var (
	errIntakeWriterChanged = errors.New("intake writer or origin changed during an uncertain submission")
	errIntakeUncertain     = errors.New("intake job submission reconciliation window elapsed")
)

func (s *Intake) job(ctx context.Context, c dbgen.GameIntakeCandidate, req request) (intakeJobDetail, error) {
	r := s.collection.remote
	var detail intakeJobDetail
	err := r.call(ctx, r.reader, http.MethodGet, "jobs/"+c.JobID.String(), nil, nil, &detail, http.StatusOK)
	if err == nil && (!sameIntakeJob(detail.Job, req, c.Namespace) || detail.Job.ID != *c.JobID || len(detail.Attempts) > 3) {
		err = errRemoteConflict
	}
	return detail, err
}

func (s *Intake) attributedObservation(ctx context.Context, c dbgen.GameIntakeCandidate, j intakeJobDetail, expected segmentScope, key string) (intakeObservation, error) {
	var out intakeObservation
	if j.Job.AcceptedAttempt == nil || !v7(*j.Job.AcceptedAttempt) {
		return out, errProtocol
	}
	var attempt intakeAttempt
	for _, a := range j.Attempts {
		if a.ID == *j.Job.AcceptedAttempt {
			attempt = a
			break
		}
	}
	if attempt.Claimed.IsZero() || !attempt.Ended.Equal(j.Job.Updated) || attempt.Ended.Before(attempt.Claimed) ||
		attempt.Ended.After(j.Job.Deadline) || attempt.State != j.Job.State {
		return out, errProtocol
	}
	q := url.Values{"stream": {c.Namespace}, "target": {c.TargetID.String()}, "capability": {intakeCapability}, "scope": {key},
		"since": {attempt.Claimed.UTC().Format(time.RFC3339Nano)}, "until": {attempt.Ended.UTC().Format(time.RFC3339Nano)}, "limit": {"8"}}
	r := s.collection.remote
	for range 2 {
		var page struct {
			Items []observation `json:"items"`
			After string        `json:"after"`
		}
		if err := r.call(ctx, r.reader, http.MethodGet, "observations/history", q, nil, &page, http.StatusOK); err != nil {
			return out, err
		}
		if len(page.Items) > 8 || page.After != "" && (!tokenOK(page.After) || len(page.Items) == 0 || page.After == q.Get("after")) {
			return out, errProtocol
		}
		for _, o := range page.Items {
			if o.JobID != j.Job.ID || o.AttemptID != attempt.ID {
				continue
			}
			if out.ID != uuid.Nil() || o.TargetID != *c.TargetID || o.Capability != intakeCapability || o.Schema != 1 ||
				o.ScopeKey != key || o.Scope.Namespace != expected.Namespace || o.Scope.scope != expected.scope ||
				len(o.Scope.Dimensions) != 1 || o.Scope.Dimensions["section"] != "details" || !scopeMatches(o.Scope, key) ||
				!o.ReceivedAt.Equal(attempt.Ended) || o.ObservedAt.Before(attempt.Claimed.Add(-2*time.Minute)) ||
				!v7(o.ID) || o.Source != "steam" || o.Sample {
				return out, errProtocol
			}
			if o.State == "unavailable" || o.State == "invalid" || o.State == "not_covered" {
				return out, nil
			}
			if o.State != "present" || o.Evidence != "" || !validObservation(result{observation: o}) {
				return out, errProtocol
			}
			normalized, err := normalizeSteam(o, c.ExternalKey)
			if err != nil || json.Unmarshal(normalized.Data, &out.detail) != nil || out.detail.Name == "" {
				return out, errProtocol
			}
			out.observation = o
		}
		if page.After == "" {
			return out, nil
		}
		q.Set("after", page.After)
	}
	return intakeObservation{}, errProtocol
}
