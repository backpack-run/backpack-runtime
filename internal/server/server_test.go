package server

import (
	"context"
	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
	"github.com/backpack-run/backpack-runtime/internal/sessions"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthAndUnknownRoute(t *testing.T) {
	c, _ := catalog.Load()
	s := Server{Version: "test", Catalog: c, Models: models.NewManager(config.NewPaths(t.TempDir()))}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/backpack/v1/health", nil))
	if w.Code != 200 {
		t.Fatalf("health status %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/private", nil))
	if w.Code != 404 {
		t.Fatalf("unknown status %d", w.Code)
	}
}

type fakeSessions struct{ endpoint string }

func (fakeSessions) List() []*backruntime.Session {
	return []*backruntime.Session{{ID: "sess-1", Status: "ready"}}
}
func (fakeSessions) Create(context.Context, sessions.CreateRequest) (*backruntime.Session, error) {
	return &backruntime.Session{ID: "sess-1"}, nil
}
func (fakeSessions) Get(string) (*backruntime.Session, error) {
	return &backruntime.Session{ID: "sess-1"}, nil
}
func (fakeSessions) Stop(context.Context, string) error { return nil }
func (f fakeSessions) Ensure(context.Context, string) (*backruntime.Session, error) {
	return &backruntime.Session{ID: "sess-1"}, nil
}
func (f fakeSessions) Endpoint(string) (string, error) { return f.endpoint, nil }

func TestStreamingChatIsProxied(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer child.Close()
	s := Server{Sessions: fakeSessions{child.URL}}
	w := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"smollm2-135m","messages":[],"stream":true}`))
	s.Handler().ServeHTTP(w, request)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}
