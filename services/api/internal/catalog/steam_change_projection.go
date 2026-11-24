package catalog

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func (s *Service) ValidateSteamDetectors(ctx context.Context) error {
	rows, err := dbgen.New(s.pool).SteamDetectorRegistry(ctx)
	if err != nil {
		return err
	}
	return validateSteamDetectors(rows)
}

func (s *Service) RunSteamChanges(ctx context.Context, report func(error)) {
	timer := time.NewTicker(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			tick, cancel := context.WithTimeout(ctx, 12*time.Second)
			if err := s.reconcileSteamChanges(tick); err != nil && ctx.Err() == nil {
				report(err)
			}
			cancel()
		}
	}
}

func (s *Service) reconcileSteamChanges(ctx context.Context) error {
	var failed error
	for range 32 {
		var item dbgen.SteamChangeDay
		err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			q := dbgen.New(tx)
			var err error
			item, err = q.NextSteamChangeDay(ctx)
			if err != nil {
				return err
			}
			_, err = projectSteamChanges(ctx, q, item.BindingID, item.Day, false)
			return err
		})
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			if item.BindingID == uuid.Nil() {
				return err
			}
			failed = errors.Join(failed, err)
			if err := dbgen.New(s.pool).DelaySteamChangeDay(ctx, dbgen.DelaySteamChangeDayParams{BindingID: item.BindingID, Day: item.Day}); err != nil {
				return errors.Join(failed, err)
			}
		}
	}
	return errors.Join(failed, dbgen.New(s.pool).PruneSteamFactHistory(ctx))
}

func steamChangeInputs(ctx context.Context, q *dbgen.Queries, binding uuid.UUID, day pgtype.Date, enabled map[string]bool) ([]dbgen.PutSteamChangeParams, error) {
	events := []dbgen.PutSteamChangeParams{}
	appendEvents := func(e dbgen.PutSteamChangeParams, codes []string) {
		for _, code := range codes {
			e.Code = code
			events = append(events, e)
		}
	}
	facts, err := q.SteamDailyPairs(ctx, dbgen.SteamDailyPairsParams{BindingID: binding, Day: day})
	if err != nil {
		return nil, err
	}
	price, err := q.SteamPriceDailyPairInputs(ctx, dbgen.SteamPriceDailyPairInputsParams{BindingID: binding, Day: day})
	if err != nil {
		return nil, err
	}
	for _, d := range steamDetectors {
		if !enabled[d.key] {
			continue
		}
		if d.kind != "price" {
			for _, f := range facts {
				if f.Kind != d.kind {
					continue
				}
				codes, err := d.factCodes(f.BeforeValue, f.AfterValue)
				if err != nil {
					return nil, err
				}
				appendEvents(dbgen.PutSteamChangeParams{BindingID: binding, Day: day, Detector: d.key, Version: 1, Market: f.Market, Locale: f.Locale,
					Egress: f.Egress, BeforeValue: f.BeforeValue, AfterValue: f.AfterValue, ObservationID: f.ObservationID, ObservedAt: f.ObservedAt}, codes)
			}
			continue
		}
		type scope struct{ market, locale, egress string }
		prior := make(map[scope]SteamPriceFact)
		for _, p := range price {
			key := scope{p.StoreCountry, p.Locale, p.EgressRegion}
			b := priceFact(p)
			if p.UtcDay.Time.Before(day.Time) {
				prior[key] = b
				continue
			}
			a, ok := prior[key]
			if !ok {
				continue
			}
			before, err := json.Marshal(a)
			if err != nil {
				return nil, err
			}
			after, err := json.Marshal(b)
			if err != nil {
				return nil, err
			}
			appendEvents(dbgen.PutSteamChangeParams{BindingID: binding, Day: day, Detector: d.key, Version: 1, Market: key.market, Locale: key.locale,
				Egress: key.egress, BeforeValue: before, AfterValue: after, ObservationID: p.ObservationID, ObservedAt: p.ObservedAt}, d.priceCodes(a, b))
		}
	}
	return events, nil
}

func projectSteamChanges(ctx context.Context, q *dbgen.Queries, binding uuid.UUID, day pgtype.Date, dryRun bool) (int, error) {
	rows, err := q.SteamDetectorRegistry(ctx)
	if err != nil {
		return 0, err
	}
	if err := validateSteamDetectors(rows); err != nil {
		return 0, err
	}
	enabled := make(map[string]bool)
	var keys []string
	for _, row := range rows {
		if row.Enabled {
			enabled[row.Detector] = true
			keys = append(keys, row.Detector)
		}
	}
	events, err := steamChangeInputs(ctx, q, binding, day, enabled)
	if err != nil || dryRun {
		return len(events), err
	}
	kept := make([]uuid.UUID, 0, len(events))
	for _, e := range events {
		id, err := q.PutSteamChange(ctx, e)
		if err != nil {
			return 0, err
		}
		kept = append(kept, id)
	}
	if err := q.RemoveSupersededSteamChanges(ctx, dbgen.RemoveSupersededSteamChangesParams{BindingID: binding, Day: day, Detectors: keys, Kept: kept}); err != nil {
		return 0, err
	}
	for _, key := range keys {
		if err := q.AdvanceSteamCheckpoint(ctx, dbgen.AdvanceSteamCheckpointParams{Detector: key, Version: 1, SourceStart: day}); err != nil {
			return 0, err
		}
	}
	return len(events), q.ClearSteamChangeDay(ctx, dbgen.ClearSteamChangeDayParams{BindingID: binding, Day: day})
}
