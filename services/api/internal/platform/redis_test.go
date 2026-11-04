package platform

import (
	"context"
	"crypto/rand"
	"flag"
	"strings"
	"testing"
	"time"
)

var redisConfig = flag.String("d1-config", "", "Explicit development YAML for opt-in Redis checks")

func TestRedisLiveBoundsTTLAndLease(t *testing.T) {
	if *redisConfig == "" {
		t.Skip("requires explicit development YAML")
	}
	cfg, err := LoadConfig(*redisConfig)
	if err != nil {
		t.Fatal("development config failed to load")
	}
	client := NewRedis(cfg.Redis)
	if client == nil {
		t.Fatal("Redis not configured")
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if client.Ping(ctx).Err() != nil {
		t.Fatal("development Redis authentication/PING failed")
	}
	region := "CN-d1-" + rand.Text()
	index := RedisKey(region, "test", "index")
	keys := []string{RedisKey(region, "test", "furry"), RedisKey(region, "test", "8.8.8.8"), RedisKey(region, "test", "30.67,104.07")}
	defer client.Del(context.Background(), append(keys, index, keys[0]+":lease")...)
	for _, key := range keys {
		digest := key[strings.LastIndex(key, ":")+1:]
		if len(digest) != 64 || strings.Contains(digest, "furry") || strings.Contains(digest, "8.8.8.8") || strings.Contains(digest, "30.67") {
			t.Fatal("key leaks raw input")
		}
	}
	RedisWrite(ctx, client, keys[0], index, []byte(`{"ok":true}`), 10*time.Minute, 2)
	raw, hit, available := RedisRead(ctx, client, keys[0])
	if !hit || !available || string(raw) != `{"ok":true}` {
		t.Fatal("real Redis write/read failed")
	}
	ttl, err := client.PTTL(ctx, keys[0]).Result()
	if err != nil || ttl < 590*time.Second || ttl > 600*time.Second {
		t.Fatal("10 minute Redis TTL incorrect")
	}
	for _, key := range keys[1:] {
		RedisWrite(ctx, client, key, index, []byte(`{}`), 10*time.Minute, 2)
	}
	if count, _ := client.Exists(ctx, keys...).Result(); count != 2 {
		t.Fatal("shared capacity bound failed")
	}
	if count, _ := client.ZCard(ctx, index).Result(); count != 2 {
		t.Fatal("index capacity bound failed")
	}
	release, owned, ready := RedisLease(ctx, client, keys[0], time.Second)
	if !owned || !ready {
		t.Fatal("lease failed")
	}
	_, other, ready := RedisLease(ctx, client, keys[0], time.Second)
	if other || !ready {
		t.Fatal("duplicate lease owner")
	}
	client.Set(ctx, keys[0]+":lease", "replacement", time.Second)
	release()
	if value, _ := client.Get(ctx, keys[0]+":lease").Result(); value != "replacement" {
		t.Fatal("old owner deleted a replacement lease")
	}
	t.Log("real Redis AUTH/PING, GET/SET, 10m TTL, bounded index, ownership-checked lease: passed")
}

func TestRedisConfigSingleYAML(t *testing.T) {
	// Existing single-source cases cover unknown fields and environment isolation.
	if NewRedis(nil) != nil || NewRedis(&RedisConfig{}) != nil {
		t.Fatal("blank Redis must permit local fallback")
	}
	client := NewRedis(&RedisConfig{Address: "127.0.0.1:1"})
	defer client.Close()
	start := time.Now()
	_, hit, available := RedisRead(context.Background(), client, "test")
	if hit || available || time.Since(start) > time.Second {
		t.Fatal("Redis outage is unbounded")
	}
}
