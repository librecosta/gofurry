package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/spf13/cobra"
)

func gameAssetCommand(cfg *platform.Config) *cobra.Command {
	c := &cobra.Command{Use: "game-assets", Short: "Regional game asset maintenance"}
	c.AddCommand(&cobra.Command{
		Use: "backfill", Args: cobra.NoArgs, Short: "Normalize existing trusted media without changing existing game publications",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cfg.Database == nil {
				return errors.New("game asset backfill requires an explicit GoFurry database")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Minute)
			defer cancel()
			pool, err := database.Open(ctx, *cfg.Database)
			if err != nil {
				return err
			}
			defer pool.Close()
			report, err := catalog.BackfillGameAssets(ctx, pool)
			if encodingErr := json.MarshalWrite(cmd.OutOrStdout(), report); encodingErr != nil {
				return errors.Join(err, encodingErr)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return err
		},
	})
	return c
}
