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
}

// Manage commits one verified operator receipt with all local catalogue changes.
func (s *Service) Manage(ctx context.Context, receipt dbgen.RecordManagementOperationParams, change func(context.Context, *Service, *dbgen.Queries) (ManagementResult, error)) (ManagementResult, error) {
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		result, err := change(ctx, s.InTransaction(q), q)
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
		out.LogoURL = s.assets.SiteAssetURL(storage.SiteLogo, row.LogoAssetKey.String)
		out.CoverURL = s.assets.SiteAssetURL(storage.SiteCover, row.FeaturedCoverAssetKey.String)
		return out, err
	})
}

type GameManagement struct {
	PublicID            string                                `json:"public_id"`
	Slug                string                                `json:"slug"`
	Status              string                                `json:"status"`
	Revision            int64                                 `json:"revision"`
	PreviouslyPublished bool                                  `json:"previously_published"`
	PublicationBlocked  bool                                  `json:"publication_blocked"`
	IntakeManaged       bool                                  `json:"intake_managed"`
	Localizations       []dbgen.ManagementGameTextRow         `json:"localizations"`
	Sources             []dbgen.ManagementGameSourcesRow      `json:"sources"`
	Facts               []dbgen.ManagementGameFactsRow        `json:"facts"`
	Approved            []dbgen.ManagementApprovedGameTextRow `json:"approved"`
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
			PreviouslyPublished: row.PublishedAt.Valid, PublicationBlocked: row.PublicationBlocked, IntakeManaged: row.IntakeManaged}
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
		out.Approved, err = q.ManagementApprovedGameText(ctx, row.ID)
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
	Slug          string         `json:"slug"`
	Localizations []Localization `json:"localizations"`
	Revision      int64          `json:"revision"`
}

func (s *Service) SaveManagementGame(ctx context.Context, id string, in GameManagementEdit) (ManagementResult, error) {
	var row dbgen.Game
	var err error
	data := GameInput{Editorial: Editorial{Slug: in.Slug, Localizations: in.Localizations}}
	if id == "" {
		if in.Revision != 0 || len(in.Localizations) == 0 {
			return ManagementResult{}, ErrInvalid
		}
		row, err = s.CreateGame(ctx, data)
	} else {
		row, err = write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (dbgen.Game, error) {
			g, e := q.LockGamePublicID(ctx, id)
			if e != nil {
				return g, e
			}
			return s.editGame(ctx, g.ID, in.Revision, data, true)
		})
	}
	return ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
}

func (s *Service) CorrectGameSources(ctx context.Context, id string, rev int64, src []Source, decision PublicationDecision) (ManagementResult, error) {
	if !decision.Valid() || src == nil {
		return ManagementResult{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		row, err := q.LockGamePublicID(ctx, id)
		if err != nil {
			return ManagementResult{}, err
		}
		local, err := q.ManagementGameText(ctx, row.ID)
		if err != nil {
			return ManagementResult{}, err
		}
		input := GameInput{Editorial: Editorial{Slug: row.Slug, Localizations: []Localization{}}, Sources: src}
		for _, item := range local {
			input.Localizations = append(input.Localizations, Localization(item))
		}
		row, err = s.EditGame(ctx, row.ID, rev, input)
		return ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
	})
}

// PublishManagementGame applies the same publication rules as the CLI while
// recording the authenticated actor and reason on the domain decision.
func (s *Service) PublishManagementGame(ctx context.Context, id string, rev int64, d PublicationDecision) (ManagementResult, error) {
	if !d.Valid() {
		return ManagementResult{}, ErrInvalid
	}
	return write(ctx, s, func(ctx context.Context, q *dbgen.Queries) (ManagementResult, error) {
		g, err := q.LockGamePublicID(ctx, id)
		if err != nil {
			return ManagementResult{}, err
		}
		next, err := s.setGameStatus(ctx, g.ID, rev, Published, d)
		return ManagementResult{Target: next.PublicID, Revision: next.Revision}, err
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
