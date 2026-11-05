package main

import (
	"errors"
	"fmt"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"github.com/librecosta/gofurry/services/api/internal/wallpaper"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
)

func backgroundImportCommand(cfg *platform.Config) *cobra.Command {
	return &cobra.Command{Use: "background-import FILE", Short: "Import one fully validated development object without creating a wallpaper record", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Server.Environment != "development" || cfg.Storage == nil {
			return errors.New("background-import requires development YAML with explicit storage configuration")
		}
		file, e := os.Open(args[0])
		if e != nil {
			return errors.New("cannot open background file")
		}
		defer file.Close()
		store := storage.New(*cfg.Storage)
		defer store.Close()
		typ := "image/" + strings.TrimPrefix(strings.ToLower(filepath.Ext(args[0])), ".")
		a, e := wallpaper.New(nil, store, false).ImportObject(cmd.Context(), file, typ)
		if e != nil {
			return errors.New("file must contain AVIF or WebP meeting public format, privacy and resource limits; storage verification must succeed")
		}
		data, e := platform.MarshalJSON(a)
		if e != nil {
			return e
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}}
}
