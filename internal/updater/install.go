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
		// Releases before v0.2.0-alpha.7 left a verified side-by-side file on
		// Windows and required the user to replace the running executable
		// manually. Windows permits renaming the mapped executable, so remove
		// that legacy staging file and use the same rollback-safe activation as
		// the other platforms. The freshly downloaded executable above has
		// already passed the release checksum and embedded-version checks.
		if removeErr := os.Remove(pending); removeErr != nil && !os.IsNotExist(removeErr) {
			_ = os.Remove(staged)
			return InstallResult{}, fmt.Errorf("remove legacy Windows staged update %s: %w", pending, removeErr)
		}
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
