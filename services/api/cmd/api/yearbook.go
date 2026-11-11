package main

import (
	json "encoding/json/v2"
	"fmt"
	"strconv"

	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/yearbook"
	"github.com/spf13/cobra"
)

func yearbookCommand(cfg *platform.Config) *cobra.Command {
	root := &cobra.Command{Use: "yearbook", Short: "Review and maintain regional Site yearbooks"}
	action := func(use, short string, args cobra.PositionalArgs, run func(*cobra.Command, []string, *yearbook.Service) error) *cobra.Command {
		return &cobra.Command{Use: use, Short: short, Args: args, RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Database == nil {
				return yearbook.ErrUnavailable
			}
			pool, err := database.Open(cmd.Context(), *cfg.Database)
			if err != nil {
				return yearbook.ErrUnavailable
			}
			defer pool.Close()
			return run(cmd, args, yearbook.New(pool))
		}}
	}
	emit := func(cmd *cobra.Command, value any, err error) error {
		if err != nil {
			return err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return yearbook.ErrUnavailable
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return err
	}
	root.AddCommand(action("preview PUBLIC_ID YEAR", "Read the proposed document and eligibility without writing", cobra.ExactArgs(2),
		func(cmd *cobra.Command, args []string, svc *yearbook.Service) error {
			year, err := yearbook.ParseYear(args[1])
			if err != nil {
				return err
			}
			result, err := svc.Preview(cmd.Context(), args[0], year)
			return emit(cmd, result, err)
		}))
	var apply, dry bool
	var selectedYear, site string
	reconcile := action("reconcile", "Preview a bounded batch or explicitly apply generated revisions", cobra.NoArgs,
		func(cmd *cobra.Command, _ []string, svc *yearbook.Service) error {
			var year int32
			var err error
			if apply && dry {
				return yearbook.ErrInvalid
			}
			if selectedYear != "" {
				year, err = yearbook.ParseYear(selectedYear)
				if err != nil {
					return err
				}
			}
			if site != "" {
				if year == 0 {
					return yearbook.ErrInvalid
				}
				if apply {
					result, err := svc.Generate(cmd.Context(), site, year)
					return emit(cmd, result, err)
				}
				result, err := svc.Preview(cmd.Context(), site, year)
				return emit(cmd, result, err)
			}
			result, err := svc.Reconcile(cmd.Context(), year, apply)
			if outputErr := emit(cmd, result, nil); outputErr != nil {
				return outputErr
			}
			return err
		})
	reconcile.Flags().StringVar(&selectedYear, "year", "", "UTC calendar year")
	reconcile.Flags().StringVar(&site, "site", "", "Restrict to one Site Public ID")
	reconcile.Flags().BoolVar(&dry, "dry-run", false, "Preview only (default)")
	reconcile.Flags().BoolVar(&apply, "apply", false, "Persist qualified snapshots and first publications")
	root.AddCommand(reconcile)
	root.AddCommand(action("publish PUBLIC_ID YEAR REVISION DIGEST", "Publish a reviewed latest revision with its exact digest", cobra.ExactArgs(4),
		func(cmd *cobra.Command, args []string, svc *yearbook.Service) error {
			year, err := yearbook.ParseYear(args[1])
			if err != nil {
				return err
			}
			revision, err := strconv.ParseInt(args[2], 10, 32)
			if err != nil {
				return yearbook.ErrInvalid
			}
			return svc.Publish(cmd.Context(), args[0], year, int32(revision), args[3])
		}))
	root.AddCommand(action("hide PUBLIC_ID YEAR", "Withdraw public visibility while retaining revisions", cobra.ExactArgs(2),
		func(cmd *cobra.Command, args []string, svc *yearbook.Service) error {
			year, err := yearbook.ParseYear(args[1])
			if err != nil {
				return err
			}
			return svc.Hide(cmd.Context(), args[0], year)
		}))
	root.AddCommand(action("compact", "Compact one bounded set of eligible coverage rows", cobra.NoArgs,
		func(cmd *cobra.Command, _ []string, svc *yearbook.Service) error {
			count, err := svc.Compact(cmd.Context())
			return emit(cmd, map[string]int32{"daily_rows_compacted": count}, err)
		}))
	root.AddCommand(action("audit", "Check ownership and revision references without writing", cobra.NoArgs,
		func(cmd *cobra.Command, _ []string, svc *yearbook.Service) error {
			result, err := svc.Audit(cmd.Context())
			if err := emit(cmd, result, err); err != nil {
				return err
			}
			for _, row := range result {
				if row.Mismatches != 0 {
					return yearbook.ErrConflict
				}
			}
			return nil
		}))
	return root
}
