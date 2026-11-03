package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/rs/zerolog"
)

func TestStartBackgroundsMetadataAndErrorContract(t *testing.T) {
	type asset struct {
		Key         string `json:"key"`
		SHA256      string `json:"sha256"`
		Bytes       int64  `json:"bytes"`
		ContentType string `json:"content_type"`
	}
	var catalog []struct {
		Preview asset `json:"preview"`
		Image   asset `json:"image"`
	}
	data, err := os.ReadFile("../handlers/backgrounds.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := platform.UnmarshalJSON(data, &catalog); err != nil {
		t.Fatal(err)
	}
	var denied atomic.Bool
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Error("background API proxied image bytes")
		}
		if denied.Load() {
			w.WriteHeader(403)
			return
		}
		// Reflect the actual curated object's hash/type/length to the S3 client.
		for _, item := range catalog {
			for _, asset := range []asset{item.Preview, item.Image} {
				if strings.HasSuffix(r.URL.Path, asset.Key) {
					w.Header().Set("Content-Type", asset.ContentType)
					w.Header().Set("x-amz-meta-sha256", asset.SHA256)
					w.Header().Set("Content-Length", strconv.FormatInt(asset.Bytes, 10))
					return
				}
			}
		}
		w.WriteHeader(404)
	}))
	defer backend.Close()
	var cfg platform.Config
	cfg.Deployment.Region = "CN"
	cfg.Storage = &platform.StorageConfig{Endpoint: backend.URL, Region: "auto", Bucket: "development", AccessKeyID: "key", SecretAccessKey: "secret", PublicBaseURL: "https://public.example"}
	s := New(zerolog.Nop(), cfg)
	for _, fail := range []bool{false, true} {
		denied.Store(fail)
		req := httptest.NewRequest("GET", "/api/v1/start/backgrounds?region=INTL", nil)
		req.Header.Set("X-Request-ID", "background-check")
		res, err := s.App.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.Header.Get("X-GoFurry-Region") != "CN" || res.Header.Get("X-GoFurry-Content-Mode") != "SFW" {
			t.Fatal("request changed region or content policy")
		}
		if !fail && (res.StatusCode != 200 || !strings.Contains(string(body), "https://public.example/start/backgrounds/")) {
			t.Fatalf("metadata failed: %s", body)
		}
		if fail && (res.StatusCode != 503 || !strings.Contains(string(body), `"request_id":"background-check"`) || !strings.Contains(string(body), `"message":"Service Unavailable"`)) {
			t.Fatalf("error contract changed: %s", body)
		}
	}
}
