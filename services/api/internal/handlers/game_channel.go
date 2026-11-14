package handlers

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
)

func GameChannel(service *catalog.Service, market catalog.ChannelMarket, view string) fiber.Handler {
	return func(c fiber.Ctx) error {
		var filters catalog.ChannelCriteria
		filters.Locale = "zh"
		if market.Region == "INTL" {
			filters.Locale = "en"
		}
		seen := map[string]bool{}
		invalid := false
		c.Request().URI().QueryArgs().VisitAll(func(rawKey, rawValue []byte) {
			key, value := string(rawKey), string(rawValue)
			if seen[key] || value == "" || len(value) > 2048 {
				invalid = true
				return
			}
			seen[key] = true
			switch {
			case key == "locale":
				filters.Locale = value
			case key == "after" && view != "home":
				filters.After = value
			case key == "limit" && view != "home":
				n, err := strconv.ParseInt(value, 10, 32)
				invalid = invalid || err != nil || n < 1 || n > 48
				filters.Limit = int32(n)
			case key == "q" && (view == "directory" || view == "collections"):
				filters.Keyword = value
			case key == "genre" && view == "directory":
				filters.Genres = strings.Split(value, ",")
			case key == "store" && view == "directory":
				filters.Store = value
			case key == "system" && view == "directory":
				filters.System = value
			case key == "phase" && view == "directory":
				filters.Phase = value
			case key == "price" && view == "directory":
				filters.Price = value
			case key == "sort" && view == "directory", key == "view" && view == "members":
				filters.Sort = value
			default:
				invalid = true
			}
		})
		if invalid {
			return catalogResponse(c, nil, catalog.ErrInvalid)
		}
		ctx := c.Context()
		switch view {
		case "home":
			data, err := service.GameHome(ctx, market, filters.Locale)
			return catalogResponse(c, data, err)
		case "directory":
			data, err := service.GameDirectory(ctx, market, filters)
			return catalogResponse(c, data, err)
		case "collections":
			data, err := service.GameCollections(ctx, market, filters)
			return catalogResponse(c, data, err)
		default:
			data, err := service.PublicGameCollection(ctx, market, c.Params("publicID"), filters)
			return catalogResponse(c, data, err)
		}
	}
}
