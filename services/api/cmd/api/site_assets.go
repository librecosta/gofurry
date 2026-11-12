package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"github.com/spf13/cobra"
)

func siteAssetsCommand(cfg *platform.Config) *cobra.Command {
	root := &cobra.Command{Use: "site-assets", Short: "Import and bind reviewed Site display assets"}
	printResult := func(cmd *cobra.Command, result any) error {
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return err
	}
	var reviewed, preflight bool
	upload := &cobra.Command{Use: "import KIND FILE", Short: "Inspect or import a reviewed logo or cover from a local file", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if !preflight && (!reviewed || cfg.Storage == nil) {
			return errors.New("import requires explicit storage and --confirm-reviewed")
		}
		file, err := os.Open(args[1])
		if err != nil {
			return errors.New("cannot open local Site asset")
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return storage.ErrSiteAsset
		}
		var store *storage.Store
		if !preflight {
			store = storage.New(*cfg.Storage)
			defer store.Close()
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
		defer cancel()
		result, err := store.ImportSiteAsset(ctx, storage.SiteAssetKind(args[0]), file, preflight)
		if err != nil {
			return err
		}
		return printResult(cmd, result)
	}}
	upload.Flags().BoolVar(&reviewed, "confirm-reviewed", false, "Confirm safe content and permission to publish")
	upload.Flags().BoolVar(&preflight, "preflight", false, "Inspect bytes without writing storage")
	root.AddCommand(upload)
	var bindingReviewed bool
	bind := &cobra.Command{Use: "bind PUBLIC_ID REVISION KIND KEY_OR_DASH", Short: "Bind a verified asset or clear its reference with '-'", Args: cobra.ExactArgs(4), RunE: func(cmd *cobra.Command, args []string) error {
		id, key, kind := args[0], args[3], storage.SiteAssetKind(args[2])
		revision, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil || revision < 1 || !catalog.ValidPublicID(id) || cfg.Database == nil {
			return catalog.ErrInvalid
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
		defer cancel()
		if key == "-" {
			key = ""
		} else {
			if cfg.Storage == nil || !bindingReviewed || !kind.ValidKey(key) {
				return storage.ErrSiteAsset
			}
			store := storage.New(*cfg.Storage)
			defer store.Close()
			if _, err = store.VerifySiteAsset(ctx, kind, key); err != nil {
				return err
			}
		}
		pool, err := database.Open(ctx, *cfg.Database)
		if err != nil {
			return catalog.ErrUnavailable
		}
		defer pool.Close()
		next, err := catalog.New(pool).SetSiteAsset(ctx, id, revision, kind, key)
		if err != nil {
			return err
		}
		return printResult(cmd, map[string]any{"public_id": id, "revision": next})
	}}
	bind.Flags().BoolVar(&bindingReviewed, "confirm-reviewed", false, "Confirm asset and permission review for this Site")
	root.AddCommand(bind)
	return root
}
