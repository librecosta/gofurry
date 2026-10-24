package middleware

import (
	fiberzerolog "github.com/gofiber/contrib/v3/zerolog"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/rs/zerolog"
)

func Register(app *fiber.App, logger zerolog.Logger) {
	// ID -> access logger -> recover -> headers -> handlers. Recover converts panics
	// before the outer access logger renders the unified error and records its status.
	app.Use(requestid.New())
	app.Use(fiberzerolog.New(fiberzerolog.Config{
		Logger:          &logger,
		FieldsSnakeCase: true,
		Fields: []string{fiberzerolog.FieldMethod, fiberzerolog.FieldRoute,
			fiberzerolog.FieldStatus, fiberzerolog.FieldLatency, fiberzerolog.FieldRequestID},
	}))
	// Do not log raw errors, paths, queries, headers or bodies: these can contain credentials.
	app.Use(recover.New())
	app.Use(helmet.New())
}
