package handlers

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	app.Post("/nearest", NearestCity(catalog))
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
	for _, tc := range []struct {
		body, media string
		code        int
		ready       bool
	}{
		{`{"latitude":30.670123,"longitude":104.070456}`, "application/json; charset=utf-8", 200, true},
		{`{"latitude":0,"longitude":0}`, "application/json", 200, false},
		{`{"latitude":30,"longitude":104}`, "text/plain", 415, false},
		{`{"latitude":91,"longitude":104}`, "application/json", 400, false},
		{`{"latitude":30,"longitude":-181}`, "application/json", 400, false},
		{`{"latitude":30}`, "application/json", 400, false},
		{`{"latitude":null,"longitude":104}`, "application/json", 400, false},
		{`{"latitude":30,"latitude":31,"longitude":104}`, "application/json", 400, false},
		{`{"latitude":30,"longitude":104,"ip":"8.8.8.8"}`, "application/json", 400, false},
		{`{"latitude":"NaN","longitude":104}`, "application/json", 400, false},
		{`{"latitude":1e999,"longitude":104}`, "application/json", 400, false},
		{`{`, "application/json", 400, false},
		{strings.Repeat(" ", 257), "application/json", 413, false},
	} {
		req := httptest.NewRequest("POST", "/nearest", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.media)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != tc.code {
			t.Fatalf("nearest %s: status %d", tc.body, res.StatusCode)
		}
		if tc.code == 200 {
			var value struct {
				State string       `json:"state"`
				City  *cities.City `json:"city"`
			}
			if err := platform.UnmarshalJSON(body, &value); err != nil {
				t.Fatal(err)
			}
			if (value.State == "ready") != tc.ready || (value.City != nil) != tc.ready {
				t.Fatal("incorrect nearest state")
			}
			if tc.ready && value.City.ID != "1815286" {
				t.Fatal("unverified city returned")
			}
			if strings.Contains(string(body), "latitude") || strings.Contains(string(body), "longitude") || res.Header.Get("Cache-Control") != "private, no-store" {
				t.Fatal("position exposed or cached")
			}
		}
	}
	if service.calls != before {
		t.Fatal("nearest endpoint called weather provider")
	}
	disabled := fiber.New(fiber.Config{JSONEncoder: platform.MarshalJSON})
	disabled.Get("/weather", Weather(nil, service, nil))
	disabled.Get("/cities", Cities(nil))
	disabled.Post("/nearest", NearestCity(nil))
	req := httptest.NewRequest("POST", "/nearest", strings.NewReader(`{"latitude":30.67,"longitude":104.07}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := disabled.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), `"state":"unavailable"`) {
		t.Fatal("disabled nearest must degrade")
	}
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
