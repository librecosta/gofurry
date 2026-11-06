package server

import (
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/rs/zerolog"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestUnconfiguredCatalogNeverReadsStaticOrStorage(t *testing.T) {
	var calls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer backend.Close()
	var cfg platform.Config
	cfg.Deployment.Region = "CN"
	cfg.Storage = &platform.StorageConfig{Endpoint: backend.URL, Region: "auto", Bucket: "development", AccessKeyID: "key", SecretAccessKey: "secret", PublicBaseURL: "https://public.example"}
	s := New(zerolog.Nop(), cfg)
	res, e := s.App.Test(httptest.NewRequest("GET", "/api/v1/start/backgrounds?region=INTL", nil))
	if e != nil {
		t.Fatal(e)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || strings.TrimSpace(string(body)) != `{"backgrounds":[],"total":0,"page":1,"page_size":9}` || calls.Load() != 0 {
		t.Fatalf("static/storage fallback: %d %s calls=%d", res.StatusCode, body, calls.Load())
	}
	if res.Header.Get("X-GoFurry-Region") != "CN" || res.Header.Get("X-GoFurry-Content-Mode") != "SFW" {
		t.Fatal("region contract changed")
	}
	for _, path := range []string{"/internal/v1/start/wallpapers", "/internal/v1/readyz"} {
		res, e = s.App.Test(httptest.NewRequest("GET", path, nil))
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode != 404 {
			t.Fatal("management route exposed publicly")
		}
	}
	if s.Management != nil {
		t.Fatal("unconfigured management enabled")
	}
	for _, path := range []string{"?page=0", "?page=1001", "?page_size=10", "?q=%00", "?page=abc", "/not-a-uuid"} {
		res, err := s.App.Test(httptest.NewRequest("GET", "/api/v1/start/backgrounds"+path, nil))
		if err != nil || res.StatusCode != 400 {
			t.Fatal("unbounded public input", path, err, res.StatusCode)
		}
		res.Body.Close()
	}
	res, e = s.App.Test(httptest.NewRequest("GET", "/api/v1/start/backgrounds/random", nil))
	if e != nil || res.StatusCode != 404 {
		t.Fatal("random route captured as UUID", e)
	}
	res.Body.Close()
}
