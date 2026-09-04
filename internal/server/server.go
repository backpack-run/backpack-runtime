package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
	"github.com/backpack-run/backpack-runtime/internal/sessions"
)

type SessionService interface {
	List() []*backruntime.Session
	Create(context.Context, sessions.CreateRequest) (*backruntime.Session, error)
	Get(string) (*backruntime.Session, error)
	Stop(context.Context, string) error
	Ensure(context.Context, string) (*backruntime.Session, error)
	Endpoint(string) (string, error)
}

type Server struct {
	Version  string
	Catalog  catalog.Catalog
	Models   *models.Manager
	Sessions SessionService
	Hardware func() (compute.Hardware, error)
	Client   *http.Client
	Targets  compute.TargetStore
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
			writeError(w, 500, err)
			return
		}
		write(w, 200, installed)
	})
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, _ *http.Request) {
		installed, err := s.Models.List()
		if err != nil {
			writeError(w, 500, err)
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
			writeError(w, 500, err)
			return
		}
		write(w, 200, h)
	})
	mux.HandleFunc("GET /api/backpack/v1/compute", func(w http.ResponseWriter, _ *http.Request) {
		items, err := s.Targets.List()
		if err != nil {
			writeError(w, 500, err)
			return
		}
		write(w, 200, map[string]any{"data": items, "built_in": []string{"local"}})
	})
	mux.HandleFunc("GET /api/backpack/v1/sessions", func(w http.ResponseWriter, _ *http.Request) { write(w, 200, map[string]any{"data": s.Sessions.List()}) })
	mux.HandleFunc("POST /api/backpack/v1/sessions", s.createSession)
	mux.HandleFunc("GET /api/backpack/v1/sessions/{id}", s.getSession)
	mux.HandleFunc("DELETE /api/backpack/v1/sessions/{id}", s.deleteSession)
	mux.HandleFunc("POST /v1/chat/completions", s.chatCompletions)
	return security(mux)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var request sessions.CreateRequest
	if err := decode(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	session, err := s.Sessions.Create(r.Context(), request)
	if err != nil {
		writeError(w, 422, err)
		return
	}
	write(w, 201, session)
}
func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	session, err := s.Sessions.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, err)
		return
	}
	write(w, 200, session)
}
func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.Sessions.Stop(ctx, r.PathValue("id")); err != nil {
		writeError(w, 404, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) chatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var envelope struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err = json.Unmarshal(body, &envelope); err != nil || envelope.Model == "" {
		writeError(w, 400, fmt.Errorf("model is required"))
		return
	}
	session, err := s.Sessions.Ensure(r.Context(), envelope.Model)
	if err != nil {
		writeError(w, 422, err)
		return
	}
	endpoint, err := s.Sessions.Endpoint(session.ID)
	if err != nil {
		writeError(w, 503, err)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		writeError(w, 500, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	res, err := client.Do(req)
	if err != nil {
		writeError(w, 502, fmt.Errorf("llama.cpp request failed: %w", err))
		return
	}
	defer res.Body.Close()
	for k, values := range res.Header {
		if strings.EqualFold(k, "Connection") {
			continue
		}
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(res.StatusCode)
	if envelope.Stream {
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		buffer := make([]byte, 32<<10)
		for {
			n, readErr := res.Body.Read(buffer)
			if n > 0 {
				if _, err = w.Write(buffer[:n]); err != nil {
					return
				}
				flusher.Flush()
			}
			if readErr != nil {
				return
			}
		}
	}
	_, _ = io.Copy(w, res.Body)
}

func (s *Server) Serve(ctx context.Context, address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("refusing non-loopback bind %q; remote API authentication is not implemented", host)
	}
	httpServer := &http.Server{Addr: address, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(listener) }()
	select {
	case err := <-done:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
func (s *Server) Listen(address string) error { return s.Serve(context.Background(), address) }
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
func decode(r *http.Request, v any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON request: %w", err)
	}
	return nil
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, err error) {
	write(w, status, map[string]any{"error": map[string]string{"message": err.Error(), "type": "runtime_error"}})
}
