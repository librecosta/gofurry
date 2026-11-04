package handlers

import (
	"encoding/json/v2"
	"math"
	"mime"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/cities"
)

func Cities(catalog *cities.Catalog) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		q := c.Query("q")
		if !cities.ValidQuery(q) {
			return fiber.ErrBadRequest
		}
		state := "ready"
		if catalog == nil {
			state = "unavailable"
		}
		return c.JSON(struct {
			State  string        `json:"state"`
			Cities []cities.City `json:"cities"`
		}{state, catalog.Search(q)})
	}
}

// POST body only: device coordinates must not enter URLs or access logs.
func NearestCity(catalog *cities.Catalog) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		media, _, err := mime.ParseMediaType(c.Get("Content-Type"))
		if err != nil || media != "application/json" {
			return fiber.ErrUnsupportedMediaType
		}
		if len(c.Body()) > 256 {
			return fiber.ErrRequestEntityTooLarge
		}
		var position struct {
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
		}
		if err := json.Unmarshal(c.Body(), &position, json.RejectUnknownMembers(true)); err != nil || position.Latitude == nil || position.Longitude == nil {
			return fiber.ErrBadRequest
		}
		lat, lon := *position.Latitude, *position.Longitude
		if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
			return fiber.ErrBadRequest
		}
		city, state := catalog.Nearest(lat, lon), "unavailable"
		if city != nil {
			state = "ready"
		}
		return c.JSON(struct {
			State string       `json:"state"`
			City  *cities.City `json:"city"`
		}{state, city})
	}
}
