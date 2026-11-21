package collection

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

var ErrCollectionPermission = errors.New("collection operation not permitted")

type EntityRef struct {
	Kind     string `json:"kind"`
	PublicID string `json:"public_id"`
	Name     string `json:"name"`
}

type ManagedPlan struct {
	Entity EntityRef `json:"entity"`
	CollectionPlan
}

type ManagedPlans struct {
	Items []ManagedPlan `json:"items"`
	Next  *uuid.UUID    `json:"next,omitempty"`
}

func (s *Service) ManagedPlans(ctx context.Context, q *dbgen.Queries, filter dbgen.ListManagementPlanOwnersParams) (ManagedPlans, error) {
	out := ManagedPlans{Items: []ManagedPlan{}}
	if s == nil {
		return out, nil
	}
	filter.Namespace = s.namespace
	list, err := q.ListManagementPlanOwners(ctx, filter)
	if err != nil {
		return out, err
	}
	if len(list) > entityPlanPageSize {
		list = list[:entityPlanPageSize]
		out.Next = &list[len(list)-1].ID
	}
	rows := make([]dbgen.ListEntityCollectionPlansRow, 0, len(list))
	for _, owner := range list {
		row, err := s.managedPlan(ctx, q, owner.EntityKind, owner.PublicID, owner.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return out, err
		}
		rows = append(rows, row)
		out.Items = append(out.Items, ManagedPlan{Entity: EntityRef{owner.EntityKind, owner.PublicID, owner.Name}})
	}
	remote, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var work sync.WaitGroup
	for i, row := range rows {
		work.Go(func() { out.Items[i].CollectionPlan = s.planDetail(remote, row) })
	}
	work.Wait()
	return out, nil
}

func (s *Service) managedPlan(ctx context.Context, q *dbgen.Queries, kind, id string, plan uuid.UUID) (dbgen.ListEntityCollectionPlansRow, error) {
	var zero dbgen.ListEntityCollectionPlansRow
	if s == nil {
		return zero, pgx.ErrNoRows
	}
	entity, err := q.ManagementCollectionEntity(ctx, dbgen.ManagementCollectionEntityParams{Kind: kind, EntityPublicID: id})
	if err != nil {
		return zero, err
	}
	rows, err := q.ListEntityCollectionPlans(ctx, dbgen.ListEntityCollectionPlansParams{Kind: kind, EntityID: entity, OnlyPlan: &plan})
	if err == nil {
		if len(rows) == 0 || rows[0].Namespace != s.namespace {
			return zero, pgx.ErrNoRows
		}
		return rows[0], nil
	}
	return zero, err
}

type PlanControlInput struct {
	Enabled  *bool `json:"enabled"`
	Expected *bool `json:"expected"`
}

func (s *Service) SetPlanEnabled(ctx context.Context, q *dbgen.Queries, kind, id string, plan uuid.UUID, input PlanControlInput) error {
	if input.Enabled == nil || input.Expected == nil || *input.Enabled == *input.Expected {
		return ErrIntent
	}
	row, err := s.managedPlan(ctx, q, kind, id, plan)
	if err != nil {
		return err
	}
	if err := s.lockManagedBinding(ctx, q, kind, id, row); err != nil {
		return err
	}
	count, err := q.SetManagedPlanBinding(ctx, dbgen.SetManagedPlanBindingParams{
		PlanID: plan, BindingID: row.BindingID, Enabled: *input.Enabled, Expected: *input.Expected,
	})
	if err == nil && count != 1 {
		err = ErrIntakeState
	}
	return err
}

type AddPlanInput struct {
	SourceID        uuid.UUID `json:"source_id"`
	Capability      string    `json:"capability"`
	IntervalSeconds int32     `json:"interval_seconds,omitempty"`
}

func (s *Service) CreateManagedPlan(ctx context.Context, q *dbgen.Queries, kind, id, region string, in AddPlanInput) error {
	if s == nil || !v7(in.SourceID) {
		return ErrIntent
	}
	binding, err := q.ManagementEntityBinding(ctx, dbgen.ManagementEntityBindingParams{Namespace: s.namespace, SourceID: in.SourceID, Kind: kind, PublicID: id})
	if err != nil {
		return err
	}
	p := Plan{Capability: in.Capability, EgressRegion: s.editorialEgress(region), Anchor: planAnchor}
	switch in.Capability {
	case "steam.news", "web.http", "web.dns", "web.tls":
		p.Interval = 24 * time.Hour
	case "steam.players":
		p.Interval = 6 * time.Hour
	case "web.rdap":
		p.Interval = 7 * 24 * time.Hour
	default:
		return ErrIntent
	}
	if in.IntervalSeconds != 0 {
		p.Interval = time.Duration(in.IntervalSeconds) * time.Second
	}
	_, err = s.attachEditorialPlan(ctx, q, binding.ID, p)
	return err
}

// Hold the exact binding through receipt commit. Removing an address or source
// waits for a command that has already obtained this lock.
func (s *Service) lockManagedBinding(ctx context.Context, q *dbgen.Queries, kind, id string, row dbgen.ListEntityCollectionPlansRow) error {
	if !row.Actionable {
		return ErrCollectionPermission
	}
	binding, err := q.ManagementEntityBinding(ctx, dbgen.ManagementEntityBindingParams{Namespace: s.namespace, SourceID: row.SourceID, Kind: kind, PublicID: id})
	if err != nil {
		return err
	}
	if binding.ID != row.BindingID {
		return ErrIntakeState
	}
	return nil
}

func (s *Service) RunManagedPlan(ctx context.Context, q *dbgen.Queries, kind, id string, plan uuid.UUID) error {
	row, err := s.managedPlan(ctx, q, kind, id, plan)
	if err != nil {
		return err
	}
	if s.remote.writer == "" {
		return ErrCollectionPermission
	}
	if err := s.lockManagedBinding(ctx, q, kind, id, row); err != nil {
		return err
	}
	if row.ScheduleID == nil {
		return ErrIntakeState
	}
	spec, err := planSpec(row.TargetID, row.Capability, row.RequestSchema, row.Params, row.Scope, row.IntervalSeconds, row.AnchorAt.Time, row.Policy)
	if err != nil {
		return err
	}
	var schedule scheduleView
	if err := s.remote.call(ctx, s.remote.reader, http.MethodGet, "schedules/"+row.ScheduleID.String(), nil, nil, &schedule, 200); err != nil {
		return collectionControlError(err)
	}
	if schedule.ID != *row.ScheduleID || !samePlan(schedule, spec, s.namespace) {
		return errRemoteConflict
	}
	var out intakeJob
	input := struct {
		ScheduleID uuid.UUID `json:"schedule_id"`
	}{schedule.ID}
	key := "gofurry-run-" + schedule.ID.String() + "-" + time.Now().UTC().Format("20060102T1504")
	if err := s.remote.callWithKey(ctx, s.remote.writer, http.MethodPost, "jobs", nil, input, &out, 202, key); err != nil {
		return collectionControlError(err)
	}
	if out.ScheduleID == nil || *out.ScheduleID != schedule.ID || out.Namespace != s.namespace || !sameJobRequest(out.Request, spec.Request) || out.Cause != "manual" {
		return errRemoteConflict
	}
	return nil
}

func sameJobRequest(a, b request) bool {
	return a.TargetID == b.TargetID && a.Capability == b.Capability && a.Schema == b.Schema &&
		a.Scope == b.Scope && sameParams(a.Params, b.Params)
}

func collectionControlError(err error) error {
	switch {
	case errors.Is(err, remoteStatus(401)), errors.Is(err, remoteStatus(403)):
		return ErrCollectionPermission
	case errors.Is(err, remoteStatus(404)):
		return pgx.ErrNoRows
	case errors.Is(err, remoteStatus(409)):
		return ErrIntakeState
	case errors.Is(err, remoteStatus(429)):
		return ErrIntakeCapacity
	default:
		return err
	}
}

type ManagedJob struct {
	CollectionJob
	Entity     EntityRef `json:"entity"`
	PlanID     uuid.UUID `json:"plan_id"`
	Capability string    `json:"capability"`
	Provider   string    `json:"provider"`
	External   string    `json:"external_key"`
	Locale     string    `json:"locale,omitempty"`
	Region     string    `json:"region,omitempty"`
	CanCancel  bool      `json:"can_cancel"`
	CanRetry   bool      `json:"can_retry"`
}

type ManagedJobs struct {
	Connection string       `json:"connection"`
	Items      []ManagedJob `json:"items"`
	Next       *uuid.UUID   `json:"next,omitempty"`
}

type JobsFilter struct {
	Values     url.Values
	Entity     string
	Keyword    string
	EntityKind string
	Plan       *uuid.UUID
}

func (s *Service) tokenForOperator() string {
	if s.remote.operator != "" {
		return s.remote.operator
	}
	return s.remote.writer
}

func (s *Service) ManagedJobs(ctx context.Context, q *dbgen.Queries, filter JobsFilter) (ManagedJobs, error) {
	out := ManagedJobs{Connection: "disabled", Items: []ManagedJob{}}
	if s == nil || s.remote.reader == "" {
		return out, nil
	}
	filter.Values.Set("namespace", s.namespace)
	var page struct {
		Items  []scheduledJob `json:"items"`
		Before *uuid.UUID     `json:"before"`
	}
	if err := s.remote.call(ctx, s.remote.reader, http.MethodGet, "jobs/collection", filter.Values, nil, &page, 200); err != nil {
		out.Connection = "unavailable"
		return out, nil
	}
	if len(page.Items) > 20 {
		return out, errProtocol
	}
	scheduleIDs := make([]uuid.UUID, 0, len(page.Items))
	for i, j := range page.Items {
		if j.Namespace != s.namespace || j.ScheduleID == nil || !v7(j.ID) || j.CreatedAt.IsZero() ||
			i > 0 && bytes.Compare(j.ID[:], page.Items[i-1].ID[:]) >= 0 {
			return out, errProtocol
		}
		if !slices.Contains(scheduleIDs, *j.ScheduleID) {
			scheduleIDs = append(scheduleIDs, *j.ScheduleID)
		}
	}
	if page.Before != nil && (len(page.Items) != 20 || *page.Before != page.Items[len(page.Items)-1].ID) {
		return out, errProtocol
	}
	owners, err := q.ManagementJobOwners(ctx, dbgen.ManagementJobOwnersParams{Namespace: s.namespace, ScheduleIds: scheduleIDs, Entity: filter.Entity})
	if err != nil {
		return out, err
	}
	out.Connection, out.Next = "connected", page.Before
	for _, job := range page.Items {
		job.Created = job.CreatedAt
		for _, owner := range owners {
			if (filter.Plan != nil && owner.ID != *filter.Plan) || !matchesJobOwner(job.intakeJob, owner) || (filter.EntityKind != "" && owner.EntityKind != filter.EntityKind) ||
				(filter.Keyword != "" && !strings.Contains(strings.ToLower(owner.ExternalKey+" "+owner.Name), strings.ToLower(filter.Keyword))) {
				continue
			}
			out.Items = append(out.Items, s.projectManagedJob(job.intakeJob, owner))
			break
		}
	}
	return out, nil
}

func matchesJobOwner(j intakeJob, owner dbgen.ManagementJobOwnersRow) bool {
	if j.ScheduleID == nil || owner.ScheduleID == nil || *j.ScheduleID != *owner.ScheduleID || owner.TargetID == nil ||
		!owner.BoundAt.Valid || j.Created.IsZero() || j.Created.Before(owner.BoundAt.Time) ||
		(owner.EffectiveUntil.Valid && !j.Created.Before(owner.EffectiveUntil.Time)) {
		return false
	}
	var semantic scope
	return json.Unmarshal(owner.Scope, &semantic) == nil &&
		sameJobRequest(j.Request, request{*owner.TargetID, owner.Capability, owner.RequestSchema, owner.Params, semantic})
}

func (s *Service) projectManagedJob(j intakeJob, o dbgen.ManagementJobOwnersRow) ManagedJob {
	return ManagedJob{
		CollectionJob: CollectionJob{j.ID, j.AcceptedAttempt, j.AttemptCount, j.State, safeJobCode(j.Code), j.Cause, j.Created, j.Updated},
		Entity:        EntityRef{o.EntityKind, o.PublicID, o.Name}, PlanID: o.ID, Capability: o.Capability, Provider: o.Provider, External: o.ExternalKey,
		Locale: j.Request.Scope.Locale, Region: j.Request.Scope.Region,
		CanCancel: o.Actionable && s.tokenForOperator() != "" && slices.Contains([]string{"queued", "running", "retry_wait"}, j.State),
		CanRetry:  o.Actionable && s.tokenForOperator() != "" && slices.Contains([]string{"failed", "partial", "cancelled"}, j.State),
	}
}

type ManagedObservation struct {
	ID          *uuid.UUID     `json:"id,omitempty"`
	State       string         `json:"state"`
	ObservedAt  *time.Time     `json:"observed_at,omitempty"`
	Dimension   string         `json:"dimension,omitempty"`
	Evidence    string         `json:"evidence,omitempty"`
	Description string         `json:"description,omitempty"`
	Data        jsontext.Value `json:"data"`
}

type ManagedJobDetail struct {
	Job      ManagedJob           `json:"job"`
	Waiting  string               `json:"waiting_reason,omitempty"`
	Attempts []intakeAttempt      `json:"attempts"`
	Results  []ManagedObservation `json:"results"`
}

func (s *Service) readManagedJob(ctx context.Context, q *dbgen.Queries, jobID uuid.UUID, entity string) (intakeJobDetail, dbgen.ManagementJobOwnersRow, error) {
	var detail intakeJobDetail
	var empty dbgen.ManagementJobOwnersRow
	if s == nil || s.remote.reader == "" {
		return detail, empty, pgx.ErrNoRows
	}
	if err := s.remote.call(ctx, s.remote.reader, http.MethodGet, "jobs/"+jobID.String(), nil, nil, &detail, 200); err != nil {
		return detail, empty, collectionControlError(err)
	}
	j := detail.Job
	if j.ID != jobID || j.Namespace != s.namespace || j.ScheduleID == nil || len(detail.Attempts) > 8 || len(detail.Results) > 16 {
		return detail, empty, pgx.ErrNoRows
	}
	owners, err := q.ManagementJobOwners(ctx, dbgen.ManagementJobOwnersParams{Namespace: s.namespace, ScheduleIds: []uuid.UUID{*j.ScheduleID}, Entity: entity})
	if err != nil {
		return detail, empty, err
	}
	for _, owner := range owners {
		if matchesJobOwner(j, owner) {
			return detail, owner, nil
		}
	}
	return detail, empty, pgx.ErrNoRows
}

func (s *Service) ManagedJob(ctx context.Context, q *dbgen.Queries, jobID uuid.UUID, entity string) (ManagedJobDetail, error) {
	detail, owner, err := s.readManagedJob(ctx, q, jobID, entity)
	if err != nil {
		return ManagedJobDetail{}, err
	}
	out := ManagedJobDetail{Job: s.projectManagedJob(detail.Job, owner), Waiting: safeJobCode(detail.Waiting),
		Attempts: detail.Attempts, Results: []ManagedObservation{}}
	for i := range out.Attempts {
		out.Attempts[i].Code = safeJobCode(out.Attempts[i].Code)
	}
	items := detail.Results
	if owner.Provider == "web" && slices.Contains([]string{"succeeded", "partial", "failed"}, detail.Job.State) {
		items = s.webJobObservations(ctx, detail, owner)
	}
	for _, item := range items {
		req := detail.Job.Request
		if item.TargetID != req.TargetID || item.Capability != req.Capability || item.Scope.Namespace != s.namespace ||
			item.Scope.scope != req.Scope || !scopeMatches(item.Scope, item.ScopeKey) {
			continue
		}
		confirmed := item.State == "present" || item.State == "confirmed_empty"
		if (confirmed || owner.Provider != "web") && !validObservation(result{observation: item}) {
			continue
		}
		itemResult := ManagedObservation{ID: &item.ID, State: item.State, ObservedAt: &item.ObservedAt,
			Evidence: safeJobCode(item.Evidence), Data: jsontext.Value("{}")}
		if owner.Provider == "steam" {
			itemResult.Dimension = item.Scope.Dimensions["section"]
		} else {
			itemResult.Dimension = item.Scope.Dimensions["rrtype"]
			if itemResult.Dimension == "" {
				itemResult.Dimension = item.Scope.Dimensions["section"]
			}
		}
		if item.State == "present" || owner.Provider == "web" && item.State == "confirmed_empty" {
			var value compact
			if owner.Provider == "steam" {
				value, err = normalizeSteam(item, owner.ExternalKey)
			} else {
				value, err = normalizeWeb(item, owner.ExternalKey)
			}
			if err == nil {
				itemResult.Data = value.Data
				if itemResult.Dimension == "html" {
					var summary struct {
						Description string `json:"description"`
					}
					if json.Unmarshal(item.Data, &summary) == nil {
						itemResult.Description = clean(summary.Description, 600)
					}
				}
			}
		}
		out.Results = append(out.Results, itemResult)
	}
	return out, nil
}

// Web history retains new observations and diagnostics. An unchanged fact
// can be confirmed without writing another observation for this attempt.
// readManagedJob already authorized the entity and its plan.
func (s *Service) webJobObservations(ctx context.Context, detail intakeJobDetail, owner dbgen.ManagementJobOwnersRow) []observation {
	j := detail.Job
	var attempt intakeAttempt
	for _, candidate := range detail.Attempts {
		if candidate.Ended.After(attempt.Ended) {
			attempt = candidate
		}
	}
	if !v7(attempt.ID) || attempt.Claimed.IsZero() || attempt.Ended.Before(attempt.Claimed) {
		return nil
	}
	filter := url.Values{"stream": {j.Namespace}, "target": {j.Request.TargetID.String()}, "capability": {j.Request.Capability},
		"since": {attempt.Ended.UTC().Format(time.RFC3339Nano)}, "until": {attempt.Ended.UTC().Format(time.RFC3339Nano)}, "limit": {"16"}}
	var page struct {
		Items []observation `json:"items"`
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if s.remote.call(ctx, s.remote.reader, http.MethodGet, "observations/history", filter, nil, &page, 200) != nil || len(page.Items) > 16 {
		return nil
	}
	req := j.Request
	spec := capabilities[req.Capability]
	var out []observation
	for _, item := range page.Items {
		if item.JobID != j.ID || item.AttemptID != attempt.ID || item.TargetID != req.TargetID || item.Schema != 1 ||
			item.Capability != req.Capability || item.Scope.Namespace != j.Namespace || item.Scope.scope != req.Scope ||
			item.Source != owner.Provider || item.Sample || !v7(item.ID) || !item.ReceivedAt.Equal(attempt.Ended) ||
			!scopeMatches(item.Scope, item.ScopeKey) || !slices.Contains([]string{"present", "confirmed_empty", "unavailable", "invalid", "not_covered"}, item.State) {
			continue
		}
		dimension := item.Scope.Dimensions["section"]
		if !slices.Contains(spec.sections, dimension) {
			continue
		}
		if dimension == "dns" {
			dimension = item.Scope.Dimensions["rrtype"]
			if len(item.Scope.Dimensions) != 3 || !slices.Contains(dnsTypes, dimension) || item.Scope.Dimensions["resolver"] != "google-public-dns" {
				continue
			}
		} else if len(item.Scope.Dimensions) != 1 {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (s *Service) ChangeManagedJob(ctx context.Context, q *dbgen.Queries, jobID uuid.UUID, entity, action string) error {
	detail, owner, err := s.readManagedJob(ctx, q, jobID, entity)
	if err != nil {
		return err
	}
	j := detail.Job
	projected := s.projectManagedJob(j, owner)
	if !owner.Actionable || s.tokenForOperator() == "" {
		return ErrCollectionPermission
	}
	row, err := s.managedPlan(ctx, q, owner.EntityKind, owner.PublicID, owner.ID)
	if err != nil {
		return err
	}
	if err := s.lockManagedBinding(ctx, q, owner.EntityKind, owner.PublicID, row); err != nil {
		return err
	}

	if action == "cancel" && (j.State == "cancel_requested" || j.State == "cancelled") {
		return nil // The same outcome is already confirmed by Harvest.
	}
	if (action == "cancel" && !projected.CanCancel) || (action == "retry" && !projected.CanRetry) ||
		(action != "cancel" && action != "retry") {
		return ErrIntakeState
	}
	key := ""
	if action == "retry" {
		key = "gofurry-retry-" + j.ID.String()
	}
	var next intakeJob
	if err := s.remote.callWithKey(ctx, s.tokenForOperator(), http.MethodPost, "jobs/"+j.ID.String()+"/"+action, nil, nil, &next, 202, key); err != nil {
		return collectionControlError(err)
	}
	if !v7(next.ID) || next.Namespace != j.Namespace || next.ScheduleID == nil || *next.ScheduleID != *j.ScheduleID ||
		!sameJobRequest(next.Request, j.Request) ||
		(action == "retry" && (next.OriginJobID == nil || *next.OriginJobID != j.ID)) ||
		(action == "cancel" && next.ID != j.ID) {
		return errRemoteConflict
	}
	if action == "cancel" && !slices.Contains([]string{"cancel_requested", "cancelled"}, next.State) {
		return ErrIntakeState
	}
	return nil
}
