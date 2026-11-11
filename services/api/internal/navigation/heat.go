package navigation

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

const heatInterval = 30 * time.Minute

type RefreshResult struct {
	Refreshed bool  `json:"refreshed"`
	Ranked    int64 `json:"ranked"`
}

func (s *Service) RefreshHeat(ctx context.Context, force bool) (RefreshResult, error) {
	var result RefreshResult
	if s.pool == nil {
		return result, catalog.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// Read the head after obtaining the lock. A repeatable-read snapshot taken
	// by the lock statement can predate the preceding refresh's commit.
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		locked, err := q.LockNavigationHeat(ctx)
		if err != nil || !locked {
			return err
		}
		now := s.now().UTC()
		last, err := q.NavigationHeatState(ctx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if last.ComputedAt.Valid && (!now.After(last.ComputedAt.Time) || !force &&
			last.Algorithm == Algorithm && last.ComputedAt.Time.Truncate(heatInterval).Equal(now.Truncate(heatInterval))) {
			return nil
		}
		result.Ranked, err = q.RefreshNavigationHeat(ctx, dbgen.RefreshNavigationHeatParams{WindowEnd: pgDay(now),
			ComputedAt: pgtype.Timestamptz{Time: now, Valid: true}})
		result.Refreshed = err == nil
		return err
	})
	if err != nil {
		return RefreshResult{}, catalog.ErrUnavailable
	}
	return result, nil
}

func (s *Service) Run(ctx context.Context, report func()) {
	for ctx.Err() == nil {
		if _, err := s.RefreshHeat(ctx, false); err != nil && ctx.Err() == nil {
			report()
		}
		now := s.now().UTC()
		timer := time.NewTimer(now.Truncate(heatInterval).Add(heatInterval).Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
