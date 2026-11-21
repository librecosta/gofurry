package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
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
	TextSourceID       *uuid.UUID `json:"text_source_id,omitempty"`
	TextObservationID  *uuid.UUID `json:"text_observation_id,omitempty"`
	TextSource         *Source    `json:"text_source,omitempty"`
	MediaSourceID      *uuid.UUID `json:"media_source_id,omitempty"`
	MediaObservationID *uuid.UUID `json:"media_observation_id,omitempty"`
	MediaSource        *Source    `json:"media_source,omitempty"`
	MediaObservedAt    *time.Time `json:"media_observed_at,omitempty"`
	MediaStoreCountry  string     `json:"media_store_country,omitempty"`
	Media              SteamMedia `json:"media"`
	CoverURL           string     `json:"cover_url,omitempty"`
}

type GamePublicationReview struct {
	Revision         int64  `json:"revision"`
	ConfirmRelevance bool   `json:"confirm_relevance"`
	ContentRating    string `json:"content_rating"`
	PreviewToken     string `json:"preview_token"`
}

func gamePresentation(ctx context.Context, q *dbgen.Queries, id uuid.UUID, published bool) ([]GamePublicationLocale, error) {
	var raw []byte
	var err error
	if published {
		raw, err = q.ReadManagementPublishedGame(ctx, id)
	} else {
		raw, err = q.PreviewGamePublication(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	var locales []GamePublicationLocale
	if err = json.Unmarshal(raw, &locales); err != nil {
		return nil, err
	}
	for i := range locales {
		locale := &locales[i]
		locale.Media = reviewedSteamMedia(locale.Media)
		locale.CoverURL = profileMedia(locale.CoverURL, false)
		if locale.MediaObservedAt != nil {
			utc := locale.MediaObservedAt.UTC()
			locale.MediaObservedAt = &utc
		}
	}
	return locales, nil
}

func gamePublicationContent(locales []GamePublicationLocale) ([]byte, error) {
	content := make([]GamePublicationLocale, len(locales))
	copy(content, locales)
	for i := range content {
		content[i].TextSource, content[i].MediaSource = nil, nil
	}
	return json.Marshal(content, json.Deterministic(true))
}

func gamePreviewToken(game dbgen.Game, draft []byte) (string, error) {
	raw, err := json.Marshal(struct {
		GameID   uuid.UUID
		Revision int64
		Slug     string
		Content  jsontext.Value
	}{game.ID, game.Revision, gameEditSlug(game), draft}, json.Deterministic(true))
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

func gameDraftDiffers(game dbgen.Game, draft, published []byte) bool {
	return game.DraftSlug.Valid || !bytes.Equal(draft, published)
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
	return s.publishManagementGame(ctx, id, in, "publish", Draft)
}

func (s *Service) PublishManagementGameChanges(ctx context.Context, id string, in GamePublicationReview) (ManagementResult, error) {
	return s.publishManagementGame(ctx, id, in, "publish_changes", Published)
}

func (s *Service) publishManagementGame(ctx context.Context, id string, in GamePublicationReview, action, status string) (ManagementResult, error) {
	if !in.ConfirmRelevance || !validGameRating(in.ContentRating) || in.ContentRating == "unreviewed" {
		return ManagementResult{}, ErrGamePublicationReview
	}
	if in.ContentRating != "sfw" {
		return ManagementResult{}, ErrGamePublicationPolicy
	}
	return s.changeManagementGame(ctx, id, in.Revision, action,
		func(ctx context.Context, q *dbgen.Queries, g dbgen.Game) (dbgen.Game, error) {
			if g.Status != status || (status == Published && g.ContentRating != "sfw") {
				return g, ErrConflict
			}
			locales, err := gamePresentation(ctx, q, g.ID, false)
			if err != nil {
				return g, err
			}
			if len(locales) == 0 {
				return g, ErrGamePublicationText
			}
			draft, err := gamePublicationContent(locales)
			if err != nil {
				return g, err
			}
			token, err := gamePreviewToken(g, draft)
			if err != nil {
				return g, err
			}
			if token != in.PreviewToken {
				return g, ErrGamePreviewChanged
			}
			var next dbgen.Game
			if status == Published {
				published, err := gamePresentation(ctx, q, g.ID, true)
				if err != nil {
					return g, err
				}
				current, err := gamePublicationContent(published)
				if err != nil {
					return g, err
				}
				if !gameDraftDiffers(g, draft, current) {
					return g, ErrConflict
				}
				next, err = q.PublishReviewedGameChanges(ctx, g.ID)
				if err != nil {
					return g, err
				}
			} else {
				next, err = q.PublishReviewedGame(ctx, g.ID)
			}
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
				err = q.SaveGamePublicationLocale(ctx, dbgen.SaveGamePublicationLocaleParams{
					GameID: g.ID, Locale: locale.Locale, Name: locale.Name, Summary: locale.Summary, TextSourceID: locale.TextSourceID,
					TextObservationID: locale.TextObservationID, MediaSourceID: locale.MediaSourceID, Media: media,
					MediaObservationID: locale.MediaObservationID, MediaObservedAt: publicationTimestamp(locale.MediaObservedAt),
					MediaStoreCountry: locale.MediaStoreCountry, CoverUrl: locale.CoverURL, Revision: next.Revision,
				})
				if err != nil {
					return g, err
				}
			}
			return next, nil
		})
}

func (s *Service) ChangeManagementGameVisibility(ctx context.Context, id string, revision int64, action string) (ManagementResult, error) {
	return s.changeManagementGame(ctx, id, revision, action, func(ctx context.Context, q *dbgen.Queries, g dbgen.Game) (dbgen.Game, error) {
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

func (s *Service) RateManagementGame(ctx context.Context, id string, revision int64, rating string) (ManagementResult, error) {
	if !validGameRating(rating) {
		return ManagementResult{}, ErrInvalid
	}
	return s.changeManagementGame(ctx, id, revision, "rating", func(ctx context.Context, q *dbgen.Queries, g dbgen.Game) (dbgen.Game, error) {
		if g.Status != Draft {
			return g, ErrConflict
		}
		return q.SetDraftGameRating(ctx, dbgen.SetDraftGameRatingParams{ID: g.ID, ContentRating: rating})
	})
}

func (s *Service) changeManagementGame(ctx context.Context, id string, revision int64, action string,
	change func(context.Context, *dbgen.Queries, dbgen.Game) (dbgen.Game, error)) (ManagementResult, error) {
	actor := s.management
	if actor == nil || actor.ActorID == uuid.Nil() || actor.RequestID == uuid.Nil() ||
		(actor.ActorRole != "owner" && actor.ActorRole != "operator") || actor.Action != "game."+action {
		return ManagementResult{}, ErrGamePublicationIdentity
	}
	if !ValidPublicID(id) || revision < 1 {
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
		before, after := row.Status, next.Status
		if action == "rating" {
			before, after = row.ContentRating, next.ContentRating
		}
		err = q.AddCatalogPublicationDecision(ctx, dbgen.AddCatalogPublicationDecisionParams{
			ID: uuid.NewV7(), EntityKind: "game", EntityID: row.ID, Revision: next.Revision, Action: action,
			Actor: actor.ActorID.String(), Reason: publicationReason("Game", action, before, after), ManagementRequestID: &actor.RequestID,
			ContentRating: pgtype.Text{String: next.ContentRating, Valid: true},
		})
		return ManagementResult{Target: id, Revision: next.Revision}, err
	})
}
