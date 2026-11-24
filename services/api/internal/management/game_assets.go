package management

import (
	"context"
	"net/url"
	"strconv"
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func gameAssetQuery(c fiber.Ctx, allowed ...string) (url.Values, error) {
	raw := c.Request().URI().QueryString()
	if len(raw) > 1024 {
		return nil, fiber.ErrBadRequest
	}
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, fiber.ErrBadRequest
	}
	for _, key := range allowed {
		if v, ok := values[key]; ok && (len(v) != 1 || len(v[0]) == 0 || len(v[0]) > 100) {
			return nil, fiber.ErrBadRequest
		}
	}
	for key := range values {
		known := false
		for _, candidate := range allowed {
			known = known || key == candidate
		}
		if !known {
			return nil, fiber.ErrBadRequest
		}
	}
	return values, nil
}

func (o *Operations) gameAssets(g fiber.Router) {
	g.Get("/games/:id/assets", o.gameAssetList(false))
	g.Get("/games/:id/asset-library", o.gameAssetList(true))
	g.Get("/games/:id/assets/:asset/versions/:version", func(c fiber.Ctx) error {
		asset, err := collectionUUID(c.Params("asset"))
		version, versionErr := collectionUUID(c.Params("version"))
		if err != nil || versionErr != nil || asset == nil || version == nil || len(c.Request().URI().QueryString()) != 0 {
			return fiber.ErrBadRequest
		}
		out, err := o.Catalog.ManagementGameAsset(c.Context(), c.Params("id"), *asset, *version)
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
	g.Patch("/games/:id/asset-library", editOnly, writeOperation(o, "game.asset_library",
		func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, id string, in catalog.GameAssetLibraryEdit, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
			return s.EditGameAssetLibrary(ctx, id, in)
		}))
	g.Get("/games/:id/asset-presentation", func(c fiber.Ctx) error {
		q, err := gameAssetQuery(c, "locale", "surface", "slot", "after")
		if err != nil {
			return err
		}
		after := int64(-1)
		if raw := q.Get("after"); raw != "" {
			after, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || after < 0 || strconv.FormatInt(after, 10) != raw {
				return fiber.ErrBadRequest
			}
		}
		out, err := o.Catalog.ManagementGameAssetPresentation(c.Context(), c.Params("id"), q.Get("locale"), q.Get("surface"), q.Get("slot"), after)
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
	g.Patch("/games/:id/asset-presentation", editOnly, writeOperation(o, "game.asset_presentation",
		func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, id string, in catalog.GameAssetPresentationEdit, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
			return s.EditGameAssetPresentation(ctx, id, in)
		}))
}

func (o *Operations) gameAssetList(library bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		q, err := gameAssetQuery(c, "after", "asset_id", "platform_code", "kind", "category", "locale", "source_role")
		if err != nil {
			return err
		}
		after, err := collectionUUID(q.Get("after"))
		asset, assetErr := collectionUUID(q.Get("asset_id"))
		if err != nil || assetErr != nil {
			return fiber.ErrBadRequest
		}
		f := catalog.GameAssetFilter{AssetID: asset, Platform: q.Get("platform_code"), Kind: q.Get("kind"), Category: q.Get("category"),
			Locale: q.Get("locale"), Role: q.Get("source_role"), After: uuid.Nil()}
		if after != nil {
			f.After = *after
		}
		out, err := o.Catalog.ManagementGameAssets(c.Context(), c.Params("id"), f, library)
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	}
}
