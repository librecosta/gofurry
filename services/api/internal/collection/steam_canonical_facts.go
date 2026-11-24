package collection

import (
	"context"
	json "encoding/json/v2"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func saveSteamCanonicalFacts(ctx context.Context, q *dbgen.Queries, generation uuid.UUID, source *uuid.UUID, scope string) error {
	params := dbgen.SteamCanonicalInputsParams{Generation: generation, SourceID: source, ScopeKey: scope}
	for {
		rows, err := q.SteamCanonicalInputs(ctx, params)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := saveSteamCanonicalInput(ctx, q, row); err != nil {
				return err
			}
			params.AfterBinding, params.AfterObservation, params.AfterEgress = row.BindingID, *row.ObservationID, row.Egress
		}
		if len(rows) < 64 {
			return nil
		}
	}
}

func saveSteamCanonicalInput(ctx context.Context, q *dbgen.Queries, row dbgen.SteamCanonicalInputsRow) error {
	var input catalog.SteamRichSource
	if err := json.Unmarshal(row.Payload, &input); err != nil {
		return err
	}
	facts, available, err := catalog.SteamFactValues(input)
	if err != nil {
		return err
	}
	if available && row.Locale == "en" && row.Market == "us" {
		if err := q.PutSteamFirstObserved(ctx, dbgen.PutSteamFirstObservedParams{BindingID: row.BindingID, ObservedAt: row.ObservedAt, ObservationID: *row.ObservationID}); err != nil {
			return err
		}
	}
	for _, fact := range facts {
		if err := q.PutSteamDailyFact(ctx, dbgen.PutSteamDailyFactParams{BindingID: row.BindingID, Locale: row.Locale, Market: row.Market,
			Egress: row.Egress, Kind: fact.Kind, Value: fact.Value, ObservationID: *row.ObservationID, ObservedAt: row.ObservedAt, ConfirmedAt: row.ConfirmedAt}); err != nil {
			return err
		}
	}
	return nil
}
