package routers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/handlers"
)

func registerURLs(v1 fiber.Router) {
	v1.Get("/status", handlers.Status)
}
