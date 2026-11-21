package collection

import (
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func TestScheduleHistoryRequiresExactBinding(t *testing.T) {
	since := time.Now().UTC().Truncate(time.Second)
	schedule := uuid.NewV7()
	plan := dbgen.ListEntityCollectionPlansRow{
		ScheduleID: &schedule, Namespace: "gofurry-cn", Since: pgtype.Timestamptz{Time: since, Valid: true},
	}
	req := request{
		TargetID: uuid.NewV7(), Capability: "steam.details", Schema: 1, Params: []byte("{}"),
		Scope: scope{Locale: "zh-cn", Region: "cn", EgressRegion: "cn"},
	}
	job := scheduledJob{intakeJob: intakeJob{
		ID: uuid.NewV7(), Namespace: plan.Namespace, ScheduleID: &schedule, Request: req,
		State: "failed", Updated: since.Add(time.Minute),
	}, AttemptCount: 2, CreatedAt: since}
	page := scheduleJobsPage{Items: []scheduledJob{job}, Before: &job.ID, LastFailure: &job}
	if !validScheduleJobs(page, req, plan, nil, 1) {
		t.Fatal("matching job and failure should be accepted")
	}
	if validScheduleJobs(page, req, plan, &job.ID, 1) {
		t.Fatal("the cursor must exclude the last job from the previous page")
	}
	for _, tc := range []struct {
		name string
		edit func(*scheduledJob)
	}{
		{"language", func(j *scheduledJob) { j.Request.Scope.Locale = "en" }},
		{"namespace", func(j *scheduledJob) { j.Namespace = "other" }},
		{"target", func(j *scheduledJob) { j.Request.TargetID = uuid.NewV7() }},
		{"schedule", func(j *scheduledJob) { other := uuid.NewV7(); j.ScheduleID = &other }},
		{"superseded binding", func(j *scheduledJob) { j.CreatedAt = since.Add(-time.Second) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			override := job
			tc.edit(&override)
			wrongItem := scheduleJobsPage{Items: []scheduledJob{override}}
			if validScheduleJobs(wrongItem, req, plan, nil, 1) {
				t.Fatal("accepted an unrelated job in history")
			}
			wrongFailure := scheduleJobsPage{Items: []scheduledJob{job}, LastFailure: &override}
			if validScheduleJobs(wrongFailure, req, plan, nil, 1) {
				t.Fatal("accepted an unrelated job as the last failure")
			}
		})
	}
}
