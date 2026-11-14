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

func gameChannelCommand(cfg *platform.Config) *cobra.Command {
	root := &cobra.Command{Use: "game-channel", Short: "Curate game features and collections"}
	use := func(usage string, count int, run func(*cobra.Command, *catalog.Service, []string) (any, error)) {
		root.AddCommand(&cobra.Command{Use: usage, Args: cobra.ExactArgs(count), RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Database == nil {
				return catalog.ErrUnavailable
			}
			pool, err := database.Open(cmd.Context(), *cfg.Database)
			if err != nil {
				return catalog.ErrUnavailable
			}
			defer pool.Close()
			result, err := run(cmd, catalog.New(pool), args)
			if err != nil {
				return err
			}
			raw, err := json.Marshal(result)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return err
		}})
	}
	use("features", 0, func(cmd *cobra.Command, s *catalog.Service, _ []string) (any, error) {
		return s.GameChannelFeatures(cmd.Context())
	})
	use("feature SLOT REVISION PUBLIC_ID_OR_DASH", 3, func(cmd *cobra.Command, s *catalog.Service, args []string) (any, error) {
		rev, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return nil, catalog.ErrInvalid
		}
		id := args[2]
		if id == "-" {
			id = ""
		}
		next, err := s.SetGameChannelFeature(cmd.Context(), args[0], id, rev)
		return map[string]any{"slot": args[0], "public_id": id, "revision": next}, err
	})
	use("inspect PUBLIC_ID", 1, func(cmd *cobra.Command, s *catalog.Service, args []string) (any, error) {
		return s.InspectGameCollection(cmd.Context(), args[0])
	})
	use("create EDITORIAL_JSON", 1, func(cmd *cobra.Command, s *catalog.Service, args []string) (any, error) {
		var text struct {
			Slug          string                 `json:"slug"`
			Localizations []catalog.Localization `json:"localizations"`
		}
		if err := readChannelJSON(args[0], &text); err != nil {
			return nil, err
		}
		return s.CreateGameCollection(cmd.Context(), catalog.Editorial{Slug: text.Slug, Localizations: text.Localizations})
	})
	change := func(usage string, run func(*cobra.Command, *catalog.Service, string, int64, string) (any, error)) {
		use(usage, 3, func(cmd *cobra.Command, s *catalog.Service, args []string) (any, error) {
			rev, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return nil, catalog.ErrInvalid
			}
			return run(cmd, s, args[0], rev, args[2])
		})
	}
	change("edit PUBLIC_ID REVISION EDITORIAL_JSON", func(cmd *cobra.Command, s *catalog.Service, id string, rev int64, path string) (any, error) {
		var text struct {
			Slug          string                 `json:"slug"`
			Localizations []catalog.Localization `json:"localizations"`
		}
		if err := readChannelJSON(path, &text); err != nil {
			return nil, err
		}
		return s.EditGameCollection(cmd.Context(), id, rev, catalog.Editorial{Slug: text.Slug, Localizations: text.Localizations})
	})
	change("members PUBLIC_ID REVISION ORDERED_PUBLIC_IDS_JSON", func(cmd *cobra.Command, s *catalog.Service, id string, rev int64, path string) (any, error) {
		var ids []string
		if err := readChannelJSON(path, &ids); err != nil || ids == nil {
			return nil, catalog.ErrInvalid
		}
		return s.SetGameCollectionMembers(cmd.Context(), id, rev, ids)
	})
	change("status PUBLIC_ID REVISION published|archived", func(cmd *cobra.Command, s *catalog.Service, id string, rev int64, status string) (any, error) {
		return s.SetGameCollectionStatus(cmd.Context(), id, rev, status)
	})
	return root
}

func readChannelJSON(path string, out any) error {
	const maxBytes = 20 << 10
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
		return catalog.ErrInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return catalog.ErrInvalid
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || len(body) > maxBytes || json.Unmarshal(body, out, json.RejectUnknownMembers(true)) != nil {
		return catalog.ErrInvalid
	}
	return nil
}
