package diagnostics

import (
	"path/filepath"
	"strings"
	"testing"
)

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
