package management

import (
	"bytes"
	"context"
	"errors"
	"github.com/librecosta/gofurry/services/api/internal/yearbook"
	"io"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/collection"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
	"github.com/librecosta/gofurry/services/api/internal/storage"
)

type publication struct {
	Action     string `json:"action"`
	Reason     string `json:"reason"`
	Revision   int64  `json:"revision"`
	ConfirmSFW bool   `json:"confirm_sfw"`
}
type policy struct {
	Rating   string `json:"rating"`
	Reason   string `json:"reason"`
	Revision int64  `json:"revision"`
}
type sources struct {
	Sources          []catalog.Source `json:"sources"`
	Reason           string           `json:"reason"`
	Revision         int64            `json:"revision"`
	ResumeSteamAppID string           `json:"resume_steam_app_id,omitempty"`
}
type feature struct {
	Slot     string `json:"slot"`
	PublicID string `json:"public_id"`
	Revision int64  `json:"revision"`
}
type featureChange struct {
	PublicID string `json:"public_id"`
	Revision int64  `json:"revision"`
	Reason   string `json:"reason"`
}

func (o *Operations) sites(g fiber.Router) {
	g.Get("/sites", func(c fiber.Ctx) error {
		v, err := search(c, "q", "status", "after", "rating", "uncategorized")
		if err != nil {
			return err
		}
		rating, uncategorized := v.Get("rating"), v.Get("uncategorized")
		switch rating {
		case "", "unreviewed", "sfw", "mixed", "adult":
		default:
			return fiber.ErrBadRequest
		}
		if uncategorized != "" && uncategorized != "true" {
			return fiber.ErrBadRequest
		}
		rows, err := dbgen.New(o.Pool).ManagementSites(c.Context(), dbgen.ManagementSitesParams{AfterID: v.Get("after"), Keyword: v.Get("q"), State: v.Get("status"), Rating: rating, Uncategorized: uncategorized == "true", TakeRows: pageSize + 1})
		return page(c, rows, err, func(row dbgen.ManagementSitesRow) string { return row.PublicID })
	})
	g.Get("/sites/taxonomy", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"categories": catalog.NavigationCategories(), "tags": catalog.NavigationTags(), "assets_enabled": o.Assets != nil})
	})
	g.Get("/sites/features", func(c fiber.Ctx) error {
		rows, err := o.Catalog.NavigationFeatures(c.Context())
		if err != nil {
			return failure(err)
		}
		result := make([]feature, 0, len(rows))
		for _, row := range rows {
			result = append(result, feature{row.Slot, row.PublicID, row.Revision})
		}
		return c.JSON(result)
	})
	o.features(g, "/sites/features/:slot", "site.feature", true)
	g.Get("/sites/:id", readOperation(o.site))
	save := func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, id string, in catalog.SiteManagementEdit, d catalog.PublicationDecision) (catalog.ManagementResult, error) {
		return s.SaveManagementSite(ctx, id, in, o.Region, d.Actor)
	}
	g.Post("/sites", editOnly, writeOperation(o, "site.create", save))
	g.Patch("/sites/:id", editOnly, writeOperation(o, "site.edit", save))
	for _, action := range []string{"publish", "archive", "restore", "unpublish"} {
		g.Post("/sites/:id/"+action, editOnly, writeOperation(o, "site."+action, func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, id string, in publication, d catalog.PublicationDecision) (catalog.ManagementResult, error) {
			d.Reason = in.Reason
			row, err := s.ChangeSitePublication(ctx, id, in.Revision, action, in.ConfirmSFW, d)
			return catalog.ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
		}))
	}
	g.Post("/sites/:id/policy", ownerOnly, writeOperation(o, "site.policy", func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, id string, in policy, d catalog.PublicationDecision) (catalog.ManagementResult, error) {
		row, err := s.ReviewSiteNavigation(ctx, o.Region, id, in.Revision, catalog.NavigationEdit{Rating: in.Rating, Actor: d.Actor, Reason: in.Reason, KeepTaxonomy: true})
		return catalog.ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
	}))
	g.Post("/sites/:id/assets/:kind", editOnly, o.asset)
}

type siteDetail struct {
	catalog.SiteManagement
	Yearbooks []int32 `json:"yearbooks"`
}

func (o *Operations) site(ctx context.Context, id string) (siteDetail, error) {
	record, err := o.Catalog.ManagementSite(ctx, id)
	detail := siteDetail{SiteManagement: record, Yearbooks: []int32{}}
	if err != nil {
		return detail, err
	}
	years, err := yearbook.New(o.Pool).Years(ctx, id)
	if err == nil {
		detail.Yearbooks = years
	} else if !errors.Is(err, yearbook.ErrMissing) {
		return detail, err
	}
	return detail, nil
}

func (o *Operations) asset(c fiber.Ctx) error {
	id, kind := c.Params("id"), storage.SiteAssetKind(c.Params("kind"))
	values, err := search(c, "revision")
	if err != nil {
		return err
	}
	rev, err := strconv.ParseInt(values.Get("revision"), 10, 64)
	if !catalog.ValidPublicID(id) || err != nil || rev < 1 {
		return fiber.ErrBadRequest
	}
	var mime string
	var limit int
	switch kind {
	case storage.SiteLogo:
		mime, limit = "image/webp", 1<<20
	case storage.SiteCover:
		mime, limit = "image/avif", 4<<20
	default:
		return fiber.ErrBadRequest
	}
	if c.Get("Content-Type") != mime {
		return fiber.ErrUnsupportedMediaType
	}
	if o.Assets == nil {
		return fiber.ErrServiceUnavailable
	}
	if c.Request().Header.ContentLength() > limit {
		return fiber.ErrRequestEntityTooLarge
	}
	select {
	case o.assetSlots <- struct{}{}:
		defer func() { <-o.assetSlots }()
	default:
		return fiber.ErrTooManyRequests
	}
	site, err := o.Catalog.ManagementSite(c.Context(), id)
	if err != nil {
		return failure(err)
	}
	if site.Revision != rev {
		return fiber.ErrConflict
	}
	var reader io.Reader = bytes.NewReader(c.Body())
	if c.Request().IsBodyStream() {
		reader = c.Request().BodyStream()
	}
	asset, err := o.Assets.ImportSiteAsset(c.Context(), kind, reader, false)
	if errors.Is(err, storage.ErrSiteAsset) {
		return fiber.ErrBadRequest
	}
	if err != nil {
		return failure(err)
	}
	result, err := o.Catalog.Manage(c.Context(), receipt(c, "site.asset."+string(kind)), func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries) (catalog.ManagementResult, error) {
		next, err := s.SetSiteAsset(ctx, id, rev, kind, asset.Key)
		return catalog.ManagementResult{Target: id, Revision: next}, err
	})
	if err != nil {
		return failure(err)
	}
	return c.JSON(result)
}

func (o *Operations) features(g fiber.Router, path, action string, site bool) {
	g.Patch(path, editOnly, func(c fiber.Ctx) error {
		var in featureChange
		if err := body(c, &in); err != nil {
			return err
		}
		slot := c.Params("slot")
		d := catalog.PublicationDecision{Actor: Current(c).ID.String(), Reason: in.Reason}
		if !d.Valid() {
			return fiber.ErrBadRequest
		}
		result, err := o.Catalog.Manage(c.Context(), receipt(c, action), func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries) (catalog.ManagementResult, error) {
			var rev int64
			var err error
			if site {
				rev, err = s.SetNavigationFeature(ctx, slot, in.PublicID, in.Revision, d.Actor, d.Reason)
			} else {
				rev, err = s.SetGameChannelFeature(ctx, slot, in.PublicID, in.Revision)
			}
			return catalog.ManagementResult{Target: slot, Revision: rev}, err
		})
		if err != nil {
			return failure(err)
		}
		return c.JSON(result)
	})
}

type gameDetail struct {
	catalog.GameManagement
	ReattachEnabled bool `json:"reattach_enabled"`
}

func (o *Operations) game(ctx context.Context, id string) (gameDetail, error) {
	game, err := o.Catalog.ManagementGame(ctx, id)
	return gameDetail{game, o.Intake != nil}, err
}

func (o *Operations) games(g fiber.Router) {
	g.Get("/games", func(c fiber.Ctx) error {
		v, err := search(c, "q", "status", "after", "platform", "restricted")
		if err != nil {
			return err
		}
		restricted := v.Get("restricted")
		if restricted != "" && restricted != "true" {
			return fiber.ErrBadRequest
		}
		rows, err := dbgen.New(o.Pool).ManagementGames(c.Context(), dbgen.ManagementGamesParams{AfterID: v.Get("after"), Keyword: v.Get("q"), State: v.Get("status"), Platform: v.Get("platform"), Restricted: restricted == "true", TakeRows: pageSize + 1})
		return page(c, rows, err, func(row dbgen.ManagementGamesRow) string { return row.PublicID })
	})
	o.collections(g)
	g.Get("/games/features", func(c fiber.Ctx) error {
		rows, err := o.Catalog.GameChannelFeatures(c.Context())
		if err != nil {
			return failure(err)
		}
		return c.JSON(rows)
	})
	o.features(g, "/games/features/:slot", "game.feature", false)
	g.Get("/games/:id", readOperation(o.game))
	save := func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, id string, in catalog.GameManagementEdit, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
		return s.SaveManagementGame(ctx, id, in)
	}
	g.Post("/games", editOnly, writeOperation(o, "game.create", save))
	g.Patch("/games/:id", editOnly, writeOperation(o, "game.edit", save))
	for _, action := range []string{"publish", "archive", "restore", "unpublish", "block", "clear_block"} {
		auth := editOnly
		if action == "block" || action == "clear_block" {
			auth = ownerOnly
		}
		g.Post("/games/:id/"+action, auth, writeOperation(o, "game."+action, func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, id string, in publication, d catalog.PublicationDecision) (catalog.ManagementResult, error) {
			d.Reason = in.Reason
			if action == "publish" {
				return s.PublishManagementGame(ctx, id, in.Revision, d)
			}
			row, err := s.ChangeGamePublication(ctx, id, in.Revision, action, d)
			return catalog.ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
		}))
	}
	g.Post("/games/:id/sources", ownerOnly, writeOperation(o, "game.sources", func(ctx context.Context, s *catalog.Service, q *dbgen.Queries, id string, in sources, d catalog.PublicationDecision) (catalog.ManagementResult, error) {
		d.Reason = in.Reason
		result, err := s.CorrectGameSources(ctx, id, in.Revision, in.Sources, d)
		if err == nil && in.ResumeSteamAppID != "" {
			err = o.Intake.ReattachManagementSteam(ctx, q, id, in.ResumeSteamAppID)
			if errors.Is(err, collection.ErrIntakeInput) || errors.Is(err, collection.ErrIntent) {
				err = catalog.ErrInvalid
			}
			if errors.Is(err, collection.ErrIntakeState) {
				err = catalog.ErrConflict
			}
		}
		return result, err
	}))
}

type collectionEdit struct {
	catalog.GameManagementEdit
	Members []string `json:"members"`
}

func (o *Operations) collections(g fiber.Router) {
	const base = "/games/collections"
	g.Get(base, func(c fiber.Ctx) error {
		v, err := search(c, "q", "status", "after")
		if err != nil {
			return err
		}
		rows, err := dbgen.New(o.Pool).ManagementCollections(c.Context(), dbgen.ManagementCollectionsParams{AfterID: v.Get("after"), Keyword: v.Get("q"), State: v.Get("status"), TakeRows: pageSize + 1})
		return page(c, rows, err, func(row dbgen.ManagementCollectionsRow) string { return row.PublicID })
	})
	g.Get(base+"/:id", readOperation(o.Catalog.InspectGameCollection))
	save := func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, id string, in collectionEdit, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
		editorial := catalog.Editorial{Slug: in.Slug, Localizations: in.Localizations}
		var row catalog.CollectionRecord
		var err error
		if id == "" {
			if in.Revision != 0 {
				return catalog.ManagementResult{}, catalog.ErrInvalid
			}
			row, err = s.CreateGameCollection(ctx, editorial)
		} else {
			row, err = s.EditGameCollection(ctx, id, in.Revision, editorial)
		}
		if err == nil {
			row, err = s.SetGameCollectionMembers(ctx, row.PublicID, row.Revision, in.Members)
		}
		return catalog.ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
	}
	g.Post(base, editOnly, writeOperation(o, "collection.create", save))
	g.Patch(base+"/:id", editOnly, writeOperation(o, "collection.edit", save))
	for _, action := range []string{"publish", "archive", "restore"} {
		g.Post(base+"/:id/"+action, editOnly, writeOperation(o, "collection."+action, func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, id string, in publication, d catalog.PublicationDecision) (catalog.ManagementResult, error) {
			d.Reason = in.Reason
			if !d.Valid() {
				return catalog.ManagementResult{}, catalog.ErrInvalid
			}
			status := catalog.Published
			if action == "archive" {
				status = catalog.Archived
			}
			row, err := s.SetGameCollectionStatus(ctx, id, in.Revision, status)
			return catalog.ManagementResult{Target: row.PublicID, Revision: row.Revision}, err
		}))
	}
}
