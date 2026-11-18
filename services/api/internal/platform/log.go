package platform

import (
	"io"

	"github.com/rs/zerolog"
)

func NewLogger(output io.Writer) zerolog.Logger {
	return zerolog.New(output).With().Timestamp().Str("service", "gofurry-api").Logger()
}
