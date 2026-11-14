package main

import (
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/librecosta/gofurry/services/api/internal/collection"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/spf13/cobra"
)

func gameDiscoveryCommand(cfg *platform.Config) *cobra.Command {
	root := &cobra.Command{Use: "game-discovery", Short: "Operate the private Steam discovery cursor"}
	for _, op := range []string{"status", "scan-once", "pause", "resume", "backfill"} {
		root.AddCommand(&cobra.Command{Use: op, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			if cfg.Database == nil || cfg.Harvest == nil || cfg.Harvest.Discovery == nil {
				return errors.New("game discovery requires a regional database and explicit harvest.discovery YAML")
			}
			ctx := cmd.Context()
			pool, err := database.Open(ctx, *cfg.Database)
			if err != nil {
				return errors.New("game discovery database unavailable")
			}
			defer pool.Close()
			intake := collection.NewIntake(pool, cfg.Harvest)
			defer intake.Close()
			d := collection.NewDiscovery(pool, cfg.Harvest, intake)
			defer d.Close()
			switch op {
			case "scan-once":
				err = d.Tick(ctx)
			case "pause", "resume", "backfill":
				err = d.Configure(ctx, op == "pause", op == "backfill")
			}
			if err != nil {
				return errors.New("game discovery is busy or could not apply the requested operation")
			}
			status, err := d.Status(ctx)
			if err != nil {
				return errors.New("game discovery status unavailable")
			}
			data, err := json.Marshal(status)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		}})
	}
	return root
}
