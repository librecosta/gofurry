package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/redis/go-redis/v9"
)

// Existing official in-process Fiber limits remain in front of this shared
// budget, including during Redis outages. No raw client address enters Redis.
func RedisLimit(client *redis.Client, region, route string, global, perClient int) fiber.Handler {
	return func(c fiber.Ctx) error {
		if client == nil {
			return c.Next()
		}
		ctx, cancel := context.WithTimeout(c.Context(), 250*time.Millisecond)
		defer cancel()
		allowed, err := client.Eval(ctx, `
local total=redis.call('INCR',KEYS[1]);if total==1 then redis.call('EXPIRE',KEYS[1],60) end
local visitor=redis.call('INCR',KEYS[2]);if visitor==1 then redis.call('EXPIRE',KEYS[2],60) end
if total>tonumber(ARGV[1]) or visitor>tonumber(ARGV[2]) then return 0 end return 1`,
			[]string{platform.RedisKey(region, "rate:"+route, "global"), platform.RedisKey(region, "rate:"+route, c.IP())}, global, perClient).Int()
		if err == nil && allowed == 0 {
			c.Set("Cache-Control", "no-store")
			c.Set("Retry-After", "60")
			return fiber.ErrTooManyRequests
		}
		return c.Next()
	}
}
