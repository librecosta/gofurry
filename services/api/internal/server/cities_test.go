package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestNearestCityHTTPPrivacyAndLimits(t *testing.T) {
	for _, region := range []string{"CN", "INTL"} {
		var cfg platform.Config
		cfg.Deployment.Region = region
		var logs bytes.Buffer
		s := New(platform.NewLogger(&logs, "development"), cfg)
		for i := range 7 {
			req := httptest.NewRequest("POST", "/api/v1/start/cities/nearest?region=OTHER", strings.NewReader(`{"latitude":30.671234,"longitude":104.071234}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GoFurry-Region", "OTHER")
			req.Header.Set("X-Forwarded-For", "8.8.8.8")
			res, err := s.App.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.Header.Get("X-GoFurry-Region") != region || res.Header.Get("X-GoFurry-Content-Mode") != "SFW" {
				t.Fatal("nearest region boundary missing")
			}
			if i < 6 {
				if res.StatusCode != 200 || !strings.Contains(string(body), `"city":null`) {
					t.Fatal("optional nearest contract changed")
				}
			} else if res.StatusCode != 429 || res.Header.Get("Retry-After") != "60" {
				t.Fatal("nearest client quota missing")
			}
		}
		for _, path := range []string{"/api/v1/start/weather", "/api/v1/start/location", "/api/v1/start/backgrounds", "/api/v1/start/cities?q=Chengdu", "/api/v1/start/search/suggestions?q="} {
			res, _ := s.App.Test(httptest.NewRequest("GET", path, nil))
			res.Body.Close()
			if res.StatusCode != 200 {
				t.Fatal("nearest quota affected existing API")
			}
		}
		for _, secret := range []string{"30.671234", "104.071234", "latitude", "longitude", "8.8.8.8", "region=OTHER"} {
			if strings.Contains(logs.String(), secret) {
				t.Fatal("device location request data logged")
			}
		}
	}
	var cfg platform.Config
	cfg.Deployment.Region = "CN"
	cfg.Server.TrustedProxies = []string{"127.0.0.1"}
	s := New(platform.NewLogger(io.Discard, "production"), cfg)
	base := listenLocationTest(t, s)
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	for i := range 61 {
		req, _ := http.NewRequest("POST", base+"/api/v1/start/cities/nearest", strings.NewReader(`{"latitude":0,"longitude":0}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("8.8.0.%d", i+1))
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		want := 200
		if i == 60 {
			want = 429
		}
		if res.StatusCode != want {
			t.Fatal("nearest global quota missing")
		}
	}
}

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
