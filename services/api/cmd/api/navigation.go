package main

import (
	json "encoding/json/v2"
	"fmt"
	"strconv"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/navigation"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/spf13/cobra"
)

func navigationCommand(cfg *platform.Config) *cobra.Command {
	root := &cobra.Command{Use: "navigation", Short: "Review regional Sites and maintain fixed navigation slots"}
	type action func(*cobra.Command, []string, *catalog.Service, *navigation.Service) (any, error)
	add := func(use, short string, args cobra.PositionalArgs, run action) *cobra.Command {
		cmd := &cobra.Command{Use: use, Short: short, Args: args, RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Database == nil {
				return catalog.ErrUnavailable
			}
			pool, err := database.Open(cmd.Context(), *cfg.Database)
			if err != nil {
				return catalog.ErrUnavailable
			}
			defer pool.Close()
			value, err := run(cmd, args, catalog.New(pool), navigation.New(pool))
			if err != nil {
				return err
			}
			raw, err := json.Marshal(value)
			if err != nil {
				return catalog.ErrUnavailable
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return err
		}}
		root.AddCommand(cmd)
		return cmd
	}
	add("inspect PUBLIC_ID", "Read the current Site navigation profile and revision", cobra.ExactArgs(1),
		func(cmd *cobra.Command, args []string, edit *catalog.Service, _ *navigation.Service) (any, error) {
			return edit.InspectSiteNavigation(cmd.Context(), args[0])
		})
	var in catalog.NavigationEdit
	review := add("review PUBLIC_ID REVISION", "Replace the audited rating and navigation profile", cobra.ExactArgs(2),
		func(cmd *cobra.Command, args []string, edit *catalog.Service, _ *navigation.Service) (any, error) {
			rev, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return nil, catalog.ErrInvalid
			}
			return edit.ReviewSiteNavigation(cmd.Context(), cfg.Deployment.Region, args[0], rev, in)
		})
	review.Flags().StringVar(&in.Rating, "rating", "", "unreviewed, sfw, mixed, or adult (subject to deployment)")
	review.Flags().StringVar(&in.Primary, "primary", "", "Primary category code")
	review.Flags().StringSliceVar(&in.Secondary, "secondary", nil, "Up to two supplemental category codes")
	review.Flags().StringSliceVar(&in.Tags, "tags", nil, "Reviewed tag codes")
	review.Flags().StringSliceVar(&in.Languages, "languages", nil, "Verified supported language codes")
	review.Flags().StringVar(&in.Actor, "actor", "", "Operator assertion")
	review.Flags().StringVar(&in.Reason, "reason", "", "Review reason")
	add("features", "Preview all three slots and their effective visibility", cobra.NoArgs,
		func(cmd *cobra.Command, _ []string, edit *catalog.Service, _ *navigation.Service) (any, error) {
			return edit.NavigationFeatures(cmd.Context())
		})
	var actor, reason string
	feature := add("set-feature SLOT REVISION PUBLIC_ID_OR_DASH", "Assign a reviewed Site or clear a fixed slot with '-'",
		cobra.ExactArgs(3), func(cmd *cobra.Command, args []string, edit *catalog.Service, _ *navigation.Service) (any, error) {
			rev, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return nil, catalog.ErrInvalid
			}
			id := args[2]
			if id == "-" {
				id = ""
			}
			rev, err = edit.SetNavigationFeature(cmd.Context(), args[0], id, rev, actor, reason)
			return map[string]int64{"revision": rev}, err
		})
	feature.Flags().StringVar(&actor, "actor", "", "Operator assertion")
	feature.Flags().StringVar(&reason, "reason", "", "Editorial reason")
	add("refresh-heat", "Explicitly recompute Heat v1 in the regional database", cobra.NoArgs,
		func(cmd *cobra.Command, _ []string, _ *catalog.Service, service *navigation.Service) (any, error) {
			return service.RefreshHeat(cmd.Context(), true)
		})
	return root
}
