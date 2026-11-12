package handlers

import (
	json "encoding/json/v2"
	"errors"
	"mime"
	"net/netip"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/navigation"
)

func NavigationHome(service *navigation.Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Request().URI().QueryArgs().Len() != 0 {
			return navigationResponse(c, nil, catalog.ErrInvalid)
		}
		result, err := service.Home(c.Context())
		return navigationResponse(c, result, err)
	}
}

func NavigationDirectory(service *navigation.Service, region string) fiber.Handler {
	return func(c fiber.Ctx) error {
		f := navigation.Filters{Locale: "en"}
		if region == "CN" {
			f.Locale = "zh"
		}
		if len(c.Request().URI().QueryString()) > 2048 {
			return navigationResponse(c, nil, catalog.ErrInvalid)
		}
		var cursor string
		invalid := false
		seen := map[string]bool{}
		c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
			name, text := string(key), string(value)
			if seen[name] || (name != "q" && text == "") {
				invalid = true
				return
			}
			seen[name] = true
			switch name {
			case "q":
				f.Keyword = text
			case "category":
				f.Categories = strings.Split(text, ",")
			case "tags":
				f.Tags = strings.Split(text, ",")
			case "language":
				f.Languages = strings.Split(text, ",")
			case "sort":
				f.Sort = text
			case "locale":
				f.Locale = text
			case "cursor":
				cursor = text
			case "page_size":
				n, err := strconv.ParseInt(text, 10, 32)
				invalid = invalid || err != nil || n < 1 || n > 48
				f.PageSize = int32(n)
			default:
				invalid = true
			}
		})
		if invalid {
			return navigationResponse(c, nil, catalog.ErrInvalid)
		}
		result, err := service.Directory(c.Context(), f, cursor)
		return navigationResponse(c, result, err)
	}
}

func NavigationStats(service *navigation.Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Request().URI().QueryArgs().Len() != 0 {
			return navigationResponse(c, nil, catalog.ErrInvalid)
		}
		result, err := service.Stats(c.Context(), c.Params("publicID"))
		return navigationResponse(c, result, err)
	}
}

func NavigationRelated(service *navigation.Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Request().URI().QueryArgs().Len() != 0 {
			return navigationResponse(c, nil, catalog.ErrInvalid)
		}
		result, err := service.Related(c.Context(), c.Params("publicID"))
		return navigationResponse(c, result, err)
	}
}

func navigationResponse(c fiber.Ctx, result any, err error) error {
	if err == nil {
		c.Set("Cache-Control", "no-store")
		return c.JSON(result)
	}
	return catalogResponse(c, nil, err)
}

func NavigationInteraction(counter *navigation.Counter, origin string, production, trustedPeers bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		if c.Path() != "/api/v1/sites/"+c.Params("publicID")+"/interactions" {
			return fiber.ErrNotFound
		}
		if origin == "" {
			return fiber.ErrServiceUnavailable
		}
		if c.Get("Origin") != origin || c.Get("Sec-Fetch-Site") != "same-origin" {
			return fiber.ErrForbidden
		}
		if c.Request().URI().QueryArgs().Len() != 0 || len(c.BodyRaw()) > 80 ||
			c.Get("Content-Encoding") != "" {
			return fiber.ErrBadRequest
		}
		contentType, params, err := mime.ParseMediaType(c.Get("Content-Type"))
		if err != nil || contentType != "application/json" ||
			len(params) > 0 && (len(params) != 1 || !strings.EqualFold(params["charset"], "utf-8")) {
			return fiber.ErrUnsupportedMediaType
		}
		ip, ok := interactionIP(c, production, trustedPeers)
		if !ok {
			return fiber.ErrForbidden
		}
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(c.BodyRaw(), &event, json.RejectUnknownMembers(true)); err != nil ||
			event.Type != navigation.DetailView && event.Type != navigation.OutboundClick {
			return fiber.ErrBadRequest
		}
		counted, err := counter.Count(c.Context(), ip, c.Params("publicID"), event.Type)
		if errors.Is(err, navigation.ErrLimited) {
			c.Set("Retry-After", "60")
			return fiber.ErrTooManyRequests
		}
		if err != nil {
			return navigationResponse(c, nil, err)
		}
		if counted {
			return c.Status(fiber.StatusNoContent).Send(nil)
		}
		return c.Status(fiber.StatusAccepted).Send(nil)
	}
}

func interactionIP(c fiber.Ctx, production, trustedPeers bool) (netip.Addr, bool) {
	forwarded := c.Get(fiber.HeaderXForwardedFor)
	var raw string
	if trustedPeers && c.IsProxyTrusted() {
		// The edge supplies exactly one address, replacing inbound forwarding headers.
		raw = forwarded
	} else if !production && forwarded == "" && c.Get("Forwarded") == "" && c.Get("X-Real-IP") == "" {
		raw = c.RequestCtx().RemoteIP().String()
	} else {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(raw)
	return ip.Unmap(), err == nil && ip.Zone() == "" && !ip.IsUnspecified() && !ip.IsMulticast()
}
