package collection

import (
	"context"
	"net/http"
	"net/url"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type ControlSummary struct {
	Server        string     `json:"server"`
	ReadyWorkers  *int64     `json:"ready_workers,omitempty"`
	Running       *int64     `json:"running,omitempty"`
	Waiting       *int64     `json:"waiting,omitempty"`
	RecentFailure *int64     `json:"recent_failures,omitempty"`
	Unsynced      int64      `json:"unsynced"`
	LastSuccess   *time.Time `json:"last_success_at,omitempty"`
	CanManageJobs bool       `json:"can_manage_jobs"`
}

func (s *Service) ControlSummary(ctx context.Context, q *dbgen.Queries) (ControlSummary, error) {
	out := ControlSummary{Server: "disabled"}
	if s == nil {
		return out, nil
	}
	var err error
	out.Unsynced, err = q.CountUnsynchronizedCollectionPlans(ctx, s.namespace)
	if err != nil {
		return out, err
	}
	last, err := q.LatestCollectionConfirmation(ctx, s.namespace)
	if err != nil {
		return out, err
	}
	out.LastSuccess = knownTime(last)
	remote, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	reader := s.remote.reader
	if reader == "" {
		reader = s.remote.writer
	}
	statusPath := "jobs/collection/status"
	filter := url.Values{"namespace": {s.namespace}}
	type status struct {
		Draining       bool
		ReadyWorkers   *int64 `json:"ready_workers"`
		Running        *int64 `json:"running"`
		Waiting        *int64 `json:"waiting"`
		RecentFailures *int64 `json:"recent_failures"`
		CanManageJobs  bool   `json:"can_manage_jobs"`
	}
	var health status
	out.Server = "unavailable"
	if reader == "" || s.remote.call(remote, reader, http.MethodGet, statusPath, filter, nil, &health, 200) != nil {
		return out, nil
	}
	out.Server = "connected"
	if health.Draining {
		out.Server = "draining"
	}
	out.ReadyWorkers, out.Waiting, out.Running, out.RecentFailure = health.ReadyWorkers, health.Waiting, health.Running, health.RecentFailures
	out.CanManageJobs = health.CanManageJobs
	if token := s.tokenForOperator(); !out.CanManageJobs && token != "" && token != reader && !health.Draining {
		var authority status
		if s.remote.call(remote, token, http.MethodGet, statusPath, filter, nil, &authority, 200) == nil {
			out.CanManageJobs = authority.CanManageJobs
		}
	}
	return out, nil
}

func (s *Service) PlanResults(ctx context.Context, q *dbgen.Queries, kind, id string, plan uuid.UUID) ([]ManagedObservation, error) {
	out := []ManagedObservation{}
	row, err := s.managedPlan(ctx, q, kind, id, plan)
	if err != nil {
		return out, err
	}
	items, err := q.ManagementPlanResults(ctx, dbgen.ManagementPlanResultsParams{PlanID: plan, BoundAt: row.Since})
	if err != nil {
		return out, err
	}
	for _, item := range items {
		out = append(out, ManagedObservation{item.ObservationID, item.State, knownTime(item.ObservedAt), item.Dimension, item.Data})
	}
	return out, nil
}
