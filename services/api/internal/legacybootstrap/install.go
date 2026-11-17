package legacybootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/librecosta/gofurry/services/api/internal/legacyimport"
)

// Verify and installation share exactly the same archive and D5-A checks.
// The temporary extracted bundle belongs only to this invocation.
func Run(ctx context.Context, c InstallConfig, mode, planPath, planSHA string) (Report, []byte, error) {
	report := Report{Status: "failed"}
	if mode == "apply" || mode == "verify-target" {
		report.BootstrapReadiness = "FAIL"
	}
	if !filepath.IsAbs(c.Archive) || !filepath.IsAbs(c.WorkDir) || !digestPattern.MatchString(c.SHA) {
		return report, nil, errors.New("verify requires absolute archive and private work paths plus the independently confirmed SHA256")
	}
	if mode != "verify" && mode != "plan" && mode != "apply" && mode != "verify-target" {
		return report, nil, errors.New("unsupported bootstrap operation")
	}
	if mode != "verify" && (!filepath.IsAbs(planPath) || (mode != "plan" && !digestPattern.MatchString(planSHA))) {
		return report, nil, errors.New("target operations require an absolute plan path and an independently confirmed hash for apply and verify-target")
	}
	file, err := openFile(c.Archive, archiveLimit)
	if err != nil {
		return report, nil, err
	}
	defer file.Close()
	dir, err := os.MkdirTemp(c.WorkDir, ".bootstrap-*")
	if err != nil {
		return report, nil, errors.New("cannot stage an archive in the explicit private work directory")
	}
	defer os.RemoveAll(dir)

	m, err := unpack(ctx, file, c.SHA, dir)
	if err != nil {
		return report, nil, err
	}
	decisions, err := privateFile(filepath.Join(dir, decisionsName), decisionLimit)
	if err != nil {
		return report, nil, err
	}
	b, inventory, err := openSeed(ctx, filepath.Join(dir, "seed"), m.Seed, decisions, m.Version)
	if err != nil {
		return report, nil, err
	}
	b.Close()
	actual, _ := canonical(inventory)
	expected, _ := canonical(m.Inventory)
	if !bytes.Equal(actual, expected) {
		return report, nil, errors.New("bootstrap manifest totals disagree with verified Seed and decisions")
	}
	report.PackageSHA, report.Manifest = c.SHA, &m
	initialization := m.Version == legacyimport.BootstrapInitialization
	if initialization {
		report.PublicationReadiness = "INCOMPLETE"
	} else {
		report.BootstrapReadiness = ""
	}
	if mode == "verify" {
		report.Status = "verified_for_initialization"
		if !initialization {
			report.Status = "verified_for_isolated_acceptance"
		}
		return report, nil, nil
	}

	target, err := installTarget(c.Target, m, dir)
	if err != nil {
		return report, nil, err
	}
	importMode := mode
	if mode == "verify-target" {
		importMode = "verify"
	}
	plan, result, err := legacyimport.RunBootstrap(ctx, target, legacyimport.Options{Mode: importMode, PlanFile: planPath, PlanSHA: planSHA}, m.Version, c.SHA)
	report.Status, report.Import = result.Status, &result
	if mode == "plan" && err == nil {
		return report, plan.Bytes(), nil
	}
	if !initialization && err == nil && inventory.Projection["logo_gaps"] > 0 {
		report.Status = "incomplete"
		return report, nil, errors.New("bootstrap has Site Logos without an approved asset")
	}
	if err == nil && initialization {
		report.BootstrapReadiness = "PASS"
	}
	return report, nil, err
}
