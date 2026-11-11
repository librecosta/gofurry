package routers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/redis/go-redis/v9"
)

func Register(app *fiber.App, ready func() bool, backgrounds, suggestions, location, weather, cities, nearestCity fiber.Handler, shared *redis.Client, region string, directory *catalog.Service) {
	registerHealth(app, ready)
	registerURLs(app.Group("/api").Group("/v1"), backgrounds, suggestions, location, weather, cities, nearestCity, shared, region, directory)
}
