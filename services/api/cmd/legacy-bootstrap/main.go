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
	"time"

	"github.com/librecosta/gofurry/services/api/internal/legacybootstrap"
	"github.com/spf13/cobra"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := command().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func command() *cobra.Command {
	var config, reportPath string
	root := &cobra.Command{Use: "legacy-bootstrap", Short: "Build and validate an isolated GoFurry Bootstrap v1 package",
		SilenceUsage: true, SilenceErrors: true}
	root.SetFlagErrorFunc(func(*cobra.Command, error) error { return errors.New("invalid bootstrap flags; see --help") })
	root.PersistentFlags().StringVar(&config, "config", "", "explicit private YAML file")
	root.PersistentFlags().StringVar(&reportPath, "report", "", "new absolute private report path")
	for _, operation := range []struct{ name, help string }{
		{"build", "build a deterministic v1 archive from a sealed Seed"},
		{"verify", "verify the archive offline"},
		{"plan", "read-only check of packaged inputs and the fresh target"},
		{"apply", "initialize or resume the independently approved isolated plan"},
		{"verify-target", "read back the plan receipts and assets without writes"},
	} {
		var planPath, planSHA string
		cmd := &cobra.Command{Use: operation.name, Short: operation.help, Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.New("unexpected bootstrap arguments")
			}
			return nil
		}, RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Minute)
			defer cancel()
			reportFile, err := newOutput(reportPath)
			if err != nil {
				return err
			}
			defer reportFile.Close()
			var report legacybootstrap.Report
			var runErr error
			if operation.name == "build" {
				c, err := legacybootstrap.LoadBuildConfig(config)
				if err != nil {
					return err
				}
				report, runErr = legacybootstrap.Build(ctx, c)
			} else {
				c, err := legacybootstrap.LoadInstallConfig(config)
				if err != nil {
					return err
				}
				var planFile *os.File
				if operation.name == "plan" {
					planFile, err = newOutput(planPath)
					if err != nil {
						return err
					}
					defer planFile.Close()
				}
				var plan []byte
				report, plan, runErr = legacybootstrap.Run(ctx, c, operation.name, planPath, planSHA)
				if planFile != nil && runErr == nil {
					if _, err := planFile.Write(plan); err != nil || planFile.Sync() != nil {
						return errors.New("cannot persist complete bootstrap plan")
					}
				}
			}
			if json.MarshalWrite(reportFile, report) != nil || reportFile.Sync() != nil {
				return errors.New("cannot persist bootstrap operation report")
			}
			summary := struct {
				Status  string         `json:"status"`
				Package string         `json:"package_sha256,omitempty"`
				Plan    string         `json:"plan_sha256,omitempty"`
				Counts  map[string]int `json:"counts,omitempty"`
			}{Status: report.Status, Package: report.PackageSHA}
			if report.Import != nil {
				summary.Plan, summary.Counts = report.Import.PlanSHA, report.Import.Counts
			}
			if json.MarshalWrite(cmd.OutOrStdout(), summary) != nil {
				return errors.New("cannot write bootstrap summary")
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return runErr
		}}
		if operation.name == "plan" || operation.name == "apply" || operation.name == "verify-target" {
			cmd.Flags().StringVar(&planPath, "plan", "", "absolute private plan path")
		}
		if operation.name == "apply" || operation.name == "verify-target" {
			cmd.Flags().StringVar(&planSHA, "plan-sha256", "", "independently confirmed SHA256 of the exact plan bytes")
		}
		root.AddCommand(cmd)
	}
	return root
}

func newOutput(path string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("bootstrap report and plan outputs require absolute paths")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, errors.New("bootstrap report or plan output cannot be created without overwriting")
	}
	return f, nil
}
