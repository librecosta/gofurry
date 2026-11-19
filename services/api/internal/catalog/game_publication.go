package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

var (
	ErrGamePublicationIdentity = errors.New("authenticated admin operator required")
	ErrGamePublicationText     = errors.New("game needs a public name")
	ErrGamePublicationReview   = errors.New("confirm Game relevance and content rating")
	ErrGamePublicationPolicy   = errors.New("only SFW games can be published")
	ErrGamePreviewChanged      = errors.New("game publication preview changed")
)

type GamePublicationLocale struct {
	Localization
	TextSourceID      *uuid.UUID `json:"text_source_id,omitempty"`
	TextObservationID *uuid.UUID `json:"text_observation_id,omitempty"`
	TextSource        *Source    `json:"text_source,omitempty"`
	MediaSourceID     *uuid.UUID `json:"media_source_id,omitempty"`
	MediaSource       *Source    `json:"media_source,omitempty"`
	MediaObservedAt   *time.Time `json:"media_observed_at,omitempty"`
	MediaStoreCountry string     `json:"media_store_country,omitempty"`
	Media             SteamMedia `json:"media"`
	CoverURL          string     `json:"cover_url,omitempty"`
}

type GamePublicationReview struct {
	Revision         int64  `json:"revision"`
	ConfirmRelevance bool   `json:"confirm_relevance"`
	ContentRating    string `json:"content_rating"`
	PreviewToken     string `json:"preview_token"`
}

func gamePublicationPreview(ctx context.Context, q *dbgen.Queries, id uuid.UUID, published bool) ([]GamePublicationLocale, string, error) {
	var raw []byte
	var err error
	if published {
		raw, err = q.ReadManagementPublishedGame(ctx, id)
	} else {
		raw, err = q.PreviewGamePublication(ctx, id)
	}
	if err != nil {
		return nil, "", err
	}
	var locales []GamePublicationLocale
	if err = json.Unmarshal(raw, &locales); err != nil {
		return nil, "", err
	}
	for i := range locales {
		locale := &locales[i]
		m := &locale.Media
		*m = reviewedSteamMedia(*m)
		locale.CoverURL = m.Header
		if locale.CoverURL == "" {
			locale.CoverURL = m.Capsule
		}
	}
	hash := sha256.Sum256(raw)
	return locales, hex.EncodeToString(hash[:]), nil
}

func reviewedSteamMedia(media SteamMedia) SteamMedia {
	return SteamMedia{Header: profileMedia(media.Header, false), Capsule: profileMedia(media.Capsule, false),
		Screenshots: profileMediaList(media.Screenshots, false, 8), Movies: profileMediaList(media.Movies, true, 4)}
}

func validGameRating(rating string) bool {
	switch rating {
	case "unreviewed", "sfw", "mixed", "adult":
		return true
	default:
		return false
	}
}

// PublishManagementGame may only run inside the authenticated Management operation
// that records the operator, decision and snapshot in the same transaction.
func (s *Service) PublishManagementGame(ctx context.Context, id string, in GamePublicationReview) (ManagementResult, error) {
	if !in.ConfirmRelevance || !validGameRating(in.ContentRating) || in.ContentRating == "unreviewed" {
		return ManagementResult{}, ErrGamePublicationReview
	}
	if in.ContentRating != "sfw" {
		return ManagementResult{}, ErrGamePublicationPolicy
	}
	return s.changeManagementGame(ctx, id, in.Revision, "publish", "Confirmed GoFurry relevance, SFW rating and public presentation",
		func(ctx context.Context, q *dbgen.Queries, g dbgen.Game) (dbgen.Game, error) {
			if g.Status != Draft {
				return g, ErrConflict
			}
			locales, token, err := gamePublicationPreview(ctx, q, g.ID, false)
			if err != nil {
				return g, err
			}
			if len(locales) == 0 {
				return g, ErrGamePublicationText
			}
			if token != in.PreviewToken {
				return g, ErrGamePreviewChanged
			}
			next, err := q.PublishReviewedGame(ctx, g.ID)
			if err != nil {
				return g, err
			}
			if err = q.ClearGamePublicationSnapshot(ctx, g.ID); err != nil {
				return g, err
			}
			for _, locale := range locales {
				media, err := json.Marshal(locale.Media)
				if err != nil {
					return g, err
				}
				var observed pgtype.Timestamptz
				if locale.MediaObservedAt != nil {
					observed = pgtype.Timestamptz{Time: *locale.MediaObservedAt, Valid: true}
				}
				err = q.SaveGamePublicationLocale(ctx, dbgen.SaveGamePublicationLocaleParams{
					GameID: g.ID, Locale: locale.Locale, Name: locale.Name, Summary: locale.Summary, TextSourceID: locale.TextSourceID,
					TextObservationID: locale.TextObservationID, MediaSourceID: locale.MediaSourceID, Media: media,
					MediaObservedAt: observed, MediaStoreCountry: locale.MediaStoreCountry, CoverUrl: locale.CoverURL, Revision: next.Revision,
				})
				if err != nil {
					return g, err
				}
			}
			return next, nil
		})
}

func (s *Service) ChangeManagementGameVisibility(ctx context.Context, id string, revision int64, action, reason string) (ManagementResult, error) {
	return s.changeManagementGame(ctx, id, revision, action, reason, func(ctx context.Context, q *dbgen.Queries, g dbgen.Game) (dbgen.Game, error) {
		var desired string
		switch {
		case action == "archive" && (g.Status == Draft || g.Status == Published):
			desired = Archived
		case action == "unpublish" && g.Status == Published, action == "restore" && g.Status == Archived:
			desired = Draft
		default:
			return g, ErrConflict
		}
		return q.ChangeGameVisibility(ctx, dbgen.ChangeGameVisibilityParams{ID: g.ID, NewStatus: desired, ContentRating: g.ContentRating})
	})
}

func (s *Service) RateManagementGame(ctx context.Context, id string, revision int64, rating, reason string) (ManagementResult, error) {
	if !validGameRating(rating) {
		return ManagementResult{}, ErrInvalid
	}
	return s.changeManagementGame(ctx, id, revision, "rating", reason, func(ctx context.Context, q *dbgen.Queries, g dbgen.Game) (dbgen.Game, error) {
		if g.Status != Draft {
			return g, ErrConflict
		}
		return q.SetDraftGameRating(ctx, dbgen.SetDraftGameRatingParams{ID: g.ID, ContentRating: rating})
	})
}

func (s *Service) changeManagementGame(ctx context.Context, id string, revision int64, action, reason string,
	change func(context.Context, *dbgen.Queries, dbgen.Game) (dbgen.Game, error)) (ManagementResult, error) {
	actor := s.management
	if actor == nil || actor.ActorID == uuid.Nil() || actor.RequestID == uuid.Nil() ||
		(actor.ActorRole != "owner" && actor.ActorRole != "operator") || actor.Action != "game."+action {
		return ManagementResult{}, ErrGamePublicationIdentity
	}
	if !ValidPublicID(id) || revision < 1 || !reviewText(actor.ActorID.String(), reason) {
		return ManagementResult{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		row, err := q.LockGamePublicID(ctx, id)
		if err != nil {
			return ManagementResult{}, err
		}
		if row.Revision != revision {
			return ManagementResult{}, ErrConflict
		}
		next, err := change(ctx, q, row)
		if err != nil {
			return ManagementResult{}, err
		}
		err = q.AddCatalogPublicationDecision(ctx, dbgen.AddCatalogPublicationDecisionParams{
			ID: uuid.NewV7(), EntityKind: "game", EntityID: row.ID, Revision: next.Revision, Action: action,
			Actor: actor.ActorID.String(), Reason: strings.TrimSpace(reason), ManagementRequestID: &actor.RequestID,
			ContentRating: pgtype.Text{String: next.ContentRating, Valid: true},
		})
		return ManagementResult{Target: id, Revision: next.Revision}, err
	})
}
