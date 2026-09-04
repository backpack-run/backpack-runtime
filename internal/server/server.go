package server

import (
	"encoding/json"
	"fmt"
	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/models"
	"net"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	Version  string
	Catalog  catalog.Catalog
	Models   *models.Manager
	Hardware func() (compute.Hardware, error)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/backpack/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		write(w, 200, map[string]any{"status": "ok", "version": s.Version})
	})
	mux.HandleFunc("GET /api/backpack/v1/version", func(w http.ResponseWriter, _ *http.Request) { write(w, 200, map[string]string{"version": s.Version}) })
	mux.HandleFunc("GET /api/backpack/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		installed, err := s.Models.List()
		if err != nil {
			writeError(w, err)
			return
		}
		write(w, 200, installed)
	})
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, _ *http.Request) {
		installed, err := s.Models.List()
		if err != nil {
			writeError(w, err)
			return
		}
		data := make([]map[string]any, 0, len(installed))
		for _, m := range installed {
			data = append(data, map[string]any{"id": m.ID, "object": "model", "owned_by": "backpack-run"})
		}
		write(w, 200, map[string]any{"object": "list", "data": data})
	})
	mux.HandleFunc("GET /api/backpack/v1/hardware", func(w http.ResponseWriter, _ *http.Request) {
		h, err := s.Hardware()
		if err != nil {
			writeError(w, err)
			return
		}
		write(w, 200, h)
	})
	return security(mux)
}
func (s *Server) Listen(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("refusing non-loopback bind %q; remote API authentication is not implemented", host)
	}
	server := &http.Server{Addr: address, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	return server.ListenAndServe()
}
func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !strings.HasPrefix(r.URL.Path, "/v1/") && !strings.HasPrefix(r.URL.Path, "/api/backpack/v1/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, err error) {
	write(w, 500, map[string]any{"error": map[string]string{"message": err.Error(), "type": "runtime_error"}})
}
