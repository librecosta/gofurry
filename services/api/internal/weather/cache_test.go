package weather

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeFetcher struct {
	fetch func(context.Context, float64, float64, string) (*observation, error)
}

func (f *fakeFetcher) Fetch(ctx context.Context, lat, lon float64, lang string) (*observation, error) {
	return f.fetch(ctx, lat, lon, lang)
}
func (*fakeFetcher) Close() {}

func sample(now time.Time) *observation {
	return &observation{current: Current{TemperatureC: 22.5, ConditionCode: "101", ConditionText: "test", FetchedAt: now}, attributions: []string{"provider attribution"}}
}

func TestWeatherFreshStaleExpiredAndCooldown(t *testing.T) {
	now := time.Unix(1800000000, 0)
	start := now
	calls := 0
	fail := false
	s := NewService(nil, nil, "CN")
	s.now = func() time.Time { return now }
	s.provider = &fakeFetcher{fetch: func(context.Context, float64, float64, string) (*observation, error) {
		calls++
		if fail {
			return nil, ErrUnavailable
		}
		return sample(now), nil
	}}
	get := func() Result { return s.Get(context.Background(), 30.67, 104.07, "zh") }
	if r := get(); r.State != "ready" || calls != 1 {
		t.Fatal("initial fetch failed")
	}
	now = start.Add(freshTTL - time.Nanosecond)
	if r := get(); r.State != "ready" || calls != 1 {
		t.Fatal("fresh miss")
	}
	now = start.Add(freshTTL)
	fail = true
	if r := get(); r.State != "stale" || r.Weather == nil || !r.Weather.FetchedAt.Equal(start) || calls != 2 {
		t.Fatal("stale fallback changed fetched time")
	}
	if r := get(); r.State != "stale" || calls != 2 {
		t.Fatal("same-key failure cooling failed")
	}
	if r := s.Get(context.Background(), 31, 105, "en"); r.State != "unavailable" || calls != 2 {
		t.Fatal("global failure cooling failed")
	}
	now = start.Add(maxAge - time.Nanosecond)
	if r := get(); r.State != "stale" {
		t.Fatal("usable stale lost")
	}
	now = start.Add(maxAge)
	if r := get(); r.State != "unavailable" || r.Weather != nil {
		t.Fatal("60 minute data exposed as current")
	}
	now = now.Add(16 * time.Second)
	fail = false
	if r := get(); r.State != "ready" || !r.Weather.FetchedAt.Equal(now) {
		t.Fatal("recovery did not refresh")
	}
	// Cached values handed to callers cannot mutate the next caller's result.
	r := get()
	r.Weather.ConditionText = "mutated"
	r.Attributions[0] = "mutated"
	if r := get(); r.Weather.ConditionText != "test" || r.Attributions[0] != "provider attribution" {
		t.Fatal("mutable cached response shared")
	}
}

func TestWeatherCapacityNormalizationAndLanguage(t *testing.T) {
	s := NewService(nil, nil, "CN")
	now := time.Unix(1800000000, 0)
	s.now = func() time.Time { return now }
	calls := 0
	s.provider = &fakeFetcher{fetch: func(context.Context, float64, float64, string) (*observation, error) {
		calls++
		return sample(now), nil
	}}
	s.Get(context.Background(), 30.674, 104.074, "zh")
	s.Get(context.Background(), 30.67, 104.07, "zh")
	if calls != 1 {
		t.Fatal("two-decimal coordinates did not share cache")
	}
	s.Get(context.Background(), 30.67, 104.07, "en")
	if calls != 2 {
		t.Fatal("language cache separation lost")
	}
	for i := range maxEntries + 10 {
		now = now.Add(time.Second)
		s.Get(context.Background(), float64(i)/100, 100, "en")
	}
	if len(s.cache) != maxEntries || len(s.flights) != 0 {
		t.Fatal("cache/flight capacity not bounded")
	}
	if _, ok := s.cache["30.67/104.07/zh"]; ok {
		t.Fatal("least used entry not evicted")
	}
	for _, lang := range []string{"", "fr"} {
		if r := s.Get(context.Background(), 30, 100, lang); r.State != "unavailable" {
			t.Fatal("invalid language cached")
		}
	}
}

func TestWeatherSingleFlightCancellationAndStaleWaiters(t *testing.T) {
	s := NewService(nil, nil, "CN")
	now := time.Unix(1800000000, 0)
	s.now = func() time.Time { return now }
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	s.provider = &fakeFetcher{fetch: func(context.Context, float64, float64, string) (*observation, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return sample(now), nil
	}}
	var wg sync.WaitGroup
	results := make(chan Result, 20)
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.Get(context.Background(), 30, 100, "zh") }()
	}
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := s.Get(ctx, 30, 100, "zh"); r.State != "unavailable" {
		t.Fatal("canceled request waited")
	}
	close(release)
	wg.Wait()
	close(results)
	for r := range results {
		if r.State != "ready" {
			t.Fatal("joined query lost response")
		}
	}
	if calls.Load() != 1 || len(s.flights) != 0 {
		t.Fatal("duplicate flights or flight leak")
	}
	now = now.Add(freshTTL)
	entered = make(chan struct{}, 1)
	release = make(chan struct{})
	finished := make(chan Result, 1)
	go func() { finished <- s.Get(context.Background(), 30, 100, "zh") }()
	<-entered
	if r := s.Get(context.Background(), 30, 100, "zh"); r.State != "stale" {
		t.Fatal("concurrent stale caller waited for refresh")
	}
	close(release)
	if r := <-finished; r.State != "ready" {
		t.Fatal("stale refresh failed")
	}
	s.Close()
	if r := s.Get(context.Background(), 30, 100, "zh"); r.State != "unavailable" {
		t.Fatal("closed service active")
	}
}

func TestWeatherInstancesDoNotShareCache(t *testing.T) {
	for _, temperature := range []float64{10, 20} {
		s := NewService(nil, nil, "CN")
		calls := 0
		s.provider = &fakeFetcher{fetch: func(context.Context, float64, float64, string) (*observation, error) {
			calls++
			data := sample(time.Now())
			data.current.TemperatureC = temperature
			return data, nil
		}}
		first := s.Get(context.Background(), 30, 100, "en")
		second := s.Get(context.Background(), 30, 100, "en")
		if calls != 1 || first.Weather.TemperatureC != temperature || second.Weather.TemperatureC != temperature {
			t.Fatal("service instances shared weather/configuration")
		}
	}
}

func TestWeatherConcurrencyAnd429Cooling(t *testing.T) {
	s := NewService(nil, nil, "CN")
	entered := make(chan struct{}, maxConcurrent)
	release := make(chan struct{})
	var calls atomic.Int32
	s.provider = &fakeFetcher{fetch: func(context.Context, float64, float64, string) (*observation, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return sample(time.Now()), nil
	}}
	var wg sync.WaitGroup
	for i := range maxConcurrent {
		wg.Add(1)
		go func() { defer wg.Done(); s.Get(context.Background(), float64(i), 100, "en") }()
	}
	for range maxConcurrent {
		<-entered
	}
	if r := s.Get(context.Background(), 10, 100, "en"); r.State != "unavailable" || calls.Load() != maxConcurrent {
		t.Fatal("global concurrent flight bound exceeded")
	}
	close(release)
	wg.Wait()
	now := time.Unix(1800000000, 0)
	s = NewService(nil, nil, "CN")
	s.now = func() time.Time { return now }
	calls.Store(0)
	s.provider = &fakeFetcher{fetch: func(context.Context, float64, float64, string) (*observation, error) {
		calls.Add(1)
		return nil, &upstreamError{status: 429, cooldown: time.Minute}
	}}
	s.Get(context.Background(), 30, 100, "zh")
	now = now.Add(59 * time.Second)
	s.Get(context.Background(), 31, 100, "en")
	if calls.Load() != 1 {
		t.Fatal("429 did not cool other keys")
	}
	now = now.Add(time.Second)
	s.Get(context.Background(), 30, 100, "zh")
	if calls.Load() != 2 {
		t.Fatal("cooldown never recovered")
	}
}
