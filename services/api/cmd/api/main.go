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
	"github.com/spf13/cobra"
)

func newCommand() *cobra.Command {
	var path string
	var cfg platform.Config
	cmd := &cobra.Command{
		Use: "api", Short: "GoFurry API", SilenceUsage: true, SilenceErrors: true,
		Args:              cobra.NoArgs,
		PersistentPreRunE: func(*cobra.Command, []string) (err error) { cfg, err = platform.LoadConfig(path); return err },
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := platform.NewLogger(cmd.OutOrStdout(), cfg.Server.Environment)
			return server.New(logger, cfg).Listen(cmd.Context(), cfg.Server.Address)
		},
	}
	cmd.PersistentFlags().StringVar(&path, "config", "", "single YAML configuration file (required)")
	_ = cmd.MarkPersistentFlagRequired("config")
	cmd.AddCommand(backgroundImportCommand(&cfg))
	cmd.AddCommand(migrateCommand(&cfg))
	cmd.AddCommand(collectionCommand(&cfg))
	cmd.AddCommand(gameIntakeCommand(&cfg))
	cmd.AddCommand(catalogPublicationCommand(&cfg))
	cmd.AddCommand(yearbookCommand(&cfg))
	cmd.AddCommand(navigationCommand(&cfg))
	cmd.AddCommand(siteAssetsCommand(&cfg))
	cmd.AddCommand(siteProfileCommand(&cfg))
	cmd.AddCommand(wallpaperImportCommand(&cfg))
	cmd.AddCommand(&cobra.Command{
		Use: "healthcheck", Short: "Probe the listener configured by --config", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := http.Client{Timeout: 2 * time.Second}
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, cfg.ProbeURL(), nil)
			if err != nil {
				return err
			}
			res, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("healthcheck failed")
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("healthcheck status %d", res.StatusCode)
			}
			return nil
		},
	})
	return cmd
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd := newCommand()
	cmd.SetOut(os.Stdout)
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
