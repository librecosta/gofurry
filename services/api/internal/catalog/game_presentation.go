package catalog

import (
	"context"
	json "encoding/json/v2"
	"regexp"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type GamePresentation struct {
	PreferredSource *uuid.UUID
	Summary
	Presentation []PublishedGameMedia
	Sources      []GameSourceSnapshot
}

type PublishedGameMedia struct {
	Locale       string     `json:"locale"`
	SourceID     *uuid.UUID `json:"source_id"`
	Media        SteamMedia `json:"media"`
	ObservedAt   *time.Time `json:"observed_at"`
	StoreCountry string     `json:"store_country"`
	CoverURL     string     `json:"cover_url"`
}

// GameSourceSnapshot keeps scoped Steam observations on their source. Other
// platforms can have identity and a URL without having Steam observations.
type GameSourceSnapshot struct {
	Source
	ID       uuid.UUID       `json:"id"`
	Active   bool            `json:"active"`
	Metadata []SteamMetadata `json:"metadata"`
	Prices   []SteamPrice    `json:"prices"`
	News     []SteamNews     `json:"news"`
	Players  *SteamPlayers   `json:"players"`
}

type SteamObservation struct {
	ScopeKey      string    `json:"scope_key"`
	OutputSchema  int       `json:"output_schema"`
	Ruleset       string    `json:"ruleset"`
	ObservationID uuid.UUID `json:"observation_id"`
	ContentHash   string    `json:"content_hash"`
	ObservedAt    time.Time `json:"observed_at"`
	ConfirmedAt   time.Time `json:"confirmed_at"`
}

type SteamStoreScope struct {
	Locale       string `json:"locale"`
	StoreCountry string `json:"store_country"`
	EgressRegion string `json:"egress_region"`
}

type SteamMetadata struct {
	SteamStoreScope
	SteamObservation
	GenreCodes       []string      `json:"genre_codes"`
	ReleasePrecision *string       `json:"release_precision"`
	ReleaseDay       *string       `json:"release_day"`
	Section          string        `json:"section"`
	State            string        `json:"state"`
	BindingID        uuid.UUID     `json:"binding_id"`
	TargetID         *uuid.UUID    `json:"target_id"`
	Namespace        *string       `json:"namespace"`
	VerifiedChinese  bool          `json:"verified_chinese"`
	Details          *SteamDetails `json:"details,omitempty"`
	Media            *SteamMedia   `json:"media,omitempty"`
}

type SteamPrice struct {
	SteamStoreScope
	SteamObservation
	State   string        `json:"state"`
	Amounts *SteamAmounts `json:"amounts"`
}

type SteamPlayers struct {
	SteamObservation
	EgressRegion string `json:"egress_region"`
	Count        int64  `json:"players"`
}

type SteamNews struct {
	ScopeKey      string    `json:"scope_key"`
	ObservationID uuid.UUID `json:"observation_id"`
	ObservedAt    time.Time `json:"observed_at"`
	Item          struct {
		GID         string    `json:"gid"`
		Title       string    `json:"title"`
		URL         string    `json:"url"`
		Publisher   string    `json:"publisher"`
		Feed        string    `json:"feed"`
		PublishedAt time.Time `json:"published_at"`
	} `json:"value"`
}

var steamEgress = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,34}$`)

// ReadGamePresentation is the regional internal read model. The existing
// public API keeps its compact DTO and never requests this expanded payload.
func (s *Service) ReadGamePresentation(ctx context.Context, publicID, egressRegion string) (GamePresentation, error) {
	if !ValidPublicID(publicID) {
		return GamePresentation{}, ErrNotFound
	}
	if !steamEgress.MatchString(egressRegion) {
		return GamePresentation{}, ErrInvalid
	}
	if s.pool == nil {
		return GamePresentation{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return readGamePresentation(ctx, dbgen.New(s.pool), publicID, egressRegion, true)
}

func readGamePresentation(ctx context.Context, q *dbgen.Queries, publicID, egressRegion string, news bool) (GamePresentation, error) {
	row, err := q.ReadGamePresentation(ctx, dbgen.ReadGamePresentationParams{PublicID: publicID, EgressRegion: egressRegion, OmitNews: !news})
	if err != nil {
		return GamePresentation{}, classify(err)
	}
	base, err := summary(row.PublicID, row.Slug, row.Localizations)
	out := GamePresentation{Summary: base, PreferredSource: row.PreferredSource}
	if err != nil || json.Unmarshal(row.Presentation, &out.Presentation) != nil || json.Unmarshal(row.Sources, &out.Sources) != nil {
		return GamePresentation{}, ErrUnavailable
	}
	return out, nil
}
