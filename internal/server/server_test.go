package server

import (
	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/models"
	"net/http"
	"net/http/httptest"
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
