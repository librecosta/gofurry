package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestWeatherHTTPBoundaryLimitAndDegradation(t *testing.T) {
	for _, region := range []string{"CN", "INTL"} {
		var cfg platform.Config
		cfg.Deployment.Region = region
		cfg.QWeather = &platform.QWeatherConfig{Enabled: true} // Missing values degrade, never stop the API.
		var logs bytes.Buffer
		s := New(platform.NewLogger(&logs, "production"), cfg)
		for i := range 21 {
			req := httptest.NewRequest("GET", "/api/v1/start/weather?lang=en&ip=223.5.5.5&latitude=30&longitude=104&region=OTHER", nil)
			req.Header.Set("X-Forwarded-For", "223.5.5.5")
			req.Header.Set("Cookie", "gf-region=OTHER")
			res, err := s.App.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.Header.Get("X-GoFurry-Region") != region || res.Header.Get("X-GoFurry-Content-Mode") != "SFW" || !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
				t.Fatal("regional/privacy boundary missing")
			}
			if i < 20 {
				if res.StatusCode != 200 || !bytes.Contains(body, []byte(`"state":"unavailable"`)) || !bytes.Contains(body, []byte(`"location":null`)) || !bytes.Contains(body, []byte(`"weather":null`)) {
					t.Fatal("unavailable contract changed")
				}
			} else if res.StatusCode != 429 || res.Header.Get("Retry-After") != "60" || !bytes.Contains(body, []byte(`"request_id"`)) {
				t.Fatal("weather rate/error contract missing")
			}
		}
		for _, path := range []string{"/api/v1/status", "/api/v1/start/location", "/api/v1/start/backgrounds", "/api/v1/start/search/suggestions?q="} {
			res, err := s.App.Test(httptest.NewRequest("GET", path, nil))
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != 200 {
				t.Fatal("weather quota affected another API")
			}
		}
		for _, value := range []string{"223.5.5.5", "latitude", "Authorization", "Bearer", "PRIVATE KEY"} {
			if strings.Contains(logs.String(), value) {
				t.Fatal("private weather request data logged")
			}
		}
	}
	var cfg platform.Config
	cfg.Deployment.Region = "CN"
	s := New(platform.NewLogger(io.Discard, "production"), cfg)
	res, err := s.App.Test(httptest.NewRequest(http.MethodGet, "/api/v1/start/weather?lang=fr", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatal("unsupported language accepted")
	}
}

func TestWeatherHTTPGlobalLimit(t *testing.T) {
	var cfg platform.Config
	cfg.Deployment.Region = "CN"
	cfg.Server.TrustedProxies = []string{"127.0.0.1"}
	s := New(platform.NewLogger(io.Discard, "production"), cfg)
	base := listenLocationTest(t, s)
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	for i := range 121 {
		req, _ := http.NewRequest("GET", base+"/api/v1/start/weather", nil)
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("8.8.0.%d", i+1))
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		want := 200
		if i == 120 {
			want = 429
		}
		if res.StatusCode != want {
			t.Fatal("global weather quota did not bound distinct clients")
		}
	}
}
