package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClaudeAppGatewayCatalogCountAndModelRewrite(t *testing.T) {
	t.Setenv("BACKPACK_API_KEY", "cloud-upstream-key")
	var upstreamModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer cloud-upstream-key" {
			t.Fatalf("unexpected upstream authorization %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &payload)
		upstreamModel = payload.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","content":[],"model":"coder:cloud","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer upstream.Close()
	s := Server{Cloud: serverCloudClient(t, upstream.URL), CloudProxyToken: "daemon-secret"}
	base := "/api/backpack/v1/integrations/claude-app/daemon-secret/32768/coder:cloud/v1"

	models := httptest.NewRecorder()
	s.Handler().ServeHTTP(models, httptest.NewRequest(http.MethodGet, base+"/models", nil))
	if models.Code != http.StatusOK || !strings.Contains(models.Body.String(), claudeAppRouteModel) || !strings.Contains(models.Body.String(), "coder:cloud") {
		t.Fatalf("models status=%d body=%s", models.Code, models.Body.String())
	}

	count := httptest.NewRecorder()
	s.Handler().ServeHTTP(count, httptest.NewRequest(http.MethodPost, base+"/messages/count_tokens", strings.NewReader(`{"model":"claude-sonnet-5","messages":[{"role":"user","content":"hello"}]}`)))
	if count.Code != http.StatusOK || !strings.Contains(count.Body.String(), "input_tokens") {
		t.Fatalf("count status=%d body=%s", count.Code, count.Body.String())
	}

	message := httptest.NewRecorder()
	s.Handler().ServeHTTP(message, httptest.NewRequest(http.MethodPost, base+"/messages", strings.NewReader(`{"model":"claude-sonnet-5","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)))
	if message.Code != http.StatusOK || upstreamModel != "coder:cloud" {
		t.Fatalf("message status=%d model=%q body=%s", message.Code, upstreamModel, message.Body.String())
	}
}

func TestClaudeAppGatewayRejectsWrongToken(t *testing.T) {
	s := Server{CloudProxyToken: "daemon-secret"}
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/backpack/v1/integrations/claude-app/wrong/32768/coder:cloud/v1/models", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
