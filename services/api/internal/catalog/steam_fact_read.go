package catalog

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type SteamPlayerQuality struct {
	Days        int32      `json:"days"`
	Samples     int32      `json:"samples"`
	MissingDays int32      `json:"missing_days"`
	ObservedAt  *time.Time `json:"observed_at,omitempty"`
	Quality     string     `json:"quality"`
}

type SteamMarketPrice struct {
	SteamPriceFact
	Market     string    `json:"market"`
	Day        string    `json:"day"`
	Locale     string    `json:"locale"`
	ObservedAt time.Time `json:"observed_at"`
	Quality    string    `json:"quality"`
}

type SteamFirstAvailable struct {
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
}

type SteamFactStatus struct {
	Quality    string     `json:"quality"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

type PublicSteamFacts struct {
	Quality           map[string]SteamFactStatus `json:"quality"`
	OfficialRelease   *SteamReleaseFact          `json:"official_release,omitempty"`
	ReleaseObservedAt *time.Time                 `json:"release_observed_at,omitempty"`
	FirstAvailable    *SteamFirstAvailable       `json:"first_available,omitempty"`
	Languages         []SteamLanguageFact        `json:"languages,omitempty"`
	Free              *bool                      `json:"free,omitzero"`
	Platforms         RichPlatforms              `json:"platforms"`
	Prices            []SteamMarketPrice         `json:"prices"`
	Players           SteamPlayerQuality         `json:"players"`
	Changes           []PublicSteamChange        `json:"changes"`
}

type SteamFactEvidence struct {
	dbgen.CurrentSteamDailyFactsRow
	Value jsontext.Value `json:"value"`
}

type SteamChangeEvidence struct {
	PublicSteamChange
	ID             uuid.UUID `json:"id"`
	BindingID      uuid.UUID `json:"binding_id"`
	ObservationID  uuid.UUID `json:"observation_id"`
	Detector       string    `json:"detector"`
	Version        int32     `json:"version"`
	Locale         string    `json:"locale"`
	Egress         string    `json:"egress"`
	ObservedAt     time.Time `json:"observed_at"`
	MaterializedAt time.Time `json:"materialized_at"`
}

func SteamChangeForManagement(row dbgen.SteamGameChange) SteamChangeEvidence {
	return SteamChangeEvidence{
		PublicSteamChange: PublicSteamChange{row.Code, dayString(row.Day), row.Market, jsontext.Value(row.BeforeValue), jsontext.Value(row.AfterValue), "observed"},
		ID:                row.ID, BindingID: row.BindingID, ObservationID: row.ObservationID, Detector: row.Detector, Version: row.Version, Locale: row.Locale,
		Egress: row.Egress, ObservedAt: row.ObservedAt.Time, MaterializedAt: row.MaterializedAt.Time,
	}
}

type SteamFactsManagement struct {
	PublicSteamFacts
	TrackingID         uuid.UUID                             `json:"tracking_id"`
	AppID              string                                `json:"app_id"`
	FirstObserved      *dbgen.SteamFirstObservedAvailableRow `json:"first_observed_available,omitempty"`
	AvailabilityReview *dbgen.SteamFirstAvailableReview      `json:"availability_review,omitempty"`
	Sources            []SteamFactEvidence                   `json:"sources"`
	PriceEvidence      []dbgen.CurrentSteamMarketPricesRow   `json:"price_evidence"`
	PriceDays          []dbgen.RecentSteamPriceDailyRow      `json:"price_days"`
}

func freshness(at time.Time, tolerance time.Duration) string {
	if at.IsZero() {
		return "not_observed"
	}
	if time.Since(at) > tolerance {
		return "stale"
	}
	return "observed"
}

func (s *Service) SteamManagementFacts(ctx context.Context, id, egress string) (SteamFactsManagement, error) {
	if !ValidPublicID(id) {
		return SteamFactsManagement{}, ErrInvalid
	}
	if s.pool == nil {
		return SteamFactsManagement{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, q, err := s.channelRead(ctx)
	if err != nil {
		return SteamFactsManagement{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := q.SteamManagementIdentity(ctx, id); err != nil {
		return SteamFactsManagement{}, classify(err)
	}
	out, err := readSteamFacts(ctx, q, id, egress, "", "en", true)
	if err != nil {
		return out, classify(err)
	}
	return out, classify(tx.Commit(ctx))
}

func readSteamFacts(ctx context.Context, q *dbgen.Queries, id, egress, market, locale string, management bool) (SteamFactsManagement, error) {
	out := SteamFactsManagement{PublicSteamFacts: PublicSteamFacts{Prices: []SteamMarketPrice{}, Changes: []PublicSteamChange{},
		Players: SteamPlayerQuality{MissingDays: 30, Quality: "not_observed"}}}
	binding, err := q.CurrentSteamFactBinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.TrackingID, out.AppID = binding.ID, binding.ExternalKey
	rows, err := q.CurrentSteamDailyFacts(ctx, binding.ID)
	if err != nil {
		return out, err
	}
	out.Sources = make([]SteamFactEvidence, 0, len(rows))
	withdrawals, err := q.SteamCanonicalScopeWithdrawals(ctx, binding.ID)
	if err != nil {
		return out, err
	}
	latestWithdrawal := make(map[string]time.Time, len(withdrawals))
	for _, withdrawal := range withdrawals {
		latestWithdrawal[withdrawal.EgressRegion] = withdrawal.EventAt.Time
	}
	out.Quality = make(map[string]SteamFactStatus, 6)
	for _, kind := range []string{"release", "languages", "free", "windows", "mac", "linux"} {
		out.Quality[kind] = SteamFactStatus{Quality: "not_observed"}
	}
	for _, source := range rows {
		out.Sources = append(out.Sources, SteamFactEvidence{source, jsontext.Value(source.Value)})
		status := freshness(source.ObservedAt.Time, 7*24*time.Hour)
		if latestWithdrawal[source.Egress].After(source.ConfirmedAt.Time) {
			status = "unavailable"
		}
		out.Quality[source.Kind] = SteamFactStatus{Quality: status, ObservedAt: &source.ObservedAt.Time}
		switch source.Kind {
		case "release":
			err = json.Unmarshal(source.Value, &out.OfficialRelease)
			out.ReleaseObservedAt = &source.ObservedAt.Time
		case "languages":
			var languages SteamLanguagesFact
			err = json.Unmarshal(source.Value, &languages)
			out.Languages = languages.Items
		case "free":
			err = json.Unmarshal(source.Value, &out.Free)
		case "windows":
			err = json.Unmarshal(source.Value, &out.Platforms.Windows)
		case "mac":
			err = json.Unmarshal(source.Value, &out.Platforms.Mac)
		case "linux":
			err = json.Unmarshal(source.Value, &out.Platforms.Linux)
		}
		if err != nil {
			return out, err
		}
	}
	if review, err := q.SteamFirstAvailableReview(ctx, binding.ID); err == nil {
		out.AvailabilityReview = &review
		out.FirstAvailable = &SteamFirstAvailable{dayString(review.WindowStart), dayString(review.WindowEnd)}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	players, err := q.SteamFactsPlayerQuality(ctx, dbgen.SteamFactsPlayerQualityParams{ID: binding.ID, EgressRegion: egress})
	if err != nil {
		return out, err
	}
	out.Players = SteamPlayerQuality{Days: players.Days, Samples: players.Samples, MissingDays: 30 - players.Days, Quality: freshness(players.ObservedAt.Time, 48*time.Hour)}
	if players.ObservedAt.Valid {
		out.Players.ObservedAt = &players.ObservedAt.Time
	}
	out.PriceEvidence, err = q.CurrentSteamMarketPrices(ctx, dbgen.CurrentSteamMarketPricesParams{BindingID: binding.ID, Egress: egress, Market: market, Locale: locale})
	if err != nil {
		return out, err
	}
	for _, p := range out.PriceEvidence {
		price := priceFact(dbgen.SteamPriceDailyPairInputsRow{State: p.State, Currency: p.Currency, Original: p.Original, Final: p.Final, Discount: p.Discount})
		out.Prices = append(out.Prices, SteamMarketPrice{price, p.StoreCountry, dayString(p.UtcDay), p.Locale, p.ObservedAt.Time, freshness(p.ObservedAt.Time, 48*time.Hour)})
	}
	if management {
		if first, err := q.SteamFirstObservedAvailable(ctx, binding.ID); err == nil {
			out.FirstObserved = &first
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		out.PriceDays, err = q.RecentSteamPriceDaily(ctx, dbgen.RecentSteamPriceDailyParams{BindingID: binding.ID, Egress: egress, Market: market,
			Locale: locale, FromDay: pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, -29), Valid: true}, TakeRows: 90})
	}
	return out, err
}

func dayString(d pgtype.Date) string { return d.Time.Format(time.DateOnly) }

func parseSteamDay(value string) (pgtype.Date, error) {
	d, err := time.Parse(time.DateOnly, value)
	if err != nil || d.Format(time.DateOnly) != value {
		return pgtype.Date{}, ErrInvalid
	}
	return pgtype.Date{Time: d, Valid: true}, nil
}

type SteamFirstAvailableEdit struct {
	Revision    int64     `json:"revision"`
	TrackingID  uuid.UUID `json:"tracking_id"`
	WindowStart string    `json:"window_start"`
	WindowEnd   string    `json:"window_end"`
	EvidenceURL string    `json:"evidence_url"`
	Reason      string    `json:"reason"`
}

func RecordSteamFirstAvailable(ctx context.Context, q *dbgen.Queries, id string, in SteamFirstAvailableEdit, actor uuid.UUID) (ManagementResult, error) {
	start, e1 := parseSteamDay(in.WindowStart)
	end, e2 := parseSteamDay(in.WindowEnd)
	if e1 != nil || e2 != nil || end.Time.Before(start.Time) || end.Time.After(time.Now().UTC().Truncate(24*time.Hour)) ||
		!validText(in.Reason, 1, 512, false) || in.Revision < 0 {
		return ManagementResult{}, ErrInvalid
	}
	game, err := q.LockSteamGame(ctx, id)
	if err != nil {
		return ManagementResult{}, err
	}
	if game.Status == "archived" {
		return ManagementResult{}, ErrConflict
	}
	b, err := q.CurrentSteamFactBinding(ctx, id)
	if err != nil {
		return ManagementResult{}, err
	}
	if b.ID != in.TrackingID {
		return ManagementResult{}, ErrConflict
	}
	if officialStoryURL(in.EvidenceURL, b.ExternalKey) != in.EvidenceURL ||
		!strings.HasPrefix(in.EvidenceURL, "https://store.steampowered.com/news/app/"+b.ExternalKey+"/view/") {
		return ManagementResult{}, ErrInvalid
	}
	prior, err := q.SteamFirstAvailableReview(ctx, b.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ManagementResult{}, err
	}
	if prior.Revision != in.Revision {
		return ManagementResult{}, ErrConflict
	}
	observed, err := q.SteamFirstObservedAvailable(ctx, b.ID)
	if err != nil {
		return ManagementResult{}, err
	}
	if in.WindowEnd > observed.ObservedAt.Time.UTC().Format(time.DateOnly) {
		return ManagementResult{}, ErrInvalid
	}
	err = q.RecordSteamFirstAvailable(ctx, dbgen.RecordSteamFirstAvailableParams{BindingID: b.ID, Revision: in.Revision + 1,
		WindowStart: start, WindowEnd: end, EvidenceUrl: in.EvidenceURL, Reason: in.Reason, ActorID: actor})
	return ManagementResult{Target: id, Revision: in.Revision + 1}, err
}

type PublicSteamChange struct {
	Code    string         `json:"code"`
	Day     string         `json:"day"`
	Market  string         `json:"market"`
	Before  jsontext.Value `json:"before"`
	After   jsontext.Value `json:"after"`
	Quality string         `json:"quality"`
}

type SteamChangePage[T any] struct {
	Items []T    `json:"items"`
	After string `json:"after,omitempty"`
}

type SteamChangeFilter struct{ Market, Code, From, Through, After string }

func steamChanges(ctx context.Context, q *dbgen.Queries, id, egress string, filter SteamChangeFilter, public bool) (SteamChangePage[dbgen.SteamGameChange], error) {
	var out SteamChangePage[dbgen.SteamGameChange]
	if !ValidPublicID(id) || filter.Market != "" && storeCurrency(filter.Market) == "" ||
		filter.Code != "" && !slices.ContainsFunc(steamDetectors, func(d steamDetector) bool { return slices.Contains(d.codes, filter.Code) }) {
		return out, ErrInvalid
	}
	bounds, err := q.SteamChangeWorkBounds(ctx)
	if err != nil {
		return out, err
	}
	a := dbgen.ListSteamGameChangesParams{PublicID: id, Egress: egress, Market: filter.Market, Code: filter.Code, FromDay: bounds.FirstDay,
		ThroughDay: bounds.LastDay, PublishedOnly: public, AfterDay: pgtype.Date{Time: bounds.LastDay.Time.AddDate(0, 0, 1), Valid: true}}
	if filter.From != "" {
		a.FromDay, err = parseSteamDay(filter.From)
		if err != nil {
			return out, err
		}
	}
	if filter.Through != "" {
		a.ThroughDay, err = parseSteamDay(filter.Through)
		if err != nil {
			return out, err
		}
	}
	if a.ThroughDay.Time.Before(a.FromDay.Time) {
		return out, ErrInvalid
	}
	if filter.After != "" {
		day, value, ok := strings.Cut(filter.After, "~")
		id, err := uuid.Parse(value)
		if !ok || err != nil || id.String() != value || id[6]>>4 != 7 || id[8]>>6 != 2 {
			return out, ErrInvalid
		}
		a.AfterDay, err = parseSteamDay(day)
		if err != nil {
			return out, err
		}
		a.AfterID = id
	}
	out.Items, err = q.ListSteamGameChanges(ctx, a)
	if err != nil {
		return out, err
	}
	if len(out.Items) > 30 {
		out.Items = out.Items[:30]
		last := out.Items[29]
		out.After = dayString(last.Day) + "~" + last.ID.String()
	}
	return out, nil
}

func publicSteamChange(row dbgen.SteamGameChange) PublicSteamChange {
	// Release events expose only comparable calendar facts. Raw source text and
	// private tracking information stay in the operator's history.
	before, after := row.BeforeValue, row.AfterValue
	if row.Detector == "game_release_transition" {
		scrub := func(data []byte) []byte {
			var v SteamReleaseFact
			if json.Unmarshal(data, &v) != nil {
				return []byte("null")
			}
			v.RawText = ""
			body, _ := json.Marshal(v)
			return body
		}
		before, after = scrub(before), scrub(after)
	}
	return PublicSteamChange{row.Code, dayString(row.Day), row.Market, jsontext.Value(before), jsontext.Value(after), "observed"}
}

func (s *Service) SteamGameChangeHistory(ctx context.Context, id, egress string, filter SteamChangeFilter, public bool) (SteamChangePage[dbgen.SteamGameChange], error) {
	if s.pool == nil {
		return SteamChangePage[dbgen.SteamGameChange]{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, q, err := s.channelRead(ctx)
	if err != nil {
		return SteamChangePage[dbgen.SteamGameChange]{}, err
	}
	defer tx.Rollback(ctx)
	if public {
		_, err = q.PublicGameIdentity(ctx, id)
	} else {
		_, err = q.SteamManagementIdentity(ctx, id)
	}
	if err != nil {
		return SteamChangePage[dbgen.SteamGameChange]{}, classify(err)
	}
	out, err := steamChanges(ctx, q, id, egress, filter, public)
	if err != nil {
		return out, classify(err)
	}
	return out, classify(tx.Commit(ctx))
}

func (s *Service) PublicSteamChanges(ctx context.Context, id string, market ChannelMarket, filter SteamChangeFilter) (SteamChangePage[PublicSteamChange], error) {
	country, _, egress := market.scope()
	filter.Market = country
	page, err := s.SteamGameChangeHistory(ctx, id, egress, filter, true)
	out := SteamChangePage[PublicSteamChange]{Items: make([]PublicSteamChange, 0, len(page.Items)), After: page.After}
	for _, row := range page.Items {
		out.Items = append(out.Items, publicSteamChange(row))
	}
	return out, err
}
