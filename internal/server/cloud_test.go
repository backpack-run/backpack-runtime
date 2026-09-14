package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/cloud"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/models"
)

func TestCloudProtocolsProxyRawPayloadAndStripLocalCredential(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "test-proxy-key")
	var pathsSeen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathsSeen = append(pathsSeen, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer test-proxy-key" {
			t.Error("Cloud request did not use the configured credential")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"sentinel":"preserved"`) {
			t.Errorf("raw payload was not preserved: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "request-safe")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()
	cloudClient := serverCloudClient(t, upstream.URL)
	s := Server{Cloud: cloudClient, CloudProxyToken: "daemon-secret"}
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
		body := `{"model":"coder:cloud","sentinel":"preserved","messages":[],"max_tokens":1}`
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer daemon-secret")
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("X-Request-ID") != "request-safe" || response.Body.String() != `{"ok":true}` {
			t.Fatalf("path=%s status=%d headers=%v body=%s", path, response.Code, response.Header(), response.Body.String())
		}
	}
	if len(pathsSeen) != 3 {
		t.Fatalf("unexpected Cloud requests: %#v", pathsSeen)
	}
}

func TestCloudStreamingIsForwardedWithoutBufferingContractChanges(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "test-stream-key")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, "event: response.output_text.delta\ndata: {\"delta\":\"ok\"}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()
	s := Server{Cloud: serverCloudClient(t, upstream.URL), CloudProxyToken: "daemon-secret"}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"coder:cloud","input":"hello","stream":true}`))
	request.Header.Set("Authorization", "Bearer daemon-secret")
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "response.output_text.delta") || !strings.Contains(response.Body.String(), "[DONE]") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCloudProxyRejectsUnauthenticatedLocalRequest(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "upstream-secret")
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { upstreamCalls++ }))
	defer upstream.Close()
	s := Server{Cloud: serverCloudClient(t, upstream.URL), CloudProxyToken: "daemon-secret"}
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coder:cloud","messages":[]}`)))
	if response.Code != http.StatusUnauthorized || upstreamCalls != 0 {
		t.Fatalf("unauthenticated local request status=%d upstream_calls=%d", response.Code, upstreamCalls)
	}
}

func TestDaemonShutdownRequiresLocalAuthorization(t *testing.T) {
	stopped := make(chan struct{}, 1)
	s := Server{CloudProxyToken: "daemon-secret", shutdown: func() { stopped <- struct{}{} }}

	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/backpack/v1/shutdown", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated shutdown status=%d", response.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/backpack/v1/shutdown", nil)
	request.Header.Set("Authorization", "Bearer daemon-secret")
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("authorized shutdown status=%d body=%s", response.Code, response.Body.String())
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("authorized shutdown did not invoke the server callback")
	}
}

func TestCodexAppRouteUsesPathTokenAndReplacesDesktopAuthorization(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "upstream-secret")
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Errorf("desktop authorization reached Cloud: %q", r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()
	s := Server{Cloud: serverCloudClient(t, upstream.URL), CloudProxyToken: "daemon-secret"}
	body := `{"model":"coder:cloud","input":"hello"}`

	request := httptest.NewRequest(http.MethodPost, "/api/backpack/v1/integrations/codex-app/daemon-secret/v1/responses", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer existing-codex-login")
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != `{"ok":true}` || upstreamCalls != 1 {
		t.Fatalf("authorized route status=%d calls=%d body=%s", response.Code, upstreamCalls, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/backpack/v1/integrations/codex-app/wrong-token/v1/responses", strings.NewReader(body))
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || upstreamCalls != 1 {
		t.Fatalf("invalid path token status=%d calls=%d", response.Code, upstreamCalls)
	}
}

func TestModelsDoNotFailWhenCloudIsLoggedOut(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "")
	paths := config.NewPaths(t.TempDir())
	cloudClient, err := cloud.New(paths)
	if err != nil {
		t.Fatal(err)
	}
	s := Server{Models: models.NewManager(paths), Cloud: cloudClient}
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("logged-out local model listing failed: %d %s", response.Code, response.Body.String())
	}
}

func serverCloudClient(t *testing.T, baseURL string) *cloud.Client {
	t.Helper()
	t.Setenv("BACKPACK_CLOUD_URL", baseURL)
	client, err := cloud.New(config.NewPaths(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	client.HTTP = &http.Client{}
	return client
}
