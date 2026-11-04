package weather

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const goodWeather = `{"condition":{"code":"101","text":"多云"},"temperature":{"value":22.5,"unit":"°C"},"metadata":{"attributions":["https://developer.qweather.com/attribution.html","upstream attribution text"]}}`

func mockProvider(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	cfg := testConfig(t)
	p, err := NewProvider(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Cleanup(p.Close)
	p.baseURL = server.URL
	return p
}

func TestProviderOptionalAndInvalidCredentials(t *testing.T) {
	if p, err := NewProvider(nil); p != nil || err != nil {
		t.Fatal("unconfigured provider did not disable")
	}
	cfg := testConfig(t)
	cfg.Enabled = false
	if p, err := NewProvider(&cfg); p != nil || err != nil {
		t.Fatal("disabled provider initialized")
	}
	cfg.Enabled = true
	cfg.DeveloperID = ""
	if _, err := NewProvider(&cfg); err != ErrConfig {
		t.Fatal("incomplete credentials accepted")
	}
	cfg.DeveloperID = "testDeveloper"
	cfg.PrivateKeyPath += "-missing"
	if _, err := NewProvider(&cfg); err != errKey {
		t.Fatal("unreadable private key accepted")
	}
}

func TestProviderCurrentContract(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	var calls atomic.Int32
	p := mockProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/weather/v1/current/30.67/104.07" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || r.URL.Query().Get("lang") != "zh" {
			t.Error("provider path/language/auth contract mismatch")
		}
		_, _ = io.WriteString(w, goodWeather)
	})
	if p.transport.Proxy != nil {
		t.Fatal("environment proxy used")
	}
	got, err := p.Fetch(context.Background(), 30.674, 104.074, "zh")
	if err != nil || got == nil || got.current.TemperatureC != 22.5 || got.current.ConditionCode != "101" || len(got.attributions) != 2 || time.Since(got.current.FetchedAt) > time.Second {
		t.Fatal("current response parsing failed")
	}
	if _, err := p.Fetch(context.Background(), 30.67, 104.07, "unsupported"); err == nil {
		t.Fatal("unsupported locale accepted")
	}
	if calls.Load() != 1 {
		t.Fatal("invalid query reached upstream")
	}
}

func TestProviderFailures(t *testing.T) {
	for _, code := range []int{401, 403, 429, 500, 503, 302} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var calls atomic.Int32
			p := mockProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "120")
				w.Header().Set("Location", "http://127.0.0.1:1/private")
				w.WriteHeader(code)
				_, _ = io.WriteString(w, "private upstream error")
			})
			_, err := p.Fetch(context.Background(), 39.92, 116.41, "en")
			if err == nil || strings.Contains(err.Error(), "private") {
				t.Fatal("upstream failure exposed or accepted")
			}
			if calls.Load() != 1 {
				t.Fatal("unexpected retry/redirect")
			}
			if code == 429 {
				up, ok := err.(*upstreamError)
				if !ok || up.cooldown != 2*time.Minute {
					t.Fatal("429 cooldown missing")
				}
			}
		})
	}
	for name, body := range map[string]string{
		"invalid JSON": "{", "trailing JSON": goodWeather + "{}", "duplicate JSON": strings.Replace(goodWeather, `"value":22.5`, `"value":22.5,"value":5`, 1),
		"old v7": `{"now":{"temp":"22","icon":"101"}}`, "missing value": strings.Replace(goodWeather, `"value":22.5,`, "", 1),
		"null temperature": strings.Replace(goodWeather, "22.5", "null", 1), "string temperature": strings.Replace(goodWeather, "22.5", `"22.5"`, 1), "unphysical temperature": strings.Replace(goodWeather, "22.5", "200", 1),
		"wrong unit": strings.Replace(goodWeather, "°C", "°F", 1), "invalid code": strings.Replace(goodWeather, "101", "x01", 1), "missing code": strings.Replace(goodWeather, `"code":"101",`, "", 1),
		"missing attribution": strings.Replace(goodWeather, `["https://developer.qweather.com/attribution.html","upstream attribution text"]`, `[]`, 1), "too large": strings.Repeat(" ", maxBody+1),
	} {
		t.Run(name, func(t *testing.T) {
			p := mockProvider(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
			if _, err := p.Fetch(context.Background(), 39.92, 116.41, "en"); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}

func TestProviderTimeoutAndClose(t *testing.T) {
	p := mockProvider(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := p.Fetch(ctx, 30.67, 104.07, "zh"); err == nil || time.Since(start) > time.Second {
		t.Fatal("request timeout not bounded")
	}
	done := make(chan error, 1)
	go func() { _, err := p.Fetch(context.Background(), 30.67, 104.07, "zh"); done <- err }()
	deadline := time.Now().Add(time.Second)
	for len(p.slots) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	p.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("close did not cancel fetch")
		}
	case <-time.After(time.Second):
		t.Fatal("close left request active")
	}
}

func TestProviderConcurrencyBound(t *testing.T) {
	entered := make(chan struct{}, maxConcurrent)
	release := make(chan struct{})
	p := mockProvider(t, func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = io.WriteString(w, goodWeather)
	})
	done := make(chan error, maxConcurrent)
	for range maxConcurrent {
		go func() { _, err := p.Fetch(context.Background(), 30.67, 104.07, "zh"); done <- err }()
	}
	for range maxConcurrent {
		<-entered
	}
	if _, err := p.Fetch(context.Background(), 30.67, 104.07, "zh"); err == nil {
		t.Fatal("provider concurrent bound exceeded")
	}
	close(release)
	for range maxConcurrent {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
