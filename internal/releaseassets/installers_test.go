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
		for _, required := range []string{"BACKPACK_VERSION", "BACKPACK_CHANNEL", "latest", "stable", "release.json"} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s is missing release selection control %q", name, required)
			}
		}
	}
}

func TestPowerShellInstallerHardening(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"AllowAutoRedirect = $false", "Refusing non-HTTPS download", "Unsafe or unexpected archive entry", "Release archive is incomplete", "Downloaded binary did not report expected version", "[IO.File]::Replace", "$backupBinary", "@($metadata)[0]", "Refusing to install a draft release"} {
		if !strings.Contains(text, required) {
			t.Fatalf("PowerShell installer is missing %q", required)
		}
	}
	if strings.Contains(text, "Expand-Archive") {
		t.Fatal("PowerShell installer performs broad archive extraction")
	}
	if strings.Contains(text, "[IO.File]::Replace($stagedBinary, $destination, $null)") {
		t.Fatal("PowerShell 5.1 rejects a null File.Replace backup path")
	}
}

func TestShellInstallerHardening(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"umask 077", "--proto-redir '=https'", "tar -tzf", "tar -tvzf", "downloaded binary did not report expected version", "staged_binary=", "metadata_value", "refusing to install a draft release"} {
		if !strings.Contains(text, required) {
			t.Fatalf("shell installer is missing %q", required)
		}
	}
}

func TestReleaseWorkflowPublishesProvenance(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"id-token: write", "attestations: write", "actions/attest@", "dist/*.zip", "dist/*.tar.gz", "dist/checksums.txt"} {
		if !strings.Contains(text, required) {
			t.Fatalf("release workflow is missing provenance setting %q", required)
		}
	}
}

func TestReleaseMetadataIsAlphaSafe(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, ".goreleaser.yml"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(data)
	for _, required := range []string{"prerelease: auto", "draft: false", "make_latest: false", "THIRD_PARTY_NOTICES.md"} {
		if !strings.Contains(config, required) {
			t.Fatalf("release configuration is missing %q", required)
		}
	}
	notes, err := os.ReadFile(filepath.Join(root, "docs", "releases", "v0.1.0-alpha.1.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(notes), "first public alpha") || !strings.Contains(string(notes), "Windows x64") {
		t.Fatal("alpha release notes are incomplete")
	}
}

func TestReleaseWorkflowActionsAreImmutable(t *testing.T) {
	root := filepath.Join("..", "..", ".github", "workflows")
	for _, name := range []string{"catalog-verify.yml", "ci.yml", "extended-qualification.yml", "release.yml", "release-qualification.yml"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- uses:") || strings.HasPrefix(line, "uses:") {
				at := strings.LastIndex(line, "@")
				if at < 0 || len(strings.Fields(line[at+1:])[0]) != 40 {
					t.Fatalf("%s has an unpinned action: %s", name, line)
				}
			}
		}
	}
}
