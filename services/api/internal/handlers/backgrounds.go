package handlers

import (
	"errors"
	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
	"strconv"
	"strings"
	"uuid"
)

func Backgrounds(service *wallpaper.Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		idText := c.Params("id")
		random := strings.HasSuffix(c.Route().Path, "/random")
		if idText != "" || random {
			var id uuid.UUID
			var err error
			if random {
				idText = c.Query("exclude")
			}
			if idText != "" {
				id, err = uuid.Parse(idText)
				if err != nil {
					return fiber.ErrBadRequest
				}
			}
			item, err := service.PublicOne(c.Context(), id, random)
			if errors.Is(err, wallpaper.ErrNotFound) {
				return fiber.ErrNotFound
			}
			if err != nil {
				return fiber.ErrServiceUnavailable
			}
			return c.JSON(item)
		}
		page, e := strconv.Atoi(c.Query("page", "1"))
		if e != nil {
			return fiber.ErrBadRequest
		}
		size, e := strconv.Atoi(c.Query("page_size", "9"))
		if e != nil {
			return fiber.ErrBadRequest
		}
		items, e := service.PublicPage(c.Context(), page, size, c.Query("q"))
		if errors.Is(e, wallpaper.ErrInvalid) {
			return fiber.ErrBadRequest
		}
		if e != nil {
			c.Set("Cache-Control", "no-store")
			return fiber.ErrServiceUnavailable
		}
		c.Set("Cache-Control", "public, max-age=0, must-revalidate")
		return c.JSON(items)
	}
}
