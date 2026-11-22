package catalog

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/url"
	"path"
	"reflect"
	"slices"
	"strings"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

const mediaCandidatePageSize = 12

type GameMediaPick struct {
	GameSourceID  uuid.UUID `json:"game_source_id"`
	ObservationID uuid.UUID `json:"observation_id"`
	ID            string    `json:"id"`
}

type GameMediaLocaleEdit struct {
	Locale      string          `json:"locale"`
	Cover       *GameMediaPick  `json:"cover,omitempty"`
	Images      []GameMediaPick `json:"images,omitempty"`
	Screenshots []GameMediaPick `json:"screenshots,omitempty"`
	Movies      []GameMediaPick `json:"movies,omitempty"`
}

type GameMediaEditV2 struct {
	Revision   int64                 `json:"revision"`
	Selections []GameMediaLocaleEdit `json:"selections"`
}

type GameSelectedImage struct {
	Origin GameMediaOrigin `json:"origin"`
	ID     string          `json:"id"`
	Type   string          `json:"asset_type"`
	Family string          `json:"asset_family"`
	Slot   string          `json:"source_slot"`
	URL    string          `json:"url"`
	Asset  *SteamAsset     `json:"asset,omitempty"`
}

type GameSelectedScreenshot struct {
	Origin GameMediaOrigin `json:"origin"`
	SteamScreenshot
}

type GameSelectedMovie struct {
	Origin GameMediaOrigin `json:"origin"`
	SteamMovie
}

type GameMediaSelectionV2 struct {
	Version           int                      `json:"version"`
	SelectionRevision int64                    `json:"selection_revision"`
	Cover             *GameSelectedImage       `json:"cover,omitempty"`
	Images            []GameSelectedImage      `json:"images"`
	Screenshots       []GameSelectedScreenshot `json:"screenshots"`
	Movies            []GameSelectedMovie      `json:"movies"`
}

type GameMediaLocaleV2 struct {
	Locale              string               `json:"locale"`
	PublicationRevision int64                `json:"publication_revision,omitzero"`
	Media               GameMediaSelectionV2 `json:"media"`
}

type GameMediaCandidatePage struct {
	Items []GameMediaCandidateV2 `json:"items"`
	Next  *uuid.UUID             `json:"next,omitempty"`
}

func mediaCandidate(row dbgen.GameSourceMedium) (out GameMediaCandidateV2, err error) {
	out.State = row.State
	if row.ObservationID == nil || !row.ContentHash.Valid {
		return out, ErrUnavailable
	}
	out.GameMediaOrigin = GameMediaOrigin{
		GameSourceID: row.GameSourceID, CollectionSourceID: row.CollectionSourceID, BindingID: row.BindingID, TargetID: row.TargetID, Namespace: row.Namespace,
		Capability: row.Capability, OutputSchema: row.OutputSchema, Locale: row.Locale, StoreCountry: row.StoreCountry,
		EgressRegion: row.EgressRegion, RequestedEgressRegion: row.RequestedEgressRegion, ScopeKey: row.ScopeKey,
		ObservationID: *row.ObservationID, ContentHash: row.ContentHash.String, Ruleset: row.Ruleset,
		ObservedAt: row.ObservedAt.Time.UTC(), ConfirmedAt: row.ConfirmedAt.Time.UTC(),
	}
	switch row.Capability {
	case "steam.details":
		err = json.Unmarshal(row.Payload, &out.Media, json.RejectUnknownMembers(true))
	case "steam.assets":
		err = json.Unmarshal(row.Payload, &out.Assets, json.RejectUnknownMembers(true))
	default:
		err = ErrUnavailable
	}
	return out, err
}

func readGameMediaCandidates(ctx context.Context, q *dbgen.Queries, gameID, after uuid.UUID) (GameMediaCandidatePage, error) {
	out := GameMediaCandidatePage{Items: []GameMediaCandidateV2{}}
	rows, err := q.ListGameSourceMedia(ctx, dbgen.ListGameSourceMediaParams{GameID: gameID, AfterID: after, TakeRows: mediaCandidatePageSize + 1})
	if err != nil {
		return out, err
	}
	if len(rows) > mediaCandidatePageSize {
		rows = rows[:mediaCandidatePageSize]
		out.Next = &rows[len(rows)-1].ID
	}
	for _, row := range rows {
		candidate, err := mediaCandidate(row)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, candidate)
	}
	return out, nil
}

func (s *Service) ManagementGameMediaCandidates(ctx context.Context, id string, after uuid.UUID) (GameMediaCandidatePage, error) {
	if !ValidPublicID(id) {
		return GameMediaCandidatePage{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (GameMediaCandidatePage, error) {
		g, err := q.ManagementGame(ctx, id)
		if err != nil {
			return GameMediaCandidatePage{}, err
		}
		return readGameMediaCandidates(ctx, q, g.ID, after)
	})
}

func readGameDraftMediaV2(ctx context.Context, q *dbgen.Queries, id uuid.UUID) ([]GameMediaLocaleV2, error) {
	rows, err := q.ReadGameDraftMediaV2(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]GameMediaLocaleV2, 0, len(rows))
	for _, row := range rows {
		next := GameMediaLocaleV2{Locale: row.Locale}
		if err := json.Unmarshal(row.Media, &next.Media, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		out = append(out, next)
	}
	return out, nil
}

// The same Management transaction persists the editorial receipt and revision.
func (s *Service) SaveManagementGameMediaV2(ctx context.Context, id string, in GameMediaEditV2) (ManagementResult, error) {
	if in.Selections == nil || len(in.Selections) > 2 {
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
		locales, err := q.ManagementGameText(ctx, g.ID)
		if err != nil {
			return g, err
		}
		old, err := readGameDraftMediaV2(ctx, q, g.ID)
		if err != nil {
			return g, err
		}
		next := make([]GameMediaLocaleV2, 0, len(in.Selections))
		seen := make(map[string]bool, len(locales))
		lookup := gameMediaLookup(ctx, q, g.ID)
		for _, selection := range in.Selections {
			if seen[selection.Locale] || !slices.ContainsFunc(locales, func(l dbgen.ManagementGameTextRow) bool { return l.Locale == selection.Locale }) {
				return g, ErrInvalid
			}
			seen[selection.Locale] = true
			var saved GameMediaSelectionV2
			for _, prior := range old {
				if prior.Locale == selection.Locale {
					saved = prior.Media
					break
				}
			}
			media, err := resolveGameMedia(selection, saved, lookup)
			if err != nil {
				return g, err
			}
			if media.Cover == nil && len(media.Images)+len(media.Movies)+len(media.Screenshots) == 0 {
				continue
			}
			media.SelectionRevision = saved.SelectionRevision
			if !reflect.DeepEqual(media, saved) {
				media.SelectionRevision = g.Revision + 1
			}
			next = append(next, GameMediaLocaleV2{Locale: selection.Locale, Media: media})
		}
		slices.SortFunc(next, func(a, b GameMediaLocaleV2) int { return strings.Compare(a.Locale, b.Locale) })
		if reflect.DeepEqual(next, old) {
			return g, nil
		}
		if err := q.ClearGameDraftMediaV2(ctx, g.ID); err != nil {
			return g, err
		}
		for _, item := range next {
			data, err := json.Marshal(item.Media)
			if err != nil {
				return g, err
			}
			// Reserve space for PostgreSQL's JSONB rendering overhead.
			if len(data) > 120<<10 {
				return g, ErrInvalid
			}
			if err := q.SaveGameDraftMediaV2(ctx, dbgen.SaveGameDraftMediaV2Params{GameID: g.ID, Locale: item.Locale, Media: data}); err != nil {
				return g, err
			}
		}
		return q.UpdateGameDraft(ctx, dbgen.UpdateGameDraftParams{GameID: g.ID, EditSlug: gameEditSlug(g)})
	})
	return ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
}

type mediaLookup func(GameMediaPick) (GameMediaCandidateV2, error)

func gameMediaLookup(ctx context.Context, q *dbgen.Queries, id uuid.UUID) mediaLookup {
	type source struct{ gameSource, observation uuid.UUID }
	loaded := map[source]GameMediaCandidateV2{}
	return func(p GameMediaPick) (GameMediaCandidateV2, error) {
		key := source{p.GameSourceID, p.ObservationID}
		if candidate, found := loaded[key]; found {
			return candidate, nil
		}
		row, err := q.GetGameSourceMediaForSelection(ctx, dbgen.GetGameSourceMediaForSelectionParams{
			GameID: id, GameSourceID: p.GameSourceID, ObservationID: &p.ObservationID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				err = ErrInvalid
			}
			return GameMediaCandidateV2{}, err
		}
		candidate, err := mediaCandidate(row)
		if err == nil {
			loaded[key] = candidate
		}
		return candidate, err
	}
}

func sameMediaPick(p GameMediaPick, origin GameMediaOrigin, id string) bool {
	return p.ID == id && p.GameSourceID == origin.GameSourceID && p.ObservationID == origin.ObservationID
}

func resolveGameMedia(in GameMediaLocaleEdit, old GameMediaSelectionV2, lookup mediaLookup) (GameMediaSelectionV2, error) {
	out := GameMediaSelectionV2{Version: 2, Images: []GameSelectedImage{}, Movies: []GameSelectedMovie{}, Screenshots: []GameSelectedScreenshot{}}
	if len(in.Screenshots) > 8 || len(in.Movies) > 4 || len(in.Images) > 22 {
		return out, ErrInvalid
	}
	images := in.Images
	if in.Cover != nil {
		images = append([]GameMediaPick{*in.Cover}, images...)
	}
	seenImages := map[GameMediaPick]bool{}
	roles := map[string]bool{}
	for i, p := range images {
		if seenImages[p] {
			return out, ErrInvalid
		}
		seenImages[p] = true
		image, err := selectGameImage(p, old, lookup)
		if err != nil {
			return out, err
		}
		if i == 0 && in.Cover != nil {
			switch image.Type {
			case "library_logo", "library_logo_2x", "community_icon":
				return out, ErrInvalid
			}
			out.Cover = &image
		} else {
			if roles[image.Type] {
				return out, ErrInvalid
			}
			roles[image.Type] = true
			out.Images = append(out.Images, image)
		}
	}
	var err error
	out.Screenshots, err = selectGameMediaItems(in.Screenshots, old.Screenshots, lookup,
		func(v GameSelectedScreenshot) (GameMediaOrigin, string) { return v.Origin, v.ID },
		func(c GameMediaCandidateV2, id string) (GameSelectedScreenshot, bool) {
			if c.Media != nil {
				for _, v := range c.Media.Screenshots {
					if v.ID == id {
						return GameSelectedScreenshot{Origin: c.GameMediaOrigin, SteamScreenshot: v}, true
					}
				}
			}
			return GameSelectedScreenshot{}, false
		})
	if err != nil {
		return out, err
	}
	out.Movies, err = selectGameMediaItems(in.Movies, old.Movies, lookup,
		func(v GameSelectedMovie) (GameMediaOrigin, string) { return v.Origin, v.ID },
		func(c GameMediaCandidateV2, id string) (GameSelectedMovie, bool) {
			if c.Media != nil {
				for _, v := range c.Media.Movies {
					if v.ID == id {
						return GameSelectedMovie{Origin: c.GameMediaOrigin, SteamMovie: v}, true
					}
				}
			}
			return GameSelectedMovie{}, false
		})
	return out, err
}

func selectGameMediaItems[T any](picks []GameMediaPick, saved []T, lookup mediaLookup,
	identity func(T) (GameMediaOrigin, string), choose func(GameMediaCandidateV2, string) (T, bool)) ([]T, error) {
	out := make([]T, 0, len(picks))
	seen := make(map[GameMediaPick]bool, len(picks))
	for _, p := range picks {
		key := GameMediaPick{GameSourceID: p.GameSourceID, ID: p.ID}
		if seen[key] {
			return nil, ErrInvalid
		}
		seen[key] = true
		if index := slices.IndexFunc(saved, func(v T) bool {
			origin, id := identity(v)
			return sameMediaPick(p, origin, id)
		}); index >= 0 {
			out = append(out, saved[index])
			continue
		}
		candidate, err := lookup(p)
		if err != nil {
			return nil, err
		}
		item, ok := choose(candidate, p.ID)
		if !ok {
			return nil, ErrInvalid
		}
		out = append(out, item)
	}
	return out, nil
}

func selectGameImage(p GameMediaPick, saved GameMediaSelectionV2, lookup mediaLookup) (GameSelectedImage, error) {
	if saved.Cover != nil && sameMediaPick(p, saved.Cover.Origin, saved.Cover.ID) {
		return *saved.Cover, nil
	}
	for _, item := range saved.Images {
		if sameMediaPick(p, item.Origin, item.ID) {
			return item, nil
		}
	}
	candidate, err := lookup(p)
	if err != nil {
		return GameSelectedImage{}, err
	}
	origin := candidate.GameMediaOrigin
	if candidate.Assets != nil {
		for _, a := range candidate.Assets.Assets {
			if a.ID == p.ID {
				return GameSelectedImage{Origin: origin, ID: a.ID, Type: a.Type, Family: a.Family, Slot: a.Slot, URL: a.URL, Asset: &a}, nil
			}
		}
	}
	if candidate.Media != nil && candidate.Media.Images != nil {
		v := candidate.Media.Images
		for _, image := range []struct{ slot, kind, url string }{
			{"header_url", "header", v.Header}, {"capsule_url", "capsule", v.Capsule},
			{"capsule_v5_url", "capsule_v5", v.CapsuleV5}, {"background_url", "background", v.Background},
			{"background_raw_url", "background_raw", v.BackgroundRaw},
		} {
			if image.slot == p.ID && image.url != "" {
				return GameSelectedImage{Origin: origin, ID: image.slot, Type: image.kind, Family: "store", Slot: image.slot, URL: image.url}, nil
			}
		}
	}
	return GameSelectedImage{}, ErrInvalid
}

func (m GameMediaSelectionV2) legacyMedia(source uuid.UUID) SteamMedia {
	out := SteamMedia{Screenshots: []string{}, Movies: []string{}}
	for _, image := range m.Images {
		if image.Origin.GameSourceID != source {
			continue
		}
		switch image.Type {
		case "header":
			out.Header = image.URL
		case "capsule", "main_capsule":
			if out.Capsule == "" {
				out.Capsule = image.URL
			}
		}
	}
	for _, shot := range m.Screenshots {
		if shot.Origin.GameSourceID == source {
			link := firstMediaURL(shot.Full, shot.Thumbnail)
			if !slices.Contains(out.Screenshots, link) {
				out.Screenshots = append(out.Screenshots, link)
			}
		}
	}
	for _, movie := range m.Movies {
		if movie.Origin.GameSourceID != source {
			continue
		}
		s := movie.Sources
		for _, link := range []string{s.MP4Max, s.MP4480, s.WebMMax, s.WebM480} {
			u, err := url.Parse(link)
			if err == nil && (strings.EqualFold(path.Ext(u.Path), ".mp4") || strings.EqualFold(path.Ext(u.Path), ".webm")) {
				if !slices.Contains(out.Movies, link) {
					out.Movies = append(out.Movies, link)
				}
				break
			}
		}
	}
	return out
}

func (m GameMediaSelectionV2) displayOrigin() GameMediaOrigin {
	switch {
	case len(m.Screenshots) > 0:
		return m.Screenshots[0].Origin
	case len(m.Movies) > 0:
		return m.Movies[0].Origin
	case m.Cover != nil:
		return m.Cover.Origin
	case len(m.Images) > 0:
		return m.Images[0].Origin
	default:
		return GameMediaOrigin{}
	}
}

func (m PublishedGameMedia) displayCover() string {
	if m.MediaV2 != nil && m.MediaV2.Cover != nil {
		return m.MediaV2.Cover.URL
	}
	return profileMedia(m.CoverURL, false)
}

func firstMediaURL(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
