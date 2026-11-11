package collection

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

type Service struct {
	pool      *pgxpool.Pool
	remote    *client
	namespace string
	consumer  uuid.UUID
	poll      time.Duration
	pageSize  int
}

func New(pool *pgxpool.Pool, cfg platform.HarvestConfig) *Service {
	consumer, _ := uuid.Parse(cfg.ConsumerID)
	return &Service{pool: pool, remote: newClient(cfg), namespace: cfg.Namespace, consumer: consumer, poll: cfg.PollInterval, pageSize: cfg.PageSize}
}

func (s *Service) Close() { s.remote.http.CloseIdleConnections() }

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: !value.IsZero()}
}

func (s *Service) locked(ctx context.Context, run func(*pgx.Conn, *dbgen.Queries) error) error {
	c, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer c.Release()
	q := dbgen.New(c.Conn())
	locked, err := q.TryCollectionLock(ctx, s.namespace)
	if err != nil {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = c.Hijack().Close(closeCtx)
		return err
	}
	if !locked {
		return errBusy
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		unlocked, err := q.ReleaseCollectionLock(closeCtx, s.namespace)
		if err != nil || !unlocked {
			_ = c.Hijack().Close(closeCtx)
		}
	}()
	return run(c.Conn(), q)
}

func write(ctx context.Context, conn *pgx.Conn, run func(*dbgen.Queries) error) error {
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { return run(dbgen.New(tx)) })
}

// Run is owned by the server lifecycle. No DB session or network request is
// created when the optional feature is omitted.
func (s *Service) Run(ctx context.Context, report func(string)) {
	delay := time.Duration(0)
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		err := s.Tick(ctx)
		if err == nil || errors.Is(err, errBusy) {
			delay = s.poll
			continue
		}
		if ctx.Err() != nil {
			return
		}
		report(Diagnostic(err))
		delay = min(max(s.poll, delay*2), max(time.Minute, s.poll))
	}
}

func Diagnostic(err error) string {
	switch {
	case errors.Is(err, ErrIntent):
		return "invalid_collection_intent"
	case errors.Is(err, errBusy):
		return "collection_already_active"
	case errors.Is(err, errNetwork):
		return "harvest_connection"
	case errors.Is(err, errProtocol):
		return "harvest_protocol"
	case errors.Is(err, errSchema):
		return "harvest_schema"
	case errors.Is(err, errAmbiguous):
		return "schedule_outcome_unknown"
	case errors.Is(err, errRemoteConflict):
		return "harvest_resource_conflict"
	default:
		var status remoteStatus
		if errors.As(err, &status) {
			return status.Error()
		}
		return "collection_database_or_state"
	}
}

func (s *Service) Tick(ctx context.Context) error {
	return s.locked(ctx, func(conn *pgx.Conn, q *dbgen.Queries) error {
		var readErr, writeErr error
		if v7(s.consumer) {
			readErr = s.consume(ctx, conn, q)
		}
		if s.remote.writer != "" {
			writeErr = s.reconcile(ctx, q)
		}
		return errors.Join(readErr, writeErr)
	})
}
