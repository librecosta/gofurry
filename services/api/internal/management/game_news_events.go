package management

import (
	"context"
	"net/url"
	"slices"
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func (o *Operations) gameNews(g fiber.Router) {
	g.Get("/games/:id/steam-events", func(c fiber.Ctx) error {
		raw := c.Request().URI().QueryString()
		if len(raw) > 1024 {
			return fiber.ErrBadRequest
		}
		values, err := url.ParseQuery(string(raw))
		if err != nil {
			return fiber.ErrBadRequest
		}
		for k, v := range values {
			if !slices.Contains([]string{"cursor", "locale", "q"}, k) || len(v) != 1 || len(v[0]) > 400 {
				return fiber.ErrBadRequest
			}
		}
		before := uuid.Nil()
		if values.Has("cursor") {
			id, err := collectionUUID(values.Get("cursor"))
			if err != nil || id == nil {
				return fiber.ErrBadRequest
			}
			before = *id
		}
		out, err := o.Catalog.SteamEventOperations(c.Context(), c.Params("id"), values.Get("locale"), values.Get("q"), before)
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
	g.Get("/games/:id/steam-events/:candidate", func(c fiber.Ctx) error {
		id, err := collectionUUID(c.Params("candidate"))
		if err != nil || id == nil || len(c.Request().URI().QueryString()) != 0 {
			return fiber.ErrBadRequest
		}
		out, err := o.Catalog.ManagementSteamEvent(c.Context(), c.Params("id"), *id)
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
	g.Patch("/games/:id/steam-events", editOnly, writeOperation(o, "game.news.visibility",
		func(ctx context.Context, _ *catalog.Service, q *dbgen.Queries, game string, in catalog.SteamEventVisibility, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
			return catalog.ChangeSteamEventVisibility(ctx, q, game, in)
		}))
}
