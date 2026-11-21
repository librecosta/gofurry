package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type SteamPrefill struct {
	Enabled      bool           `json:"enabled"`
	ExistingGame string         `json:"existing_game,omitempty"`
	Scopes       []PrefillScope `json:"scopes"`
}

type PrefillScope struct {
	Locale           string             `json:"locale"`
	Region           string             `json:"region"`
	State            string             `json:"state"`
	Code             string             `json:"code,omitempty"`
	Name             string             `json:"name,omitempty"`
	Summary          string             `json:"summary,omitempty"`
	Developers       []string           `json:"developers,omitempty"`
	Publishers       []string           `json:"publishers,omitempty"`
	Media            catalog.SteamMedia `json:"media"`
	Observation      *uuid.UUID         `json:"observation_id,omitempty"`
	FactsObservation *uuid.UUID         `json:"facts_observation_id,omitempty"`
	MediaObservation *uuid.UUID         `json:"media_observation_id,omitempty"`
}

func editorialLocale(locale string) string {
	if locale == "zh-cn" {
		return "zh"
	}
	return locale
}

func (s *Service) prefillEnabled() bool {
	return s != nil && s.remote != nil && s.remote.writer != "" && s.remote.reader != ""
}

func (s *Service) AdmitSteamPrefill(ctx context.Context, q *dbgen.Queries, app string) error {
	if !s.prefillEnabled() || !steamID(app) {
		return ErrIntakeInput
	}
	existing, err := q.ExistingSteamGame(ctx, app)
	if err == nil {
		return SteamGameExists(existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err := q.LockSteamPrefillAdmission(ctx, s.namespace); err != nil {
		return err
	}
	rows, err := s.prefillRows(ctx, q, app)
	if err != nil || len(rows) == 2 {
		return err
	}
	recent, err := q.CountRecentSteamPrefillStarts(ctx, s.namespace)
	if err != nil {
		return err
	}
	if recent >= 40 {
		return ErrIntakeCapacity
	}
	for _, locale := range []string{"en", "zh-cn"} {
		if err := q.AdmitSteamPrefill(ctx, dbgen.AdmitSteamPrefillParams{Namespace: s.namespace, AppID: app, Locale: locale, Generation: uuid.NewV7()}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) prefillRows(ctx context.Context, q *dbgen.Queries, app string) ([]dbgen.SteamEditorialPrefill, error) {
	return q.ListSteamEditorialPrefills(ctx, dbgen.ListSteamEditorialPrefillsParams{Namespace: s.namespace, AppID: app})
}

// KickSteamPrefill runs only after the local admission receipt has committed.
// Each generation uses one Harvest idempotency key, including a lost reply.
func (s *Service) KickSteamPrefill(ctx context.Context, q *dbgen.Queries, app, region string, retry bool) error {
	rows, err := s.prefillRows(ctx, q, app)
	if err != nil {
		return err
	}
	var work sync.WaitGroup
	for _, row := range rows {
		work.Go(func() { s.kickPrefillScope(ctx, q, row, region, retry) })
	}
	work.Wait()
	return ctx.Err()
}

const prefillResendWindow = 14 * time.Minute

func (s *Service) kickPrefillScope(ctx context.Context, q *dbgen.Queries, row dbgen.SteamEditorialPrefill, region string, retry bool) {
	if row.JobID == nil && row.TextObservationID != nil && row.MediaObservationID != nil {
		return
	}
	// After the submission window, the old Harvest Job can no longer be active.
	// An explicit retry starts a new generation and checks retained results again.
	renew := retry && row.JobID == nil && time.Since(row.StartedAt.Time) > prefillResendWindow
	if retry && row.JobID != nil {
		view := s.readPrefillScope(ctx, q, row, region)
		renew = view.State == "failed" || view.State == "partial" || view.State == "cancelled"
	}
	if renew {
		current, err := q.ReplaceSteamPrefill(ctx, dbgen.ReplaceSteamPrefillParams{
			Namespace: row.Namespace, AppID: row.AppID, Locale: row.Locale,
			ExpectedGeneration: row.Generation, NewGeneration: uuid.NewV7(),
		})
		if err != nil {
			return // Another confirmation renewed this scope, it is cooling down, or storage was unavailable.
		}
		row = current
	}
	if row.JobID == nil && time.Since(row.StartedAt.Time) <= prefillResendWindow {
		// Keep the durable request available for an explicit idempotent resend.
		if err := s.submitPrefillScope(ctx, q, row, region); err != nil {
			code := "submission_unconfirmed"
			var status remoteStatus
			if errors.As(err, &status) {
				code = "harvest_unavailable"
				if status == http.StatusTooManyRequests {
					code = "rate_limited"
				}
			}
			save, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer cancel()
			_ = q.SaveSteamPrefillSubmissionCode(save, dbgen.SaveSteamPrefillSubmissionCodeParams{
				Namespace: row.Namespace, AppID: row.AppID, Locale: row.Locale, Generation: row.Generation, SubmissionCode: code,
			})
		}
	}
}
func (s *Service) submitPrefillScope(ctx context.Context, q *dbgen.Queries, row dbgen.SteamEditorialPrefill, region string) error {
	target := targetKey{s.namespace, "steam", "app", row.AppID}
	var resolved targetView
	if err := s.remote.call(ctx, s.remote.writer, http.MethodPost, "targets", nil, target, &resolved, http.StatusCreated); err != nil {
		return err
	}
	if resolved.targetKey != target || !v7(resolved.ID) || resolved.State != "active" {
		return errRemoteConflict
	}
	req := s.prefillRequest(row, region, resolved.ID)
	// A verified prior result can populate the form without another Steam call.
	// Look up only the two exact scopes of this target; prices are not prefilled.
	if row.TextObservationID != nil && row.MediaObservationID != nil {
		return nil
	}
	saved := s.cachedPrefill(row)
	for _, section := range []string{"details", "media"} {
		var page struct{ Items []observation }
		key := prefillScopeKey(row.Namespace, req.Scope, section)
		filter := url.Values{"stream": {row.Namespace}, "target": {resolved.ID.String()}, "capability": {intakeCapability},
			"scope": {key}, "limit": {"1"}, "since": {time.Now().Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)}}
		if err := s.remote.call(ctx, s.remote.reader, http.MethodGet, "observations/history", filter, nil, &page, 200); err == nil && len(page.Items) == 1 {
			observed := page.Items[0]
			if observed.TargetID == resolved.ID && observed.Capability == intakeCapability && observed.Schema == 1 &&
				observed.Scope.Namespace == row.Namespace && observed.Scope.scope == req.Scope && observed.Scope.Dimensions["section"] == section &&
				len(observed.Scope.Dimensions) == 1 && observed.ScopeKey == key && scopeMatches(observed.Scope, key) &&
				observed.Source == "steam" && !observed.Sample && v7(observed.JobID) && v7(observed.AttemptID) && validObservation(result{observation: observed}) {
				applyPrefillObservation(&saved, row.AppID, observed)
			}
		}
	}
	if saved.Observation != nil || saved.MediaObservation != nil || saved.FactsObservation != nil {
		if err := persistSteamPrefill(ctx, q, row, saved); err != nil {
			return err
		}
	}
	if saved.Observation != nil && saved.MediaObservation != nil {
		return nil
	}
	in := struct {
		Request request `json:"request"`
	}{req}
	var job intakeJob
	if err := s.remote.callWithKey(ctx, s.remote.writer, http.MethodPost, "jobs", nil, in, &job, http.StatusAccepted, "gofurry-prefill-"+row.Generation.String()); err != nil {
		return err
	}
	if !sameIntakeJob(job, req, s.namespace) {
		return errRemoteConflict
	}
	return q.SaveSteamPrefillJob(ctx, dbgen.SaveSteamPrefillJobParams{
		Namespace: row.Namespace, AppID: row.AppID, Locale: row.Locale, Generation: row.Generation, JobID: &job.ID, TargetID: &resolved.ID,
	})
}

func (s *Service) prefillRequest(row dbgen.SteamEditorialPrefill, region string, target uuid.UUID) request {
	plans := intakePlans(s.editorialEgress(region))
	plan := plans[0]
	if row.Locale == "zh-cn" {
		plan = plans[1]
	}
	params, _, _, _ := plan.params("app", row.AppID)
	return request{target, intakeCapability, 1, params, scope{Locale: plan.Locale, Region: plan.Region, EgressRegion: plan.EgressRegion}}
}

func (s *Service) SteamPrefill(ctx context.Context, q *dbgen.Queries, app, region string) (SteamPrefill, error) {
	out := SteamPrefill{Enabled: s.prefillEnabled(), Scopes: []PrefillScope{}}
	existing, err := q.ExistingSteamGame(ctx, app)
	if err == nil {
		out.ExistingGame = existing
		return out, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || !out.Enabled {
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		}
		return out, err
	}
	rows, err := s.prefillRows(ctx, q, app)
	if err != nil {
		return out, err
	}
	remote, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out.Scopes = make([]PrefillScope, len(rows))
	var work sync.WaitGroup
	for i, row := range rows {
		work.Go(func() { out.Scopes[i] = s.readPrefillScope(remote, q, row, region) })
	}
	work.Wait()
	return out, nil
}

type prefillFacts struct {
	Developers, Publishers []string
	Observation            *uuid.UUID
}

func (*Service) cachedPrefill(row dbgen.SteamEditorialPrefill) PrefillScope {
	out := PrefillScope{Locale: row.Locale, Region: "US", State: "pending", Name: row.Name, Summary: row.Summary,
		Observation: row.TextObservationID, MediaObservation: row.MediaObservationID}
	var facts prefillFacts
	_ = json.Unmarshal(row.Facts, &facts)
	out.Developers, out.Publishers, out.FactsObservation = facts.Developers, facts.Publishers, facts.Observation
	_ = json.Unmarshal(row.Media, &out.Media)
	if row.Locale == "zh-cn" {
		out.Region = "CN"
	}
	return out
}

func (s *Service) readPrefillScope(ctx context.Context, q *dbgen.Queries, row dbgen.SteamEditorialPrefill, region string) PrefillScope {
	out := s.cachedPrefill(row)
	if row.JobID == nil || row.TargetID == nil {
		if out.Observation != nil && out.MediaObservation != nil {
			out.State = "succeeded"
		} else if time.Since(row.StartedAt.Time) > prefillResendWindow {
			out.State, out.Code = "unknown", "submission_unconfirmed"
		} else if row.SubmissionCode != "" {
			out.State, out.Code = "unavailable", row.SubmissionCode
		}
		return out
	}
	req := s.prefillRequest(row, region, *row.TargetID)
	var detail intakeJobDetail
	if err := s.remote.call(ctx, s.remote.reader, http.MethodGet, "jobs/"+row.JobID.String(), nil, nil, &detail, 200); err != nil {
		out.State, out.Code = "unavailable", Diagnostic(err)
		return out
	}
	if detail.Job.ID != *row.JobID || !sameIntakeJob(detail.Job, req, row.Namespace) || len(detail.Results) > 3 {
		out.State, out.Code = "unavailable", "unverified_job"
		return out
	}
	out.State, out.Code = detail.Job.State, safeJobCode(detail.Job.Code)
	if out.State != "succeeded" && out.State != "partial" {
		return out
	}
	attempt, err := acceptedIntakeAttempt(detail)
	if err != nil {
		out.State, out.Code = "unavailable", "unverified_attempt"
		return out
	}
	for _, observed := range detail.Results {
		section := observed.Scope.Dimensions["section"]
		if section != "details" && section != "media" || observed.State != "present" {
			continue
		}
		expected := segmentScope{Namespace: row.Namespace, scope: req.Scope, Dimensions: map[string]string{"section": section}}
		if acceptedObservation(observed, detail.Job, attempt, expected, prefillScopeKey(row.Namespace, req.Scope, section)) {
			applyPrefillObservation(&out, row.AppID, observed)
		}
	}
	if out.Observation == nil || out.MediaObservation == nil || out.Name == "" {
		out.State = "partial"
		if out.Observation == nil && out.MediaObservation == nil {
			out.State = "failed"
		}
	}
	if out.Observation != nil || out.MediaObservation != nil || out.FactsObservation != nil {
		_ = persistSteamPrefill(ctx, q, row, out)
	}
	return out
}

func prefillScopeKey(namespace string, semantic scope, section string) string {
	value := segmentScope{Namespace: namespace, scope: semantic, Dimensions: map[string]string{"section": section}}
	raw, _ := json.Marshal(value, json.Deterministic(true))
	key := sha256.Sum256(raw)
	return hex.EncodeToString(key[:])
}

func applyPrefillObservation(out *PrefillScope, app string, observed observation) {
	if observed.State != "present" {
		return
	}
	normalized, err := normalizeSteam(observed, app)
	if err != nil {
		return
	}
	switch observed.Scope.Dimensions["section"] {
	case "media":
		var media steamMedia
		if json.Unmarshal(normalized.Data, &media) == nil {
			out.Media, out.MediaObservation = media.SteamMedia, &observed.ID
		}
	case "details":
		var text steamDetail
		if json.Unmarshal(normalized.Data, &text) == nil && text.Type == "game" {
			out.Developers, out.Publishers, out.FactsObservation = text.Developers, text.Publishers, &observed.ID
			if out.Locale != "zh-cn" || text.Localized {
				out.Name, out.Summary, out.Observation = clean(text.Name, 160), text.Description, &observed.ID
			}
		}
	}
}

func persistSteamPrefill(ctx context.Context, q *dbgen.Queries, row dbgen.SteamEditorialPrefill, out PrefillScope) error {
	factsJSON, _ := json.Marshal(prefillFacts{out.Developers, out.Publishers, out.FactsObservation})
	mediaJSON, _ := json.Marshal(out.Media)
	return q.SaveSteamPrefillFacts(ctx, dbgen.SaveSteamPrefillFactsParams{
		Namespace: row.Namespace, AppID: row.AppID, Locale: row.Locale, Generation: row.Generation,
		TextObservationID: out.Observation, Name: out.Name, Summary: out.Summary, Facts: factsJSON,
		MediaObservationID: out.MediaObservation, Media: mediaJSON,
	})
}
