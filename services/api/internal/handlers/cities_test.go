package handlers

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/cities"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestManualWeatherAndCitySearch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	data := `{"format":1,"source":"GeoNames cities5000","cities":[{"city_id":"1815286","name":"Chengdu","names":{"zh":"成都","en":"Chengdu"},"aliases":["成都"],"country_code":"CN","admin":"Sichuan","latitude":30.67,"longitude":104.07}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := cities.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeWeatherReader{state: "ready"}
	app := fiber.New(fiber.Config{JSONEncoder: platform.MarshalJSON})
	app.Get("/weather", Weather(nil, service, catalog))
	app.Get("/cities", Cities(catalog))
	for _, lang := range []string{"zh", "en"} {
		res, err := app.Test(httptest.NewRequest("GET", "/weather?city_id=1815286&lang="+lang+"&latitude=0&longitude=0&ip=8.8.8.8&api_host=attacker.invalid", nil))
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		body, _ := io.ReadAll(res.Body)
		if err := platform.UnmarshalJSON(body, &value); err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 200 || value["location_source"] != "manual" || value["city_id"] != "1815286" || service.lat != 30.67 || service.lon != 104.07 || service.lang != lang {
			t.Fatal("manual resolution bypassed or failed")
		}
		attr := value["attribution"].(map[string]any)
		if attr["geolocation"] != "GeoNames" || attr["weather"] != "QWeather" {
			t.Fatal("incorrect source")
		}
	}
	before := service.calls
	for _, tc := range []struct {
		path string
		code int
	}{{"/weather?city_id=", 400}, {"/weather?city_id=-1", 400}, {"/weather?city_id=999999", 404}, {"/weather?city_id=https://example.com", 400}, {"/weather?city_id=1815286&lang=fr", 400}, {"/weather?latitude=30&longitude=104", 200}, {"/cities?q=%3Cscript%3E", 400}, {"/cities?q=成都", 200}} {
		res, err := app.Test(httptest.NewRequest("GET", tc.path, nil))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.code {
			t.Fatalf("%s: %d", tc.path, res.StatusCode)
		}
	}
	if service.calls != before {
		t.Fatal("invalid city/loopback called weather provider")
	}
	disabled := fiber.New(fiber.Config{JSONEncoder: platform.MarshalJSON})
	disabled.Get("/weather", Weather(nil, service, nil))
	disabled.Get("/cities", Cities(nil))
	for _, tc := range []struct {
		path string
		code int
	}{{"/weather", 200}, {"/weather?city_id=1815286", 503}, {"/cities?q=成都", 200}} {
		res, _ := disabled.Test(httptest.NewRequest("GET", tc.path, nil))
		res.Body.Close()
		if res.StatusCode != tc.code {
			t.Fatal("disabled catalog affected degradation")
		}
	}
}
