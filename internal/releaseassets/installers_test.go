package releaseassets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallersRequireChecksumsAndHTTPS(t *testing.T) {
	for _, name := range []string{"install.ps1", "install.sh"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "https://github.com/backpack-run/backpack-runtime") || !strings.Contains(strings.ToLower(text), "sha256") {
			t.Fatalf("%s does not enforce HTTPS and SHA-256", name)
		}
		if strings.Contains(text, "StrictHostKeyChecking=no") || strings.Contains(text, "http://") {
			t.Fatalf("%s contains an insecure transport option", name)
		}
	}
}
