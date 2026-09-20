package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/cloud"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/models"
	"github.com/klauspost/compress/zstd"
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
		_, _ = io.WriteString(w, "event: response.completed\ndata: {\"type\":\"response.completed\"}\n\ndata: [DONE]\n\n")
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

func TestCloudResponsesStreamEndsWithExplicitFailureWhenUpstreamDisconnects(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "test-stream-key")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-ID", "request-123")
		_, _ = io.WriteString(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
	}))
	defer upstream.Close()
	s := Server{Cloud: serverCloudClient(t, upstream.URL), CloudProxyToken: "daemon-secret"}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"coder:cloud","input":"hello","stream":true}`))
	request.Header.Set("Authorization", "Bearer daemon-secret")
	s.Handler().ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "response.failed") || !strings.Contains(body, "upstream_stream_terminated") || !strings.Contains(body, "request-123") {
		t.Fatalf("status=%d body=%s", response.Code, body)
	}
	if strings.Contains(body, "response.completed") {
		t.Fatalf("interrupted stream was incorrectly marked complete: %s", body)
	}
}

func TestCloudLegacyStreamsEndWithProtocolErrorsWhenUpstreamDisconnects(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "test-stream-key")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"partial\":true}\n\n")
	}))
	defer upstream.Close()
	for _, test := range []struct{ path, marker string }{{"/v1/messages", "event: error"}, {"/v1/chat/completions", "upstream_stream_terminated"}} {
		s := Server{Cloud: serverCloudClient(t, upstream.URL), CloudProxyToken: "daemon-secret"}
		response := httptest.NewRecorder()
		body := `{"model":"coder:cloud","messages":[],"max_tokens":1,"stream":true}`
		request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer daemon-secret")
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.marker) {
			t.Fatalf("path=%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestCloudPlainTextFailureBecomesStructuredAPIError(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "test-stream-key")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = io.WriteString(w, "provider-specific failure details")
	}))
	defer upstream.Close()
	s := Server{Cloud: serverCloudClient(t, upstream.URL), CloudProxyToken: "daemon-secret"}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"coder:cloud","input":"hello","stream":true}`))
	request.Header.Set("Authorization", "Bearer daemon-secret")
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusGatewayTimeout || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status=%d content-type=%q body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "GPU worker becomes ready") || strings.Contains(response.Body.String(), "provider-specific") {
		t.Fatalf("unsafe or unhelpful normalized error: %s", response.Body.String())
	}
}

func TestCloudStructuredUnavailablePreservesSafeRetryContract(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "test-stream-key")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "15")
		w.Header().Set("X-Provider-Request-ID", "provider-safe-123")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"type":"cloud_inference_unavailable","message":"Cloud inference is temporarily unavailable"}}`)
	}))
	defer upstream.Close()
	s := Server{Cloud: serverCloudClient(t, upstream.URL), CloudProxyToken: "daemon-secret"}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"coder:cloud","messages":[],"max_tokens":1}`))
	request.Header.Set("Authorization", "Bearer daemon-secret")
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") != "15" || response.Header().Get("X-Provider-Request-ID") != "provider-safe-123" {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"type":"cloud_inference_unavailable"`) || strings.Contains(response.Body.String(), `"type":"runtime_error"`) {
		t.Fatalf("structured Cloud error was not preserved: %s", response.Body.String())
	}
}

func TestCloudInterruptedStreamUsesProviderCorrelationID(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "test-stream-key")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-ID", "gateway-request")
		w.Header().Set("X-Provider-Request-ID", "provider-request")
		_, _ = io.WriteString(w, "event: response.output_text.delta\ndata: {\"delta\":\"partial\"}\n\n")
	}))
	defer upstream.Close()
	s := Server{Cloud: serverCloudClient(t, upstream.URL), CloudProxyToken: "daemon-secret"}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"coder:cloud","input":"hello","stream":true}`))
	request.Header.Set("Authorization", "Bearer daemon-secret")
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("X-Provider-Request-ID") != "provider-request" || !strings.Contains(response.Body.String(), "provider-request") {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
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

func TestCloudProxyTranslatesEntitlementFailure(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "upstream-secret")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"type":"cloud_access_required","message":"internal entitlement state"}}`)
	}))
	defer upstream.Close()
	s := Server{Cloud: serverCloudClient(t, upstream.URL), CloudProxyToken: "daemon-secret"}
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"coder:cloud","input":"hello"}`))
	request.Header.Set("Authorization", "Bearer daemon-secret")
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "private preview") || !strings.Contains(response.Body.String(), "use Backpack locally") {
		t.Fatalf("unexpected product error: status=%d body=%s", response.Code, response.Body.String())
	}
	for _, leaked := range []string{"cloud_access_required", "internal entitlement state"} {
		if strings.Contains(response.Body.String(), leaked) {
			t.Fatalf("backend detail %q leaked: %s", leaked, response.Body.String())
		}
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

func TestCodexAppRouteDecodesZstdRequest(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "upstream-secret")
	var upstreamBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"response-test","object":"response","status":"completed","output":[]}`)
	}))
	defer upstream.Close()
	s := Server{Cloud: serverCloudClient(t, upstream.URL), CloudProxyToken: "daemon-secret"}
	body := []byte(`{"model":"coder:cloud","input":"hello","stream":false}`)
	var compressed bytes.Buffer
	encoder, err := zstd.NewWriter(&compressed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = encoder.Write(body); err != nil {
		t.Fatal(err)
	}
	if err = encoder.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/backpack/v1/integrations/codex-app/daemon-secret/v1/responses", bytes.NewReader(compressed.Bytes()))
	request.Header.Set("Content-Encoding", "zstd")
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("zstd request status=%d body=%s", response.Code, response.Body.String())
	}
	if !bytes.Equal(upstreamBody, body) {
		t.Fatalf("upstream did not receive decoded request: %q", upstreamBody)
	}
}

func TestCodexAppRouteRejectsUnknownContentEncoding(t *testing.T) {
	s := Server{CloudProxyToken: "daemon-secret"}
	request := httptest.NewRequest(http.MethodPost, "/api/backpack/v1/integrations/codex-app/daemon-secret/v1/responses", strings.NewReader("not-json"))
	request.Header.Set("Content-Encoding", "br")
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unsupported Codex App content encoding") {
		t.Fatalf("unknown encoding status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCodexAppNativeModelKeepsOpenAISessionAwayFromBackpackCloud(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "backpack-cloud-secret")
	cloudCalls := 0
	cloudUpstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { cloudCalls++ }))
	defer cloudUpstream.Close()
	var nativeAuthorization, nativeAccount, nativePath string
	nativeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nativeAuthorization = r.Header.Get("Authorization")
		nativeAccount = r.Header.Get("ChatGPT-Account-ID")
		nativePath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: native-ok\n\n")
	}))
	defer nativeUpstream.Close()
	s := Server{
		Cloud:           serverCloudClient(t, cloudUpstream.URL),
		CloudProxyToken: "daemon-secret",
		codexChatGPTURL: nativeUpstream.URL + "/backend-api/codex/responses",
		Client:          &http.Client{},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/backpack/v1/integrations/codex-app/daemon-secret/v1/responses", strings.NewReader(`{"model":"gpt-account-model","input":"hello","stream":true}`))
	request.Header.Set("Authorization", "Bearer native-codex-session")
	request.Header.Set("ChatGPT-Account-ID", "account-123")
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "data: native-ok\n\n" {
		t.Fatalf("native response status=%d body=%q", response.Code, response.Body.String())
	}
	if cloudCalls != 0 || nativeAuthorization != "Bearer native-codex-session" || nativeAccount != "account-123" || nativePath != "/backend-api/codex/responses" {
		t.Fatalf("routing cloud_calls=%d auth=%q account=%q path=%q", cloudCalls, nativeAuthorization, nativeAccount, nativePath)
	}
}

func TestCodexAppRefusesDaemonCredentialForNativeModel(t *testing.T) {
	nativeCalls := 0
	nativeUpstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nativeCalls++ }))
	defer nativeUpstream.Close()
	s := Server{CloudProxyToken: "daemon-secret", codexOpenAIURL: nativeUpstream.URL + "/v1/responses", Client: &http.Client{}}
	request := httptest.NewRequest(http.MethodPost, "/api/backpack/v1/integrations/codex-app/daemon-secret/v1/responses", strings.NewReader(`{"model":"gpt-account-model","input":"hello"}`))
	request.Header.Set("Authorization", "Bearer daemon-secret")
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || nativeCalls != 0 || !strings.Contains(response.Body.String(), "require Codex authentication") {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, nativeCalls, response.Body.String())
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
