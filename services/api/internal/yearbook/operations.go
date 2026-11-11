package yearbook

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

const reconcileBatch = 96

type Reconciled struct {
	PublicID  string   `json:"public_id"`
	Year      int32    `json:"year"`
	Eligible  bool     `json:"eligible"`
	Reasons   []string `json:"reasons,omitempty"`
	Revision  int32    `json:"revision,omitempty"`
	Digest    string   `json:"digest,omitempty"`
	Published bool     `json:"published"`
}

func (s *Service) Reconcile(ctx context.Context, year int32, apply bool) ([]Reconciled, error) {
	return s.reconcile(ctx, year, apply, false)
}

func (s *Service) reconcile(ctx context.Context, year int32, apply, automatic bool) ([]Reconciled, error) {
	if s.pool == nil {
		return nil, ErrUnavailable
	}
	now := s.now().UTC()
	first, last := int32(2000), eligibleThrough(now)
	if year != 0 {
		if !validYear(year) {
			return nil, ErrInvalid
		}
		if year > last {
			return nil, ErrEarly
		}
		first, last = year, year
	}
	ctx, cancel := context.WithTimeout(ctx, 80*time.Second)
	defer cancel()
	rows, err := dbgen.New(s.pool).YearbookCandidates(ctx, dbgen.YearbookCandidatesParams{
		FirstYear: first, LastYear: last, Automatic: automatic, BatchSize: reconcileBatch, RecheckBefore: stamp(now.AddDate(0, 0, -7)),
	})
	if err != nil {
		return nil, known(err)
	}
	results := make([]Reconciled, 0, len(rows))
	var failure error
	for _, row := range rows {
		result, err := s.generate(ctx, row.PublicID, row.Year, apply, automatic)
		if err != nil {
			if failure == nil {
				failure = err
			}
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if result.Year != 0 {
			results = append(results, Reconciled{row.PublicID, row.Year, result.Eligible, result.Reasons, result.Revision, result.Digest, result.Published})
		}
	}
	return results, failure
}

func (s *Service) Compact(ctx context.Context) (int32, error) {
	total, err := s.compact(ctx)
	return total, known(err)
}

func (s *Service) compact(ctx context.Context) (total int32, err error) {
	if s.pool == nil {
		return 0, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	started := time.Now()
	// Retain at least 45 days and wait for the entire month to leave the
	// retention window. Late B1 confirmations can reappear in the daily table;
	// the next OR merge consumes them without replacing previously set bits.
	cutoff := s.now().UTC().AddDate(0, 0, -45)
	closed := date(time.Date(cutoff.Year(), cutoff.Month(), 1, 0, 0, 0, 0, time.UTC))
	const batchSize = 2048
	args := dbgen.CompactSiteCoverageParams{BeforeDay: closed, AfterDay: date(time.Time{}), BatchSize: batchSize}
	for range 16 {
		var result dbgen.CompactSiteCoverageRow
		err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			q := dbgen.New(tx)
			locked, err := q.TryYearbookCompaction(ctx)
			if err != nil || !locked {
				return err
			}
			result, err = q.CompactSiteCoverage(ctx, args)
			if err != nil {
				return err
			}
			if result.Selected != result.Pruned {
				return ErrConflict
			}
			return nil
		})
		if err != nil {
			return total, err
		}
		total += result.Pruned
		// Finish before the hard deadline when there is a large backlog.
		// Completed batches are durable; the next pass scans the remaining keys.
		if result.Scanned < batchSize || time.Since(started) >= 22*time.Second {
			break
		}
		args.AfterDay, args.AfterSite, args.AfterSource = result.LastDay, result.LastSite, result.LastSource
		args.AfterCapability, args.AfterSchema, args.AfterScope = result.LastCapability, result.LastSchema, result.LastScope
	}
	return total, nil
}

func (s *Service) Audit(ctx context.Context) ([]dbgen.AuditYearbookReferencesRow, error) {
	if s.pool == nil {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var result []dbgen.AuditYearbookReferencesRow
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var err error
		result, err = dbgen.New(tx).AuditYearbookReferences(ctx)
		return err
	})
	return result, known(err)
}

func nextDay(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
}

func (s *Service) Run(ctx context.Context, report func(error)) {
	for ctx.Err() == nil {
		_, closeErr := s.reconcile(ctx, 0, true, true)
		_, compactErr := s.Compact(ctx)
		if ctx.Err() != nil {
			return
		}
		if closeErr != nil || compactErr != nil {
			report(errors.Join(closeErr, compactErr))
		}
		now := s.now()
		timer := time.NewTimer(nextDay(now).Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
