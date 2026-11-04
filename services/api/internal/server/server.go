package server

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/librecosta/gofurry/services/api/internal/geoip"
	"github.com/librecosta/gofurry/services/api/internal/handlers"
	"github.com/librecosta/gofurry/services/api/internal/middleware"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/search"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"github.com/librecosta/gofurry/services/api/internal/weather"
	"github.com/librecosta/gofurry/services/api/routers"
	"github.com/rs/zerolog"
)

type Server struct {
	App   *fiber.App
	ready atomic.Bool
}

func New(logger zerolog.Logger, cfg platform.Config) *Server {
	logger = logger.With().Str("region", cfg.Deployment.Region).Logger()
	s := &Server{}
	s.App = fiber.New(fiber.Config{
		AppName:            "gofurry-api",
		JSONEncoder:        platform.MarshalJSON,
		JSONDecoder:        platform.UnmarshalJSON,
		ErrorHandler:       errorHandler,
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       10 * time.Second,
		IdleTimeout:        30 * time.Second,
		BodyLimit:          1024 * 1024,
		TrustProxy:         len(cfg.Server.TrustedProxies) > 0,
		TrustProxyConfig:   fiber.TrustProxyConfig{Proxies: cfg.Server.TrustedProxies},
		ProxyHeader:        fiber.HeaderXForwardedFor,
		EnableIPValidation: true,
	})
	middleware.Register(s.App, logger, cfg.Server.Environment == "production", cfg.Logging.SlowRequest)
	s.App.Use(func(c fiber.Ctx) error {
		c.Set("X-GoFurry-Region", cfg.Deployment.Region)
		c.Set("X-GoFurry-Content-Mode", "SFW")
		return c.Next()
	})
	var objects *storage.Store
	if cfg.Storage != nil {
		objects = storage.New(*cfg.Storage)
	}
	suggestions := search.New(cfg.SearchSuggestions, cfg.Deployment.Region)
	var cities *geoip.Resolver
	if cfg.GeoIP != nil && cfg.GeoIP.DatabasePath != "" {
		var err error
		cities, err = geoip.Open(cfg.GeoIP.DatabasePath)
		if err != nil {
			logger.Warn().Msg("city database unavailable")
		} else {
			logger.Info().Msg("city database loaded")
		}
	}
	provider, weatherErr := weather.NewProvider(cfg.QWeather)
	if weatherErr != nil {
		logger.Warn().Strs("missing_fields", cfg.QWeather.MissingFields()).Msg("weather provider unavailable")
	}
	weatherService := weather.NewService(provider)
	routers.Register(s.App, s.ready.Load, handlers.Backgrounds(objects), handlers.Suggestions(suggestions), handlers.Location(cities), handlers.Weather(cities, weatherService))
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
		suggestions.Close()
		weatherService.Close()
		if closeErr := cities.Close(); closeErr != nil {
			logger.Warn().Msg("city database close failed")
		}
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
