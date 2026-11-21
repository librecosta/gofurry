package catalog

import (
	"context"
	"fmt"
	"strings"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

type PublicationDecision struct {
	Actor               string
	Reason              string
	ManagementRequestID *uuid.UUID
}

func (d PublicationDecision) Valid() bool { return reviewText(d.Actor, d.Reason) }

func (d PublicationDecision) Save(ctx context.Context, q *dbgen.Queries, kind string, id uuid.UUID, revision int64, action string) error {
	if !d.Valid() {
		return ErrInvalid
	}
	return q.AddCatalogPublicationDecision(ctx, dbgen.AddCatalogPublicationDecisionParams{
		ID: uuid.NewV7(), EntityKind: kind, EntityID: id, Revision: revision, Action: action,
		Actor: d.Actor, Reason: strings.TrimSpace(d.Reason), ManagementRequestID: d.ManagementRequestID,
	})
}

func publicationReason(kind, action, before, after string) string {
	var intent string
	switch action {
	case "publish":
		intent = "Publication confirmed by operator"
		if kind == "Site" || kind == "Game" {
			intent = "SFW publication confirmed by operator"
		}
	case "publish_changes":
		intent = "Publication changes confirmed by operator"
	case "unpublish":
		intent = "Publication withdrawn by operator"
	case "archive":
		intent = "Archive confirmed by operator"
	case "restore":
		intent = "Restore confirmed by operator"
	case "rating":
		intent = "Content rating updated by operator"
	}
	return fmt.Sprintf("System: %s %s (%s → %s)", kind, intent, before, after)
}

func (s *Service) ChangeSitePublication(ctx context.Context, publicID string, revision int64, action string, sfw bool, d PublicationDecision) (dbgen.Site, error) {
	if !ValidPublicID(publicID) || sfw && action != "publish" || s.management == nil && !d.Valid() {
		return dbgen.Site{}, ErrInvalid
	}
	if actor := s.management; actor != nil && (actor.ActorID == uuid.Nil() || actor.RequestID == uuid.Nil() ||
		(actor.ActorRole != "owner" && actor.ActorRole != "operator") || actor.Action != "site."+action) {
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
		if actor := s.management; actor != nil {
			desired := Published
			switch action {
			case "unpublish":
				desired = Draft
			case "archive":
				desired = Archived
			}
			d = PublicationDecision{Actor: actor.ActorID.String(), Reason: publicationReason("Site", action, site.Status, desired), ManagementRequestID: &actor.RequestID}
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
		if err = requireSitePublication(ctx, q, site); err == nil {
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
			err = requireSitePublication(ctx, q, site)
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
		return out, err
	})
}
