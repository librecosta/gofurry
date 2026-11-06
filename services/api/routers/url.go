package routers

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/librecosta/gofurry/services/api/internal/handlers"
	"github.com/librecosta/gofurry/services/api/internal/middleware"
	"github.com/redis/go-redis/v9"
)

func registerURLs(v1 fiber.Router, backgrounds, suggestions, location, weather, cities, nearestCity fiber.Handler, shared *redis.Client, region string) {
	v1.Get("/status", handlers.Status)
	// Per-client and deployment-wide bounds also cover cache misses/unique queries.
	limited := func(max int, key func(fiber.Ctx) string) fiber.Handler {
		return limiter.New(limiter.Config{Max: max, Expiration: time.Minute, KeyGenerator: key,
			LimitReached: func(c fiber.Ctx) error {
				c.Set("Cache-Control", "no-store")
				c.Set("Retry-After", "60")
				return fiber.ErrTooManyRequests
			},
		})
	}
	publicBackgrounds := v1.Group("/start/backgrounds", limited(600, func(fiber.Ctx) string { return "backgrounds" }), limited(120, func(c fiber.Ctx) string { return c.IP() }))
	publicBackgrounds.Get("/", backgrounds)
	publicBackgrounds.Get("/random", backgrounds)
	publicBackgrounds.Get("/:id", backgrounds)
	v1.Get("/start/search/suggestions", limited(240, func(fiber.Ctx) string { return "suggestions" }), limited(60, func(c fiber.Ctx) string { return c.IP() }), middleware.RedisLimit(shared, region, "suggestions", 240, 60), suggestions)
	v1.Get("/start/location", limited(600, func(fiber.Ctx) string { return "location" }), limited(30, func(c fiber.Ctx) string { return c.IP() }), location)
	v1.Get("/start/weather", limited(120, func(fiber.Ctx) string { return "weather" }), limited(20, func(c fiber.Ctx) string { return c.IP() }), middleware.RedisLimit(shared, region, "weather", 120, 20), weather)
	v1.Get("/start/cities", limited(240, func(fiber.Ctx) string { return "cities" }), limited(40, func(c fiber.Ctx) string { return c.IP() }), cities)
	v1.Post("/start/cities/nearest", limited(60, func(fiber.Ctx) string { return "nearest-city" }), limited(6, func(c fiber.Ctx) string { return c.IP() }), nearestCity)
}
