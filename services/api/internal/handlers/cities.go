package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/cities"
)

func Cities(catalog *cities.Catalog) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		q := c.Query("q")
		if !cities.ValidQuery(q) {
			return fiber.ErrBadRequest
		}
		state := "ready"
		if catalog == nil {
			state = "unavailable"
		}
		return c.JSON(struct {
			State  string        `json:"state"`
			Cities []cities.City `json:"cities"`
		}{state, catalog.Search(q)})
	}
}
