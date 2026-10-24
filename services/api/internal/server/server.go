package server

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/librecosta/gofurry/services/api/internal/middleware"
	"github.com/librecosta/gofurry/services/api/routers"
	"github.com/rs/zerolog"
)

type Server struct {
	App   *fiber.App
	ready atomic.Bool
}

func New(logger zerolog.Logger) *Server {
	s := &Server{}
	s.App = fiber.New(fiber.Config{
		AppName:      "gofurry-api",
		JSONEncoder:  sonic.Marshal,
		JSONDecoder:  sonic.Unmarshal,
		ErrorHandler: errorHandler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
		BodyLimit:    1024 * 1024,
	})
	middleware.Register(s.App, logger)
	routers.Register(s.App, s.ready.Load)
	s.App.Hooks().OnListen(func(data fiber.ListenData) error {
		s.ready.Store(true)
		logger.Info().Str("host", data.Host).Str("port", data.Port).Msg("listening")
		return nil
	})
	s.App.Hooks().OnPreShutdown(func() error {
		s.ready.Store(false)
		logger.Info().Msg("draining")
		return nil
	})
	s.App.Hooks().OnPostShutdown(func(err error) error {
		// Future owned resources must be closed here, after request draining.
		if err != nil {
			logger.Error().Err(err).Msg("shutdown failed")
		} else {
			logger.Info().Msg("shutdown complete")
		}
		return nil
	})
	return s
}

func (s *Server) Listen(ctx context.Context, address string) error {
	return s.App.Listen(address, fiber.ListenConfig{
		GracefulContext:       ctx,
		ShutdownTimeout:       10 * time.Second,
		DisableStartupMessage: true,
	})
}

func errorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	var httpError *fiber.Error
	if errors.As(err, &httpError) {
		code = httpError.Code
	}
	// Even public errors use a bounded message; raw errors may contain user data.
	return c.Status(code).JSON(fiber.Map{
		"error":      fiber.Map{"code": code, "message": http.StatusText(code)},
		"request_id": requestid.FromContext(c),
	})
}
