package collection

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type PlanExecutionPolicy struct {
	Regions     []string `json:"regions,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	MaxAttempts int32    `json:"max_attempts,omitempty"`
	Backoff     string   `json:"backoff,omitempty"`
}

type PlanConfigInput struct {
	Revision        int64                `json:"revision"`
	IntervalSeconds int32                `json:"interval_seconds"`
	ExecutionPolicy *PlanExecutionPolicy `json:"execution_policy"`
}

type EffectivePlanPolicy struct {
	Revision        int64         `json:"revision"`
	Profile         string        `json:"profile,omitempty"`
	DefaultMode     string        `json:"default_mode,omitempty"`
	EffectiveMode   string        `json:"effective_mode,omitempty"`
	Regions         []string      `json:"regions"`
	AllowedRegions  []string      `json:"allowed_regions"`
	AllowedModes    []string      `json:"allowed_modes"`
	MinimumInterval int32         `json:"minimum_interval_seconds"`
	MaxInterval     int32         `json:"maximum_interval_seconds"`
	MaxAttempts     int32         `json:"max_attempts"`
	Backoff         string        `json:"backoff"`
	PointInTime     bool          `json:"point_in_time"`
	Workers         []EgressMatch `json:"workers"`
	Reason          string        `json:"reason,omitempty"`
}

type ManagedPlanConfig struct {
	CollectionPlan
	Revision        int64                `json:"revision"`
	ExecutionPolicy PlanExecutionPolicy  `json:"execution_policy"`
	Effective       *EffectivePlanPolicy `json:"effective,omitempty"`
	CanEdit         bool                 `json:"can_edit"`
	Blocked         string               `json:"blocked,omitempty"`
}

type planRejection string

func retryBlocked(j intakeJob) bool {
	if j.Request.Capability == "steam.players" {
		return true
	}
	switch j.Code {
	case "upstream_throttled", "upstream_auth", "policy_denied":
		return true
	default:
		return false
	}
}

func (e planRejection) Error() string { return string(e) }

func validPlanRejection(code string) bool {
	switch code {
	case "schedule_interval_not_allowed", "schedule_egress_region_not_allowed", "schedule_egress_mode_not_allowed", "schedule_retry_not_allowed":
		return true
	default:
		return false
	}
}

func IsPlanRejection(err error) bool {
	var rejection planRejection
	return errors.As(err, &rejection)
}

var (
	ErrSharedPlan  = errors.New("collection_plan_shared")
	ErrPlanSync    = errors.New("collection_plan_sync_pending")
	ErrPlanChanged = errors.New("collection_plan_revision_changed")
)

func planSyncFailure(err error) string {
	var rejection planRejection
	switch {
	case errors.As(err, &rejection):
		return string(rejection)
	case errors.Is(err, errRemoteConflict), errors.Is(err, remoteStatus(409)):
		return "collection_plan_remote_conflict"
	case errors.Is(err, remoteStatus(401)), errors.Is(err, remoteStatus(403)):
		return "collection_plan_permission"
	default:
		return "collection_plan_sync_unavailable"
	}
}

func applyLocalPlanConfig(spec *scheduleSpec, interval int32, settings []byte) error {
	if interval != 0 {
		spec.Recurrence.Interval = interval
	}
	if json.Unmarshal(settings, &spec.ExecutionPolicy) != nil {
		return errProtocol
	}
	return nil
}

func planSchedule(row dbgen.ListEntityCollectionPlansRow) (scheduleSpec, error) {
	spec, err := planSpec(row.TargetID, row.Capability, row.RequestSchema, row.Params, row.Scope, row.IntervalSeconds, row.AnchorAt.Time, row.Policy)
	if err != nil {
		return spec, err
	}
	return spec, applyLocalPlanConfig(&spec, row.IntervalOverrideSeconds, row.ExecutionPolicy)
}

func (s *Service) ManagedPlanConfig(ctx context.Context, q *dbgen.Queries, kind, id string, plan uuid.UUID) (ManagedPlanConfig, error) {
	row, err := s.managedPlan(ctx, q, kind, id, plan)
	if err != nil {
		return ManagedPlanConfig{}, err
	}
	out := ManagedPlanConfig{CollectionPlan: s.planDetail(ctx, row), Revision: row.ConfigRevision}
	if json.Unmarshal(row.ExecutionPolicy, &out.ExecutionPolicy) != nil {
		return out, errProtocol
	}
	if row.ScheduleID == nil || (out.Connection != "connected" && out.Connection != "history_unavailable") {
		out.Blocked = "collection_plan_sync_unavailable"
		return out, nil
	}
	var view EffectivePlanPolicy
	if err := s.remote.call(ctx, s.remote.reader, http.MethodGet, "schedules/"+row.ScheduleID.String()+"/execution-policy", nil, nil, &view, 200); err != nil {
		out.Blocked = "collection_plan_sync_unavailable"
		return out, nil
	}
	count, err := q.CountActivePlanBindings(ctx, plan)
	if err != nil {
		return out, err
	}
	out.Effective = &view
	switch {
	case count != 1:
		out.Blocked = "collection_plan_shared"
	case row.ConfigRevision != row.SyncedConfigRevision && row.ConfigSyncError == "":
		out.Blocked = "collection_plan_sync_pending"
	case row.RemoteRevision.Valid && view.Revision != row.RemoteRevision.Int64:
		out.Blocked = "collection_plan_remote_conflict"
	default:
		out.CanEdit = row.Actionable && s.remote.writer != ""
	}
	return out, nil
}

func (s *Service) SetManagedPlanConfig(ctx context.Context, q *dbgen.Queries, kind, id string, plan uuid.UUID, input PlanConfigInput) (int64, error) {
	if input.Revision < 1 || input.IntervalSeconds < 900 || input.IntervalSeconds > 30*24*3600 || input.ExecutionPolicy == nil {
		return 0, ErrIntent
	}
	row, err := s.managedPlan(ctx, q, kind, id, plan)
	if err != nil {
		return 0, err
	}
	if s.remote.writer == "" {
		return 0, ErrCollectionPermission
	}
	if err := s.lockManagedBinding(ctx, q, kind, id, row); err != nil {
		return 0, err
	}
	revision, err := q.LockCollectionPlanConfig(ctx, plan)
	if err != nil {
		return 0, err
	}
	if revision != input.Revision || revision != row.ConfigRevision {
		return 0, ErrPlanChanged
	}
	count, err := q.CountActivePlanBindings(ctx, plan)
	if err != nil {
		return 0, err
	}
	if count != 1 {
		return 0, ErrSharedPlan
	}
	if row.ScheduleID == nil || !row.RemoteRevision.Valid || (row.ConfigRevision != row.SyncedConfigRevision && row.ConfigSyncError == "") {
		return 0, ErrPlanSync
	}
	expected, err := planSchedule(row)
	if err != nil {
		return 0, err
	}
	path := "schedules/" + row.ScheduleID.String()
	var schedule scheduleView
	if err := s.remote.call(ctx, s.remote.writer, http.MethodGet, path, nil, nil, &schedule, 200); err != nil {
		return 0, collectionControlError(err)
	}
	if schedule.ID != *row.ScheduleID || !sameScheduleIdentity(schedule, expected, s.namespace) ||
		schedule.Revision != row.RemoteRevision.Int64 || row.ConfigRevision == row.SyncedConfigRevision && !samePlan(schedule, expected, s.namespace) {
		return 0, ErrPlanChanged
	}
	preview := input
	preview.Revision = schedule.Revision
	var validated EffectivePlanPolicy
	if err := s.remote.call(ctx, s.remote.writer, http.MethodPost, path+"/execution-policy/preview", nil, preview, &validated, 200); err != nil {
		if errors.Is(err, remoteStatus(http.StatusConflict)) {
			return 0, ErrPlanChanged
		}
		return 0, collectionControlError(err)
	}
	if validated.Revision != schedule.Revision {
		return 0, ErrPlanChanged
	}
	if input.IntervalSeconds == expected.Recurrence.Interval && sameExecutionPolicy(*input.ExecutionPolicy, expected.ExecutionPolicy) {
		return revision, nil
	}
	override := input.IntervalSeconds
	if override == row.IntervalSeconds {
		override = 0
	}
	settings, err := json.Marshal(input.ExecutionPolicy)
	if err != nil || len(settings) > 512 {
		return 0, ErrIntent
	}
	return q.SetCollectionPlanConfig(ctx, dbgen.SetCollectionPlanConfigParams{
		ID: plan, ExpectedRevision: revision, IntervalOverrideSeconds: override, ExecutionPolicy: settings,
	})
}
