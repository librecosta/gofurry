package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/geoip"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/rs/zerolog"
)

func TestLocationUnavailableAndRateLimit(t *testing.T) {
	for _, database := range []string{"", "missing-private-city-file"} {
		var cfg platform.Config
		cfg.Deployment.Region = "CN"
		cfg.GeoIP = &platform.GeoIPConfig{DatabasePath: database}
		var logs bytes.Buffer
		s := New(platform.NewLogger(&logs, "production"), cfg)
		for i := range 31 {
			req := httptest.NewRequest("GET", "/api/v1/start/location?ip=8.8.8.8&region=INTL", nil)
			req.Header.Set("X-Forwarded-For", "8.8.8.8")
			res, err := s.App.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.Header.Get("X-GoFurry-Region") != "CN" || res.Header.Get("X-GoFurry-Content-Mode") != "SFW" {
				t.Fatal("regional boundary missing")
			}
			if !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
				t.Fatal("personalized response cached")
			}
			if i < 30 {
				if res.StatusCode != 200 || string(body) != `{"state":"unavailable","source":"ip","location":null}` {
					t.Fatalf("unavailable contract: %d %s", res.StatusCode, body)
				}
			} else if res.StatusCode != 429 || !strings.Contains(string(body), `"code":429`) || !strings.Contains(string(body), `"request_id"`) || res.Header.Get("Retry-After") != "60" {
				t.Fatalf("rate/error contract: %d %s", res.StatusCode, body)
			}
		}
		for _, path := range []string{"/api/v1/status", "/api/v1/start/backgrounds", "/api/v1/start/search/suggestions?q=", "/livez"} {
			res, err := s.App.Test(httptest.NewRequest("GET", path, nil))
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != 200 {
				t.Fatalf("location limiter affected %s: %d", path, res.StatusCode)
			}
		}
		for _, private := range []string{"8.8.8.8", "missing-private-city-file", "latitude", "longitude"} {
			if strings.Contains(logs.String(), private) {
				t.Fatalf("location details leaked to logs: %s", private)
			}
		}
	}
}

func TestLocationCorruptDatabaseDegrades(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private-corrupt.mmdb")
	if err := os.WriteFile(path, []byte("not a database"), 0600); err != nil {
		t.Fatal(err)
	}
	var cfg platform.Config
	cfg.Deployment.Region = "INTL"
	cfg.GeoIP = &platform.GeoIPConfig{DatabasePath: path}
	var logs bytes.Buffer
	s := New(platform.NewLogger(&logs, "production"), cfg)
	res, err := s.App.Test(httptest.NewRequest("GET", "/api/v1/start/location", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), `"state":"unavailable"`) {
		t.Fatal(string(body))
	}
	if !strings.Contains(logs.String(), "city database unavailable") || strings.Contains(logs.String(), "private-corrupt") {
		t.Fatal("missing bounded startup diagnostic or exposed path")
	}
}

// Real TCP connections exercise Fiber's peer-based trust check; App.Test uses a
// placeholder 0.0.0.0 peer and cannot demonstrate this security boundary.
func listenLocationTest(t *testing.T, s *Server) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	address := make(chan string, 1)
	stopped := make(chan error, 1)
	s.App.Hooks().OnListen(func(data fiber.ListenData) error { address <- "http://127.0.0.1:" + data.Port; return nil })
	go func() { stopped <- s.Listen(ctx, "127.0.0.1:0") }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-stopped:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("test API did not drain")
		}
	})
	select {
	case base := <-address:
		return base
	case <-time.After(5 * time.Second):
		t.Fatal("test API did not start")
	}
	return ""
}

func TestLocalLocationHTTPTrustAndRegions(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "data", "geoip", "dbip-city-lite-2026-11.mmdb")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("local November 2026 MMDB not installed")
	}
	for _, tc := range []struct {
		name, region string
		proxies      []string
		trusted      bool
	}{
		{"default untrusted", "CN", nil, false},
		{"different peer untrusted", "CN", []string{"10.0.0.2/32"}, false},
		{"explicit CIDRs", "CN", []string{"127.0.0.1/32", "10.0.0.2/32"}, true},
		{"INTL exact peer", "INTL", []string{"127.0.0.1", "10.0.0.2"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg platform.Config
			cfg.Deployment.Region = tc.region
			cfg.Server.TrustedProxies = tc.proxies
			cfg.GeoIP = &platform.GeoIPConfig{DatabasePath: path}
			s := New(zerolog.Nop(), cfg)
			base := listenLocationTest(t, s)
			client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
			defer client.CloseIdleConnections()
			for _, query := range []struct{ forwarded, country string }{
				{"8.8.8.8", "US"},
				{"223.5.5.5", "CN"},
				{"2001:4860:4860::8888", "CA"},
				{"2400:3200::1", "CN"},
				{"1.1.1.1, 223.5.5.5, 10.0.0.2", "CN"}, // Ignore injected left-hand address.
				{"1.1.1.1, 10.0.0.3", ""},              // Never skip an untrusted intermediary.
				{"127.0.0.1", ""}, {"10.0.0.1", ""}, {"invalid", ""}, {"", ""},
			} {
				req, _ := http.NewRequest("GET", base+"/api/v1/start/location?ip=8.8.8.8&region=OTHER&locale=OTHER", nil)
				req.Header.Set("X-Forwarded-For", query.forwarded)
				req.Header.Set("X-Real-IP", "8.8.8.8")
				req.Header.Set("Forwarded", "for=8.8.8.8")
				req.Header.Set("CF-Connecting-IP", "8.8.8.8")
				req.Header.Set("Cookie", "gf-region=OTHER")
				res, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(res.Body)
				res.Body.Close()
				var result struct {
					State    string          `json:"state"`
					Source   string          `json:"source"`
					Location *geoip.Location `json:"location"`
				}
				if err := platform.UnmarshalJSON(body, &result); err != nil {
					t.Fatal(err)
				}
				if res.StatusCode != 200 || result.Source != "ip" || res.Header.Get("X-GoFurry-Region") != tc.region || res.Header.Get("X-GoFurry-Content-Mode") != "SFW" {
					t.Fatalf("region/contract changed: %s", body)
				}
				if tc.trusted && query.country != "" {
					if result.State != "ready" || result.Location == nil || result.Location.CountryCode != query.country {
						t.Fatalf("trusted lookup: %s", body)
					}
				} else if result.State != "unavailable" || result.Location != nil {
					t.Fatalf("untrusted/invalid IP selected a city: %s", body)
				}
				for _, private := range []string{"8.8.8.8", "223.5.5.5", "2001:4860", "database_path", "geoname_id", ".mmdb"} {
					if bytes.Contains(body, []byte(private)) {
						t.Fatalf("private/raw record leaked: %s", body)
					}
				}
			}
		})
	}
}
