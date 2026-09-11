package diagnostics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/daemon"
	"github.com/backpack-run/backpack-runtime/internal/integrations"
	"github.com/backpack-run/backpack-runtime/internal/models"
	"github.com/backpack-run/backpack-runtime/internal/runtimebundle"
	clientapi "github.com/backpack-run/backpack-runtime/pkg/client"
)

type IntegritySummary struct {
	Installed int `json:"installed"`
	Verified  int `json:"verified"`
	Corrupt   int `json:"corrupt"`
}
type ComputeSummary struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}
type DaemonSummary struct {
	Running   bool      `json:"running"`
	Version   string    `json:"version,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
}
type BinarySummary struct {
	Path   string `json:"path"`
	InPath bool   `json:"in_path"`
}
type IntegrationSummary struct {
	ID        string `json:"id"`
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
}
type UpdateSummary struct {
	Status string `json:"status"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason,omitempty"`
}
type Report struct {
	Version                string               `json:"version"`
	Platform               string               `json:"platform"`
	BackpackHome           string               `json:"backpack_home"`
	Hardware               compute.Hardware     `json:"hardware"`
	Daemon                 DaemonSummary        `json:"daemon"`
	Binary                 BinarySummary        `json:"binary"`
	Runtimes               IntegritySummary     `json:"runtimes"`
	Models                 IntegritySummary     `json:"models"`
	ComputeTargets         []ComputeSummary     `json:"compute_targets"`
	PythonRuntimes         []string             `json:"python_runtimes"`
	Integrations           []IntegrationSummary `json:"coding_agent_integrations"`
	ConfigurationOverrides []string             `json:"configuration_overrides"`
	Update                 UpdateSummary        `json:"update"`
	KnownProblems          []string             `json:"known_problems"`
}

func Collect(ctx context.Context, version string, paths config.Paths, local compute.Local, modelManager *models.Manager, runtimeManager *runtimebundle.Manager, targets compute.TargetStore) Report {
	home, _ := os.UserHomeDir()
	report := Report{Version: version, Platform: runtime.GOOS + "/" + runtime.GOARCH, BackpackHome: SanitizePath(paths.Root, home), ComputeTargets: []ComputeSummary{{Name: "local", Kind: "local"}}, PythonRuntimes: []string{}, Integrations: []IntegrationSummary{}, ConfigurationOverrides: []string{}, KnownProblems: []string{}}
	if executable, err := os.Executable(); err == nil {
		report.Binary.Path = SanitizePath(executable, home)
		if found, pathErr := exec.LookPath(filepath.Base(executable)); pathErr == nil {
			resolvedExecutable, _ := filepath.Abs(executable)
			resolvedFound, _ := filepath.Abs(found)
			report.Binary.InPath = strings.EqualFold(resolvedExecutable, resolvedFound)
		}
	} else {
		report.KnownProblems = append(report.KnownProblems, "Backpack executable path: "+err.Error())
	}
	if registry, err := integrations.Builtins(); err == nil {
		discovery := integrations.NewDiscovery()
		for _, descriptor := range registry.List() {
			summary := IntegrationSummary{ID: descriptor.ID}
			if _, detectErr := discovery.Detect(descriptor); detectErr == nil {
				summary.Installed = true
			}
			report.Integrations = append(report.Integrations, summary)
		}
	}
	report.PythonRuntimes = pythonRuntimeInventory(paths)
	for _, name := range []string{"BACKPACK_LLAMA_SERVER", "BACKPACK_HOME"} {
		if os.Getenv(name) != "" {
			report.ConfigurationOverrides = append(report.ConfigurationOverrides, name)
		}
	}
	if hardware, err := local.Inspect(ctx); err == nil {
		report.Hardware = hardware
	} else {
		report.KnownProblems = append(report.KnownProblems, "hardware inspection: "+err.Error())
	}
	if state, err := daemon.Read(paths); err == nil {
		check, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
		err = clientapi.New(state.Endpoint).Health(check)
		cancel()
		if err == nil {
			report.Daemon = DaemonSummary{Running: true, Version: state.Version, StartedAt: state.StartedAt}
		} else {
			report.KnownProblems = append(report.KnownProblems, "stale or unhealthy runtime daemon state")
		}
	}
	if installed, err := runtimeManager.List(); err == nil {
		report.Runtimes.Installed = len(installed)
		for _, item := range installed {
			if runtimeManager.Verify(item) == nil {
				report.Runtimes.Verified++
			} else {
				report.Runtimes.Corrupt++
			}
		}
	} else {
		report.KnownProblems = append(report.KnownProblems, "runtime inventory: "+err.Error())
	}
	if installed, err := modelManager.List(); err == nil {
		report.Models.Installed = len(installed)
		for _, item := range installed {
			if modelManager.Verify(item) == nil {
				report.Models.Verified++
			} else {
				report.Models.Corrupt++
			}
		}
	} else {
		report.KnownProblems = append(report.KnownProblems, "model inventory: "+err.Error())
	}
	if saved, err := targets.List(); err == nil {
		for _, target := range saved {
			report.ComputeTargets = append(report.ComputeTargets, ComputeSummary{Name: target.ID, Kind: "ssh"})
		}
	} else {
		report.KnownProblems = append(report.KnownProblems, "compute target inventory: "+err.Error())
	}
	if report.Models.Corrupt > 0 {
		report.KnownProblems = append(report.KnownProblems, fmt.Sprintf("%d installed model package(s) failed integrity verification", report.Models.Corrupt))
	}
	if report.Runtimes.Corrupt > 0 {
		report.KnownProblems = append(report.KnownProblems, fmt.Sprintf("%d installed runtime bundle(s) failed integrity verification", report.Runtimes.Corrupt))
	}
	for i := range report.KnownProblems {
		report.KnownProblems[i] = SanitizeText(report.KnownProblems[i], home)
	}
	return report
}

func pythonRuntimeInventory(paths config.Paths) []string {
	root := filepath.Join(paths.Runtimes, "python", "environments")
	engines, err := os.ReadDir(root)
	if err != nil {
		return []string{}
	}
	var result []string
	for _, engine := range engines {
		if !engine.IsDir() {
			continue
		}
		versions, _ := os.ReadDir(filepath.Join(root, engine.Name()))
		for _, version := range versions {
			if !version.IsDir() {
				continue
			}
			platforms, _ := os.ReadDir(filepath.Join(root, engine.Name(), version.Name()))
			for _, platform := range platforms {
				if platform.IsDir() {
					result = append(result, engine.Name()+"/"+version.Name()+"/"+platform.Name())
				}
			}
		}
	}
	return result
}

func SanitizeText(value, home string) string {
	if home == "" {
		return value
	}
	result := strings.ReplaceAll(value, home, "<home>")
	result = strings.ReplaceAll(result, strings.ReplaceAll(home, "\\", "/"), "<home>")
	return result
}

func SanitizePath(path, home string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "<unavailable>"
	}
	absHome, homeErr := filepath.Abs(home)
	if homeErr == nil {
		if relative, relErr := filepath.Rel(absHome, absPath); relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			if relative == "." {
				return "<home>"
			}
			return filepath.Join("<home>", relative)
		}
	}
	return filepath.Base(absPath)
}
