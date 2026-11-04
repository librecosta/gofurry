package server

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/rs/zerolog"
)

func TestSuggestionsHTTPContractAndRateLimit(t *testing.T) {
	var cfg platform.Config
	cfg.Deployment.Region = "CN"
	cfg.SearchSuggestions = &platform.SearchSuggestionsConfig{Enabled: false}
	s := New(zerolog.Nop(), cfg)
	for i := range 61 {
		response, err := s.App.Test(httptest.NewRequest("GET", "/api/v1/start/search/suggestions?q=go", nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.Header.Get("X-GoFurry-Region") != "CN" || response.Header.Get("X-GoFurry-Content-Mode") != "SFW" {
			t.Fatal("regional boundary missing")
		}
		if i < 60 {
			if response.StatusCode != 503 || response.Header.Get("X-GoFurry-Suggestions-State") != "disabled" {
				t.Fatalf("disabled contract: %d %s", response.StatusCode, body)
			}
		} else if response.StatusCode != 429 || !strings.Contains(string(body), `"request_id"`) || !strings.Contains(string(body), `"code":429`) || response.Header.Get("Retry-After") == "" {
			t.Fatalf("rate/error contract: %d %s", response.StatusCode, body)
		}
	}
}
