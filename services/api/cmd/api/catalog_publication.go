package main

import (
	json "encoding/json/v2"
	"fmt"
	"strconv"

	"github.com/librecosta/gofurry/services/api/internal/catalog"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/spf13/cobra"
)

func catalogPublicationCommand(cfg *platform.Config) *cobra.Command {
	root := &cobra.Command{Use: "catalog-publication", Short: "Apply exceptional catalog publication decisions"}
	type action func(*cobra.Command, []string, *catalog.Service) (any, error)
	add := func(use, short string, count int, apply action) *cobra.Command {
		cmd := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(count), RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Database == nil {
				return catalog.ErrUnavailable
			}
			pool, err := database.Open(cmd.Context(), *cfg.Database)
			if err != nil {
				return catalog.ErrUnavailable
			}
			defer pool.Close()
			out, err := apply(cmd, args, catalog.New(pool))
			if err != nil {
				return err
			}
			data, err := json.Marshal(out)
			if err != nil {
				return catalog.ErrUnavailable
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		}}
		root.AddCommand(cmd)
		return cmd
	}
	var decision catalog.PublicationDecision
	var sfw bool
	change := add("set KIND PUBLIC_ID REVISION ACTION", "Publish a Site or apply an explicit visibility decision", 4,
		func(cmd *cobra.Command, a []string, s *catalog.Service) (any, error) {
			revision, err := strconv.ParseInt(a[2], 10, 64)
			if err != nil || revision < 1 {
				return nil, catalog.ErrInvalid
			}
			switch a[0] {
			case "site":
				return s.ChangeSitePublication(cmd.Context(), a[1], revision, a[3], sfw, decision)
			case "game":
				if !sfw {
					return s.ChangeGamePublication(cmd.Context(), a[1], revision, a[3], decision)
				}
			}
			return nil, catalog.ErrInvalid
		})
	change.Flags().StringVar(&decision.Actor, "actor", "", "Operator assertion")
	change.Flags().StringVar(&decision.Reason, "reason", "", "Decision reason and evidence reference")
	change.Flags().BoolVar(&sfw, "confirm-sfw", false, "Record known SFW evidence as part of the initial Site publication")
	var inherit bool
	diagnose := add("diagnose", "Report legacy Site policy and published Game presentation gaps", 0,
		func(cmd *cobra.Command, _ []string, s *catalog.Service) (any, error) {
			return s.DiagnosePublication(cmd.Context(), inherit)
		})
	diagnose.Flags().BoolVar(&inherit, "inherit-legacy-sfw", false, "Inherit up to 128 Sites' latest recorded SFW decisions")
	return root
}
