package handlers

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/geoip"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/weather"
	"github.com/valyala/fasthttp"
)

type fakeWeatherReader struct {
	calls    int
	lat, lon float64
	lang     string
	state    string
}

func (f *fakeWeatherReader) Get(_ context.Context, lat, lon float64, lang string) weather.Result {
	f.calls++
	f.lat, f.lon, f.lang = lat, lon, lang
	if f.state == "unavailable" {
		return weather.Result{State: f.state, Attributions: []string{}}
	}
	return weather.Result{State: f.state, Weather: &weather.Current{TemperatureC: 22.5, ConditionCode: "101", ConditionText: "test fixture", FetchedAt: time.Unix(1800000000, 0).UTC()}, Attributions: []string{"provider attribution one", "provider attribution two"}}
}

func TestWeatherHandlerUnknownIPDoesNotFetch(t *testing.T) {
	app := fiber.New()
	ctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(ctx)
	service := &fakeWeatherReader{state: "ready"}
	if err := Weather(nil, service, nil)(ctx); err != nil {
		t.Fatal(err)
	}
	if service.calls != 0 || !strings.Contains(string(ctx.Response().Body()), `"weather":null`) {
		t.Fatal("unknown city reached provider")
	}
}

func TestLocalWeatherHandlerUsesOnlyResolvedCity(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "data", "geoip", "dbip-city-lite-2026-11.mmdb")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("local November MMDB not installed")
	}
	cities, err := geoip.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer cities.Close()
	app := fiber.New(fiber.Config{JSONEncoder: platform.MarshalJSON, JSONDecoder: platform.UnmarshalJSON})
	for _, state := range []string{"ready", "stale", "unavailable"} {
		for _, lang := range []string{"zh", "en"} {
			fast := &fasthttp.RequestCtx{}
			fast.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("223.5.5.5"), Port: 12345})
			fast.Request.SetRequestURI("/api/v1/start/weather?lang=" + lang + "&ip=8.8.8.8&latitude=0&longitude=0&region=INTL&api_host=attacker.invalid")
			fast.Request.Header.Set("Cookie", "gf-region=INTL")
			fast.Request.Header.Set("X-Forwarded-For", "8.8.8.8")
			ctx := app.AcquireCtx(fast)
			service := &fakeWeatherReader{state: state}
			if err := Weather(cities, service, nil)(ctx); err != nil {
				t.Fatal(err)
			}
			var response struct {
				State       string             `json:"state"`
				Location    *geoip.Location    `json:"location"`
				Weather     *weather.Current   `json:"weather"`
				Attribution weatherAttribution `json:"attribution"`
			}
			if err := platform.UnmarshalJSON(ctx.Response().Body(), &response); err != nil {
				t.Fatal(err)
			}
			if service.calls != 1 || service.lat != 30.27 || service.lon != 120.16 || service.lang != lang || response.Location == nil || response.Location.CountryCode != "CN" || response.State != state {
				t.Fatal("client input replaced the resolved city or language mismatch")
			}
			if state == "unavailable" {
				if response.Weather != nil {
					t.Fatal("unavailable contains fabricated weather")
				}
			} else if response.Weather == nil || response.Weather.TemperatureC != 22.5 || len(response.Attribution.WeatherAttributions) != 2 {
				t.Fatal("weather/attributions lost")
			}
			if !strings.Contains(string(ctx.Response().Header.Peek("Cache-Control")), "no-store") || response.Attribution.Geolocation != "DB-IP" || response.Attribution.Weather != "QWeather" {
				t.Fatal("privacy/attribution contract missing")
			}
			app.ReleaseCtx(ctx)
		}
	}
}
