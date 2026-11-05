package main

import (
	"errors"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/spf13/cobra"
)

func migrateCommand(cfg *platform.Config) *cobra.Command {
	var dir string
	c := &cobra.Command{Use: "migrate status|up", Short: "Explicit GoFurry PostgreSQL maintenance", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Database == nil {
			return errors.New("migrate requires an explicit GoFurry database")
		}
		return database.Migrate(cmd.Context(), *cfg.Database, dir, args[0], cmd.OutOrStdout())
	}}
	c.Flags().StringVar(&dir, "dir", "migrations", "migration directory")
	return c
}
