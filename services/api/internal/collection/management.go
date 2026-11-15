package collection

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type ConsumerHealth struct {
	State     string    `json:"state"`
	CheckedAt time.Time `json:"checked_at"`
	Code      string    `json:"code,omitempty"`
}

func (s *Service) ConsumerHealth() ConsumerHealth {
	if s == nil || !v7(s.consumer) {
		return ConsumerHealth{State: "disabled"}
	}
	if value := s.health.Load(); value != nil {
		return *value
	}
	return ConsumerHealth{State: "unknown"}
}

func (s *Intake) ManageIntake(ctx context.Context, key string, retry bool, audit dbgen.RecordManagementOperationParams) error {
	source, err := intakeSource("steam", key)
	if err != nil || source.ExternalKey != key {
		return ErrIntakeInput
	}
	return s.transact(ctx, func(ctx context.Context, q *dbgen.Queries) error {
		if retry {
			err = retryIntake(ctx, q, source)
		} else {
			_, err = s.submit(ctx, q, source, "operator")
		}
		if err != nil {
			return err
		}
		audit.Target = key
		return q.RecordManagementOperation(ctx, audit)
	})
}

func (d *Discovery) ManagePause(ctx context.Context, paused bool, expected *bool, audit dbgen.RecordManagementOperationParams) error {
	return d.locked(ctx, func(conn *pgxpool.Conn, _ *dbgen.Queries, s dbgen.SteamDiscoveryState) error {
		if expected == nil || s.Paused != *expected {
			return ErrIntakeState
		}
		return write(ctx, conn.Conn(), func(q *dbgen.Queries) error {
			if err := q.ConfigureSteamDiscovery(ctx, dbgen.ConfigureSteamDiscoveryParams{Namespace: s.Namespace, Paused: paused}); err != nil {
				return err
			}
			audit.Target = "steam_discovery"
			return q.RecordManagementOperation(ctx, audit)
		})
	})
}

func (d *Discovery) Budgets() map[string]int32 {
	if d == nil {
		return nil
	}
	return map[string]int32{"daily_jobs": d.config.JobsPerDay, "daily_details": d.config.CandidatesPerDay, "daily_explore": d.config.ExplorePerDay}
}

func ManagementBusy(err error) bool { return errors.Is(err, errBusy) }

// ReattachManagementSteam uses the normal post-Intake schedule for an active
// Steam source that already belongs to the published Game under correction.
func (s *Intake) ReattachManagementSteam(ctx context.Context, q *dbgen.Queries, publicID, appID string) error {
	if s == nil || s.collection == nil {
		return ErrIntent
	}
	source, err := intakeSource("steam", appID)
	if err != nil || source.ExternalKey != appID {
		return ErrIntakeInput
	}
	game, err := q.LockGamePublicID(ctx, publicID)
	if err != nil {
		return err
	}
	if game.Status != "published" || game.PublicationBlocked {
		return ErrIntakeState
	}
	row, err := q.CollectionSteamSource(ctx, dbgen.CollectionSteamSourceParams{PublicID: publicID, ExternalKey: appID})
	if err != nil {
		return missingIntent(err)
	}
	return intakeSchedule(ctx, q, dbgen.GameIntakeCandidate{Namespace: s.collection.namespace, EgressRegion: s.region, ExternalKey: appID}, dbgen.GameSource{ID: row.ID, GameID: row.GameID})
}

// ManageReconcile keeps the receipt lookup and reconciliation under the
// existing namespace lock. The remote service owns its individual commits.
func (s *Service) ManageReconcile(ctx context.Context, audit dbgen.RecordManagementOperationParams) error {
	if s == nil || s.remote.writer == "" {
		return ErrIntent
	}
	return s.locked(ctx, func(_ *pgx.Conn, q *dbgen.Queries) error {
		_, err := q.ManagementReceipt(ctx, dbgen.ManagementReceiptParams{RequestID: audit.RequestID, ActorID: audit.ActorID})
		if err == nil {
			return ErrIntakeState
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err = s.reconcile(ctx, q); err != nil {
			return err
		}
		return q.RecordManagementOperation(ctx, audit)
	})
}
