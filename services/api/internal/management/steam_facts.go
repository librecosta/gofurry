package management

import (
	"context"
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func (o *Operations) steamFacts(g fiber.Router) {
	g.Get("/games/:id/facts", readOperation(func(ctx context.Context, id string) (catalog.SteamFactsManagement, error) {
		return o.Catalog.SteamManagementFacts(ctx, id, o.Egress)
	}))
	g.Get("/games/:id/changes", func(c fiber.Ctx) error {
		args, err := search(c, "market", "code", "from", "through", "cursor")
		if err != nil {
			return err
		}
		page, err := o.Catalog.SteamGameChangeHistory(c.Context(), c.Params("id"), "", catalog.SteamChangeFilter{
			Market: args.Get("market"), Code: args.Get("code"), From: args.Get("from"), Through: args.Get("through"), After: args.Get("cursor"),
		}, false)
		if err != nil {
			return failure(err)
		}
		out := catalog.SteamChangePage[catalog.SteamChangeEvidence]{Items: make([]catalog.SteamChangeEvidence, 0, len(page.Items)), After: page.After}
		for _, row := range page.Items {
			out.Items = append(out.Items, catalog.SteamChangeForManagement(row))
		}
		return c.JSON(out)
	})
	g.Get("/steam-changes/status", func(c fiber.Ctx) error {
		status, err := o.Catalog.SteamChangeStatus(c.Context())
		if err != nil {
			return failure(err)
		}
		return c.JSON(status)
	})
	g.Post("/games/:id/first-available", editOnly, writeOperation(o, "game.first_available",
		func(ctx context.Context, _ *catalog.Service, q *dbgen.Queries, id string, in catalog.SteamFirstAvailableEdit, decision catalog.PublicationDecision) (catalog.ManagementResult, error) {
			actor, err := uuid.Parse(decision.Actor)
			if err != nil {
				return catalog.ManagementResult{}, catalog.ErrInvalid
			}
			return catalog.RecordSteamFirstAvailable(ctx, q, id, in, actor)
		}))
	g.Post("/games/:id/changes/rebuild", editOnly, writeOperation(o, "game.changes.rebuild",
		func(ctx context.Context, _ *catalog.Service, q *dbgen.Queries, id string, in catalog.SteamChangeRebuild, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
			return catalog.RebuildSteamChanges(ctx, q, id, in)
		}))
}
