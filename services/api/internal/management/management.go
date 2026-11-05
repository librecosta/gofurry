// Package management builds only the separate loopback App. Never mount it on Public.
package management

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/librecosta/gofurry/services/api/internal/middleware"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
	"github.com/rs/zerolog"
)

type actorKey struct{}
type invalidIDKey struct{}

func Current(c fiber.Ctx) wallpaper.Actor { return fiber.Locals[wallpaper.Actor](c, actorKey{}) }
func New(logger zerolog.Logger, cfg platform.Config, service *wallpaper.Service) *fiber.App {
	app := fiber.New(fiber.Config{AppName: "gofurry-management", JSONEncoder: platform.MarshalJSON, JSONDecoder: platform.UnmarshalJSON, ErrorHandler: ErrorHandler,
		ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 15 * time.Second, BodyLimit: 64 << 20, StreamRequestBody: true, DisablePreParseMultipartForm: true, Concurrency: 16, TrustProxy: false, EnableIPValidation: true})
	app.Use(func(c fiber.Ctx) error {
		raw := c.Get("X-Request-ID")
		id, e := uuid.Parse(raw)
		if e != nil || raw != id.String() || id == uuid.Nil() {
			fiber.Locals[bool](c, invalidIDKey{}, true)
			c.Request().Header.Set("X-Request-ID", uuid.NewV7().String())
		}
		return c.Next()
	})
	middleware.Register(app, logger, cfg.Server.Environment == "production", cfg.Logging.SlowRequest)
	// Constant global key: bounded limiter state; forwarded addresses never matter.
	app.Use(limiter.New(limiter.Config{Max: 120, Expiration: time.Minute, KeyGenerator: func(fiber.Ctx) string { return "management" }}))
	app.Use(authenticate(cfg))
	app.Use(func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(c.Context(), 90*time.Second)
		defer cancel()
		c.SetContext(ctx)
		return c.Next()
	})
	app.Get("/internal/v1/readyz", func(c fiber.Ctx) error {
		if service.Ready(c.Context()) != nil {
			return fiber.ErrServiceUnavailable
		}
		return c.JSON(fiber.Map{"ready": true})
	})
	app.Use(func(fiber.Ctx) error { return fiber.ErrNotFound })
	return app
}
func authenticate(cfg platform.Config) fiber.Handler {
	expected := ""
	if cfg.Management != nil {
		expected = cfg.Management.Token
	}
	want := sha256.Sum256([]byte(expected))
	return func(c fiber.Ctx) error {
		value := c.Get("Authorization")
		supplied, ok := strings.CutPrefix(value, "Bearer ")
		got := sha256.Sum256([]byte(supplied))
		if expected == "" || !ok || len(supplied) != len(expected) || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			return fiber.ErrUnauthorized
		}
		// These are trusted-service assertions, never independently trusted credentials.
		if c.Get("X-LC-Region") != cfg.Deployment.Region || c.Get("X-LC-Environment") != cfg.Server.Environment {
			return fiber.ErrForbidden
		}
		id, e := uuid.Parse(c.Get("X-LC-Actor-ID"))
		name := c.Get("X-LC-Actor-Name")
		role := c.Get("X-LC-Actor-Role")
		if e != nil || id == uuid.Nil() || !wallpaper.BoundedText(name, 80, true) || strings.TrimSpace(name) == "" || (role != "owner" && role != "developer" && role != "operator") || fiber.Locals[bool](c, invalidIDKey{}) {
			return fiber.ErrBadRequest
		}
		fiber.Locals[wallpaper.Actor](c, actorKey{}, wallpaper.Actor{ID: &id, Name: name, Role: role, RequestID: requestid.FromContext(c)})
		return c.Next()
	}
}
func ErrorHandler(c fiber.Ctx, err error) error {
	code := 500
	var he *fiber.Error
	if errors.As(err, &he) {
		code = he.Code
	}
	return c.Status(code).JSON(fiber.Map{"error": fiber.Map{"code": code, "message": http.StatusText(code)}, "request_id": requestid.FromContext(c)})
}
