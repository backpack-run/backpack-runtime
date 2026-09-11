package updater

import (
	"context"
	"fmt"
	"runtime"
)

type Plan struct {
	CurrentVersion string
	Target         Release
	Archive        Asset
	Checksums      Asset
	Available      bool
	Explicit       bool
	GOOS           string
	GOARCH         string
	executableName string
}

type Prepared struct {
	Plan       Plan
	Executable []byte
	SHA256     string
}

type Service struct {
	Source ReleaseSource
	GOOS   string
	GOARCH string
}

func (s Service) Check(ctx context.Context, current string, request ReleaseRequest) (Plan, error) {
	if s.Source == nil {
		return Plan{}, fmt.Errorf("update release source is not configured")
	}
	goos, goarch := s.GOOS, s.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	release, err := s.Source.Resolve(ctx, request)
	if err != nil {
		return Plan{}, err
	}
	archiveName, executableName, err := expectedArchive(release.TagName, goos, goarch)
	if err != nil {
		return Plan{}, err
	}
	archive, err := findAsset(release, archiveName)
	if err != nil {
		return Plan{}, err
	}
	checksums, err := findAsset(release, "checksums.txt")
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{CurrentVersion: current, Target: release, Archive: archive, Checksums: checksums, Explicit: request.Version != "", GOOS: goos, GOARCH: goarch, executableName: executableName}
	currentVersion, currentErr := ParseVersion(current)
	targetVersion, targetErr := ParseVersion(release.TagName)
	if targetErr != nil {
		return Plan{}, targetErr
	}
	if currentErr != nil {
		plan.Available = request.Version != ""
		return plan, nil
	}
	comparison := targetVersion.Compare(currentVersion)
	plan.Available = comparison > 0 || (request.Version != "" && comparison != 0)
	return plan, nil
}

func (s Service) Prepare(ctx context.Context, plan Plan) (Prepared, error) {
	if s.Source == nil {
		return Prepared{}, fmt.Errorf("update release source is not configured")
	}
	checksums, err := s.Source.Fetch(ctx, plan.Checksums.URL)
	if err != nil {
		return Prepared{}, fmt.Errorf("download release checksums: %w", err)
	}
	expected, err := parseChecksum(checksums, plan.Archive.Name)
	if err != nil {
		return Prepared{}, err
	}
	archive, err := s.Source.Fetch(ctx, plan.Archive.URL)
	if err != nil {
		return Prepared{}, fmt.Errorf("download release archive: %w", err)
	}
	if err = verifyArchive(archive, expected); err != nil {
		return Prepared{}, err
	}
	executable, err := extractExecutable(plan.Archive.Name, plan.executableName, archive)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{Plan: plan, Executable: executable, SHA256: expected}, nil
}
