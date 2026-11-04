package weather

import (
	"context"
	"crypto/rand"
	"flag"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

var developmentRedis = flag.String("d1-config", "", "Explicit development YAML for opt-in Redis weather checks")

func TestWeatherRedisLive(t *testing.T) {
	if *developmentRedis == "" {
		t.Skip("requires explicit development Redis YAML")
	}
	cfg, err := platform.LoadConfig(*developmentRedis)
	if err != nil {
		t.Fatal("development YAML could not load")
	}
	client := platform.NewRedis(cfg.Redis)
	if client == nil {
		t.Fatal("Redis missing")
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if client.Ping(ctx).Err() != nil {
		t.Fatal("Redis unavailable")
	}
	region := "CN-d1-" + rand.Text()
	_, _, normalizedKey, _ := normalized(30.67, 104.07, "zh")
	key := platform.RedisKey(region, "weather", normalizedKey)
	index, cooling := platform.RedisKey(region, "weather", "index"), platform.RedisKey(region, "weather", "cooldown")
	defer client.Del(context.Background(), key, key+":lease", index, cooling)
	now := time.Now()
	initial := now
	var calls atomic.Int32
	provider := &fakeFetcher{fetch: func(context.Context, float64, float64, string) (*observation, error) {
		calls.Add(1)
		return sample(now), nil
	}}
	a, b := NewService(nil, client, region), NewService(nil, client, region)
	defer a.Close()
	defer b.Close()
	a.provider = provider
	b.provider = provider
	a.now = func() time.Time { return now }
	b.now = a.now
	if a.Get(ctx, 30.67, 104.07, "zh").State != "ready" || b.Get(ctx, 30.67, 104.07, "zh").State != "ready" || calls.Load() != 1 {
		t.Fatal("weather shared Fresh hit failed")
	}
	ttl, err := client.PTTL(ctx, key).Result()
	if err != nil || ttl < 3590*time.Second || ttl > 3600*time.Second {
		t.Fatal("weather 60 minute storage TTL failed")
	}
	now = initial.Add(20 * time.Minute)
	provider.fetch = func(context.Context, float64, float64, string) (*observation, error) {
		calls.Add(1)
		return nil, &upstreamError{status: 429, cooldown: time.Minute}
	}
	if got := a.Get(ctx, 30.67, 104.07, "zh"); got.State != "stale" || got.Weather == nil || !got.Weather.FetchedAt.Equal(initial) {
		t.Fatal("Redis Stale/fetched_at contract failed")
	}
	c := NewService(nil, client, region)
	defer c.Close()
	c.provider = provider
	c.now = a.now
	if c.Get(ctx, 30.67, 104.07, "zh").State != "stale" || calls.Load() != 2 {
		t.Fatal("shared 429 cooling failed")
	}
	now = initial.Add(60 * time.Minute)
	if got := c.Get(ctx, 30.67, 104.07, "zh"); got.State != "unavailable" || got.Weather != nil {
		t.Fatal("expired weather served past 60 minutes")
	}
	t.Logf("real Redis: Fresh <20m, Stale <60m, expiry at 60m, shared 429 cooldown, TTL %.0fs: passed", ttl.Seconds())
	// Two empty local caches share a bounded lease, not a second paid fetch.
	region = "INTL-d1-" + rand.Text()
	key = platform.RedisKey(region, "weather", normalizedKey)
	defer client.Del(context.Background(), key, key+":lease", platform.RedisKey(region, "weather", "index"))
	var merged atomic.Int32
	provider = &fakeFetcher{fetch: func(context.Context, float64, float64, string) (*observation, error) {
		merged.Add(1)
		time.Sleep(100 * time.Millisecond)
		return sample(time.Now()), nil
	}}
	a, b = NewService(nil, client, region), NewService(nil, client, region)
	defer a.Close()
	defer b.Close()
	a.provider = provider
	b.provider = provider
	var wg sync.WaitGroup
	for _, s := range []*Service{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Get(ctx, 30.67, 104.07, "zh").State != "ready" {
				t.Error("shared weather merge failed")
			}
		}()
	}
	wg.Wait()
	if merged.Load() != 1 {
		t.Fatal("cross-process weather miss was not merged")
	}
	offline := platform.NewRedis(cfg.Redis)
	offline.Close()
	fallback := NewService(nil, offline, region)
	defer fallback.Close()
	fallback.provider = provider
	if fallback.Get(ctx, 31, 105, "en").State != "ready" {
		t.Fatal("Redis failure disabled weather fallback")
	}
}
