package collection

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

const resetPages = 512

func (s *Service) consumerPath(action string) string {
	return "consumers/" + s.consumer.String() + "/" + action
}
func (s *Service) streamQuery() url.Values {
	return url.Values{"stream": {s.namespace}, "limit": {s.pageLimit()}}
}
func (s *Service) consumerQuery() url.Values {
	q := s.streamQuery()
	q.Set("consumer", s.consumer.String())
	return q
}

func (s *Service) consume(ctx context.Context, conn *pgx.Conn, q *dbgen.Queries) error {
	state, err := q.GetCollectionConsumer(ctx, s.namespace)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = q.InitCollectionConsumer(ctx, dbgen.InitCollectionConsumerParams{Namespace: s.namespace, ConsumerID: s.consumer}); err != nil {
			return err
		}
		state, err = q.GetCollectionConsumer(ctx, s.namespace)
	}
	if err != nil {
		return err
	}
	if state.ConsumerID != s.consumer {
		return errRemoteConflict
	}
	if state.Generation == nil {
		return s.reset(ctx, conn, q, state)
	}
	if state.AckPending {
		err = s.ack(ctx, q, state)
		if errors.Is(err, remoteStatus(http.StatusGone)) {
			return s.reset(ctx, conn, q, state)
		}
		if err != nil {
			return err
		}
	}
	for range 4 {
		page, err := s.changes(ctx, state.Cursor, "", nil, *state.Epoch)
		if errors.Is(err, remoteStatus(http.StatusGone)) {
			return s.reset(ctx, conn, q, state)
		}
		if err != nil {
			return err
		}
		if page.Next != state.Cursor {
			err = write(ctx, conn, func(tx *dbgen.Queries) error {
				if err := s.applyPage(ctx, tx, *state.Epoch, *state.Generation, page.Items, false); err != nil {
					return err
				}
				return tx.SaveCollectionCursor(ctx, dbgen.SaveCollectionCursorParams{Namespace: s.namespace, Cursor: page.Next})
			})
			if err != nil {
				return err
			}
			state.Cursor, state.ResetAck = page.Next, nil
			if err = s.ack(ctx, q, state); err != nil {
				return err
			}
		}
		if !page.More {
			break
		}
	}
	if !state.SweepAt.Time.After(time.Now()) {
		if err = s.sweep(ctx, conn, state); err != nil {
			return err
		}
		if err = q.DeleteRetiredCollectionCurrent(ctx, s.namespace); err != nil {
			return err
		}
	}
	// Limit cleanup even when a deployment switches generations.
	if _, err = q.DeleteOldCollectionRows(ctx, dbgen.DeleteOldCollectionRowsParams{Namespace: s.namespace, Generation: state.Generation}); err != nil {
		return err
	}
	return q.PruneGameEvidence(ctx)
}

func (s *Service) changes(ctx context.Context, after, through string, reset *uuid.UUID, epoch uuid.UUID) (changesPage, error) {
	query := s.consumerQuery()
	query.Set("after", after)
	if through != "" {
		query.Set("through", through)
	}
	if reset != nil {
		query.Set("reset", reset.String())
	}
	var out changesPage
	err := s.remote.call(ctx, s.remote.reader, http.MethodGet, "changes", query, nil, &out, 200)
	if err != nil {
		return out, err
	}
	if out.Epoch != epoch || !tokenOK(out.Head) || !tokenOK(out.Next) || len(out.Items) > s.pageSize ||
		(out.Next == after && (len(out.Items) > 0 || out.More)) ||
		(through != "" && !out.More && out.Next != through) {
		return out, errProtocol
	}
	return out, nil
}

func (s *Service) ack(ctx context.Context, q *dbgen.Queries, state dbgen.CollectionConsumer) error {
	var out ackView
	input := ackInput{Stream: s.namespace, Cursor: state.Cursor, Reset: state.ResetAck}
	if err := s.remote.call(ctx, s.remote.reader, http.MethodPost, s.consumerPath("ack"), nil, input, &out, 200); err != nil {
		return err
	}
	if out.ID != s.consumer || out.Stream != s.namespace || out.Epoch != *state.Epoch || out.ACK != state.Cursor || out.ResetRequired {
		return errProtocol
	}
	return q.MarkCollectionAck(ctx, dbgen.MarkCollectionAckParams{Namespace: s.namespace, Cursor: state.Cursor})
}

func (s *Service) reset(ctx context.Context, conn *pgx.Conn, q *dbgen.Queries, state dbgen.CollectionConsumer) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	if state.Epoch != nil {
		if err := q.AddCollectionGap(ctx, dbgen.AddCollectionGapParams{
			ID: uuid.NewV7(), Namespace: s.namespace, PriorEpoch: *state.Epoch, PriorCursor: state.Cursor, PriorGeneration: state.Generation,
		}); err != nil {
			return err
		}
	}
	for i := 0; i < 64; i++ {
		n, err := q.DeleteOldCollectionRows(ctx, dbgen.DeleteOldCollectionRowsParams{Namespace: s.namespace, Generation: state.Generation})
		if err != nil {
			return err
		}
		if n < 2048 {
			break
		}
		if i == 63 {
			return errProtocol
		}
	}
	var reset resetView
	input := struct {
		Stream string `json:"stream"`
	}{s.namespace}
	if err := s.remote.call(ctx, s.remote.reader, http.MethodPost, s.consumerPath("reset"), nil, input, &reset, 200); err != nil {
		return err
	}
	if !v7(reset.ID) || !v7(reset.Epoch) || !tokenOK(reset.Baseline) || !reset.Deadline.After(time.Now()) {
		return errProtocol
	}
	generation := uuid.NewV7()
	head, err := s.stageSnapshot(ctx, conn, reset, generation)
	if err != nil {
		return err
	}
	cursor := reset.Baseline
	for i := 0; cursor != head; i++ {
		if i == resetPages {
			return errProtocol
		}
		page, err := s.changes(ctx, cursor, head, &reset.ID, reset.Epoch)
		if err != nil {
			return err
		}
		if page.Next == cursor {
			return errProtocol
		}
		err = write(ctx, conn, func(tx *dbgen.Queries) error {
			return s.applyPage(ctx, tx, reset.Epoch, generation, page.Items, true)
		})
		if err != nil {
			return err
		}
		cursor = page.Next
	}
	err = write(ctx, conn, func(tx *dbgen.Queries) error {
		if err := tx.ActivateSiteBaselines(ctx, dbgen.ActivateSiteBaselinesParams{Generation: generation, Epoch: reset.Epoch}); err != nil {
			return err
		}
		for _, apply := range []func(context.Context, uuid.UUID) error{tx.ActivateSiteCoverage, tx.ActivateGameNews, tx.BoundActivatedGameNews, tx.ActivateGamePlayers} {
			if err := apply(ctx, generation); err != nil {
				return err
			}
		}
		if err := refreshIntakeProfiles(ctx, tx, generation, nil, ""); err != nil {
			return err
		}
		return tx.ActivateCollectionGeneration(ctx, dbgen.ActivateCollectionGenerationParams{
			Namespace: s.namespace, Generation: &generation, Epoch: &reset.Epoch, Cursor: cursor, ResetAck: &reset.ID,
		})
	})
	if err != nil {
		return err
	}
	state.Generation, state.Epoch, state.Cursor, state.ResetAck = &generation, &reset.Epoch, cursor, &reset.ID
	return s.ack(ctx, q, state)
}

func (s *Service) stageSnapshot(ctx context.Context, conn *pgx.Conn, reset resetView, generation uuid.UUID) (string, error) {
	query := s.consumerQuery()
	query.Set("reset", reset.ID.String())
	for range resetPages {
		var page currentPage
		if err := s.remote.call(ctx, s.remote.reader, http.MethodGet, "snapshots/current", query, nil, &page, 200); err != nil {
			return "", err
		}
		if page.Epoch != reset.Epoch || !tokenOK(page.Head) || len(page.Items) > s.pageSize ||
			(page.After != "" && (!tokenOK(page.After) || page.After == query.Get("after") || len(page.Items) == 0)) {
			return "", errProtocol
		}
		err := write(ctx, conn, func(tx *dbgen.Queries) error {
			for _, item := range page.Items {
				if item.Confirmed.IsZero() || item.Confirmed.Before(item.ReceivedAt) {
					return errProtocol
				}
				r := result{observation: item.observation, Position: item.Position, Confirmed: item.Confirmed}
				if err := s.project(ctx, tx, generation, reset.Epoch, r, true); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		if page.After == "" {
			return page.Head, nil
		}
		query.Set("after", page.After)
	}
	return "", errProtocol
}

func (s *Service) sweep(ctx context.Context, conn *pgx.Conn, state dbgen.CollectionConsumer) error {
	query := s.streamQuery()
	if state.SweepAfter != "" {
		query.Set("after", state.SweepAfter)
	}
	var page currentPage
	err := s.remote.call(ctx, s.remote.reader, http.MethodGet, "observations/current", query, nil, &page, 200)
	if err != nil && !errors.Is(err, remoteStatus(http.StatusGone)) {
		return err
	}
	if page.After != "" && (!tokenOK(page.After) || page.After == state.SweepAfter || len(page.Items) == 0) || len(page.Items) > s.pageSize {
		return errProtocol
	}
	next := time.Now()
	if err != nil || page.Epoch != *state.Epoch {
		page = currentPage{}
	} else if page.After == "" {
		next = next.Add(6 * time.Hour)
	}
	return write(ctx, conn, func(tx *dbgen.Queries) error {
		if err := s.sweepPage(ctx, tx, *state.Generation, *state.Epoch, page.Items); err != nil {
			return err
		}
		return tx.SaveCollectionSweep(ctx, dbgen.SaveCollectionSweepParams{Namespace: s.namespace, SweepAfter: page.After, SweepAt: timestamp(next)})
	})
}
