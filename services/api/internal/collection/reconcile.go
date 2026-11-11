package collection

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func (s *Service) reconcile(ctx context.Context, q *dbgen.Queries) error {
	sources, err := q.PendingCollectionSources(ctx, s.namespace)
	if err != nil {
		return err
	}
	var joined error
	for _, source := range sources {
		if err = q.TouchCollectionSource(ctx, source.ID); err != nil {
			return err
		}
		if err = s.register(ctx, q, source); err != nil {
			joined = errors.Join(joined, err)
			if errors.Is(err, errNetwork) || ctx.Err() != nil {
				return joined
			}
		}
	}
	plans, err := q.PendingCollectionPlans(ctx, s.namespace)
	if err != nil {
		return err
	}
	var remote []scheduleView
	var listed bool
	for _, plan := range plans {
		if err = q.TouchCollectionPlan(ctx, plan.ID); err != nil {
			return err
		}
		if plan.ScheduleID == nil && (plan.Desired || plan.PostStarted) && !listed {
			remote, err = s.listSchedules(ctx)
			if err != nil {
				return err
			}
			listed = true
		}
		err = s.reconcilePlan(ctx, q, plan, remote)
		if err != nil {
			joined = errors.Join(joined, err)
			if errors.Is(err, errNetwork) || ctx.Err() != nil {
				return joined
			}
		}
	}
	return joined
}

func (s *Service) register(ctx context.Context, q *dbgen.Queries, source dbgen.CollectionSource) error {
	key := targetKey{source.Namespace, source.Provider, source.ResourceType, source.ExternalKey}
	var created targetView
	if err := s.remote.call(ctx, s.remote.writer, http.MethodPost, "targets", nil, key, &created, 201); err != nil {
		return err
	}
	if !v7(created.ID) || created.targetKey != key {
		return errRemoteConflict
	}
	if err := s.verifyTarget(ctx, created.ID, key); err != nil {
		return err
	}
	return q.SaveCollectionTarget(ctx, dbgen.SaveCollectionTargetParams{ID: source.ID, TargetID: &created.ID})
}

func (s *Service) verifyTarget(ctx context.Context, id uuid.UUID, key targetKey) error {
	var target targetView
	err := s.remote.call(ctx, s.remote.writer, http.MethodGet, "targets/"+id.String(), nil, nil, &target, 200)
	if err != nil {
		return err
	}
	if target.ID != id || target.targetKey != key || target.State != "active" {
		return errRemoteConflict
	}
	return nil
}

func (s *Service) listSchedules(ctx context.Context) ([]scheduleView, error) {
	query := url.Values{"namespace": {s.namespace}, "limit": {"100"}}
	var rows []scheduleView
	var after uuid.UUID
	for range 128 {
		var page schedulePage
		if err := s.remote.call(ctx, s.remote.writer, http.MethodGet, "schedules", query, nil, &page, 200); err != nil {
			return nil, err
		}
		if len(page.Items) > 100 {
			return nil, errProtocol
		}
		for _, row := range page.Items {
			if !v7(row.ID) || row.Namespace != s.namespace || bytes.Compare(row.ID[:], after[:]) <= 0 {
				return nil, errProtocol
			}
			after = row.ID
			rows = append(rows, row)
		}
		if page.After == nil {
			return rows, nil
		}
		if len(page.Items) == 0 || *page.After != after {
			return nil, errProtocol
		}
		query.Set("after", after.String())
	}
	return nil, errProtocol
}

func sameParams(a, b jsontext.Value) bool {
	a, b = a.Clone(), b.Clone()
	return a.Canonicalize() == nil && b.Canonicalize() == nil && bytes.Equal(a, b)
}

func samePlan(v scheduleView, plan scheduleSpec, namespace string) bool {
	a, b := v.Request, plan.Request
	x, y := v.Recurrence, plan.Recurrence
	return v.Namespace == namespace && v7(v.ID) && v.Revision > 0 && a.TargetID == b.TargetID &&
		a.Capability == b.Capability && a.Schema == b.Schema && a.Scope == b.Scope && sameParams(a.Params, b.Params) &&
		x.Interval == y.Interval && x.Policy == y.Policy && x.Cron == y.Cron && x.Anchor.Equal(y.Anchor)
}

func planSpec(target *uuid.UUID, capability string, schema int32, params, scopeJSON []byte, interval int32, anchor time.Time, policy string) (scheduleSpec, error) {
	var semantic scope
	if target == nil || json.Unmarshal(scopeJSON, &semantic) != nil {
		return scheduleSpec{}, errProtocol
	}
	return scheduleSpec{Request: request{*target, capability, schema, params, semantic},
		Recurrence: recurrence{Interval: interval, Anchor: anchor.UTC(), Policy: policy}}, nil
}

func (s *Service) reconcilePlan(ctx context.Context, q *dbgen.Queries, p dbgen.PendingCollectionPlansRow, listed []scheduleView) error {
	if p.ScheduleID == nil && !p.Desired && !p.PostStarted {
		return nil
	}
	spec, err := planSpec(p.TargetID, p.Capability, p.RequestSchema, p.Params, p.Scope, p.IntervalSeconds, p.AnchorAt.Time, p.Policy)
	if err != nil {
		return err
	}
	key := targetKey{p.Namespace, p.Provider, p.ResourceType, p.ExternalKey}
	if err := s.verifyTarget(ctx, *p.TargetID, key); err != nil {
		return err
	}
	var result scheduleView
	if p.ScheduleID != nil {
		err := s.remote.call(ctx, s.remote.writer, http.MethodGet, "schedules/"+p.ScheduleID.String(), nil, nil, &result, 200)
		if err != nil {
			return err
		}
		if result.ID != *p.ScheduleID || !samePlan(result, spec, p.Namespace) {
			return errRemoteConflict
		}
	} else {
		for _, remote := range listed {
			if !samePlan(remote, spec, p.Namespace) {
				continue
			}
			if !p.PostStarted || v7(result.ID) || remote.Enabled {
				return errRemoteConflict
			}
			result = remote
		}
		if !v7(result.ID) {
			if p.PostStarted {
				return errAmbiguous
			}
			// Persist uncertainty before any network side effect. Creation is disabled
			// until the confirmed Schedule ID is durable locally.
			n, err := q.MarkCollectionPost(ctx, p.ID)
			if err != nil {
				return err
			}
			if n != 1 {
				return errAmbiguous
			}
			err = s.remote.call(ctx, s.remote.writer, http.MethodPost, "schedules", nil, spec, &result, 201)
			if err != nil {
				var status remoteStatus
				if errors.As(err, &status) && (status == 400 || status == 401 || status == 403 || status == 404 || status == 409 || status == 413) {
					err = errors.Join(err, q.ClearCollectionPost(ctx, p.ID))
				}
				return err
			}
			if !samePlan(result, spec, p.Namespace) || result.Enabled {
				return errRemoteConflict
			}
		}
		if err := saveSchedule(ctx, q, p.ID, result); err != nil {
			return err
		}
	}
	if result.Enabled != p.Desired {
		patch := struct {
			Revision int64 `json:"revision"`
			Enabled  bool  `json:"enabled"`
		}{result.Revision, p.Desired}
		var updated scheduleView
		if err := s.remote.call(ctx, s.remote.writer, http.MethodPatch, "schedules/"+result.ID.String(), nil, patch, &updated, 200); err != nil {
			return err
		}
		if updated.ID != result.ID || updated.Revision <= result.Revision || updated.Enabled != p.Desired || !samePlan(updated, spec, p.Namespace) {
			return errRemoteConflict
		}
		result = updated
	}
	return saveSchedule(ctx, q, p.ID, result)
}

func saveSchedule(ctx context.Context, q *dbgen.Queries, id uuid.UUID, result scheduleView) error {
	return q.SaveCollectionSchedule(ctx, dbgen.SaveCollectionScheduleParams{
		ID: id, ScheduleID: &result.ID, RemoteRevision: pgtype.Int8{Int64: result.Revision, Valid: true}, RemoteEnabled: pgtype.Bool{Bool: result.Enabled, Valid: true},
	})
}

func (s *Service) Reconcile(ctx context.Context) error {
	if s.remote.writer == "" {
		return ErrIntent
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return s.locked(bounded, func(_ *pgx.Conn, q *dbgen.Queries) error { return s.reconcile(bounded, q) })
}

// RetryAbsent reopens an uncertain POST after the operator has confirmed that
// the original request can no longer commit. Reconciliation performs the POST.
func (s *Service) RetryAbsent(ctx context.Context, id uuid.UUID) error {
	if s.remote.writer == "" || !v7(id) {
		return ErrIntent
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return s.locked(ctx, func(_ *pgx.Conn, q *dbgen.Queries) error {
		p, err := q.GetUnknownCollectionPlan(ctx, dbgen.GetUnknownCollectionPlanParams{ID: id, Namespace: s.namespace})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrIntent
		}
		if err != nil {
			return err
		}
		spec, err := planSpec(p.TargetID, p.Capability, p.RequestSchema, p.Params, p.Scope, p.IntervalSeconds, p.AnchorAt.Time, p.Policy)
		if err != nil {
			return err
		}
		listed, err := s.listSchedules(ctx)
		if err != nil {
			return err
		}
		for _, row := range listed {
			if samePlan(row, spec, s.namespace) {
				return errRemoteConflict
			}
		}
		return q.ClearCollectionPost(ctx, id)
	})
}
