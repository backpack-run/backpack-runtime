package integrations

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var ErrExecutableNotFound = errors.New("integration executable was not found on PATH")

type LookPathFunc func(string) (string, error)

type Discovery struct{ LookPath LookPathFunc }

type Installation struct {
	IntegrationID string
	Executable    string
}

// Version asks an already-discovered executable for its self-reported version.
// It never invokes a shell and bounds both execution time and diagnostic size.
func Version(ctx context.Context, installation Installation) (string, error) {
	if installation.Executable == "" {
		return "", fmt.Errorf("integration executable is required")
	}
	check, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(check, installation.Executable, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect integration version: %w", err)
	}
	value := strings.TrimSpace(string(output))
	if line, _, found := strings.Cut(value, "\n"); found {
		value = strings.TrimSpace(line)
	}
	if len(value) > 256 {
		value = value[:256]
	}
	if value == "" {
		return "", fmt.Errorf("integration returned an empty version")
	}
	return value, nil
}

func NewDiscovery() Discovery { return Discovery{LookPath: exec.LookPath} }

// Detect searches only PATH candidates declared by the trusted descriptor. It
// never downloads a binary and never interprets shell commands.
func (d Discovery) Detect(descriptor Descriptor) (Installation, error) {
	if err := descriptor.validate(); err != nil {
		return Installation{}, err
	}
	if d.LookPath == nil {
		return Installation{}, fmt.Errorf("integration discovery has no PATH lookup implementation")
	}
	for _, candidate := range descriptor.ExecutableCandidates {
		path, err := d.LookPath(candidate)
		if err == nil && path != "" {
			return Installation{IntegrationID: descriptor.ID, Executable: path}, nil
		}
	}
	return Installation{}, fmt.Errorf("%w: %s (%v)", ErrExecutableNotFound, descriptor.DisplayName, descriptor.ExecutableCandidates)
}
