package catalog

import (
	"context"
	json "encoding/json/v2"
	"reflect"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type GameTextAdoption struct {
	Locale        string    `json:"locale"`
	SourceID      uuid.UUID `json:"source_id"`
	ObservationID uuid.UUID `json:"observation_id"`
	UseName       bool      `json:"use_name"`
	UseSummary    bool      `json:"use_summary"`
}

type GameMediaSelection struct {
	Locale             string     `json:"locale"`
	Media              SteamMedia `json:"media"`
	CoverURL           string     `json:"cover_url"`
	MediaSourceID      *uuid.UUID `json:"media_source_id,omitempty"`
	MediaObservationID *uuid.UUID `json:"media_observation_id,omitempty"`
}

type GameDraftMedia struct {
	GameMediaSelection
	MediaObservedAt   *time.Time `json:"media_observed_at,omitempty"`
	MediaStoreCountry string     `json:"media_store_country"`
}

type GameSourceCandidate struct {
	Localization
	SourceID      uuid.UUID  `json:"source_id"`
	ObservationID uuid.UUID  `json:"observation_id"`
	PlatformCode  string     `json:"platform_code"`
	ExternalKey   string     `json:"external_key"`
	Section       string     `json:"section"`
	StoreCountry  string     `json:"store_country"`
	ObservedAt    time.Time  `json:"observed_at"`
	Media         SteamMedia `json:"media"`
}

func readGameSourceCandidates(ctx context.Context, q *dbgen.Queries, id uuid.UUID) ([]GameSourceCandidate, error) {
	raw, err := q.ManagementGameSourceCandidates(ctx, id)
	if err != nil {
		return nil, err
	}
	candidates := []GameSourceCandidate{}
	if err := json.Unmarshal(raw, &candidates); err != nil {
		return nil, err
	}
	for i := range candidates {
		candidates[i].Media = reviewedSteamMedia(candidates[i].Media)
	}
	candidates = slices.DeleteFunc(candidates, func(c GameSourceCandidate) bool {
		if c.Section == "details" {
			return !validText(c.Name, 1, 160, false) || !validText(c.Summary, 0, 2000, true)
		}
		return c.Media.Header == "" && c.Media.Capsule == "" && len(c.Media.Screenshots)+len(c.Media.Movies) == 0
	})
	return candidates, nil
}

func readGameDraftMedia(ctx context.Context, q *dbgen.Queries, id uuid.UUID) ([]GameDraftMedia, error) {
	rows, err := q.ReadGameDraftMedia(ctx, id)
	if err != nil {
		return nil, err
	}
	selections := make([]GameDraftMedia, 0, len(rows))
	for _, row := range rows {
		next := GameDraftMedia{GameMediaSelection: GameMediaSelection{
			Locale: row.Locale, CoverURL: row.CoverUrl, MediaSourceID: row.MediaSourceID, MediaObservationID: row.MediaObservationID,
		}, MediaStoreCountry: row.MediaStoreCountry}
		if err := json.Unmarshal(row.Media, &next.Media); err != nil {
			return nil, err
		}
		if row.MediaObservedAt.Valid {
			at := row.MediaObservedAt.Time.UTC()
			next.MediaObservedAt = &at
		}
		selections = append(selections, next)
	}
	return selections, nil
}

func gameEditSlug(game dbgen.Game) string {
	if game.DraftSlug.Valid {
		return game.DraftSlug.String
	}
	return game.Slug
}

func saveGameDraft(ctx context.Context, q *dbgen.Queries, game dbgen.Game, in GameManagementEdit) (dbgen.Game, error) {
	if err := editable(in.Revision, game.Revision, game.Status); err != nil {
		return game, err
	}
	if len(in.Localizations) > 2 || len(in.TextAdoptions) > 2 || len(in.DraftMedia) > 2 {
		return game, ErrInvalid
	}
	var candidates []GameSourceCandidate
	var err error
	if len(in.TextAdoptions) > 0 || len(in.DraftMedia) > 0 {
		candidates, err = readGameSourceCandidates(ctx, q, game.ID)
		if err != nil {
			return game, err
		}
	}
	oldText, err := q.ManagementGameText(ctx, game.ID)
	if err != nil {
		return game, err
	}
	ed, text, err := gameDraftText(in, gameEditSlug(game), oldText, candidates)
	if err != nil {
		return game, err
	}
	var media []GameDraftMedia
	mediaChanged := false
	if in.DraftMedia != nil {
		oldMedia, err := readGameDraftMedia(ctx, q, game.ID)
		if err != nil {
			return game, err
		}
		media, err = gameMediaSelections(in.DraftMedia, oldMedia, candidates)
		if err != nil {
			return game, err
		}
		mediaChanged = !slices.EqualFunc(oldMedia, media, func(a, b GameDraftMedia) bool { return reflect.DeepEqual(a, b) })
	}
	textChanged := !slices.EqualFunc(oldText, text, func(a, b dbgen.ManagementGameTextRow) bool { return reflect.DeepEqual(a, b) })
	if !textChanged && !mediaChanged && ed.Slug == gameEditSlug(game) {
		return game, nil
	}
	if textChanged {
		if err := q.ClearGameLocalizations(ctx, game.ID); err != nil {
			return game, err
		}
		for _, l := range text {
			if err := q.SaveGameDraftText(ctx, dbgen.SaveGameDraftTextParams{GameID: game.ID, Locale: l.Locale,
				Name: l.Name, Summary: l.Summary, TextSourceID: l.TextSourceID, TextObservationID: l.TextObservationID}); err != nil {
				return game, err
			}
		}
	}
	if mediaChanged {
		if err := q.ClearGameDraftMedia(ctx, game.ID); err != nil {
			return game, err
		}
		for _, m := range media {
			raw, err := json.Marshal(m.Media)
			if err != nil {
				return game, err
			}
			if err := q.SaveGameDraftMedia(ctx, dbgen.SaveGameDraftMediaParams{GameID: game.ID, Locale: m.Locale, Media: raw,
				CoverUrl: m.CoverURL, MediaSourceID: m.MediaSourceID, MediaObservationID: m.MediaObservationID,
				MediaObservedAt: publicationTimestamp(m.MediaObservedAt), MediaStoreCountry: m.MediaStoreCountry}); err != nil {
				return game, err
			}
		}
	}
	return q.UpdateGameDraft(ctx, dbgen.UpdateGameDraftParams{GameID: game.ID, EditSlug: ed.Slug})
}

func gameDraftText(in GameManagementEdit, slug string, old []dbgen.ManagementGameTextRow, candidates []GameSourceCandidate) (Editorial, []dbgen.ManagementGameTextRow, error) {
	ed := Editorial{Slug: in.Slug, Localizations: slices.Clone(in.Localizations)}
	adopted := make(map[string]GameSourceCandidate, len(in.TextAdoptions))
	for _, choice := range in.TextAdoptions {
		if _, exists := adopted[choice.Locale]; exists || (!choice.UseName && !choice.UseSummary) {
			return Editorial{}, nil, ErrInvalid
		}
		i := slices.IndexFunc(candidates, func(c GameSourceCandidate) bool {
			return c.matches(choice.Locale, "details", choice.SourceID, choice.ObservationID)
		})
		j := slices.IndexFunc(ed.Localizations, func(l Localization) bool { return l.Locale == choice.Locale })
		if i < 0 || j < 0 {
			return Editorial{}, nil, ErrInvalid
		}
		c := candidates[i]
		if choice.UseName {
			ed.Localizations[j].Name = c.Name
		}
		if choice.UseSummary {
			ed.Localizations[j].Summary = c.Summary
		}
		adopted[choice.Locale] = c
	}
	ed, err := normalizeEditorial(ed, slug)
	if err != nil {
		return Editorial{}, nil, err
	}
	slices.SortFunc(ed.Localizations, func(a, b Localization) int { return strings.Compare(a.Locale, b.Locale) })
	next := make([]dbgen.ManagementGameTextRow, 0, len(ed.Localizations))
	for _, l := range ed.Localizations {
		row := dbgen.ManagementGameTextRow{Locale: l.Locale, Name: l.Name, Summary: l.Summary}
		if c, ok := adopted[l.Locale]; ok {
			if c.Name == l.Name && c.Summary == l.Summary {
				row.TextSourceID, row.TextObservationID = &c.SourceID, &c.ObservationID
			}
		} else {
			for _, p := range old {
				if p.Locale == l.Locale && p.Name == l.Name && p.Summary == l.Summary {
					row.TextSourceID, row.TextObservationID = p.TextSourceID, p.TextObservationID
				}
			}
		}
		next = append(next, row)
	}
	return ed, next, nil
}

func (c GameSourceCandidate) matches(locale, section string, source, observation uuid.UUID) bool {
	return c.Locale == locale && c.Section == section && c.SourceID == source && c.ObservationID == observation
}

func gameMediaSelections(input []GameMediaSelection, old []GameDraftMedia, candidates []GameSourceCandidate) ([]GameDraftMedia, error) {
	out := make([]GameDraftMedia, 0, len(input))
	seen := map[string]bool{}
	for _, choice := range input {
		if (choice.Locale != "en" && choice.Locale != "zh") || seen[choice.Locale] {
			return nil, ErrInvalid
		}
		seen[choice.Locale] = true
		if choice.Media.Header == "" && choice.Media.Capsule == "" && len(choice.Media.Screenshots) == 0 && len(choice.Media.Movies) == 0 && choice.CoverURL == "" {
			if choice.MediaSourceID != nil || choice.MediaObservationID != nil {
				return nil, ErrInvalid
			}
			continue
		}
		if len(choice.Media.Screenshots) == 0 {
			choice.Media.Screenshots = nil
		}
		if len(choice.Media.Movies) == 0 {
			choice.Media.Movies = nil
		}
		if i := slices.IndexFunc(old, func(m GameDraftMedia) bool { return reflect.DeepEqual(m.GameMediaSelection, choice) }); i >= 0 {
			out = append(out, old[i])
			continue
		}
		if choice.MediaSourceID == nil || choice.MediaObservationID == nil {
			return nil, ErrInvalid
		}
		i := slices.IndexFunc(candidates, func(c GameSourceCandidate) bool {
			return c.matches(choice.Locale, "media", *choice.MediaSourceID, *choice.MediaObservationID) && matchesGameMedia(c.Media, choice)
		})
		if i < 0 {
			return nil, ErrInvalid
		}
		c := candidates[i]
		at := c.ObservedAt.UTC()
		out = append(out, GameDraftMedia{GameMediaSelection: choice, MediaObservedAt: &at, MediaStoreCountry: c.StoreCountry})
	}
	slices.SortFunc(out, func(a, b GameDraftMedia) int { return strings.Compare(a.Locale, b.Locale) })
	return out, nil
}

func matchesGameMedia(candidate SteamMedia, choice GameMediaSelection) bool {
	media := choice.Media
	return (media.Header == "" || media.Header == candidate.Header) &&
		(media.Capsule == "" || media.Capsule == candidate.Capsule) &&
		mediaSubset(media.Screenshots, candidate.Screenshots) && mediaSubset(media.Movies, candidate.Movies) &&
		(choice.CoverURL == "" || choice.CoverURL == candidate.Header || choice.CoverURL == candidate.Capsule || slices.Contains(candidate.Screenshots, choice.CoverURL))
}

func mediaSubset(items, candidates []string) bool {
	if len(items) > len(candidates) {
		return false
	}
	for i, item := range items {
		if !slices.Contains(candidates, item) || slices.Contains(items[:i], item) {
			return false
		}
	}
	return true
}

func publicationTimestamp(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
