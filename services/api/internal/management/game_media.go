package management

import (
	"context"
	"net/url"
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func (o *Operations) gameMedia(g fiber.Router) {
	g.Get("/games/:id/media-candidates", func(c fiber.Ctx) error {
		raw := string(c.Request().URI().QueryString())
		if len(raw) > 1024 {
			return fiber.ErrBadRequest
		}
		query, err := url.ParseQuery(raw)
		if err != nil {
			return fiber.ErrBadRequest
		}
		after := uuid.Nil()
		if len(query) != 0 {
			values := query["after"]
			if len(query) != 1 || len(values) != 1 {
				return fiber.ErrBadRequest
			}
			cursor, err := collectionUUID(values[0])
			if err != nil || cursor == nil {
				return fiber.ErrBadRequest
			}
			after = *cursor
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
