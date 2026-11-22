package management

import (
	"context"
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func (o *Operations) gameMedia(g fiber.Router) {
	g.Get("/games/:id/media-candidates", func(c fiber.Ctx) error {
		query, err := search(c, "after")
		if err != nil {
			return err
		}
		after := uuid.Nil()
		if query.Has("after") {
			raw := query.Get("after")
			after, err = uuid.Parse(raw)
			if err != nil || after.String() != raw || after[6]>>4 != 7 || after[8]>>6 != 2 {
				return fiber.ErrBadRequest
			}
		}
		page, err := o.Catalog.ManagementGameMediaCandidates(c.Context(), c.Params("id"), after)
		if err != nil {
			return failure(err)
		}
		return c.JSON(page)
	})
	g.Patch("/games/:id/media-selection", editOnly, writeOperation(o, "game.media_selection",
		func(ctx context.Context, s *catalog.Service, _ *dbgen.Queries, id string, in catalog.GameMediaEditV2, _ catalog.PublicationDecision) (catalog.ManagementResult, error) {
			return s.SaveManagementGameMediaV2(ctx, id, in)
		}))
}
