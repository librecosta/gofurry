package main

import (
	json "encoding/json/v2"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/spf13/cobra"
)

func siteProfileCommand(cfg *platform.Config) *cobra.Command {
	root := &cobra.Command{Use: "site-profile", Short: "Edit reviewed Site profile text"}
	var reviewed bool
	set := &cobra.Command{
		Use: "set-description PUBLIC_ID LOCALE REVISION FILE_OR_DASH", Short: "Set one localized description from a text file or clear it with '-'", Args: cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, locale, file := args[0], args[1], args[3]
			revision, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil || revision < 1 || !catalog.ValidPublicID(id) || (locale != "en" && locale != "zh") || !reviewed || cfg.Database == nil {
				return catalog.ErrInvalid
			}
			body, err := readDescriptionFile(file)
			if err != nil {
				return err
			}
			pool, err := database.Open(cmd.Context(), *cfg.Database)
			if err != nil {
				return catalog.ErrUnavailable
			}
			defer pool.Close()
			next, err := catalog.New(pool).SetSiteDescription(cmd.Context(), id, revision, locale, body)
			if err != nil {
				return err
			}
			raw, err := json.Marshal(map[string]any{"public_id": id, "locale": locale, "revision": next, "source": "site-profile-cli"})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return err
		},
	}
	set.Flags().BoolVar(&reviewed, "confirm-reviewed", false, "Confirm editorial review of this description")
	root.AddCommand(set)
	return root
}

func readDescriptionFile(path string) (string, error) {
	if path == "-" {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > catalog.SiteDescriptionLimit {
		return "", catalog.ErrInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return "", catalog.ErrInvalid
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, catalog.SiteDescriptionLimit+1))
	if err != nil || len(body) > catalog.SiteDescriptionLimit {
		return "", catalog.ErrInvalid
	}
	return string(body), nil
}
