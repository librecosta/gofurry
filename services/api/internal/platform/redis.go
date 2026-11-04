package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// One YAML-configured client belongs to the server lifecycle. Redis failures are
// bounded and optional; domain services retain their bounded local fallback.
func NewRedis(cfg *RedisConfig) *redis.Client {
	if cfg == nil || cfg.Address == "" {
		return nil
	}
	return redis.NewClient(&redis.Options{Addr: cfg.Address, Username: cfg.Username, Password: cfg.Password, DB: cfg.DB,
		DialTimeout: 400 * time.Millisecond, ReadTimeout: 200 * time.Millisecond, WriteTimeout: 200 * time.Millisecond,
		PoolTimeout: 200 * time.Millisecond, PoolSize: 8, MaxActiveConns: 16, MaxRetries: -1,
		ContextTimeoutEnabled: true, DisableIdentity: true})
}

// Keys never contain search text, raw IPs or coordinates. The deployment tag
// isolates namespaces even when development deployments share one Redis server.
func RedisKey(region, kind, value string) string {
	return fmt.Sprintf("gofurry:v1:{%s}:%s:%x", region, kind, sha256.Sum256([]byte(value)))
}

func RedisRead(ctx context.Context, client *redis.Client, key string) (data []byte, hit, available bool) {
	if client == nil {
		return nil, false, false
	}
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	data, err := client.Get(ctx, key).Bytes()
	return data, err == nil, err == nil || err == redis.Nil
}

// A bounded lease coalesces cold misses across API processes. Ownership-checked
// release cannot delete a replacement lease after expiry; it never logs keys.
func RedisLease(ctx context.Context, client *redis.Client, key string, ttl time.Duration) (release func(), owned, available bool) {
	noop := func() {}
	if client == nil {
		return noop, true, false
	}
	leaseCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	token := rand.Text()
	ok, err := client.SetNX(leaseCtx, key+":lease", token, ttl).Result()
	if err != nil {
		return noop, true, false
	}
	if !ok {
		return noop, false, true
	}
	return func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		client.Eval(cleanup, `if redis.call('GET',KEYS[1]) == ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`, []string{key + ":lease"}, token)
	}, true, true
}

// One small Redis transaction bounds each domain's shared cache and TTL. Only
// this deployment/domain's own indexed keys can be evicted; no FLUSH/scan job.
func RedisWrite(ctx context.Context, client *redis.Client, key, index string, data []byte, ttl time.Duration, capacity int) {
	if client == nil || ttl <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	client.Eval(ctx, `
local clock=redis.call('TIME'); local now=clock[1]*1000+math.floor(clock[2]/1000)
redis.call('SET',KEYS[1],ARGV[1],'PX',ARGV[2])
redis.call('ZADD',KEYS[2],now+ARGV[2],KEYS[1])
redis.call('ZREMRANGEBYSCORE',KEYS[2],'-inf',now)
local excess=redis.call('ZCARD',KEYS[2])-tonumber(ARGV[3])
if excess>0 then
 local victims=redis.call('ZRANGE',KEYS[2],0,excess-1)
 for _,key in ipairs(victims) do redis.call('DEL',key);redis.call('ZREM',KEYS[2],key) end
end
if redis.call('PTTL',KEYS[2])<tonumber(ARGV[2]) then redis.call('PEXPIRE',KEYS[2],ARGV[2]) end
return 1`, []string{key, index}, data, ttl.Milliseconds(), capacity)
}
