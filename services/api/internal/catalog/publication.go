package catalog

import (
	"context"
	"strings"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type PublicationDecision struct {
	Actor  string
	Reason string
}

func (d PublicationDecision) Valid() bool { return reviewText(d.Actor, d.Reason) }

func (d PublicationDecision) Save(ctx context.Context, q *dbgen.Queries, kind string, id uuid.UUID, revision int64, action string) error {
	if !d.Valid() {
		return ErrInvalid
	}
	return q.AddCatalogPublicationDecision(ctx, dbgen.AddCatalogPublicationDecisionParams{
		ID: uuid.NewV7(), EntityKind: kind, EntityID: id, Revision: revision, Action: action,
		Actor: d.Actor, Reason: strings.TrimSpace(d.Reason),
	})
}

// SetGameBlock applies a policy intervention under the caller's Game lock.
func SetGameBlock(ctx context.Context, q *dbgen.Queries, game dbgen.Game, blocked bool, d PublicationDecision) (dbgen.Game, error) {
	if !d.Valid() || game.PublicationBlocked == blocked {
		return game, ErrInvalid
	}
	next, err := q.SetGamePublicationBlock(ctx, dbgen.SetGamePublicationBlockParams{ID: game.ID, PublicationBlocked: blocked})
	if err != nil {
		return game, err
	}
	action := "clear_block"
	if blocked {
		action = "block"
	}
	return next, d.Save(ctx, q, "game", game.ID, next.Revision, action)
}

func (s *Service) ChangeGamePublication(ctx context.Context, publicID string, revision int64, action string, d PublicationDecision) (dbgen.Game, error) {
	if !ValidPublicID(publicID) || !d.Valid() {
		return dbgen.Game{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Game, error) {
		game, err := q.LockGamePublicID(ctx, publicID)
		if err != nil {
			return game, err
		}
		if revision != game.Revision {
			return game, ErrConflict
		}
		switch action {
		case "block", "clear_block":
			return SetGameBlock(ctx, q, game, action == "block", d)
		case "archive", "unpublish", "restore":
			return changeGameVisibility(ctx, q, game, action, d)
		default:
			return game, ErrInvalid
		}
	})
}

func changeGameVisibility(ctx context.Context, q *dbgen.Queries, game dbgen.Game, action string, d PublicationDecision) (dbgen.Game, error) {
	var next dbgen.Game
	var err error
	switch action {
	case "archive":
		if game.Status == Archived {
			return game, ErrConflict
		}
		next, err = q.ChangeGameStatus(ctx, dbgen.ChangeGameStatusParams{ID: game.ID, NewStatus: Archived})
	case "unpublish":
		if game.Status != Published {
			return game, ErrConflict
		}
		next, err = q.ChangeGameStatus(ctx, dbgen.ChangeGameStatusParams{ID: game.ID, NewStatus: Draft})
	case "restore":
		if game.Status == Published || !game.PublishedAt.Valid || game.PublicationBlocked {
			return game, ErrConflict
		}
		next, err = q.RestoreGamePublication(ctx, game.ID)
	default:
		return game, ErrInvalid
	}
	if err != nil {
		return game, err
	}
	return next, d.Save(ctx, q, "game", game.ID, next.Revision, action)
}

func (s *Service) ChangeSitePublication(ctx context.Context, publicID string, revision int64, action string, sfw bool, d PublicationDecision) (dbgen.Site, error) {
	if !ValidPublicID(publicID) || !d.Valid() || sfw && action != "publish" {
		return dbgen.Site{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Site, error) {
		site, err := q.LockNavigationSite(ctx, publicID)
		if err != nil {
			return site, err
		}
		if revision != site.Revision {
			return site, ErrConflict
		}
		if sfw && site.Status == Draft && !site.PublishedAt.Valid && site.ContentRating == "unreviewed" {
			site, err = q.InitialSitePolicy(ctx, site.ID)
			if err != nil {
				return site, err
			}
			if err = q.AddNavigationReview(ctx, dbgen.AddNavigationReviewParams{ID: uuid.NewV7(), SiteID: site.ID,
				SiteRevision: site.Revision, PreviousRating: "unreviewed", Rating: "sfw", Actor: d.Actor, Reason: strings.TrimSpace(d.Reason)}); err != nil {
				return site, err
			}
		}
		return changeSiteVisibility(ctx, q, site, action, d)
	})
}

func changeSiteVisibility(ctx context.Context, q *dbgen.Queries, site dbgen.Site, action string, d PublicationDecision) (dbgen.Site, error) {
	var next dbgen.Site
	var err error
	switch action {
	case "publish":
		if site.Status != Draft || site.PublishedAt.Valid {
			return site, ErrConflict
		}
		if err = requireSitePublication(ctx, q, site.ID); err == nil {
			next, err = q.ChangeSiteStatus(ctx, dbgen.ChangeSiteStatusParams{ID: site.ID, NewStatus: Published})
		}
	case "archive":
		if site.Status == Archived {
			return site, ErrConflict
		}
		next, err = q.ChangeSiteStatus(ctx, dbgen.ChangeSiteStatusParams{ID: site.ID, NewStatus: Archived})
	case "unpublish":
		if site.Status != Published {
			return site, ErrConflict
		}
		next, err = q.ChangeSiteStatus(ctx, dbgen.ChangeSiteStatusParams{ID: site.ID, NewStatus: Draft})
	case "restore":
		if site.Status == Published || !site.PublishedAt.Valid {
			return site, ErrConflict
		}
		if !site.LegacyDetail {
			err = requireSitePublication(ctx, q, site.ID)
		}
		if err == nil {
			next, err = q.RestoreSitePublication(ctx, site.ID)
		}
	default:
		return site, ErrInvalid
	}
	if err != nil {
		return site, err
	}
	return next, d.Save(ctx, q, "site", site.ID, next.Revision, action)
}

type PublicationDiagnostics struct {
	LegacyPromoted int   `json:"legacy_promoted"`
	LegacyUnknown  int64 `json:"legacy_unknown_detail_only"`
	GamesMissing   int64 `json:"published_games_missing_presentation"`
}

func (s *Service) DiagnosePublication(ctx context.Context, upgrade bool) (PublicationDiagnostics, error) {
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (PublicationDiagnostics, error) {
		var out PublicationDiagnostics
		if upgrade {
			ids, err := q.PromoteLegacySitePolicy(ctx, 128)
			if err != nil {
				return out, err
			}
			out.LegacyPromoted = len(ids)
		}
		var err error
		out.LegacyUnknown, err = q.LegacySitePolicyPending(ctx)
		if err == nil {
			out.GamesMissing, err = q.PublishedGamesMissingPresentation(ctx)
		}
		return out, err
	})
}
