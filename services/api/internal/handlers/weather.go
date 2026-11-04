package handlers

import (
	"context"
	"net/netip"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/geoip"
	"github.com/librecosta/gofurry/services/api/internal/weather"
)

type weatherReader interface {
	Get(context.Context, float64, float64, string) weather.Result
}

type weatherAttribution struct {
	Weather             string   `json:"weather"`
	WeatherURL          string   `json:"weather_url"`
	Geolocation         string   `json:"geolocation"`
	GeolocationURL      string   `json:"geolocation_url"`
	WeatherAttributions []string `json:"weather_attributions"`
}

func Weather(cities *geoip.Resolver, service weatherReader) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		lang := c.Query("lang", "zh")
		if lang != "zh" && lang != "en" {
			return fiber.ErrBadRequest
		}
		ip, _ := netip.ParseAddr(c.IP())
		location := cities.Resolve(ip)
		result := weather.Result{State: "unavailable", Attributions: []string{}}
		if location != nil {
			result = service.Get(c.Context(), location.Latitude, location.Longitude, lang)
		}
		return c.JSON(struct {
			State       string             `json:"state"`
			Location    *geoip.Location    `json:"location"`
			Weather     *weather.Current   `json:"weather"`
			Attribution weatherAttribution `json:"attribution"`
		}{State: result.State, Location: location, Weather: result.Weather, Attribution: weatherAttribution{"QWeather", "https://www.qweather.com", "DB-IP", "https://db-ip.com", result.Attributions}})
	}
}
