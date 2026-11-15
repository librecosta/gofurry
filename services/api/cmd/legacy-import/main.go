package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/librecosta/gofurry/services/api/internal/legacyimport"
	"github.com/spf13/cobra"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := command().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func command() *cobra.Command {
	var configPath, planPath, reportPath, planSHA string
	root := &cobra.Command{Use: "legacy-import", Short: "Import a sealed GoFurry D5-A seed into isolated PostgreSQL 18",
		SilenceUsage: true, SilenceErrors: true}
	root.SetFlagErrorFunc(func(*cobra.Command, error) error { return errors.New("invalid command flags; see --help") })
	root.PersistentFlags().StringVar(&configPath, "config", "", "explicit private YAML file")
	root.PersistentFlags().StringVar(&planPath, "plan", "", "private plan path (new file for plan, input for apply or verify)")
	root.PersistentFlags().StringVar(&reportPath, "report", "", "new private report path")
	for _, entry := range []struct{ Name, Short string }{
		{"plan", "verify seed and inspect the target without writes"},
		{"apply", "apply the approved plan and report durable receipts"},
		{"verify", "read back an applied plan without writes"},
	} {
		cmd := &cobra.Command{Use: entry.Name, Short: entry.Short, Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.New("unexpected positional arguments")
			}
			return nil
		}}
		if entry.Name != "plan" {
			cmd.Flags().StringVar(&planSHA, "plan-sha256", "", "independently confirmed SHA256 of the exact plan bytes")
		}
		cmd.RunE = func(cmd *cobra.Command, _ []string) error {
			c, err := legacyimport.LoadConfig(configPath)
			if err != nil {
				return err
			}
			if !filepath.IsAbs(reportPath) || !filepath.IsAbs(planPath) {
				return errors.New("plan and report require explicit absolute paths")
			}
			report, err := createPrivate(reportPath)
			if err != nil {
				return err
			}
			defer report.Close()
			var plan *os.File
			if entry.Name == "plan" {
				plan, err = createPrivate(planPath)
				if err != nil {
					return err
				}
				defer plan.Close()
			}
			p, r, runErr := legacyimport.Run(cmd.Context(), c, legacyimport.Options{Mode: entry.Name, PlanFile: planPath, PlanSHA: planSHA})
			if plan != nil && runErr == nil {
				if _, err = plan.Write(p.Bytes()); err != nil || plan.Sync() != nil {
					return errors.New("cannot persist complete plan")
				}
			}
			if json.MarshalWrite(report, r) != nil || report.Sync() != nil {
				return errors.New("cannot persist complete import report")
			}
			summary := struct {
				Status     string         `json:"status"`
				PlanSHA    string         `json:"plan_sha256,omitempty"`
				SourceRows int            `json:"source_rows"`
				Counts     map[string]int `json:"counts"`
			}{r.Status, r.PlanSHA, r.SourceRows, r.Counts}
			if json.MarshalWrite(cmd.OutOrStdout(), summary) != nil {
				return errors.New("cannot write summary")
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return runErr
		}
		root.AddCommand(cmd)
	}
	return root
}

func createPrivate(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, errors.New("plan or report output already exists or is unavailable")
	}
	return f, nil
}
