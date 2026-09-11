package compute

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/statemigrate"
)

type TargetStore struct{ Path string }

func NewTargetStore(paths config.Paths) TargetStore {
	return TargetStore{filepath.Join(paths.Config, "compute-targets.json")}
}
func (s TargetStore) List() ([]SSHConfig, error) {
	items, err := statemigrate.ReadList[SSHConfig](s.Path, "targets")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read compute targets: %w", err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}
func (s TargetStore) Get(id string) (SSHConfig, error) {
	items, err := s.List()
	if err != nil {
		return SSHConfig{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return SSHConfig{}, fmt.Errorf("compute target %q was not found", id)
}
func (s TargetStore) Put(config SSHConfig) error {
	if err := NewSSH(config).validate(); err != nil {
		return err
	}
	items, err := s.List()
	if err != nil {
		return err
	}
	updated := false
	for i := range items {
		if items[i].ID == config.ID {
			items[i] = config
			updated = true
		}
	}
	if !updated {
		items = append(items, config)
	}
	return s.write(items)
}
func (s TargetStore) Remove(id string) error {
	items, err := s.List()
	if err != nil {
		return err
	}
	out := items[:0]
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
			continue
		}
		out = append(out, item)
	}
	if !found {
		return fmt.Errorf("compute target %q was not found", id)
	}
	return s.write(out)
}
func (s TargetStore) write(items []SSHConfig) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	return statemigrate.WriteList(s.Path, "targets", items)
}
