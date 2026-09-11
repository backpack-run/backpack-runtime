package integrations

import (
	"fmt"
	"path/filepath"
)

// Arguments preserves argv boundaries. Callers must pass Values directly to a
// process API; this package intentionally exposes no shell-command formatter.
type Arguments struct{ values []string }

func NewArguments(values ...string) (Arguments, error) {
	for _, value := range values {
		for _, char := range value {
			if char == 0 {
				return Arguments{}, fmt.Errorf("argument contains a NUL byte")
			}
		}
	}
	return Arguments{values: append([]string(nil), values...)}, nil
}

func (a Arguments) Values() []string { return append([]string(nil), a.values...) }

type Invocation struct {
	Executable              string
	ManagedArguments        Arguments
	PassthroughArguments    Arguments
	Environment             EnvironmentOverlay
	IsolatedConfigDirectory string
}

func (i Invocation) Validate() error {
	if i.Executable == "" {
		return fmt.Errorf("integration executable is required")
	}
	if i.IsolatedConfigDirectory == "" {
		return fmt.Errorf("an isolated integration config directory is required")
	}
	if !filepath.IsAbs(i.IsolatedConfigDirectory) {
		return fmt.Errorf("isolated integration config directory must be absolute")
	}
	return nil
}

// Args combines trusted builder arguments and literal user passthrough without
// invoking a shell or reparsing either group.
func (i Invocation) Args() []string {
	managed := i.ManagedArguments.Values()
	return append(managed, i.PassthroughArguments.Values()...)
}

// IsolatedConfigDirectory derives a Backpack-owned per-integration config path.
// It does not create or modify the directory.
func IsolatedConfigDirectory(root, integrationID string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("integration config root must be absolute")
	}
	if !descriptorID.MatchString(integrationID) {
		return "", fmt.Errorf("integration ID %q must be a lowercase slug", integrationID)
	}
	return filepath.Join(filepath.Clean(root), integrationID), nil
}
