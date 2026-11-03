package routers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/handlers"
)

func registerURLs(v1 fiber.Router, backgrounds fiber.Handler) {
	v1.Get("/status", handlers.Status)
	v1.Get("/start/backgrounds", backgrounds)
}
