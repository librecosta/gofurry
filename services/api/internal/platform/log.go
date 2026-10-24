package platform

import (
	"io"
	"time"

	"github.com/rs/zerolog"
)

func NewLogger(output io.Writer, environment string) zerolog.Logger {
	if environment == "development" {
		output = zerolog.ConsoleWriter{Out: output, TimeFormat: time.RFC3339}
	}
	return zerolog.New(output).With().Timestamp().Str("service", "gofurry-api").Str("environment", environment).Logger()
}
