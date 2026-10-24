package handlers

import "github.com/gofiber/fiber/v3"

func Status(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"service": "gofurry-api", "status": "ok"})
}
