package collection

import (
	"context"
	"errors"
	"net/http"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var (
	ErrIntakeInput    = errors.New("invalid intake input")
	ErrIntakeCapacity = errors.New("intake candidate capacity reached")
	ErrIntakeState    = errors.New("intake candidate missing or state changed")
)

type Intake struct {
	pool       *pgxpool.Pool
	queries    *dbgen.Queries
	catalog    *catalog.Service
	collection *Service
	region     string
}

func NewIntake(pool *pgxpool.Pool, cfg *platform.HarvestConfig) *Intake {
	s := &Intake{pool: pool, queries: dbgen.New(pool), catalog: catalog.New(pool)}
	if cfg != nil && cfg.IntakeEgressRegion != "" && cfg.WriterToken != "" && cfg.ReaderToken != "" {
		s.collection = New(pool, *cfg)
		s.region = cfg.IntakeEgressRegion
	}
	return s
}

func (s *Intake) Close() {
	if s.collection != nil {
		s.collection.Close()
	}
}

func (s *Intake) transact(ctx context.Context, f func(context.Context, *dbgen.Queries) error) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return f(ctx, dbgen.New(tx)) })
}

func affected(count int64, err error) error {
	if err == nil && count != 1 {
		return ErrIntakeState
	}
	return err
}

func (s *Intake) submit(ctx context.Context, q *dbgen.Queries, source catalog.Source, via string) (dbgen.GameIntakeCandidate, error) {
	var row dbgen.GameIntakeCandidate
	if err := q.LockIntakeAdmission(ctx); err != nil {
		return row, err
	}
	row, err := q.IntakeCandidate(ctx, dbgen.IntakeCandidateParams{PlatformCode: source.PlatformCode, ExternalKey: source.ExternalKey})
	if !errors.Is(err, pgx.ErrNoRows) {
		return row, err
	}
	row, err = q.InsertIntakeCandidate(ctx, dbgen.InsertIntakeCandidateParams{
		ID: uuid.NewV7(), PlatformCode: source.PlatformCode, ExternalKey: source.ExternalKey, DiscoveredVia: via,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrIntakeCapacity
	}
	return row, err
}

// SubmitSteam is shared by the local command and future bounded discovery.
func (s *Intake) SubmitSteam(ctx context.Context, key, via string) (CandidateStatus, error) {
	source, err := intakeSource("steam", key)
	if err != nil || via != "operator" && via != "discovery" {
		return CandidateStatus{}, ErrIntakeInput
	}
	var row dbgen.GameIntakeCandidate
	err = s.transact(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		var err error
		row, err = s.submit(ctx, q, source, via)
		return err
	})
	return candidateStatus(row), err
}

func (s *Intake) claim(ctx context.Context) (dbgen.GameIntakeCandidate, error) {
	var row dbgen.GameIntakeCandidate
	namespace := ""
	if s.collection != nil {
		namespace = s.collection.namespace
	}
	err := s.transact(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		locked, err := q.TryIntakeDispatch(ctx)
		if err != nil || !locked {
			return err
		}
		lease := uuid.NewV7()
		row, err = q.ClaimIntake(ctx, dbgen.ClaimIntakeParams{LeaseID: &lease, Namespace: namespace, EgressRegion: s.region})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return row, err
}

// Tick processes one candidate across the deployment, regardless of which API
// replica or CLI acquires the lease. Network calls never hold a transaction.
func (s *Intake) Tick(ctx context.Context) (CandidateStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	row, err := s.claim(ctx)
	if err != nil {
		return CandidateStatus{}, err
	}
	if row.LeaseID == nil {
		_, err = s.queries.PruneIntakeAbandoned(ctx)
		return CandidateStatus{}, err
	}
	if err = s.process(ctx, row); err != nil {
		if ctx.Err() != nil {
			return candidateStatus(row), ctx.Err()
		}
		reason := intakeRemoteReason(err)
		if reason == "" {
			return candidateStatus(row), err // Fenced recovery on the next lease.
		}
		failures := min(row.Failures+1, 8)
		var delay int32
		if failures < 8 && reason != "submission_identity_changed" && reason != "submission_uncertain" {
			delay = min(int32(5)<<uint(failures-1), 300)
		}
		if err = releaseIntake(ctx, s.queries, row, "deferred", reason, delay, failures); err != nil {
			return candidateStatus(row), err
		}
	}
	return s.Inspect(ctx, row.PlatformCode, row.ExternalKey)
}

func (s *Intake) process(ctx context.Context, c dbgen.GameIntakeCandidate) error {
	if c.PlatformCode != "steam" {
		return s.resolve(ctx, c, intakeObservation{}, "")
	}
	if s.collection == nil {
		return releaseIntake(ctx, s.queries, c, "deferred", "harvest_unconfigured", 300, 0)
	}
	if c.TargetID == nil {
		if _, err := s.queries.ExistingIntakeSource(ctx, dbgen.ExistingIntakeSourceParams{PlatformCode: c.PlatformCode, ExternalKey: c.ExternalKey}); err == nil {
			return s.resolve(ctx, c, intakeObservation{}, "")
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := s.target(ctx, c); err != nil {
			return err
		}
		return releaseIntake(ctx, s.queries, c, "deferred", "target_verified", 1, 0)
	}
	req, expected, scopeKey := intakeRequest(c)
	if c.JobID == nil {
		if err := s.submitJob(ctx, c, req); err != nil {
			return err
		}
		return releaseIntake(ctx, s.queries, c, "deferred", "job_submitted", 1, 0)
	}
	j, err := s.job(ctx, c, req)
	if err != nil {
		return err
	}
	switch j.Job.State {
	case "queued", "running", "retry_wait", "cancel_requested":
		if time.Now().After(j.Job.Deadline.Add(time.Minute)) {
			return releaseIntake(ctx, s.queries, c, "deferred", "job_overdue", 0, 0)
		}
		return releaseIntake(ctx, s.queries, c, "deferred", "job_pending", 5, 0)
	case "failed":
		switch j.Job.Code {
		case "worker_lost", "deadline_exceeded", "network_quota_deferred", "upstream_throttled", "transport_error",
			"timeout", "internal_unavailable", "network_policy_changed", "upstream_unavailable":
			if c.JobRound == 1 {
				return affected(s.queries.RestartIntakeJob(ctx, dbgen.RestartIntakeJobParams{ID: c.ID, LeaseID: c.LeaseID, JobID: c.JobID}))
			}
		}
		return releaseIntake(ctx, s.queries, c, "deferred", "job_failed", 0, 0)
	case "cancelled":
		return releaseIntake(ctx, s.queries, c, "deferred", "job_cancelled", 0, 0)
	case "succeeded", "partial":
		o, err := s.attributedObservation(ctx, c, j, expected, scopeKey)
		if err != nil {
			return err
		}
		if o.ID == uuid.Nil() {
			return releaseIntake(ctx, s.queries, c, "deferred", "observation_unattributed", 0, 0)
		}
		return s.resolve(ctx, c, o, scopeKey)
	default:
		return errProtocol
	}
}

func releaseIntake(ctx context.Context, q *dbgen.Queries, c dbgen.GameIntakeCandidate, state, reason string, delay, failures int32) error {
	return affected(q.ReleaseIntake(ctx, dbgen.ReleaseIntakeParams{
		ID: c.ID, LeaseID: c.LeaseID, State: state, Reason: reason, DelaySeconds: delay, Failures: failures,
	}))
}

func intakeRemoteReason(err error) string {
	switch {
	case errors.Is(err, errIntakeWriterChanged):
		return "submission_identity_changed"
	case errors.Is(err, errIntakeUncertain):
		return "submission_uncertain"
	case errors.Is(err, errNetwork):
		return "harvest_unreachable"
	case errors.Is(err, errProtocol), errors.Is(err, errRemoteConflict):
		return "harvest_contract"
	default:
		var status remoteStatus
		if errors.As(err, &status) {
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				return "harvest_unauthorized"
			}
			return "harvest_status"
		}
		return ""
	}
}

func (s *Intake) Run(ctx context.Context, report func()) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if _, err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			report()
		}
		timer.Reset(5 * time.Second)
	}
}

func (s *Intake) Retry(ctx context.Context, platform, key string) error {
	source, err := intakeSource(platform, key)
	if err != nil {
		return err
	}
	return s.transact(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		row, err := lockIntakeCandidate(ctx, q, source)
		if err != nil {
			return err
		}
		if row.State != "deferred" || row.NextAt.Valid {
			return ErrIntakeState
		}
		return q.WakeIntake(ctx, row.ID)
	})
}

func (s *Intake) Dismiss(ctx context.Context, platform, key string) error {
	source, err := intakeSource(platform, key)
	if err != nil {
		return ErrIntakeInput
	}
	return s.transact(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		row, err := lockIntakeCandidate(ctx, q, source)
		if err != nil {
			return err
		}
		return affected(q.DismissIntake(ctx, row.ID))
	})
}
