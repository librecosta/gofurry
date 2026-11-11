package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/yearbook"
)

func Yearbooks(service *yearbook.Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		years, err := service.Years(c.Context(), c.Params("publicID"))
		return yearbookResponse(c, struct {
			Years []int32 `json:"years"`
		}{years}, err)
	}
}

func Yearbook(service *yearbook.Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		year, err := yearbook.ParseYear(c.Params("year"))
		if err != nil {
			return yearbookResponse(c, nil, yearbook.ErrMissing)
		}
		doc, err := service.Public(c.Context(), c.Params("publicID"), year)
		return yearbookResponse(c, doc, err)
	}
}

func yearbookResponse(c fiber.Ctx, result any, err error) error {
	if errors.Is(err, yearbook.ErrMissing) {
		err = catalog.ErrNotFound
	}
	return navigationResponse(c, result, err)
}
