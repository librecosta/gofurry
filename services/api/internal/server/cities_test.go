package server

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestCitiesHTTPBoundaryAndLimit(t *testing.T) {
	for _, region := range []string{"CN", "INTL"} {
		var cfg platform.Config
		cfg.Deployment.Region = region
		cfg.Cities = &platform.CitiesConfig{IndexPath: filepath.Join(t.TempDir(), "missing.json")}
		s := New(platform.NewLogger(io.Discard, "production"), cfg)
		for i := range 41 {
			req := httptest.NewRequest("GET", "/api/v1/start/cities?q=Chengdu&region=OTHER", nil)
			req.Header.Set("X-GoFurry-Region", "OTHER")
			req.Header.Set("X-Forwarded-For", "8.8.8.8")
			res, err := s.App.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.Header.Get("X-GoFurry-Region") != region || res.Header.Get("X-GoFurry-Content-Mode") != "SFW" {
				t.Fatal("request crossed region")
			}
			if i < 40 {
				if res.StatusCode != 200 || !strings.Contains(string(body), `"state":"unavailable"`) {
					t.Fatal("catalog degradation failed")
				}
			} else if res.StatusCode != 429 || res.Header.Get("Retry-After") != "60" {
				t.Fatal("city client quota missing")
			}
		}
		for _, path := range []string{"/api/v1/start/weather", "/api/v1/start/location", "/api/v1/start/backgrounds"} {
			res, _ := s.App.Test(httptest.NewRequest("GET", path, nil))
			res.Body.Close()
			if res.StatusCode != 200 {
				t.Fatal("city quota affected existing API")
			}
		}
	}
}

func TestCitiesCorruptCatalogDegrades(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte(`not JSON`), 0600); err != nil {
		t.Fatal(err)
	}
	var cfg platform.Config
	cfg.Deployment.Region = "CN"
	cfg.Cities = &platform.CitiesConfig{IndexPath: path}
	s := New(platform.NewLogger(io.Discard, "production"), cfg)
	for _, path := range []string{"/api/v1/status", "/api/v1/start/weather", "/api/v1/start/location"} {
		res, _ := s.App.Test(httptest.NewRequest("GET", path, nil))
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatal("corrupt catalog stopped another route")
		}
	}
}
