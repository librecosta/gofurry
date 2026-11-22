package collection

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

const entityPlanPageSize = 8

type EntityCollection struct {
	Targets          []dbgen.ManagementCollectionTargetsRow `json:"targets"`
	TargetsTruncated bool                                   `json:"targets_truncated"`
	Plans            []CollectionPlan                       `json:"plans"`
	Next             *uuid.UUID                             `json:"next,omitempty"`
}

type CollectionPlan struct {
	ID              uuid.UUID        `json:"id"`
	SourceID        uuid.UUID        `json:"source_id"`
	GameSourceID    *uuid.UUID       `json:"game_source_id,omitempty"`
	TargetID        *uuid.UUID       `json:"target_id,omitempty"`
	BindingID       uuid.UUID        `json:"binding_id"`
	ScheduleID      *uuid.UUID       `json:"schedule_id,omitempty"`
	Provider        string           `json:"provider"`
	ExternalKey     string           `json:"external_key"`
	Capability      string           `json:"capability"`
	Locale          string           `json:"locale,omitempty"`
	Region          string           `json:"region,omitempty"`
	EgressRegion    string           `json:"egress_region,omitempty"`
	IntervalSeconds int32            `json:"interval_seconds"`
	ConfigPending   bool             `json:"config_pending,omitempty"`
	ConfigSyncError string           `json:"config_sync_error,omitempty"`
	DesiredEnabled  bool             `json:"desired_enabled"`
	Actionable      bool             `json:"actionable"`
	Connection      string           `json:"connection"`
	State           string           `json:"state"`
	Enabled         *bool            `json:"enabled,omitempty"`
	EnabledAt       *time.Time       `json:"enabled_at,omitempty"`
	NextDueAt       *time.Time       `json:"next_due_at,omitempty"`
	Latest          *CollectionJob   `json:"latest,omitempty"`
	LastFailure     *CollectionJob   `json:"last_failure,omitempty"`
	Observation     *PlanObservation `json:"observation,omitempty"`
	LastSuccessAt   *time.Time       `json:"last_success_at,omitempty"`
}

type PlanObservation struct {
	State      string     `json:"state"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
	ID         *uuid.UUID `json:"id,omitempty"`
	ScopeKey   string     `json:"scope_key,omitempty"`
}

type CollectionJob struct {
	ID           uuid.UUID  `json:"id"`
	AttemptID    *uuid.UUID `json:"attempt_id,omitempty"`
	AttemptCount int32      `json:"attempt_count"`
	State        string     `json:"state"`
	Code         string     `json:"code,omitempty"`
	Cause        string     `json:"cause,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	RetryBlocked bool       `json:"retry_blocked,omitempty"`
}

type CollectionHistory struct {
	Connection string          `json:"connection"`
	Items      []CollectionJob `json:"items"`
	Next       *uuid.UUID      `json:"next,omitempty"`
}

type scheduledJob struct {
	intakeJob
	AttemptCount int32     `json:"attempt_count"`
	CreatedAt    time.Time `json:"created_at"`
}

type scheduleJobsPage struct {
	Schedule    scheduleView   `json:"schedule"`
	Items       []scheduledJob `json:"items"`
	Before      *uuid.UUID     `json:"before"`
	LastFailure *scheduledJob  `json:"last_failure"`
}

func (s *Service) EntityCollection(ctx context.Context, q *dbgen.Queries, kind, publicID string, after *uuid.UUID) (EntityCollection, error) {
	entity, err := q.ManagementCollectionEntity(ctx, dbgen.ManagementCollectionEntityParams{Kind: kind, EntityPublicID: publicID})
	if err != nil {
		return EntityCollection{}, err
	}
	targets, err := q.ManagementCollectionTargets(ctx, dbgen.ManagementCollectionTargetsParams{Kind: kind, EntityID: entity})
	if err != nil {
		return EntityCollection{}, err
	}
	rows, err := q.ListEntityCollectionPlans(ctx, dbgen.ListEntityCollectionPlansParams{Kind: kind, EntityID: entity, AfterPlan: after})
	if err != nil {
		return EntityCollection{}, err
	}
	result := EntityCollection{Targets: targets}
	if len(targets) > 32 {
		result.Targets, result.TargetsTruncated = targets[:32], true
	}
	if len(rows) > entityPlanPageSize {
		rows = rows[:entityPlanPageSize]
		result.Next = &rows[len(rows)-1].ID
	}
	result.Plans = make([]CollectionPlan, len(rows))
	remote, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var pending sync.WaitGroup
	for i, row := range rows {
		pending.Go(func() { result.Plans[i] = s.planDetail(remote, row) })
	}
	pending.Wait()
	return result, nil
}

func (s *Service) EntityCollectionHistory(ctx context.Context, q *dbgen.Queries, kind, publicID string, plan uuid.UUID, before *uuid.UUID) (CollectionHistory, error) {
	out := CollectionHistory{Items: []CollectionJob{}}
	entity, err := q.ManagementCollectionEntity(ctx, dbgen.ManagementCollectionEntityParams{Kind: kind, EntityPublicID: publicID})
	if err != nil {
		return out, err
	}
	rows, err := q.ListEntityCollectionPlans(ctx, dbgen.ListEntityCollectionPlansParams{Kind: kind, EntityID: entity, OnlyPlan: &plan})
	if err != nil {
		return out, err
	}
	if len(rows) == 0 {
		return out, pgx.ErrNoRows
	}
	remote, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	page, connection := s.planJobs(remote, rows[0], before, 10)
	out.Connection, out.Next = connection, page.Before
	for _, job := range page.Items {
		out.Items = append(out.Items, projectJob(job))
	}
	return out, nil
}

func knownTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid || t.InfinityModifier != pgtype.Finite {
		return nil
	}
	return &t.Time
}

func (s *Service) planDetail(ctx context.Context, row dbgen.ListEntityCollectionPlansRow) CollectionPlan {
	var semantic scope
	_ = json.Unmarshal(row.Scope, &semantic)
	out := CollectionPlan{
		ID: row.ID, SourceID: row.SourceID, TargetID: row.TargetID, BindingID: row.BindingID, GameSourceID: row.GameSourceID,
		ScheduleID: row.ScheduleID, Provider: row.Provider, ExternalKey: row.ExternalKey, Capability: row.Capability,
		Locale: semantic.Locale, Region: semantic.Region, EgressRegion: semantic.EgressRegion, IntervalSeconds: row.IntervalSeconds,
		DesiredEnabled: row.DesiredEnabled, Actionable: row.Actionable,
		State: "unknown", LastSuccessAt: knownTime(row.LastSuccessAt),
		ConfigPending: row.ConfigRevision != row.SyncedConfigRevision, ConfigSyncError: row.ConfigSyncError,
	}
	if row.IntervalOverrideSeconds != 0 {
		out.IntervalSeconds = row.IntervalOverrideSeconds
	}
	if row.ObservationState != "" {
		out.Observation = &PlanObservation{row.ObservationState, knownTime(row.ObservedAt), row.ObservationID, row.ScopeKey}
	}
	if row.RemoteEnabled.Valid && knownTime(row.CheckedAt) != nil {
		out.Enabled, out.EnabledAt = &row.RemoteEnabled.Bool, knownTime(row.CheckedAt)
	}
	page, connection := s.planJobs(ctx, row, nil, 1)
	out.Connection = connection
	if connection == "connected" || connection == "history_unavailable" {
		out.Enabled, out.EnabledAt = &page.Schedule.Enabled, nil
		if page.Schedule.Enabled && page.Schedule.NextDueAt.After(time.Now()) {
			out.NextDueAt = &page.Schedule.NextDueAt
		}
		if !page.Schedule.Enabled {
			out.State = "paused"
		}
	}
	if connection == "not_scheduled" {
		out.State = "pending_sync"
	}
	if !row.DesiredEnabled || !row.Actionable {
		out.State = "paused"
	}
	if page.LastFailure != nil {
		failure := projectJob(*page.LastFailure)
		out.LastFailure = &failure
	}
	if len(page.Items) > 0 {
		latest := projectJob(page.Items[0])
		out.Latest = &latest
		if latest.State == "retry_wait" && (out.LastFailure == nil || latest.UpdatedAt.After(out.LastFailure.UpdatedAt)) {
			out.LastFailure = &latest
		}
		if out.State != "paused" {
			out.State = jobDisplayState(latest.State)
			if out.State == "recent_success" && out.Observation != nil && out.Observation.State != "present" &&
				out.Observation.ObservedAt != nil && !out.Observation.ObservedAt.Before(latest.CreatedAt) {
				out.State = "unknown"
			}
		}
	} else if connection == "connected" && out.State != "paused" {
		switch {
		case out.Observation == nil:
			out.State = "waiting"
		case out.Observation.State == "present":
			out.State = "recent_success"
		}
	}
	if out.ConfigPending && row.Actionable {
		out.State = "pending_sync"
	}
	return out
}

func (s *Service) planJobs(ctx context.Context, row dbgen.ListEntityCollectionPlansRow, before *uuid.UUID, limit int) (scheduleJobsPage, string) {
	var page scheduleJobsPage
	if row.ScheduleID == nil {
		return page, "not_scheduled"
	}
	if s == nil || s.remote.reader == "" {
		return page, "disabled"
	}
	if row.Namespace != s.namespace {
		return page, "unavailable"
	}
	spec, err := planSchedule(row)
	if err != nil || !v7(*row.ScheduleID) || !row.Since.Valid {
		return page, "unavailable"
	}
	validSchedule := func(v scheduleView) bool {
		return v.ID == *row.ScheduleID && sameScheduleIdentity(v, spec, row.Namespace)
	}
	path := "schedules/" + row.ScheduleID.String()
	query := url.Values{"limit": {strconv.Itoa(limit)}, "since": {row.Since.Time.UTC().Format(time.RFC3339Nano)}}
	if before != nil {
		query.Set("before", before.String())
	}
	err = s.remote.call(ctx, s.remote.reader, http.MethodGet, path+"/jobs", query, nil, &page, 200)
	if err != nil {
		// A deployed Harvest can expose schedules before it supports scoped history.
		if errors.Is(err, remoteStatus(http.StatusNotFound)) {
			var schedule scheduleView
			if s.remote.call(ctx, s.remote.reader, http.MethodGet, path, nil, nil, &schedule, 200) == nil && validSchedule(schedule) {
				return scheduleJobsPage{Schedule: schedule}, "history_unavailable"
			}
		}
		return scheduleJobsPage{}, "unavailable"
	}
	if !validSchedule(page.Schedule) || !validScheduleJobs(page, spec.Request, row, before, limit) {
		return scheduleJobsPage{}, "unavailable"
	}
	return page, "connected"
}

func validScheduleJobs(page scheduleJobsPage, req request, row dbgen.ListEntityCollectionPlansRow, before *uuid.UUID, limit int) bool {
	matches := func(job scheduledJob) bool {
		got := job.Request
		return v7(job.ID) && job.ScheduleID != nil && *job.ScheduleID == *row.ScheduleID && job.Namespace == row.Namespace &&
			got.TargetID == req.TargetID && got.Capability == req.Capability && got.Schema == req.Schema &&
			got.Scope == req.Scope && sameParams(got.Params, req.Params) && !job.CreatedAt.Before(row.Since.Time) &&
			!job.Updated.IsZero() && job.AttemptCount >= 0
	}
	if len(page.Items) > limit || (page.LastFailure != nil && (!matches(*page.LastFailure) ||
		(page.LastFailure.State != "failed" && page.LastFailure.State != "partial"))) {
		return false
	}
	for _, job := range page.Items {
		if !matches(job) || before != nil && bytes.Compare(job.ID[:], (*before)[:]) >= 0 {
			return false
		}
		before = &job.ID
	}
	return page.Before == nil || len(page.Items) == limit && before != nil && *page.Before == *before
}

var boundedJobCode = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

func projectJob(j scheduledJob) CollectionJob {
	return CollectionJob{j.ID, j.AcceptedAttempt, j.AttemptCount, j.State, safeJobCode(j.Code), j.Cause, j.CreatedAt, j.Updated, retryBlocked(j.intakeJob)}
}

func safeJobCode(code string) string {
	if !boundedJobCode.MatchString(code) {
		return ""
	}
	return code
}

func jobDisplayState(state string) string {
	switch state {
	case "running", "cancel_requested":
		return "collecting"
	case "succeeded":
		return "recent_success"
	case "partial", "failed", "retry_wait":
		return "recent_failure"
	case "queued":
		return "waiting"
	default:
		return "unknown"
	}
}
