package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/librecosta/gofurry/services/api/internal/search"
	"github.com/rs/zerolog"
)

func Suggestions(service *search.Service, logger zerolog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		phrases, err := service.Suggestions(c.Context(), c.Query("q"))
		if err != nil {
			state, status := search.Diagnostic(err)
			c.Set("X-GoFurry-Suggestions-State", state)
			if !errors.Is(err, search.ErrQuery) {
				logger.Warn().Str("state", state).Int("upstream_status", status).Str("request_id", requestid.FromContext(c)).Msg("search suggestions unavailable")
			}
		} else if len(phrases) == 0 {
			c.Set("X-GoFurry-Suggestions-State", "empty")
		} else {
			c.Set("X-GoFurry-Suggestions-State", "ready")
		}
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
