package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/config"
)

func TestPythonRuntimeInventoryReportsOnlyDirectoryCoordinates(t *testing.T) {
	root := t.TempDir()
	paths := config.NewPaths(root)
	directory := filepath.Join(paths.Runtimes, "python", "environments", "kokoro", "0.9.4", "windows-amd64-cpu")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if got := pythonRuntimeInventory(paths); len(got) != 1 || got[0] != "kokoro/0.9.4/windows-amd64-cpu" {
		t.Fatalf("unexpected Python runtime inventory: %#v", got)
	}
}

func TestSanitizePathRemovesPrivateHome(t *testing.T) {
	home := filepath.Join("C:", "Users", "private-user")
	got := SanitizePath(filepath.Join(home, "AppData", "Backpack"), home)
	if strings.Contains(strings.ToLower(got), "private-user") || !strings.Contains(got, "<home>") {
		t.Fatalf("path was not sanitized: %q", got)
	}
}

func TestSanitizeTextRemovesPrivateHome(t *testing.T) {
	home := `C:\Users\private-user`
	got := SanitizeText(`failed to read C:\Users\private-user\.backpack\state.json`, home)
	if strings.Contains(got, "private-user") || !strings.Contains(got, "<home>") {
		t.Fatalf("diagnostic was not sanitized: %q", got)
	}
}
