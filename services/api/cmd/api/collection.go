package main

import (
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/collection"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/spf13/cobra"
)

func collectionCommand(cfg *platform.Config) *cobra.Command {
	root := &cobra.Command{Use: "collection", Short: "Manage reviewed Harvest collection intent privately"}
	action := func(use, short string, args cobra.PositionalArgs, run func(*cobra.Command, []string, *collection.Service) error) *cobra.Command {
		return &cobra.Command{Use: use, Short: short, Args: args, RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Harvest == nil || cfg.Harvest.WriterToken == "" || cfg.Database == nil {
				return errors.New("collection requires an explicit writer YAML")
			}
			pool, err := database.Open(cmd.Context(), *cfg.Database)
			if err != nil {
				return err
			}
			defer pool.Close()
			service := collection.New(pool, *cfg.Harvest)
			defer service.Close()
			if err = run(cmd, args, service); err != nil {
				return fmt.Errorf("collection failed (%s)", collection.Diagnostic(err))
			}
			return nil
		}}
	}
	root.AddCommand(action("bind-site PUBLIC_ID ADDRESS", "Bind one existing Site address to origin and domain targets", cobra.ExactArgs(2),
		func(cmd *cobra.Command, args []string, s *collection.Service) error {
			bindings, err := s.BindSite(cmd.Context(), args[0], args[1])
			if err == nil {
				for _, b := range bindings {
					fmt.Fprintln(cmd.OutOrStdout(), b.ResourceType, b.ID)
				}
			}
			return err
		}))
	root.AddCommand(action("bind-steam PUBLIC_ID APP_ID", "Bind an existing active Steam identity", cobra.ExactArgs(2),
		func(cmd *cobra.Command, args []string, s *collection.Service) error {
			b, err := s.BindGame(cmd.Context(), args[0], args[1])
			if err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), b.ID)
			}
			return err
		}))
	root.AddCommand(action("unbind BINDING_ID", "Close an approved binding and retain attributed evidence", cobra.ExactArgs(1),
		func(cmd *cobra.Command, args []string, s *collection.Service) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return collection.ErrIntent
			}
			return s.Unbind(cmd.Context(), id)
		}))
	var plan collection.Plan
	var anchor string
	planCommand := action("plan BINDING_ID CAPABILITY", "Record one approved typed schedule", cobra.ExactArgs(2),
		func(cmd *cobra.Command, args []string, s *collection.Service) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return collection.ErrIntent
			}
			plan.Anchor, err = time.Parse(time.RFC3339Nano, anchor)
			if err != nil {
				return collection.ErrIntent
			}
			plan.Capability = args[1]
			result, err := s.AddPlan(cmd.Context(), id, plan)
			if err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), result)
			}
			return err
		})
	planCommand.Flags().StringVar(&anchor, "anchor", "", "First eligible UTC slot (RFC3339)")
	planCommand.Flags().DurationVar(&plan.Interval, "interval", 0, "Explicit recurrence interval")
	planCommand.Flags().StringVar(&plan.Locale, "locale", "", "Semantic locale, en or zh-cn for Steam")
	planCommand.Flags().StringVar(&plan.Region, "region", "", "Semantic region or Steam store country")
	planCommand.Flags().StringVar(&plan.EgressRegion, "egress-region", "", "Harvest policy's egress region")
	planCommand.Flags().StringSliceVar(&plan.RRTypes, "rrtype", nil, "Explicit DNS RR types")
	planCommand.Flags().IntVar(&plan.NewsCount, "news-count", 0, "Steam News page size")
	for _, flag := range []string{"anchor", "interval", "egress-region"} {
		_ = planCommand.MarkFlagRequired(flag)
	}
	root.AddCommand(planCommand)
	root.AddCommand(action("reconcile", "Reconcile one bounded batch of Harvest targets and schedules", cobra.NoArgs,
		func(cmd *cobra.Command, _ []string, s *collection.Service) error { return s.Reconcile(cmd.Context()) }))
	var noInflight bool
	retry := action("retry-absent PLAN_ID", "Reopen an uncertain schedule only after confirming its request has finished", cobra.ExactArgs(1),
		func(cmd *cobra.Command, args []string, s *collection.Service) error {
			id, err := uuid.Parse(args[0])
			if err != nil || !noInflight {
				return collection.ErrIntent
			}
			return s.RetryAbsent(cmd.Context(), id)
		})
	retry.Flags().BoolVar(&noInflight, "confirmed-no-request-in-flight", false, "Confirm the original Harvest request can no longer commit")
	_ = retry.MarkFlagRequired("confirmed-no-request-in-flight")
	root.AddCommand(retry)
	return root
}
