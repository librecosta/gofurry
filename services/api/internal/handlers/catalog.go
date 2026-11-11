package handlers

import (
	"context"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
)

func CatalogDetail[T any](read func(context.Context, string) (T, error)) fiber.Handler {
	return func(c fiber.Ctx) error {
		result, err := read(c.Context(), c.Params("publicID"))
		return catalogResponse(c, result, err)
	}
}

func SiteDetail(service *catalog.Service, interactionsEnabled bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		result, err := service.PublishedSite(c.Context(), c.Params("publicID"))
		if err == nil && interactionsEnabled && result.InteractionEligible {
			c.Set("X-GoFurry-Interactions", "enabled")
		}
		return navigationResponse(c, result, err)
	}
}

func CatalogPage(read func(context.Context, string, int32) (catalog.Page, error), sensitive bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		limit := int64(20)
		var after string
		var invalid bool
		seen := map[string]bool{}
		c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
			name := string(key)
			if seen[name] || len(value) == 0 {
				invalid = true
				return
			}
			seen[name] = true
			switch name {
			case "after":
				after = string(value)
			case "limit":
				parsed, err := strconv.ParseInt(string(value), 10, 32)
				invalid = invalid || err != nil
				limit = parsed
			default:
				invalid = true
			}
		})
		if invalid {
			return catalogResponse(c, nil, catalog.ErrInvalid)
		}
		result, err := read(c.Context(), after, int32(limit))
		if sensitive {
			return navigationResponse(c, result, err)
		}
		return catalogResponse(c, result, err)
	}
}

func catalogResponse(c fiber.Ctx, result any, err error) error {
	if err == nil {
		c.Set("Cache-Control", "public, max-age=60")
		return c.JSON(result)
	}
	c.Set("Cache-Control", "no-store")
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		return fiber.ErrNotFound
	case errors.Is(err, catalog.ErrInvalid):
		return fiber.ErrBadRequest
	default:
		return fiber.ErrServiceUnavailable
	}
}
