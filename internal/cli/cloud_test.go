package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/cloud"
	"github.com/backpack-run/backpack-runtime/internal/config"
)

func TestLoginExplainsOptionalPrivatePreviewBeforeDeviceFlow(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/device/start":
			_, _ = io.WriteString(w, `{"device_code":"opaque","user_code":"ABCD-EFGH","verification_uri":"https://backpack.run/device","expires_in":10,"poll_interval":1}`)
		case "/api/auth/device/status":
			_, _ = io.WriteString(w, `{"status":"approved","device_key_id":"123e4567-e89b-12d3-a456-426614174000"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("BACKPACK_CLOUD_URL", server.URL)
	client, err := cloud.New(config.NewPaths(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	a := &app{out: &output, err: &output, cloud: client}
	if err = a.login(context.Background(), []string{"--name", "test", "--no-browser"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"currently in private preview", "does not require an account", "only needed for Backpack Cloud", "Opening browser to sign in", "Signed in"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("login output missing %q: %s", want, output.String())
		}
	}
}

func TestOSSCommandsIgnoreUnavailableCloudConfigurationWhileLoggedOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BACKPACK_HOME", home)
	t.Setenv("BACKPACK_API_KEY", "")
	t.Setenv("BACKPACK_CLOUD_URL", "http://cloud.example.invalid")
	commands := [][]string{
		{"models", "--json"},
		{"compute", "add", "ssh", "test-gpu", "--host", "example.invalid"},
		{"compute", "list"},
		{"launch", "list"},
	}
	for _, command := range commands {
		var output bytes.Buffer
		if err := Run(context.Background(), command, &output, &output, "test"); err != nil {
			t.Fatalf("logged-out OSS command %v depended on Cloud: %v\n%s", command, err, output.String())
		}
	}
}

func TestLogoutRemovesOnlyCloudCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BACKPACK_HOME", home)
	t.Setenv("BACKPACK_API_KEY", "")
	paths := config.NewPaths(home)
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	store := cloud.NewCredentialStore(paths)
	if err = store.Save(cloud.Credentials{DeviceKeyID: "123e4567-e89b-12d3-a456-426614174000", PrivateKey: privateKey}); err != nil {
		t.Fatal(err)
	}
	sentinels := []string{
		filepath.Join(paths.Models, "installed-model"),
		filepath.Join(paths.Config, "compute-targets.json"),
		filepath.Join(paths.Config, "integration-config"),
		filepath.Join(paths.State, "session-state"),
	}
	for _, path := range sentinels {
		if err = os.WriteFile(path, []byte("preserve"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err = Run(context.Background(), []string{"logout"}, &output, &output, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Load(); !os.IsNotExist(err) {
		t.Fatalf("Cloud credential survived logout: %v", err)
	}
	for _, path := range sentinels {
		data, readErr := os.ReadFile(path)
		if readErr != nil || string(data) != "preserve" {
			t.Fatalf("logout changed non-Cloud state %s: %q %v", path, data, readErr)
		}
	}
}

func TestLoginPrivateAccessErrorIsFriendly(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"type": "private_access_required", "message": "allowlist implementation detail"}})
	}))
	defer server.Close()
	t.Setenv("BACKPACK_CLOUD_URL", server.URL)
	client, err := cloud.New(config.NewPaths(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	a := &app{out: &output, err: &output, cloud: client}
	err = a.login(context.Background(), []string{"--name", "test", "--no-browser"})
	if err == nil || !strings.Contains(err.Error(), "isn't available for this account yet") || strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("unexpected private-preview error: %v", err)
	}
}
