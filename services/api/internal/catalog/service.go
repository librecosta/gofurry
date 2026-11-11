package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type Service struct {
	pool     *pgxpool.Pool
	publicID func() string
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, publicID: newPublicID}
}

func write[T any](ctx context.Context, s *Service, operation func(context.Context, *dbgen.Queries) (T, error)) (result T, err error) {
	if s.pool == nil {
		return result, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	result, err = operation(ctx, dbgen.New(tx))
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		var empty T
		return empty, classify(err)
	}
	return result, nil
}

func classify(err error) error {
	switch {
	case err == nil, errors.Is(err, ErrInvalid), errors.Is(err, ErrConflict), errors.Is(err, ErrNotFound):
		return err
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	default:
		return ErrUnavailable
	}
}

func create[T any](s *Service, insert func(string) (T, error)) (row T, err error) {
	for range 8 {
		row, err = insert(s.publicID())
		if !errors.Is(err, pgx.ErrNoRows) {
			return row, err
		}
	}
	return row, ErrUnavailable
}

func editable(expected, current int64, status string) error {
	if expected <= 0 {
		return ErrInvalid
	}
	if expected != current || status == Archived {
		return ErrConflict
	}
	return nil
}

func canChange(current, desired string) error {
	switch desired {
	case Published:
		if current == Draft {
			return nil
		}
	case Archived:
		return nil // The revision check already excludes archived rows.
	default:
		return ErrInvalid
	}
	return ErrConflict
}
