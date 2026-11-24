package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"uuid"
)

func GameNewsArticle(service *catalog.Service, market catalog.ChannelMarket) fiber.Handler {
	return func(c fiber.Ctx) error {
		article := c.Params("article")
		id, err := uuid.Parse(article)
		if err != nil || id.String() != article || len(c.Request().URI().QueryString()) != 0 {
			return catalogResponse(c, nil, catalog.ErrNotFound)
		}
		out, err := service.PublicGameNewsArticle(c.Context(), market, c.Params("publicID"), id)
		return catalogResponse(c, out, err)
	}
}
