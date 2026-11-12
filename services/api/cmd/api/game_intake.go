package main

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/librecosta/gofurry/services/api/internal/collection"
	"github.com/librecosta/gofurry/services/api/internal/database"
	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/spf13/cobra"
)

func gameIntakeCommand(cfg *platform.Config) *cobra.Command {
	root := &cobra.Command{Use: "game-intake", Short: "Submit and resolve regional game candidates privately"}
	type operation func(*cobra.Command, []string, *collection.Intake) (any, error)
	add := func(use, short string, n int, op operation) *cobra.Command {
		cmd := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(n), RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Database == nil {
				return errors.New("game intake requires a regional database")
			}
			pool, err := database.Open(cmd.Context(), *cfg.Database)
			if err != nil {
				return errors.New("game intake database unavailable")
			}
			defer pool.Close()
			service := collection.NewIntake(pool, cfg.Harvest)
			defer service.Close()
			value, err := op(cmd, args, service)
			if err != nil {
				switch {
				case errors.Is(err, collection.ErrIntakeInput), errors.Is(err, collection.ErrIntakeState), errors.Is(err, collection.ErrIntakeCapacity):
					return err
				default:
					return errors.New("game intake database or processing unavailable")
				}
			}
			if value == nil {
				return nil
			}
			raw, err := json.Marshal(value)
			if err != nil {
				return errors.New("game intake status could not be encoded")
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return err
		}}
		root.AddCommand(cmd)
		return cmd
	}
	add("submit steam APP_ID", "Submit one normalized Steam AppID", 2, func(cmd *cobra.Command, a []string, s *collection.Intake) (any, error) {
		if !strings.EqualFold(strings.TrimSpace(a[0]), "steam") {
			return nil, collection.ErrIntakeInput
		}
		return s.SubmitSteam(cmd.Context(), a[1], "operator")
	})
	add("inspect PLATFORM KEY", "Read one candidate and its identity and collection outcome", 2, func(cmd *cobra.Command, a []string, s *collection.Intake) (any, error) {
		return s.Inspect(cmd.Context(), a[0], a[1])
	})
	add("status", "Count candidates by outcome", 0, func(cmd *cobra.Command, _ []string, s *collection.Intake) (any, error) {
		return s.Status(cmd.Context())
	})
	add("tick", "Process one due candidate", 0, func(cmd *cobra.Command, _ []string, s *collection.Intake) (any, error) {
		return s.Tick(cmd.Context())
	})
	add("retry PLATFORM KEY", "Recheck a parked candidate with the same durable remote intent", 2,
		func(cmd *cobra.Command, a []string, s *collection.Intake) (any, error) {
			return nil, s.Retry(cmd.Context(), a[0], a[1])
		})
	add("dismiss PLATFORM KEY", "Stop processing a candidate and retain its deduplication key", 2,
		func(cmd *cobra.Command, a []string, s *collection.Intake) (any, error) {
			return nil, s.Dismiss(cmd.Context(), a[0], a[1])
		})
	var furry, sfw bool
	review := add("review APP_ID FURRY_REF SFW_REF", "Record two positive reviews for the Steam work", 3,
		func(cmd *cobra.Command, a []string, s *collection.Intake) (any, error) {
			if !furry || !sfw {
				return nil, collection.ErrIntakeInput
			}
			return nil, s.ReviewSteam(cmd.Context(), a[0], a[1], a[2])
		})
	review.Flags().BoolVar(&furry, "confirm-furry", false, "The cited work synopsis positively establishes anthropomorphic subject matter")
	review.Flags().BoolVar(&sfw, "confirm-sfw", false, "The cited separate assessment positively establishes SFW content")
	add("revoke-review APP_ID", "Revoke auto-publication eligibility without removing identity", 1,
		func(cmd *cobra.Command, a []string, s *collection.Intake) (any, error) {
			return nil, s.RevokeSteamReview(cmd.Context(), a[0])
		})
	var reciprocal bool
	relate := add("relate PLATFORM KEY OTHER_PLATFORM OTHER_KEY FORWARD_REF REVERSE_REF", "Record reviewed reciprocal official links between two full-game products", 6,
		func(cmd *cobra.Command, a []string, s *collection.Intake) (any, error) {
			if !reciprocal {
				return nil, collection.ErrIntakeInput
			}
			return nil, s.RecordRelation(cmd.Context(), a[0], a[1], a[2], a[3], a[4], a[5])
		})
	relate.Flags().BoolVar(&reciprocal, "confirm-reciprocal-work-links", false, "Both official product records reciprocate and describe the same full work")
	add("revoke-relation PLATFORM KEY", "Deactivate an automatically attached source and preserve its history", 2,
		func(cmd *cobra.Command, a []string, s *collection.Intake) (any, error) {
			return nil, s.RevokeRelation(cmd.Context(), a[0], a[1])
		})
	return root
}
