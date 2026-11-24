package catalog

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"reflect"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type GameRichCandidateSummary struct {
	ID            uuid.UUID `json:"id"`
	ObservationID uuid.UUID `json:"observation_id"`
	Locale        string    `json:"locale"`
	StoreCountry  string    `json:"store_country"`
	EgressRegion  string    `json:"egress_region"`
	ObservedAt    time.Time `json:"observed_at"`
	ConfirmedAt   time.Time `json:"confirmed_at"`
}

type GameRichCandidate struct {
	Origin           GameMediaOrigin  `json:"origin"`
	Raw              SteamRichSource  `json:"raw"`
	Review           SteamRichContent `json:"review"`
	DetailedText     string           `json:"detailed_text"`
	AboutText        string           `json:"about_text"`
	EnglishTextMatch bool             `json:"english_text_match,omitzero"`
}

type GameRichSnapshot struct {
	SteamRichContent
	Origin *GameMediaOrigin `json:"origin,omitempty"`
}

type PublicGameRichDetails struct {
	SteamRichContent
	Locale   string `json:"locale"`
	Fallback bool   `json:"fallback"`
}

type GameRichEdit struct {
	Revision      int64      `json:"revision"`
	Locale        string     `json:"locale"`
	CandidateID   *uuid.UUID `json:"candidate_id,omitempty"`
	ObservationID *uuid.UUID `json:"observation_id,omitempty"`
	Detailed      *string    `json:"detailed_text,omitempty"`
	About         *string    `json:"about_text,omitempty"`
	UseDetailed   bool       `json:"use_detailed"`
	UseAbout      bool       `json:"use_about"`
	Clear         bool       `json:"clear,omitempty"`
}

func listGameRichDetails(ctx context.Context, q *dbgen.Queries, game uuid.UUID) ([]GameRichCandidateSummary, error) {
	rows, err := q.ListGameRichDetails(ctx, game)
	if err != nil {
		return nil, err
	}
	out := make([]GameRichCandidateSummary, 0, len(rows))
	for _, r := range rows {
		if r.ObservationID != nil {
			out = append(out, GameRichCandidateSummary{r.ID, *r.ObservationID, r.Locale, r.StoreCountry, r.EgressRegion, r.ObservedAt.Time.UTC(), r.ConfirmedAt.Time.UTC()})
		}
	}
	return out, nil
}

func (s *Service) ManagementGameRichDetails(ctx context.Context, publicID string, candidate uuid.UUID) (GameRichCandidate, error) {
	if !ValidPublicID(publicID) {
		return GameRichCandidate{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (GameRichCandidate, error) {
		game, err := q.ManagementGame(ctx, publicID)
		if err != nil {
			return GameRichCandidate{}, err
		}
		out, err := readRichCandidate(ctx, q, game.ID, candidate)
		if err != nil || out.Origin.Locale != "zh-cn" {
			return out, err
		}
		english, err := q.CompareGameRichEnglish(ctx, candidate)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		var source SteamRichSource
		if err == nil && json.Unmarshal(english, &source, json.RejectUnknownMembers(true)) == nil && source.AppID == out.Raw.AppID {
			out.EnglishTextMatch = sameRichSourceText(out.Raw.SteamRichContent, source.SteamRichContent)
		}
		return out, nil
	})
}

func sameRichSourceText(a, b SteamRichContent) bool {
	if a.Detailed == nil || b.Detailed == nil || strings.TrimSpace(*a.Detailed) == "" || *a.Detailed != *b.Detailed {
		return false
	}
	return reflect.DeepEqual(a.About, b.About) && reflect.DeepEqual(a.Short, b.Short)
}

func readRichCandidate(ctx context.Context, q *dbgen.Queries, game, candidate uuid.UUID) (GameRichCandidate, error) {
	row, err := q.GetGameRichDetails(ctx, dbgen.GetGameRichDetailsParams{GameID: game, CandidateID: candidate})
	if err != nil {
		return GameRichCandidate{}, err
	}
	origin, err := mediaOrigin(row)
	var raw SteamRichSource
	if err != nil || json.Unmarshal(row.Payload, &raw, json.RejectUnknownMembers(true)) != nil {
		return GameRichCandidate{}, ErrUnavailable
	}
	review := reviewRichContent(raw.SteamRichContent)
	out := GameRichCandidate{Origin: origin, Raw: raw, Review: review}
	if review.Detailed != nil {
		out.DetailedText = richTextSummary(*review.Detailed)
	}
	if review.About != nil {
		out.AboutText = richTextSummary(*review.About)
	}
	return out, nil
}

func (s *Service) SaveManagementGameRichDetails(ctx context.Context, id string, in GameRichEdit) (ManagementResult, error) {
	if in.Locale != "en" && in.Locale != "zh" || (in.CandidateID == nil) != (in.ObservationID == nil) ||
		in.Clear && (in.CandidateID != nil || in.Detailed != nil || in.About != nil || in.UseDetailed || in.UseAbout) ||
		in.Detailed != nil && (!in.UseDetailed || !safeEditorialText(*in.Detailed)) ||
		in.About != nil && (!in.UseAbout || !safeEditorialText(*in.About)) {
		return ManagementResult{}, ErrInvalid
	}
	row, err := write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Game, error) {
		g, err := q.LockGamePublicID(ctx, id)
		if err != nil {
			return g, err
		}
		if err := editable(in.Revision, g.Revision, g.Status); err != nil {
			return g, err
		}
		locales, err := gamePresentation(ctx, q, g.ID, false)
		if err != nil {
			return g, err
		}
		for _, locale := range locales {
			if locale.Locale != in.Locale {
				continue
			}
			next, err := applyRichEdit(ctx, q, g.ID, locale.RichDetails, in)
			if err != nil {
				return g, err
			}
			if reflect.DeepEqual(next, locale.RichDetails) {
				return g, nil
			}
			if next == nil {
				err = q.ClearGameDraftRichDetails(ctx, dbgen.ClearGameDraftRichDetailsParams{GameID: g.ID, Locale: in.Locale})
			} else {
				data, encodeErr := json.Marshal(next, json.Deterministic(true))
				if encodeErr != nil || len(data) > 480<<10 {
					return g, ErrInvalid
				}
				err = q.SaveGameDraftRichDetails(ctx, dbgen.SaveGameDraftRichDetailsParams{GameID: g.ID, Locale: in.Locale, Details: data})
			}
			if err != nil {
				return g, err
			}
			return q.UpdateGameDraft(ctx, dbgen.UpdateGameDraftParams{GameID: g.ID, EditSlug: gameEditSlug(g)})
		}
		return g, ErrInvalid
	})
	return ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
}

func applyRichEdit(ctx context.Context, q *dbgen.Queries, game uuid.UUID, prior *GameRichSnapshot, in GameRichEdit) (*GameRichSnapshot, error) {
	if in.Clear {
		return nil, nil
	}
	var next GameRichSnapshot
	if prior != nil {
		next = *prior
	}
	if in.CandidateID != nil {
		candidate, err := readRichCandidate(ctx, q, game, *in.CandidateID)
		if err != nil {
			return nil, err
		}
		if candidate.Origin.ObservationID != *in.ObservationID {
			return nil, ErrConflict
		}
		next = GameRichSnapshot{SteamRichContent: candidate.Review, Origin: &candidate.Origin}
	}
	if !in.UseDetailed {
		next.Detailed = nil
	} else if in.Detailed != nil {
		next.Detailed = richEditorialHTML(*in.Detailed)
	}
	if !in.UseAbout {
		next.About = nil
	} else if in.About != nil {
		next.About = richEditorialHTML(*in.About)
	}
	if !richHasContent(next.SteamRichContent) {
		return nil, nil
	}
	return &next, nil
}

func publishedGameRichDetails(rows []PublishedGameMedia, locale string) *PublicGameRichDetails {
	for _, pass := range []bool{false, true} {
		for _, row := range rows {
			if row.RichDetails == nil || (row.Locale != locale) != pass {
				continue
			}
			out := *row.RichDetails
			out.Locale, out.Fallback = row.Locale, pass
			// Source diagnostics remain available in Management.
			out.Invalid, out.Truncated = nil, nil
			return &out
		}
	}
	return nil
}

func richSnapshotJSON(details *GameRichSnapshot) (jsontext.Value, error) {
	if details == nil {
		return nil, nil
	}
	return json.Marshal(details, json.Deterministic(true))
}
