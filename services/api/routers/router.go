package routers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/navigation"
	"github.com/librecosta/gofurry/services/api/internal/yearbook"
	"github.com/redis/go-redis/v9"
)

func Register(app *fiber.App, ready func() bool, backgrounds, suggestions, location, weather, cities, nearestCity fiber.Handler, shared *redis.Client, region string, directory *catalog.Service, yearbooks *yearbook.Service, discovery *navigation.Service, interactions fiber.Handler, interactionsEnabled bool) {
	registerHealth(app, ready)
	registerURLs(app.Group("/api").Group("/v1"), backgrounds, suggestions, location, weather, cities, nearestCity, shared, region, directory, yearbooks, discovery, interactions, interactionsEnabled)
}
