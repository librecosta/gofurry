package main

import (
	"errors"
	"fmt"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
	"github.com/spf13/cobra"
	"net"
	"os"
)

func wallpaperImportCommand(cfg *platform.Config) *cobra.Command {
	var bucket, public string
	var reviewed bool
	cmd := &cobra.Command{Use: "wallpaper-import FILE", Short: "Import reviewed static catalog from confirmed development storage; never uploads", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		host, _, e := net.SplitHostPort(cfg.Server.Address)
		if e != nil || !net.ParseIP(host).IsLoopback() || cfg.Server.Environment != "development" || cfg.Database == nil || net.ParseIP(cfg.Database.Host) == nil || !net.ParseIP(cfg.Database.Host).IsLoopback() || cfg.Storage == nil || bucket == "" || public == "" || cfg.Storage.Bucket != bucket || cfg.Storage.PublicBaseURL != public || !reviewed {
			return errors.New("wallpaper-import requires explicit local development database/storage, matching bucket/public-base confirmation and reviewed-SFW confirmation")
		}
		f, e := os.Open(args[0])
		if e != nil {
			return errors.New("cannot open static catalog")
		}
		defer f.Close()
		items, e := wallpaper.ReadLegacy(f)
		if e != nil {
			return errors.New("invalid static catalog")
		}
		pool, e := database.Open(cmd.Context(), *cfg.Database)
		if e != nil {
			return e
		}
		defer pool.Close()
		store := storage.New(*cfg.Storage)
		defer store.Close()
		created, skipped, e := wallpaper.New(pool, store, true).Import(cmd.Context(), items)
		if e != nil {
			return errors.New("development wallpaper import failed; rerun is idempotent, no objects were changed")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Imported %d; already present %d\n", created, skipped)
		return nil
	}}
	cmd.Flags().StringVar(&bucket, "development-bucket", "", "exact isolated development bucket confirmation")
	cmd.Flags().StringVar(&public, "development-public-base", "", "exact development public base confirmation")
	cmd.Flags().BoolVar(&reviewed, "confirm-reviewed-sfw", false, "confirm human SFW and public-use review of every imported item")
	return cmd
}
