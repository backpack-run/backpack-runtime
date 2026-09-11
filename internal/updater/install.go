package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type InstallResult struct {
	Installed       bool
	RequiresRestart bool
	ExecutablePath  string
	StagedPath      string
	BackupPath      string
}

func Install(executablePath string, prepared Prepared, goos string) (InstallResult, error) {
	absolute, err := filepath.Abs(executablePath)
	if err != nil {
		return InstallResult{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return InstallResult{}, fmt.Errorf("inspect current executable: %w", err)
	}
	staged, err := stageExecutable(absolute, prepared.Executable, info.Mode())
	if err != nil {
		return InstallResult{}, err
	}
	if goos == "windows" {
		tag := strings.TrimPrefix(prepared.Plan.Target.TagName, "v")
		pending := absolute + ".update-" + tag
		if _, statErr := os.Stat(pending); statErr == nil {
			_ = os.Remove(staged)
			return InstallResult{}, fmt.Errorf("verified staged update already exists at %s", pending)
		} else if !os.IsNotExist(statErr) {
			_ = os.Remove(staged)
			return InstallResult{}, statErr
		}
		if err = os.Rename(staged, pending); err != nil {
			_ = os.Remove(staged)
			return InstallResult{}, fmt.Errorf("stage Windows update: %w", err)
		}
		return InstallResult{RequiresRestart: true, ExecutablePath: absolute, StagedPath: pending}, nil
	}
	backup := fmt.Sprintf("%s.backup-%s", absolute, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err = replaceWithRollback(absolute, staged, backup, os.Rename); err != nil {
		_ = os.Remove(staged)
		return InstallResult{}, err
	}
	return InstallResult{Installed: true, ExecutablePath: absolute, BackupPath: backup}, nil
}

func stageExecutable(current string, data []byte, mode os.FileMode) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("refusing to stage an empty executable")
	}
	file, err := os.CreateTemp(filepath.Dir(current), ".backpack-update-*")
	if err != nil {
		return "", fmt.Errorf("create update staging file: %w", err)
	}
	name := file.Name()
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if _, err = file.Write(data); err != nil {
		return "", fmt.Errorf("write staged update: %w", err)
	}
	if err = file.Sync(); err != nil {
		return "", fmt.Errorf("sync staged update: %w", err)
	}
	if err = file.Chmod(mode.Perm()); err != nil {
		return "", fmt.Errorf("set staged executable permissions: %w", err)
	}
	if err = file.Close(); err != nil {
		return "", fmt.Errorf("close staged update: %w", err)
	}
	ok = true
	return name, nil
}

type renameFunc func(string, string) error

func replaceWithRollback(current, staged, backup string, rename renameFunc) error {
	if err := rename(current, backup); err != nil {
		return fmt.Errorf("create update rollback backup: %w", err)
	}
	if err := rename(staged, current); err != nil {
		if rollbackErr := rename(backup, current); rollbackErr != nil {
			return fmt.Errorf("activate update: %v; rollback also failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("activate update (previous executable restored): %w", err)
	}
	return nil
}
