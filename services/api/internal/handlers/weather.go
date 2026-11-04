package handlers

import (
	"context"
	"net/netip"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/cities"
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

func Weather(ipCities *geoip.Resolver, service weatherReader, catalog *cities.Catalog) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		lang := c.Query("lang", "zh")
		if lang != "zh" && lang != "en" {
			return fiber.ErrBadRequest
		}
		id, source := c.Query("city_id"), "ip"
		var location *geoip.Location
		if c.Request().URI().QueryArgs().Has("city_id") {
			if !cities.ValidID(id) {
				return fiber.ErrBadRequest
			}
			if catalog == nil {
				return fiber.ErrServiceUnavailable
			}
			location = catalog.Resolve(id)
			if location == nil {
				return fiber.ErrNotFound
			}
			source = "manual"
		} else {
			ip, _ := netip.ParseAddr(c.IP())
			location = ipCities.Resolve(ip)
		}
		result := weather.Result{State: "unavailable", Attributions: []string{}}
		if location != nil {
			result = service.Get(c.Context(), location.Latitude, location.Longitude, lang)
		}
		return c.JSON(struct {
			State          string             `json:"state"`
			Location       *geoip.Location    `json:"location"`
			Weather        *weather.Current   `json:"weather"`
			Attribution    weatherAttribution `json:"attribution"`
			LocationSource string             `json:"location_source"`
			CityID         string             `json:"city_id,omitempty"`
		}{State: result.State, Location: location, Weather: result.Weather, LocationSource: source, CityID: id, Attribution: attributionFor(source, result.Attributions)})
	}
}

func attributionFor(source string, originals []string) weatherAttribution {
	name, url := "DB-IP", "https://db-ip.com"
	if source == "manual" {
		name, url = "GeoNames", "https://www.geonames.org"
	}
	return weatherAttribution{"QWeather", "https://www.qweather.com", name, url, originals}
}
