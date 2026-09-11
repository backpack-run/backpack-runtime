package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/diagnostics"
	"github.com/backpack-run/backpack-runtime/internal/updater"
)

func (a *app) update(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(a.err)
	checkOnly := fs.Bool("check", false, "check for an update without downloading or installing it")
	version := fs.String("version", "", "select an exact release version, such as v0.1.0-alpha.1")
	prerelease := fs.Bool("prerelease", false, "include alpha, beta, and release-candidate versions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: backpack update [--check] [--version version | --prerelease]")
	}
	if *version != "" && *prerelease {
		return fmt.Errorf("--version and --prerelease are mutually exclusive; an exact version already selects its channel")
	}

	service := updater.Service{
		Source: updater.NewGitHub("backpack-run/backpack-runtime", nil),
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
	}
	plan, err := service.Check(ctx, a.version, updater.ReleaseRequest{Version: *version, IncludePrerelease: *prerelease})
	if err != nil {
		return fmt.Errorf("check for Backpack update: %w", err)
	}
	fmt.Fprintf(a.out, "Current: %s\nTarget:  %s\n", displayVersion(a.version), plan.Target.TagName)
	if _, versionErr := updater.ParseVersion(a.version); versionErr != nil && *version == "" {
		fmt.Fprintln(a.out, "The current build has no comparable release version. Select an exact trusted release with --version.")
		return nil
	}
	if !plan.Available {
		fmt.Fprintln(a.out, "Backpack is already at the selected version or newer.")
		return nil
	}
	if *checkOnly {
		fmt.Fprintln(a.out, "Update available. Run the same command without --check to download and install it.")
		return nil
	}

	fmt.Fprintf(a.out, "Downloading and verifying %s...\n", plan.Archive.Name)
	prepared, err := service.Prepare(ctx, plan)
	if err != nil {
		return fmt.Errorf("prepare Backpack update: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current Backpack executable: %w", err)
	}
	result, err := updater.Install(executable, prepared, runtime.GOOS)
	if err != nil {
		return fmt.Errorf("install Backpack update: %w", err)
	}
	if result.RequiresRestart {
		fmt.Fprintf(a.out, "Verified update staged at %s\n", result.StagedPath)
		fmt.Fprintf(a.out, "Windows cannot safely replace the running executable. Exit all Backpack processes, preserve %s as a rollback backup, then replace it with the staged file.\n", result.ExecutablePath)
		return nil
	}
	fmt.Fprintf(a.out, "Updated Backpack to %s. Rollback backup: %s\n", plan.Target.TagName, result.BackupPath)
	return nil
}

func (a *app) updateDiagnostic(ctx context.Context) diagnostics.UpdateSummary {
	current, err := updater.ParseVersion(a.version)
	if err != nil {
		return diagnostics.UpdateSummary{Status: "unknown", Reason: "development or unversioned build"}
	}
	checkContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	service := updater.Service{Source: updater.NewGitHub("backpack-run/backpack-runtime", nil), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	plan, err := service.Check(checkContext, a.version, updater.ReleaseRequest{IncludePrerelease: len(current.Prerelease) > 0})
	if err != nil {
		return diagnostics.UpdateSummary{Status: "unavailable", Reason: "release check failed; run `backpack update --check` for details"}
	}
	if plan.Available {
		return diagnostics.UpdateSummary{Status: "available", Target: plan.Target.TagName}
	}
	return diagnostics.UpdateSummary{Status: "current", Target: plan.Target.TagName}
}

func displayVersion(version string) string {
	if _, err := updater.ParseVersion(version); err == nil {
		return strings.Fields(version)[0]
	}
	return version
}
