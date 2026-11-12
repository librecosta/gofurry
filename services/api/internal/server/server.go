package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/librecosta/gofurry/services/api/internal/catalog"
	citycatalog "github.com/librecosta/gofurry/services/api/internal/cities"
	"github.com/librecosta/gofurry/services/api/internal/collection"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/geoip"
	"github.com/librecosta/gofurry/services/api/internal/handlers"
	"github.com/librecosta/gofurry/services/api/internal/management"
	"github.com/librecosta/gofurry/services/api/internal/middleware"
	"github.com/librecosta/gofurry/services/api/internal/navigation"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/search"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
	"github.com/librecosta/gofurry/services/api/internal/weather"
	"github.com/librecosta/gofurry/services/api/internal/yearbook"
	"github.com/librecosta/gofurry/services/api/routers"
	"github.com/rs/zerolog"
)

type Server struct {
	App               *fiber.App
	Management        *fiber.App
	Wallpapers        *wallpaper.Service
	managementAddress string
	close             func()
	initErr           error
	ready             atomic.Bool
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
	var pool *pgxpool.Pool
	if cfg.Database != nil {
		var err error
		pool, err = database.Open(context.Background(), *cfg.Database)
		if err != nil {
			s.initErr = err
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), cfg.Database.ConnectTimeout)
			if pool.Ping(ctx) != nil {
				logger.Warn().Msg("wallpaper database unavailable; unrelated APIs remain available")
			}
			cancel()
		}
	} else {
		logger.Warn().Msg("wallpaper database is not configured; background selection is unavailable")
	}
	var wallpaperObjects wallpaper.Objects
	if objects != nil {
		wallpaperObjects = objects
	}
	s.Wallpapers = wallpaper.New(pool, wallpaperObjects, cfg.Database != nil)
	if cfg.Management != nil {
		if err := cfg.ValidateWallpaperRuntime(); err != nil {
			s.initErr = err
		}
		if s.initErr == nil {
			s.Management = management.New(logger, cfg, s.Wallpapers)
			s.managementAddress = cfg.Management.Address
		}
	}
	shared := platform.NewRedis(cfg.Redis)
	if shared != nil {
		probe, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		if shared.Ping(probe).Err() != nil {
			logger.Warn().Msg("Redis unavailable; using local cache and limits")
		} else {
			logger.Info().Msg("Redis connected")
		}
		cancel()
	}
	suggestions := search.New(cfg.SearchSuggestions, cfg.Deployment.Region, shared)
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
	weatherService := weather.NewService(provider, shared, cfg.Deployment.Region)
	var cityIndex *citycatalog.Catalog
	if cfg.Cities != nil && cfg.Cities.IndexPath != "" {
		var err error
		cityIndex, err = citycatalog.Open(cfg.Cities.IndexPath)
		if err != nil {
			logger.Warn().Msg("weather city catalog unavailable")
		} else {
			logger.Info().Msg("weather city catalog loaded")
		}
	}
	discovery := navigation.New(pool)
	var interactionOrigin string
	if cfg.Navigation != nil {
		interactionOrigin = cfg.Navigation.Origin
	}
	counter := navigation.NewCounter(discovery, shared, cfg.Navigation.InteractionKey(), cfg.Deployment.Region)
	discovery.Presentation(objects, counter.Enabled())
	interactions := handlers.NavigationInteraction(counter, interactionOrigin, cfg.Server.Environment == "production", len(cfg.Server.TrustedProxies) != 0)
	routers.Register(s.App, s.ready.Load, handlers.Backgrounds(s.Wallpapers), handlers.Suggestions(suggestions, logger), handlers.Location(cities), handlers.Weather(cities, weatherService, cityIndex), handlers.Cities(cityIndex), handlers.NearestCity(cityIndex), shared, cfg.Deployment.Region, catalog.New(pool).WithSiteAssets(objects), yearbook.New(pool), discovery, interactions, counter.Enabled())
	collectionCtx, stopCollection := context.WithCancel(context.Background())
	var collector *collection.Service
	var collectionPool *pgxpool.Pool
	if cfg.Harvest != nil && pool != nil {
		pc := pool.Config()
		// A pinned advisory-lock connection cannot consume the public pool's
		// last slot during a slow remote snapshot.
		pc.MinConns, pc.MaxConns = 0, 1
		pc.ConnConfig.RuntimeParams["application_name"] = "gofurry-collection"
		var err error
		collectionPool, err = pgxpool.NewWithConfig(context.Background(), pc)
		if err != nil {
			logger.Warn().Msg("Harvest collection pool unavailable")
		} else {
			collector = collection.New(collectionPool, *cfg.Harvest)
		}
	}
	var yearbookPool *pgxpool.Pool
	if cfg.Yearbook != nil && cfg.Yearbook.Enabled && pool != nil {
		pc := pool.Config()
		pc.MinConns, pc.MaxConns = 0, 1
		pc.ConnConfig.RuntimeParams["application_name"] = "gofurry-yearbook"
		var err error
		yearbookPool, err = pgxpool.NewWithConfig(context.Background(), pc)
		if err != nil {
			logger.Warn().Msg("Yearbook maintenance pool unavailable")
		}
	}
	var runCollection sync.Once
	var collectionWG sync.WaitGroup
	var heatPool *pgxpool.Pool
	if cfg.Navigation != nil && cfg.Navigation.HeatEnabled && pool != nil {
		pc := pool.Config()
		pc.MinConns, pc.MaxConns = 0, 1
		pc.ConnConfig.RuntimeParams["application_name"] = "gofurry-heat"
		var err error
		heatPool, err = pgxpool.NewWithConfig(context.Background(), pc)
		if err != nil {
			logger.Warn().Msg("Site heat maintenance pool unavailable")
		}
	}
	s.App.Hooks().OnListen(func(data fiber.ListenData) error {
		runCollection.Do(func() {
			if collector != nil {
				collectionWG.Go(func() {
					collector.Run(collectionCtx, func(code string) {
						logger.Warn().Str("reason", code).Msg("Harvest collection deferred")
					})
				})
			}
			if yearbookPool != nil {
				collectionWG.Go(func() {
					yearbook.New(yearbookPool).Run(collectionCtx, func(err error) {
						logger.Warn().Str("reason", err.Error()).Msg("Yearbook maintenance deferred")
					})
				})
			}
			if heatPool != nil {
				collectionWG.Go(func() {
					navigation.New(heatPool).Run(collectionCtx, func() { logger.Warn().Msg("Site heat maintenance deferred") })
				})
			}
		})
		s.ready.Store(true)
		logger.Info().Str("host", data.Host).Str("port", data.Port).Msg("listening")
		return nil
	})
	s.App.Hooks().OnPreShutdown(func() error {
		s.ready.Store(false)
		stopCollection()
		logger.Info().Msg("draining")
		return nil
	})
	var once sync.Once
	s.close = func() {
		once.Do(func() {
			stopCollection()
			collectionWG.Wait()
			if collector != nil {
				collector.Close()
				collectionPool.Close()
			}
			if yearbookPool != nil {
				yearbookPool.Close()
			}
			if heatPool != nil {
				heatPool.Close()
			}
			suggestions.Close()
			weatherService.Close()
			if shared != nil {
				if shared.Close() != nil {
					logger.Warn().Msg("Redis close failed")
				}
			}
			if closeErr := cities.Close(); closeErr != nil {
				logger.Warn().Msg("city database close failed")
			}
			if pool != nil {
				pool.Close()
			}
			if objects != nil {
				objects.Close()
			}
		})
	}
	s.App.Hooks().OnPostShutdown(func(err error) error {
		if s.Management == nil {
			s.close()
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
	defer s.close()
	if s.initErr != nil {
		return s.initErr
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	public, err := net.Listen("tcp", address)
	if err != nil {
		return errors.New("public listener unavailable")
	}
	defer public.Close()
	var internal net.Listener
	if s.Management != nil {
		internal, err = net.Listen("tcp", s.managementAddress)
		if err != nil {
			return errors.New("management listener unavailable")
		}
		defer internal.Close()
	}
	options := fiber.ListenConfig{GracefulContext: ctx, ShutdownTimeout: 10 * time.Second, DisableStartupMessage: true}
	results := make(chan error, 2)
	count := 1
	go func() { results <- s.App.Listener(public, options) }()
	if internal != nil {
		count++
		go func() { results <- s.Management.Listener(internal, options) }()
	}
	var failure error
	for range count {
		if e := <-results; e != nil {
			failure = errors.New("HTTP lifecycle failed")
		}
		cancel()
	}
	return failure
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
