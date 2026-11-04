package routers

import "github.com/gofiber/fiber/v3"

func Register(app *fiber.App, ready func() bool, backgrounds, suggestions, location, weather, cities, nearestCity fiber.Handler) {
	registerHealth(app, ready)
	registerURLs(app.Group("/api").Group("/v1"), backgrounds, suggestions, location, weather, cities, nearestCity)
}
