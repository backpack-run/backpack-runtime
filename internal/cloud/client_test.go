package cloud

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/config"
)

func TestCredentialStoreRoundTripAndDelete(t *testing.T) {
	store := NewCredentialStore(config.NewPaths(t.TempDir()))
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), privateKey...)
	credentials := Credentials{DeviceKeyID: "123e4567-e89b-12d3-a456-426614174000", PrivateKey: privateKey}
	if err = store.Save(credentials); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	defer zero(loaded.PrivateKey)
	if loaded.DeviceKeyID != credentials.DeviceKeyID || string(loaded.PrivateKey) != string(original) {
		t.Fatal("credential store round trip changed the credential")
	}
	metadata, err := os.ReadFile(store.metadataPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(metadata), base64.RawURLEncoding.EncodeToString(original)) || strings.Contains(string(metadata), string(original)) {
		t.Fatal("credential metadata contains private key material")
	}
	if err = store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Load(); !os.IsNotExist(err) {
		t.Fatalf("deleted credential remained loadable: %v", err)
	}
}

func TestAPIKeyModelsAndInferenceDoNotForwardLocalAuthorization(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "test-api-key")
	var inferenceBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("unexpected authorization header")
		}
		switch r.URL.Path {
		case "/v1/models":
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"coder:cloud","status":"available","capabilities":["code"],"context_window":65536}]}`)
		case "/v1/responses":
			data, _ := io.ReadAll(r.Body)
			inferenceBody = string(data)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"response"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := testClient(t, server.URL)
	models, err := client.Models(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "coder:cloud" {
		t.Fatalf("models=%#v err=%v", models, err)
	}
	response, err := client.Inference(context.Background(), "/v1/responses", []byte(`{"model":"coder:cloud","input":"hello"}`), http.Header{"Authorization": []string{"Bearer backpack-local"}})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if !strings.Contains(inferenceBody, `"coder:cloud"`) {
		t.Fatalf("inference body changed: %s", inferenceBody)
	}
}

func TestDeviceChallengeSignsExactInputAndCachesToken(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "")
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	paths := config.NewPaths(t.TempDir())
	store := NewCredentialStore(paths)
	if err = store.Save(Credentials{DeviceKeyID: "123e4567-e89b-12d3-a456-426614174000", PrivateKey: privateKey}); err != nil {
		t.Fatal(err)
	}
	tokenCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/device/challenge":
			_, _ = io.WriteString(w, `{"challenge":"opaque","signature_input":"sign-exactly-this","expires_in":120}`)
		case "/api/auth/device/token":
			tokenCalls++
			var body struct {
				Signature string `json:"signature"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			signature, decodeErr := base64.RawURLEncoding.DecodeString(body.Signature)
			if decodeErr != nil || !ed25519.Verify(publicKey, []byte("sign-exactly-this"), signature) {
				t.Error("device signature did not cover exact signature_input")
			}
			_, _ = io.WriteString(w, `{"access_token":"short-lived-test-token","token_type":"Bearer","expires_in":900}`)
		case "/v1/models":
			if r.Header.Get("Authorization") != "Bearer short-lived-test-token" {
				t.Error("device access token was not attached")
			}
			_, _ = io.WriteString(w, `{"object":"list","data":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := testClient(t, server.URL)
	client.Store = store
	if _, err = client.Models(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Models(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tokenCalls != 1 {
		t.Fatalf("short-lived token was not cached in memory: calls=%d", tokenCalls)
	}
}

func TestDeviceLoginStoresPrivateKeyOnlyAfterApproval(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "")
	paths := config.NewPaths(t.TempDir())
	var publicKey []byte
	statusCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/device/start":
			var request struct {
				PublicKey string `json:"public_key"`
				Name      string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Name != "test-device" {
				t.Errorf("invalid device start request: %#v %v", request, err)
			}
			publicKey, _ = base64.RawURLEncoding.DecodeString(request.PublicKey)
			_, _ = io.WriteString(w, `{"device_code":"opaque-device-code","user_code":"ABCD-EFGH","verification_uri":"https://backpack.run/device","expires_in":5,"poll_interval":1}`)
		case "/api/auth/device/status":
			statusCalls++
			_, _ = io.WriteString(w, `{"status":"approved","device_key_id":"123e4567-e89b-12d3-a456-426614174000"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := testClient(t, server.URL)
	client.Store = NewCredentialStore(paths)
	var prompt LoginPrompt
	if err := client.Login(context.Background(), "test-device", func(received LoginPrompt) error {
		prompt = received
		if _, err := client.Store.Load(); !os.IsNotExist(err) {
			t.Fatalf("credential existed before approval: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if statusCalls != 1 || prompt.UserCode != "ABCD-EFGH" || prompt.VerificationURL != "https://backpack.run/device?code=ABCD-EFGH" {
		t.Fatalf("unexpected device flow: calls=%d prompt=%#v", statusCalls, prompt)
	}
	credentials, err := client.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	defer zero(credentials.PrivateKey)
	if len(publicKey) != ed25519.PublicKeySize || !strings.EqualFold(credentials.DeviceKeyID, "123e4567-e89b-12d3-a456-426614174000") || string(credentials.PrivateKey.Public().(ed25519.PublicKey)) != string(publicKey) {
		t.Fatal("stored device key does not match the registered public key")
	}
}

func TestCloudURLRejectsInsecureRemoteOverride(t *testing.T) {
	t.Setenv("BACKPACK_CLOUD_URL", "http://example.com")
	if _, err := New(config.NewPaths(t.TempDir())); err == nil {
		t.Fatal("insecure remote Cloud URL was accepted")
	}
}

func TestAuthenticatedRequestsRejectRedirects(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "test-redirect-key")
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client := testClient(t, redirect.URL)
	if _, err := client.Models(context.Background()); err == nil || !strings.Contains(err.Error(), "redirects are not accepted") {
		t.Fatalf("authenticated redirect was not rejected: %v", err)
	}
	if targetCalls != 0 {
		t.Fatal("authenticated request followed a redirect")
	}
}

func testClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	t.Setenv("BACKPACK_CLOUD_URL", baseURL)
	client, err := New(config.NewPaths(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return client
}
