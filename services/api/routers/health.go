package routers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/healthcheck"
)

func registerHealth(app *fiber.App, ready func() bool) {
	app.Get(healthcheck.LivenessEndpoint, healthcheck.New(healthcheck.Config{ResponseFormat: healthcheck.FormatJSON}))
	app.Get(healthcheck.ReadinessEndpoint, healthcheck.New(healthcheck.Config{
		Probe:          func(fiber.Ctx) bool { return ready() },
		ResponseFormat: healthcheck.FormatJSON,
	}))
}
