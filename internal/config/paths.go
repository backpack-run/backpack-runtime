package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Paths struct{ Root, Models, Manifests, Runtimes, Cache, Logs, State, Config string }

func DefaultPaths() (Paths, error) {
	if x := os.Getenv("BACKPACK_HOME"); x != "" {
		return NewPaths(x), nil
	}
	var root string
	var err error
	if runtime.GOOS == "windows" {
		root, err = os.UserCacheDir()
		root = filepath.Join(root, "Backpack")
	} else {
		root, err = os.UserHomeDir()
		root = filepath.Join(root, ".backpack")
	}
	if err != nil {
		return Paths{}, fmt.Errorf("resolve Backpack data directory: %w", err)
	}
	return NewPaths(root), nil
}
func NewPaths(root string) Paths {
	return Paths{root, filepath.Join(root, "models"), filepath.Join(root, "manifests"), filepath.Join(root, "runtimes"), filepath.Join(root, "cache"), filepath.Join(root, "logs"), filepath.Join(root, "state"), filepath.Join(root, "config")}
}
func (p Paths) Ensure() error {
	for _, x := range []string{p.Root, p.Models, p.Manifests, p.Runtimes, p.Cache, p.Logs, p.State, p.Config} {
		if err := os.MkdirAll(x, 0700); err != nil {
			return fmt.Errorf("create %s: %w", x, err)
		}
	}
	return nil
}
