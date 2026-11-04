package search

import (
	"context"
	"crypto/rand"
	"flag"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var developmentConfig = flag.String("d1-config", "", "Explicit development YAML for opt-in Redis/DuckDuckGo checks")

func TestSuggestionsRedisLive(t *testing.T) {
	if *developmentConfig == "" {
		t.Skip("requires explicit development YAML")
	}
	cfg, err := platform.LoadConfig(*developmentConfig)
	if err != nil {
		t.Fatal("development YAML could not load")
	}
	client := platform.NewRedis(cfg.Redis)
	if client == nil {
		t.Fatal("Redis missing")
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if client.Ping(ctx).Err() != nil {
		t.Fatal("Redis unavailable")
	}
	var calls atomic.Int32
	var cleanup []string
	defer func() { client.Del(context.Background(), cleanup...) }()
	baseline := make(map[string][]string)
	for _, region := range []string{"CN", "INTL"} {
		namespace := region + "-d1-" + rand.Text()
		cleanup = append(cleanup, platform.RedisKey(namespace, "suggestions", "index"))
		newService := func() *Service {
			s := New(cfg.SearchSuggestions, namespace, client)
			base := s.client.Transport
			s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) { calls.Add(1); return base.RoundTrip(r) })
			t.Cleanup(s.Close)
			return s
		}
		s := newService()
		for _, query := range []string{"furry", "兽人", "GoFurry"} {
			before := calls.Load()
			phrases, err := s.Suggestions(ctx, query)
			if err != nil {
				kind, status := Diagnostic(err)
				t.Fatalf("real %s %s failed: %s/%d", region, query, kind, status)
			}
			if len(phrases) == 0 {
				t.Fatalf("real %s %s returned no suggestions", region, query)
			}
			key := platform.RedisKey(namespace, "suggestions", strings.ToLower(query))
			cleanup = append(cleanup, key, key+":lease")
			ttl, err := client.PTTL(ctx, key).Result()
			if err != nil || ttl < 590*time.Second || ttl > 600*time.Second {
				t.Fatal("real suggestion Redis TTL missing")
			}
			// A different service has no local cache and must reuse Redis.
			second, err := newService().Suggestions(ctx, query)
			if err != nil || !reflect.DeepEqual(phrases, second) || calls.Load() != before+1 {
				t.Fatal("shared hit repeated the upstream request")
			}
			if region == "CN" {
				baseline[query] = phrases
			} else if !reflect.DeepEqual(baseline[query], phrases) {
				t.Logf("upstream changed %s between regional requests; identical processing still applies", query)
			}
			t.Logf("%s %s: %d real suggestions, Redis TTL %.0fs, second service hit (no new upstream)", region, query, len(phrases), ttl.Seconds())
		}
	}
	// A cold key requested by two processes is coalesced through the real lease.
	region := "CN-d1-" + rand.Text()
	key := platform.RedisKey(region, "suggestions", "merge")
	cleanup = append(cleanup, key, key+":lease", platform.RedisKey(region, "suggestions", "index"))
	var merged atomic.Int32
	s1, s2 := New(nil, region, client), New(nil, region, client)
	defer s1.Close()
	defer s2.Close()
	transport := roundTrip(func(*http.Request) (*http.Response, error) {
		merged.Add(1)
		time.Sleep(100 * time.Millisecond)
		return reply(200, `[{"phrase":"merge result"}]`), nil
	})
	s1.client.Transport = transport
	s2.client.Transport = transport
	var wg sync.WaitGroup
	for _, s := range []*Service{s1, s2} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := s.Suggestions(ctx, "merge")
			if err != nil || len(got) != 1 {
				t.Error("shared merge failed")
			}
		}()
	}
	wg.Wait()
	if merged.Load() != 1 {
		t.Fatal("cross-process miss called upstream twice")
	}
	offline := platform.NewRedis(cfg.Redis)
	offline.Close()
	fallback := New(nil, region, offline)
	defer fallback.Close()
	fallback.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return reply(200, `[{"phrase":"local fallback"}]`), nil })
	if got, err := fallback.Suggestions(ctx, "fallback"); err != nil || len(got) != 1 {
		t.Fatal("Redis outage broke local suggestions")
	}
}

func TestSuggestionsDirectLiveDiagnostic(t *testing.T) {
	if *developmentConfig == "" {
		t.Skip("explicit opt-in network check")
	}
	s := New(nil, "CN", nil)
	defer s.Close()
	phrases, err := s.Suggestions(context.Background(), "furry")
	if err != nil {
		kind, status := Diagnostic(err)
		t.Logf("direct DuckDuckGo: %s, upstream_status=%d (explicit YAML proxy is tested separately)", kind, status)
	} else {
		t.Logf("direct DuckDuckGo: %d actual suggestions", len(phrases))
	}
}
