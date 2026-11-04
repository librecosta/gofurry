package weather

import (
	"context"
	"encoding/json/v2"
	"errors"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/platform"
)

type sharedObservation struct {
	Current      Current  `json:"current"`
	Attributions []string `json:"attributions"`
}

func (s *Service) sharedFetch(ctx context.Context, lat, lon float64, lang, key string, fallback *observation) (*observation, error) {
	if s.redis == nil {
		return s.provider.Fetch(ctx, lat, lon, lang)
	}
	key = platform.RedisKey(s.region, "weather", key)
	read := func() (*observation, bool, bool) {
		data, hit, available := platform.RedisRead(ctx, s.redis, key)
		var value sharedObservation
		if hit && len(data) <= maxBody && json.Unmarshal(data, &value) == nil {
			obs := &observation{current: value.Current, attributions: value.Attributions}
			if validObservation(obs) && result(obs, s.now()).Weather != nil {
				return obs, true, available
			}
		}
		return nil, false, available
	}
	data, hit, available := read()
	if hit {
		if result(data, s.now()).State == "ready" {
			return data, nil
		}
		if fallback == nil || data.current.FetchedAt.After(fallback.current.FetchedAt) {
			fallback = data
		}
	}
	if available {
		_, cooling, reachable := platform.RedisRead(ctx, s.redis, platform.RedisKey(s.region, "weather", "cooldown"))
		if cooling {
			return fallback, ErrUnavailable
		}
		available = reachable
	}
	if available {
		release, owned, reachable := platform.RedisLease(ctx, s.redis, key, 6*time.Second)
		defer release()
		if reachable && !owned {
			if result(fallback, s.now()).State == "stale" {
				return fallback, nil
			}
			wait, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-wait.Done():
					return fallback, ErrUnavailable
				case <-ticker.C:
					data, hit, available = read()
					if hit {
						return data, nil
					}
					if !available {
						return s.provider.Fetch(ctx, lat, lon, lang)
					}
				}
			}
		}
		available = reachable
		if available {
			if data, hit, _ = read(); hit && result(data, s.now()).State == "ready" {
				return data, nil
			}
		}
	}
	data, err := s.provider.Fetch(ctx, lat, lon, lang)
	if available {
		if err == nil && data != nil {
			encoded, _ := json.Marshal(sharedObservation{Current: data.current, Attributions: data.attributions})
			ttl := maxAge - s.now().Sub(data.current.FetchedAt)
			platform.RedisWrite(ctx, s.redis, key, platform.RedisKey(s.region, "weather", "index"), encoded, ttl, maxEntries)
		} else {
			cooldown := 15 * time.Second
			var upstream *upstreamError
			if errors.As(err, &upstream) {
				cooldown = upstream.cooldown
			}
			limited, cancel := context.WithTimeout(context.WithoutCancel(ctx), 250*time.Millisecond)
			s.redis.SetNX(limited, platform.RedisKey(s.region, "weather", "cooldown"), "1", cooldown)
			cancel()
		}
	}
	if err != nil {
		return fallback, err
	}
	return data, nil
}

func validObservation(data *observation) bool {
	w := data.current
	if w.TemperatureC < -100 || w.TemperatureC > 100 || w.TemperatureC != w.TemperatureC || len(w.ConditionCode) != 3 || w.ConditionCode[0] < '1' || w.ConditionCode[0] > '9' || w.ConditionCode[1] < '0' || w.ConditionCode[1] > '9' || w.ConditionCode[2] < '0' || w.ConditionCode[2] > '9' || w.ConditionText == "" || len(w.ConditionText) > 1024 || len(data.attributions) > 32 {
		return false
	}
	for _, text := range data.attributions {
		if text == "" || len(text) > 4096 {
			return false
		}
	}
	return true
}
