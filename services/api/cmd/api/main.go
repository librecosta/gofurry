package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/server"
)

func main() {
	// The same binary supplies a probe without a shell or curl in the runtime image.
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		client := http.Client{Timeout: 2 * time.Second}
		res, err := client.Get("http://127.0.0.1:8080/readyz")
		if err != nil {
			fmt.Fprintln(os.Stderr, "healthcheck failed")
			os.Exit(1)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}
	cfg, err := platform.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	logger := platform.NewLogger(os.Stdout, cfg.Environment)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.New(logger).Listen(ctx, cfg.Address); err != nil {
		logger.Error().Err(err).Msg("server failed")
		os.Exit(1)
	}
}
