package management

import (
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func collectionUUID(raw string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil || id.String() != raw || id[6]>>4 != 7 || id[8]>>6 != 2 {
		return nil, fiber.ErrBadRequest
	}
	return &id, nil
}

func (o *Operations) entityCollection(g fiber.Router, kind string) {
	base := "/" + kind + "/:id/collection"
	g.Get(base, func(c fiber.Ctx) error {
		values, err := search(c, "cursor")
		if err != nil || !catalog.ValidPublicID(c.Params("id")) {
			return fiber.ErrBadRequest
		}
		cursor, err := collectionUUID(values.Get("cursor"))
		if err != nil {
			return err
		}
		out, err := o.Collector.EntityCollection(c.Context(), dbgen.New(o.Pool), kind, c.Params("id"), cursor)
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
	g.Get(base+"/:plan/history", func(c fiber.Ctx) error {
		values, err := search(c, "cursor")
		if err != nil || !catalog.ValidPublicID(c.Params("id")) {
			return fiber.ErrBadRequest
		}
		cursor, err := collectionUUID(values.Get("cursor"))
		if err != nil {
			return err
		}
		plan, err := collectionUUID(c.Params("plan"))
		if err != nil || plan == nil {
			return fiber.ErrBadRequest
		}
		out, err := o.Collector.EntityCollectionHistory(c.Context(), dbgen.New(o.Pool), kind, c.Params("id"), *plan, cursor)
		if err != nil {
			return failure(err)
		}
		return c.JSON(out)
	})
}
