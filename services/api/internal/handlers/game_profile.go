package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
)

func gameProfileParams(c fiber.Ctx, names ...string) (map[string]string, bool) {
	params := make(map[string]string, len(names))
	valid := true
	c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
		if _, exists := params[string(key)]; exists || len(value) == 0 || len(value) > 512 {
			valid = false
			return
		}
		for _, name := range names {
			if name == string(key) {
				params[name] = string(value)
				return
			}
		}
		valid = false
	})
	return params, valid
}

func profileLocale(market catalog.ChannelMarket, value string) string {
	if value != "" {
		return value
	}
	if market.Region == "INTL" {
		return "en"
	}
	return "zh"
}

func GameProfile(service *catalog.Service, market catalog.ChannelMarket) fiber.Handler {
	compact := CatalogDetail(service.PublishedGame)
	return func(c fiber.Ctx) error {
		if !c.Request().URI().QueryArgs().Has("profile") {
			return compact(c)
		}
		params, valid := gameProfileParams(c, "profile", "locale")
		if !valid || params["profile"] != "1" {
			return catalogResponse(c, nil, catalog.ErrInvalid)
		}
		data, err := service.PublicGameProfile(c.Context(), market, c.Params("publicID"), profileLocale(market, params["locale"]))
		return catalogResponse(c, data, err)
	}
}

func GameProfileSecondary(service *catalog.Service, market catalog.ChannelMarket, news bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Params("publicID")
		if news {
			params, valid := gameProfileParams(c, "app_id", "after")
			if !valid {
				return catalogResponse(c, nil, catalog.ErrInvalid)
			}
			data, err := service.PublicGameNews(c.Context(), market, id, params["app_id"], params["after"])
			return catalogResponse(c, data, err)
		}
		params, valid := gameProfileParams(c, "app_id", "country", "locale", "range")
		days := 30
		if v := params["range"]; v != "" {
			n, err := strconv.ParseInt(v, 10, 16)
			valid = valid && err == nil && strconv.FormatInt(n, 10) == v
			days = int(n)
		}
		if !valid {
			return catalogResponse(c, nil, catalog.ErrInvalid)
		}
		data, err := service.PublicGameInsights(c.Context(), market, id, params["app_id"], params["country"], profileLocale(market, params["locale"]), int32(days))
		return catalogResponse(c, data, err)
	}
}
