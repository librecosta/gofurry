package catalog

import (
	"bytes"
	json "encoding/json/v2"
	"fmt"
	"slices"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type steamDetector struct {
	kind, key, source string
	codes             []string
}

var steamDetectors = []steamDetector{
	{"free", "free_game_transition", "steam.details/3", []string{"game_became_free", "game_became_paid"}},
	{"windows", "windows_support_transition", "steam.details/3", []string{"windows_support_added", "windows_support_removed"}},
	{"mac", "mac_support_transition", "steam.details/3", []string{"mac_support_added", "mac_support_removed"}},
	{"linux", "linux_support_transition", "steam.details/3", []string{"linux_support_added", "linux_support_removed"}},
	{"release", "game_release_transition", "steam.details/3", []string{"game_became_available", "game_availability_withdrawn", "game_release_plan_changed"}},
	{"price", "game_price_transition", "steam.price_daily/1", []string{"game_price_state_changed", "game_price_currency_changed", "game_price_decreased", "game_price_increased", "game_discount_started", "game_discount_ended", "game_discount_changed"}},
}

func validateSteamDetectors(rows []dbgen.SteamChangeRegistry) error {
	if len(rows) != len(steamDetectors) {
		return fmt.Errorf("unexpected Steam detector count %d", len(rows))
	}
	for _, row := range rows {
		i := slices.IndexFunc(steamDetectors, func(d steamDetector) bool { return d.key == row.Detector })
		if i < 0 || row.Version != 1 || row.Policy != "adjacent_utc_day" || steamDetectors[i].source != row.SourceContract || !slices.Equal(steamDetectors[i].codes, row.Codes) {
			return fmt.Errorf("Steam detector contract mismatch for %s/%d", row.Detector, row.Version)
		}
	}
	return nil
}

func (d steamDetector) factCodes(before, after []byte) ([]string, error) {
	if bytes.Equal(before, after) {
		return nil, nil
	}
	if d.kind != "release" {
		var a, b bool
		if err := json.Unmarshal(before, &a); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(after, &b); err != nil {
			return nil, err
		}
		if a == b {
			return nil, nil
		}
		if b {
			return []string{d.codes[0]}, nil
		}
		return []string{d.codes[1]}, nil
	}
	var a, b SteamReleaseFact
	if err := json.Unmarshal(before, &a); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(after, &b); err != nil {
		return nil, err
	}
	switch {
	case a.State == "upcoming" && b.State == "available":
		return []string{d.codes[0]}, nil
	case a.State == "available" && b.State == "upcoming":
		return []string{d.codes[1]}, nil
	case a.State == "upcoming" && b.State == "upcoming" && a.Precision != "unknown" && b.Precision != "unknown" &&
		a.Precision != "none" && b.Precision != "none" &&
		(a.Precision != b.Precision || a.WindowStart != b.WindowStart || a.WindowEnd != b.WindowEnd):
		return []string{d.codes[2]}, nil
	}
	return nil, nil
}

type SteamPriceFact struct {
	Currency string `json:"currency"`
	State    string `json:"state"`
	Original *int64 `json:"original,omitempty"`
	Final    *int64 `json:"final,omitempty"`
	Discount *int32 `json:"discount,omitempty"`
}

func priceFact(row dbgen.SteamPriceDailyPairInputsRow) SteamPriceFact {
	v := SteamPriceFact{Currency: row.Currency, State: row.State, Original: amount(row.Original), Final: amount(row.Final)}
	if row.Discount.Valid {
		v.Discount = &row.Discount.Int32
	}
	return v
}

func (d steamDetector) priceCodes(a, b SteamPriceFact) []string {
	var changes []string
	if a.State != b.State {
		changes = append(changes, d.codes[0])
	}
	if a.Final != nil && b.Final != nil && a.Currency != b.Currency {
		return append(changes, d.codes[1])
	}
	if a.Final != nil && b.Final != nil {
		if *b.Final < *a.Final {
			changes = append(changes, d.codes[2])
		}
		if *b.Final > *a.Final {
			changes = append(changes, d.codes[3])
		}
	}
	if a.Discount != nil && b.Discount != nil && *a.Discount != *b.Discount {
		switch {
		case *a.Discount == 0:
			changes = append(changes, d.codes[4])
		case *b.Discount == 0:
			changes = append(changes, d.codes[5])
		default:
			changes = append(changes, d.codes[6])
		}
	}
	return changes
}
