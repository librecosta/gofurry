package weather

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const freshTTL = 20 * time.Minute
const maxAge = 60 * time.Minute
const maxEntries = 512

type fetcher interface {
	Fetch(context.Context, float64, float64, string) (*observation, error)
	Close()
}
type cacheEntry struct {
	data                 *observation
	retryAfter, lastUsed time.Time
}
type flight struct {
	done chan struct{}
	data *observation
}
type Result struct {
	State        string
	Weather      *Current
	Attributions []string
}

type Service struct {
	mu       sync.Mutex
	provider fetcher
	redis    *redis.Client
	region   string
	cache    map[string]cacheEntry
	flights  map[string]*flight
	cooldown time.Time
	closed   bool
	now      func() time.Time
}

func NewService(provider *Provider, shared *redis.Client, region string) *Service {
	s := &Service{cache: make(map[string]cacheEntry), flights: make(map[string]*flight), now: time.Now, redis: shared, region: region}
	if provider != nil {
		s.provider = provider
	}
	return s
}

func result(data *observation, now time.Time) Result {
	if data == nil || now.Before(data.current.FetchedAt) || now.Sub(data.current.FetchedAt) >= maxAge {
		return Result{State: "unavailable", Attributions: []string{}}
	}
	state := "ready"
	if now.Sub(data.current.FetchedAt) >= freshTTL {
		state = "stale"
	}
	current := data.current
	return Result{State: state, Weather: &current, Attributions: append([]string{}, data.attributions...)}
}

// Get never keys by IP. One bounded flight per coordinate/language refreshes
// synchronously; concurrent callers with usable stale data need not wait.
func (s *Service) Get(ctx context.Context, lat, lon float64, lang string) Result {
	lat, lon, key, valid := normalized(lat, lon, lang)
	if !valid || ctx.Err() != nil {
		return result(nil, time.Time{})
	}
	s.mu.Lock()
	now := s.now()
	entry := s.cache[key]
	available := result(entry.data, now)
	if s.closed || s.provider == nil {
		s.mu.Unlock()
		return result(nil, now)
	}
	if available.State == "ready" {
		entry.lastUsed = now
		s.cache[key] = entry
		s.mu.Unlock()
		return available
	}
	if pending := s.flights[key]; pending != nil {
		s.mu.Unlock()
		if available.State == "stale" {
			return available
		}
		select {
		case <-ctx.Done():
			return result(nil, now)
		case <-pending.done:
		}
		return result(pending.data, s.now())
	}
	if now.Before(entry.retryAfter) || now.Before(s.cooldown) || len(s.flights) >= maxConcurrent {
		s.mu.Unlock()
		return available
	}
	pending := &flight{done: make(chan struct{})}
	s.flights[key] = pending
	s.mu.Unlock()
	data, err := s.sharedFetch(ctx, lat, lon, lang, key, entry.data)
	s.mu.Lock()
	defer s.mu.Unlock()
	now = s.now()
	if err == nil && data != nil {
		entry = cacheEntry{data: data, lastUsed: now}
	} else {
		if result(data, now).Weather != nil {
			entry.data = data
		}
		pause := 15 * time.Second
		var upstream *upstreamError
		if errors.As(err, &upstream) {
			pause = upstream.cooldown
		}
		entry.retryAfter = now.Add(pause)
		entry.lastUsed = now
		if entry.retryAfter.After(s.cooldown) {
			s.cooldown = entry.retryAfter
		}
	}
	if !s.closed {
		if _, exists := s.cache[key]; !exists && len(s.cache) >= maxEntries {
			oldest := ""
			var used time.Time
			for id, item := range s.cache {
				if oldest == "" || item.lastUsed.Before(used) {
					oldest = id
					used = item.lastUsed
				}
			}
			delete(s.cache, oldest)
		}
		s.cache[key] = entry
	}
	pending.data = entry.data
	delete(s.flights, key)
	close(pending.done)
	return result(entry.data, now)
}

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	if s.provider != nil {
		s.provider.Close()
	}
}
