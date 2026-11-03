package middleware

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/librecosta/gofurry/services/api/internal/platform"
)

func TestProductionLogPolicy(t *testing.T) {
	var logs bytes.Buffer
	app := fiber.New()
	Register(app, platform.NewLogger(&logs, "production"), true, 10*time.Millisecond)
	app.Get("/livez", func(c fiber.Ctx) error { return c.SendStatus(200) })
	app.Get("/fast", func(c fiber.Ctx) error { return c.SendStatus(200) })
	app.Get("/slow", func(c fiber.Ctx) error { time.Sleep(20 * time.Millisecond); return c.SendStatus(200) })
	app.Get("/error", func(c fiber.Ctx) error { return c.SendStatus(500) })
	for _, path := range []string{"/livez", "/fast", "/slow", "/error"} {
		res, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	output := logs.String()
	if strings.Contains(output, "/livez") || strings.Contains(output, "/fast") || !strings.Contains(output, "/slow") || !strings.Contains(output, "/error") {
		t.Fatal(output)
	}
}
