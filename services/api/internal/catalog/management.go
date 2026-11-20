package catalog

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/storage"
)

type ManagementResult struct {
	Target   string `json:"target"`
	Revision int64  `json:"revision"`
	Affected *int   `json:"affected,omitempty"`
}

// Manage commits one verified operator receipt with all local catalogue changes.
func (s *Service) Manage(ctx context.Context, receipt dbgen.RecordManagementOperationParams, change func(context.Context, *Service, *dbgen.Queries) (ManagementResult, error)) (ManagementResult, error) {
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		managed := s.InTransaction(q)
		managed.management = &receipt
		result, err := change(ctx, managed, q)
		if err != nil {
			return result, err
		}
		receipt.Target, receipt.Revision = result.Target, result.Revision
		err = q.RecordManagementOperation(ctx, receipt)
		var dbError *pgconn.PgError
		if errors.As(err, &dbError) && dbError.Code == "23505" {
			err = ErrConflict
		}
		return result, err
	})
}

type SiteManagement struct {
	NavigationProfile
	Status              string                             `json:"status"`
	Slug                string                             `json:"slug"`
	PreviouslyPublished bool                               `json:"previously_published"`
	Localizations       []dbgen.ManagementSiteTextRow      `json:"localizations"`
	Addresses           []dbgen.ManagementSiteAddressesRow `json:"addresses"`
	HasLogo             bool                               `json:"has_logo"`
	LogoURL             string                             `json:"logo_url"`
	CoverURL            string                             `json:"cover_url"`
}

func (s *Service) ManagementSite(ctx context.Context, id string) (out SiteManagement, err error) {
	if !ValidPublicID(id) {
		return out, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (SiteManagement, error) {
		row, err := q.FindYearbookSite(ctx, id)
		if err != nil {
			return out, err
		}
		out.NavigationProfile = profile(row)
		out.Status, out.Slug, out.PreviouslyPublished = row.Status, row.Slug, row.PublishedAt.Valid
		out.Localizations, err = q.ManagementSiteText(ctx, row.ID)
		if err != nil {
			return out, err
		}
		out.Addresses, err = q.ManagementSiteAddresses(ctx, row.ID)
		out.HasLogo = row.LogoAssetKey.Valid
		out.LogoURL = s.assets.SiteAssetURL(storage.SiteLogo, row.LogoAssetKey.String)
		out.CoverURL = s.assets.SiteAssetURL(storage.SiteCover, row.FeaturedCoverAssetKey.String)
		return out, err
	})
}

type GameManagement struct {
	PublicID              string                           `json:"public_id"`
	Slug                  string                           `json:"slug"`
	Status                string                           `json:"status"`
	Revision              int64                            `json:"revision"`
	ContentRating         string                           `json:"content_rating"`
	DraftSlug             *string                          `json:"draft_slug"`
	DraftMedia            []GameDraftMedia                 `json:"draft_media"`
	DraftPresentation     []GamePublicationLocale          `json:"draft_presentation"`
	PublishedPresentation []GamePublicationLocale          `json:"published_presentation"`
	SourceCandidates      []GameSourceCandidate            `json:"source_candidates"`
	HasUnpublishedChanges bool                             `json:"has_unpublished_changes"`
	PreviewToken          string                           `json:"preview_token"`
	Localizations         []dbgen.ManagementGameTextRow    `json:"localizations"`
	Sources               []dbgen.ManagementGameSourcesRow `json:"sources"`
	Facts                 []dbgen.ManagementGameFactsRow   `json:"facts"`
}

func (s *Service) ManagementGame(ctx context.Context, id string) (out GameManagement, err error) {
	if !ValidPublicID(id) {
		return out, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (GameManagement, error) {
		row, err := q.ManagementGame(ctx, id)
		if err != nil {
			return out, err
		}
		out = GameManagement{PublicID: row.PublicID, Slug: row.Slug, Revision: row.Revision, Status: row.Status,
			ContentRating: row.ContentRating, PublishedPresentation: []GamePublicationLocale{}}
		if row.DraftSlug.Valid {
			out.DraftSlug = &row.DraftSlug.String
		}
		out.Localizations, err = q.ManagementGameText(ctx, row.ID)
		if err != nil {
			return out, err
		}
		out.Sources, err = q.ManagementGameSources(ctx, row.ID)
		if err != nil {
			return out, err
		}
		out.Facts, err = q.ManagementGameFacts(ctx, row.ID)
		if err != nil {
			return out, err
		}
		out.SourceCandidates, err = readGameSourceCandidates(ctx, q, row.ID)
		if err != nil {
			return out, err
		}
		out.DraftMedia, err = readGameDraftMedia(ctx, q, row.ID)
		if err != nil {
			return out, err
		}
		out.DraftPresentation, err = gamePresentation(ctx, q, row.ID, false)
		if err != nil {
			return out, err
		}
		draft, err := gamePublicationContent(out.DraftPresentation)
		if err != nil {
			return out, err
		}
		out.PreviewToken, err = gamePreviewToken(row, draft)
		if err != nil || row.Status != Published {
			return out, err
		}
		out.PublishedPresentation, err = gamePresentation(ctx, q, row.ID, true)
		if err != nil {
			return out, err
		}
		public, err := gamePublicationContent(out.PublishedPresentation)
		out.HasUnpublishedChanges = gameDraftDiffers(row, draft, public)
		return out, err
	})
}

type SiteManagementEdit struct {
	Slug          string             `json:"slug"`
	Localizations []SiteLocalization `json:"localizations"`
	Addresses     []Address          `json:"addresses"`
	Primary       string             `json:"primary_category"`
	Secondary     []string           `json:"secondary_categories"`
	Tags          []string           `json:"tags"`
	Languages     []string           `json:"languages"`
	Revision      int64              `json:"revision"`
}

func (s *Service) SaveManagementSite(ctx context.Context, id string, in SiteManagementEdit, region, actor string) (ManagementResult, error) {
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		input := SiteInput{Editorial: Editorial{Slug: in.Slug, Localizations: []Localization{}}, Addresses: in.Addresses}
		for _, locale := range in.Localizations {
			input.Localizations = append(input.Localizations, locale.Localization)
		}
		var row dbgen.Site
		var err error
		if id == "" {
			if in.Revision != 0 || len(input.Localizations) == 0 {
				return ManagementResult{}, ErrInvalid
			}
			row, err = s.CreateSite(ctx, input)
		} else {
			row, err = q.LockNavigationSite(ctx, id)
			if err == nil {
				row, err = s.EditSite(ctx, row.ID, in.Revision, input)
			}
		}
		if err != nil {
			return ManagementResult{}, err
		}
		for _, locale := range in.Localizations {
			row.Revision, err = s.SetSiteDescription(ctx, row.PublicID, row.Revision, locale.Locale, locale.Body)
			if err != nil {
				return ManagementResult{}, err
			}
		}
		// Taxonomy edits retain the durable content policy and its prior decision.
		if in.Primary != row.PrimaryCategory.String || !slices.Equal(in.Secondary, row.SecondaryCategories) ||
			!slices.Equal(in.Tags, row.NavigationTags) || !slices.Equal(in.Languages, row.SupportedLanguages) {
			p, err := s.ReviewSiteNavigation(ctx, region, row.PublicID, row.Revision, NavigationEdit{Rating: row.ContentRating, Primary: in.Primary, Secondary: in.Secondary,
				Tags: in.Tags, Languages: in.Languages, Actor: actor, Reason: "Catalog taxonomy edit"})
			return ManagementResult{Target: p.PublicID, Revision: p.Revision}, err
		}
		return ManagementResult{Target: row.PublicID, Revision: row.Revision}, nil
	})
}

type GameManagementEdit struct {
	Slug          string               `json:"slug"`
	Localizations []Localization       `json:"localizations"`
	Revision      int64                `json:"revision"`
	TextAdoptions []GameTextAdoption   `json:"text_adoptions,omitempty"`
	DraftMedia    []GameMediaSelection `json:"draft_media,omitempty"`
}

func (s *Service) SaveManagementGame(ctx context.Context, id string, in GameManagementEdit) (ManagementResult, error) {
	var row dbgen.Game
	var err error
	data := GameInput{Editorial: Editorial{Slug: in.Slug, Localizations: in.Localizations}}
	if id == "" {
		if in.Revision != 0 || len(in.Localizations) == 0 || len(in.TextAdoptions) > 0 || len(in.DraftMedia) > 0 {
			return ManagementResult{}, ErrInvalid
		}
		row, err = s.CreateGame(ctx, data)
	} else {
		if strings.TrimSpace(in.Slug) == "" || in.Localizations == nil {
			return ManagementResult{}, ErrInvalid
		}
		row, err = write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Game, error) {
			g, e := q.LockGamePublicID(ctx, id)
			if e != nil {
				return g, e
			}
			return saveGameDraft(ctx, q, g, in)
		})
	}
	return ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
}

func (s *Service) CorrectGameSources(ctx context.Context, id string, rev int64, src []Source, decision PublicationDecision) (ManagementResult, error) {
	if !decision.Valid() || src == nil {
		return ManagementResult{}, ErrInvalid
	}
	sources, err := normalizeSources(src)
	if err != nil {
		return ManagementResult{}, err
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		row, err := q.LockGamePublicID(ctx, id)
		if err != nil {
			return ManagementResult{}, err
		}
		if err := editable(rev, row.Revision, row.Status); err != nil {
			return ManagementResult{}, err
		}
		if row.Status != Draft {
			return ManagementResult{}, ErrConflict
		}
		row, err = editGameSources(ctx, q, row, sources, rev)
		return ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
	})
}

func ManagementSearch(q string) bool {
	return validText(q, 0, 100, false) && q == strings.TrimSpace(q)
}

func ManagementConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}

func ManagementMissing(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ErrNotFound)
}
