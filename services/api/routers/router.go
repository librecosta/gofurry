package routers

import "github.com/gofiber/fiber/v3"

func Register(app *fiber.App, ready func() bool) {
	registerHealth(app, ready)
	registerURLs(app.Group("/api").Group("/v1"))
}
