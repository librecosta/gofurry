package weather

import (
	"context"
	"errors"
	"flag"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

// Explicit test-only file flag, never an application environment/config source.
var liveConfig = flag.String("qweather-live-config", "", "explicit development YAML for two real QWeather calls (zh/en)")

type countingTransport struct {
	base         http.RoundTripper
	calls        atomic.Int32
	status       atomic.Int32
	networkKind  string
	expectedLang string
}

func (r *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != "GET" || req.URL.Path != "/weather/v1/current/39.92/116.41" || req.URL.Query().Get("lang") != r.expectedLang {
		return nil, errors.New("live request coordinate/language mismatch")
	}
	r.calls.Add(1)
	res, err := r.base.RoundTrip(req)
	if res != nil {
		r.status.Store(int32(res.StatusCode))
	}
	if err != nil {
		var dns *net.DNSError
		var network net.Error
		switch {
		case errors.As(err, &dns):
			r.networkKind = "DNS lookup failed"
		case errors.As(err, &network) && network.Timeout():
			r.networkKind = "network timeout"
		default:
			r.networkKind = "connection/TLS/proxy failure"
		}
	}
	return res, err
}

func TestQWeatherLive(t *testing.T) {
	if *liveConfig == "" {
		t.Skip("real calls require -qweather-live-config; no environment credentials used")
	}
	cfg, err := platform.LoadConfig(*liveConfig)
	if err != nil {
		t.Fatal("explicit development YAML could not be loaded; check configuration locally")
	}
	if cfg.Server.Environment != "development" {
		t.Fatal("live test only permits development configuration")
	}
	if missing := cfg.QWeather.MissingFields(); len(missing) > 0 {
		t.Skip("real QWeather call blocked: missing qweather." + strings.Join(missing, ", qweather."))
	}
	if !cfg.QWeather.Enabled {
		t.Skip("real QWeather call blocked: qweather.enabled is false")
	}
	p, err := NewProvider(cfg.QWeather)
	if err != nil {
		t.Fatal("development credentials could not initialize provider")
	}
	counter := &countingTransport{base: p.transport}
	p.client.Transport = counter
	s := NewService(p)
	defer s.Close()
	for _, lang := range []string{"zh", "en"} {
		counter.expectedLang = lang
		first := s.Get(context.Background(), 39.92, 116.41, lang)
		if first.State != "ready" || first.Weather == nil {
			if counter.status.Load() == 200 {
				t.Fatal("real QWeather returned HTTP 200 but current JSON validation failed")
			}
			t.Fatalf("real QWeather unavailable: HTTP status=%d, network=%s; no weather result claimed", counter.status.Load(), counter.networkKind)
		}
		before := counter.calls.Load()
		second := s.Get(context.Background(), 39.92, 116.41, lang)
		if counter.status.Load() != 200 || second.State != "ready" || counter.calls.Load() != before || !second.Weather.FetchedAt.Equal(first.Weather.FetchedAt) || len(first.Attributions) == 0 {
			t.Fatal("real current response/cache/attribution verification failed")
		}
		t.Logf("real %s: HTTP 200, current JSON/attributions parsed, repeated query served from cache", lang)
	}
	if counter.calls.Load() != 2 {
		t.Fatal("expected exactly two upstream calls for zh/en")
	}
	t.Log("JWT authentication passed; exactly 2 upstream requests, 2 cache hits; no credential/token output")
}
