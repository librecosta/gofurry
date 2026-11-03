package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/search"
)

func Suggestions(service *search.Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		phrases, err := service.Suggestions(c.Context(), c.Query("q"))
		switch {
		case errors.Is(err, search.ErrQuery):
			return fiber.ErrBadRequest
		case errors.Is(err, search.ErrBusy):
			c.Set("Retry-After", "1")
			return fiber.ErrTooManyRequests
		case err != nil:
			return fiber.ErrServiceUnavailable
		}
		return c.JSON(fiber.Map{"suggestions": phrases})
	}
}
