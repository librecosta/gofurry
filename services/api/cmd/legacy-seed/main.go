package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/librecosta/gofurry/services/api/internal/legacyseed"
	"github.com/spf13/cobra"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := command().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func command() *cobra.Command {
	var configPath, output string
	root := &cobra.Command{
		Use:          "legacy-seed",
		Short:        "Audit isolated GoFurry legacy backups and prepare a private D5-A candidate package",
		SilenceUsage: true, SilenceErrors: true, Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return errors.New("expected inspect, extract or verify") },
	}
	root.SetFlagErrorFunc(func(*cobra.Command, error) error { return errors.New("invalid command flags; see --help") })
	root.PersistentFlags().StringVar(&configPath, "config", "", "explicit private YAML file")
	root.PersistentFlags().StringVar(&output, "output", "", "override the private output directory")
	shorts := []struct{ name, summary string }{
		{"inspect", "write only aggregate counts and source quality diagnostics"},
		{"extract", "write and verify a private NDJSON and Site Logo candidate package"},
		{"verify", "verify an existing candidate package offline"},
	}
	for _, entry := range shorts {
		cmd := &cobra.Command{Use: entry.name, Short: entry.summary, Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.New("unexpected positional arguments")
			}
			return nil
		}}
		cmd.RunE = func(cmd *cobra.Command, _ []string) error {
			config, err := legacyseed.LoadConfig(configPath, output)
			if err != nil {
				return err
			}
			result, err := legacyseed.Run(cmd.Context(), entry.name, config)
			if writeErr := json.MarshalWrite(cmd.OutOrStdout(), result); writeErr != nil {
				return errors.New("cannot write result")
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return err
		}
		root.AddCommand(cmd)
	}
	return root
}
