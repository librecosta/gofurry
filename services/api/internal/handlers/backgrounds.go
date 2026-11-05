package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
)

func Backgrounds(service *wallpaper.Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		items, e := service.Catalog(c.Context())
		if e != nil {
			c.Set("Cache-Control", "no-store")
			return fiber.ErrServiceUnavailable
		}
		c.Set("Cache-Control", "public, max-age=0, must-revalidate")
		return c.JSON(fiber.Map{"backgrounds": items})
	}
}
