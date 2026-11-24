package catalog

import (
	"context"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type SteamChangeRebuild struct {
	TrackingID uuid.UUID `json:"tracking_id"`
	From       string    `json:"from"`
	Through    string    `json:"through"`
	DryRun     bool      `json:"dry_run"`
}

func RebuildSteamChanges(ctx context.Context, q *dbgen.Queries, id string, in SteamChangeRebuild) (ManagementResult, error) {
	from, e1 := parseSteamDay(in.From)
	through, e2 := parseSteamDay(in.Through)
	if e1 != nil || e2 != nil || in.TrackingID[6]>>4 != 7 || in.TrackingID[8]>>6 != 2 ||
		through.Time.Before(from.Time) || through.Time.Sub(from.Time) > 6*24*time.Hour {
		return ManagementResult{}, ErrInvalid
	}
	bounds, err := q.SteamChangeWorkBounds(ctx)
	if err != nil {
		return ManagementResult{}, err
	}
	if from.Time.Before(bounds.FirstDay.Time) || through.Time.After(bounds.LastDay.Time) {
		return ManagementResult{}, ErrInvalid
	}
	days := make([]pgtype.Date, 0, 7)
	for day := from; !day.Time.After(through.Time); day.Time = day.Time.AddDate(0, 0, 1) {
		allowed, err := q.CanRebuildSteamBinding(ctx, dbgen.CanRebuildSteamBindingParams{PublicID: id, BindingID: in.TrackingID, Day: day})
		if err != nil {
			return ManagementResult{}, err
		}
		if !allowed {
			return ManagementResult{}, ErrInvalid
		}
		days = append(days, day)
	}
	// Lock the full interval before taking any detector checkpoint locks.
	if !in.DryRun {
		for _, day := range days {
			if err := q.LockSteamRebuildDay(ctx, dbgen.LockSteamRebuildDayParams{BindingID: in.TrackingID, Day: day}); err != nil {
				return ManagementResult{}, err
			}
		}
	}
	n := 0
	for _, day := range days {
		count, err := projectSteamChanges(ctx, q, in.TrackingID, day, in.DryRun)
		if err != nil {
			return ManagementResult{}, err
		}
		n += count
	}
	return ManagementResult{Target: id, Revision: 1, Affected: &n}, nil
}

type SteamDetectorStatus struct {
	Key              string   `json:"detector"`
	Version          int32    `json:"version"`
	SourceContract   string   `json:"source_contract"`
	Policy           string   `json:"policy"`
	Codes            []string `json:"codes"`
	Enabled          bool     `json:"enabled"`
	SourceStart      string   `json:"source_start,omitempty"`
	ProcessedThrough string   `json:"processed_through,omitempty"`
	UpstreamThrough  string   `json:"upstream_through"`
	LagDays          int      `json:"lag_days"`
	Status           string   `json:"status"`
}

type SteamChangeStatus struct {
	Detectors  []SteamDetectorStatus `json:"detectors"`
	Pending    int64                 `json:"pending"`
	Failed     int64                 `json:"failed"`
	RetryAfter *time.Time            `json:"retry_after,omitempty"`
}

func (s *Service) SteamChangeStatus(ctx context.Context) (SteamChangeStatus, error) {
	var out SteamChangeStatus
	if s.pool == nil {
		return out, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, q, err := s.channelRead(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	rows, err := q.SteamCheckpointStatus(ctx)
	if err != nil {
		return out, err
	}
	pending, err := q.SteamPendingStatus(ctx)
	if err != nil {
		return out, err
	}
	out.Pending, out.Failed = pending.Pending, pending.Failed
	if pending.RetryAfter.Valid {
		out.RetryAfter = &pending.RetryAfter.Time
	}
	bounds, err := q.SteamChangeWorkBounds(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		status := SteamDetectorStatus{Key: r.Detector, Version: r.Version, SourceContract: r.SourceContract, Policy: r.Policy, Codes: r.Codes,
			Enabled: r.Enabled, UpstreamThrough: dayString(bounds.LastDay), Status: "waiting_for_source"}
		if r.SourceStart.Valid {
			status.SourceStart = dayString(r.SourceStart)
			status.Status = "idle"
		}
		if r.ProcessedThrough.Valid {
			status.ProcessedThrough = dayString(r.ProcessedThrough)
		}
		if r.OldestPending != "" {
			day, err := parseSteamDay(r.OldestPending)
			if err != nil {
				return out, err
			}
			status.Status = "pending"
			status.LagDays = int(bounds.LastDay.Time.Sub(day.Time)/(24*time.Hour)) + 1
		}
		if pending.Failed > 0 {
			status.Status = "retrying"
		}
		if !r.Enabled {
			status.Status = "disabled"
		}
		out.Detectors = append(out.Detectors, status)
	}
	return out, classify(tx.Commit(ctx))
}
