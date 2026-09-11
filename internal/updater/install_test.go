package updater

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnixInstallCreatesRollbackBackup(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "backpack")
	if err := os.WriteFile(executable, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	result, err := Install(executable, Prepared{Plan: Plan{Target: Release{TagName: "v1.0.0"}}, Executable: []byte("new")}, "linux")
	if err != nil || !result.Installed || result.BackupPath == "" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if current, _ := os.ReadFile(executable); string(current) != "new" {
		t.Fatalf("current executable = %q", current)
	}
	if backup, _ := os.ReadFile(result.BackupPath); string(backup) != "old" {
		t.Fatalf("backup executable = %q", backup)
	}
}

func TestWindowsInstallStagesWithoutReplacingRunningExecutable(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "backpack.exe")
	if err := os.WriteFile(executable, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	result, err := Install(executable, Prepared{Plan: Plan{Target: Release{TagName: "v1.0.0"}}, Executable: []byte("new")}, "windows")
	if err != nil || result.Installed || !result.RequiresRestart || result.StagedPath == "" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if current, _ := os.ReadFile(executable); string(current) != "old" {
		t.Fatalf("running executable changed: %q", current)
	}
	if staged, _ := os.ReadFile(result.StagedPath); string(staged) != "new" {
		t.Fatalf("staged executable = %q", staged)
	}
}

func TestReplacementRollsBackActivationFailure(t *testing.T) {
	current, staged, backup := "current", "staged", "backup"
	calls := 0
	rename := func(from, to string) error {
		calls++
		switch calls {
		case 1:
			if from != current || to != backup {
				t.Fatalf("unexpected backup rename %s -> %s", from, to)
			}
			return nil
		case 2:
			return errors.New("activation failed")
		case 3:
			if from != backup || to != current {
				t.Fatalf("unexpected rollback rename %s -> %s", from, to)
			}
			return nil
		default:
			t.Fatalf("unexpected rename call")
			return nil
		}
	}
	err := replaceWithRollback(current, staged, backup, rename)
	if err == nil || !strings.Contains(err.Error(), "previous executable restored") || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
