package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/events"
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

type TranscriptionService interface {
	Transcribe(context.Context, string, string, backruntime.TranscriptionRequest) (*backruntime.Transcription, error)
}
type SpeechService interface {
	Synthesize(context.Context, string, string, backruntime.SpeechRequest) (*backruntime.Speech, error)
}

type Server struct {
	Version  string
	Catalog  catalog.Catalog
	Models   *models.Manager
	Sessions SessionService
	Audio    TranscriptionService
	Speech   SpeechService
	Hardware func() (compute.Hardware, error)
	Client   *http.Client
	Targets  compute.TargetStore
	Events   *events.Broker
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
	mux.HandleFunc("POST /v1/audio/transcriptions", s.transcriptions)
	mux.HandleFunc("POST /v1/audio/speech", s.speech)
	mux.HandleFunc("GET /api/backpack/v1/events", s.eventStream)
	return security(mux)
}

func (s *Server) eventStream(w http.ResponseWriter, r *http.Request) {
	if s.Events == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("event stream is not configured"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming is unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	channel, unsubscribe := s.Events.Subscribe()
	defer unsubscribe()
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case event := <-channel:
			data, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) speech(w http.ResponseWriter, r *http.Request) {
	if s.Speech == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("speech service is not configured"))
		return
	}
	var request struct {
		Model, Input, Voice, Format, Compute string
		Speed                                float64
		Force                                bool
	}
	if err := decode(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(request.Model) == "" || strings.TrimSpace(request.Input) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("model and input are required"))
		return
	}
	if len(request.Input) > 12000 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("input exceeds 12000 characters"))
		return
	}
	if request.Format == "" {
		request.Format = "wav"
	}
	if request.Format != "wav" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("only wav output is currently supported"))
		return
	}
	if request.Speed == 0 {
		request.Speed = 1
	}
	if request.Speed < 0.5 || request.Speed > 2 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("speed must be between 0.5 and 2.0"))
		return
	}
	result, err := s.Speech.Synthesize(r.Context(), request.Model, request.Compute, backruntime.SpeechRequest{Input: request.Input, Voice: request.Voice, Format: request.Format, Speed: request.Speed, Force: request.Force})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("X-Backpack-Model", result.Model)
	w.Header().Set("X-Backpack-Sample-Rate", fmt.Sprint(result.SampleRate))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Audio)
}

func (s *Server) transcriptions(w http.ResponseWriter, r *http.Request) {
	if s.Audio == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("transcription service is not configured"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid multipart request: %w", err))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	model := strings.TrimSpace(r.FormValue("model"))
	if model == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("model is required"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("audio file is required: %w", err))
		return
	}
	defer file.Close()
	uploadDir := filepath.Join(os.TempDir(), "backpack-runtime-uploads")
	if err = os.MkdirAll(uploadDir, 0700); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ext := filepath.Ext(filepath.Base(header.Filename))
	temporary, err := os.CreateTemp(uploadDir, "audio-*"+ext)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	path := temporary.Name()
	defer os.Remove(path)
	if _, err = io.Copy(temporary, file); err != nil {
		_ = temporary.Close()
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err = temporary.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	force, _ := strconv.ParseBool(r.FormValue("force"))
	result, err := s.Audio.Transcribe(r.Context(), model, r.FormValue("compute"), backruntime.TranscriptionRequest{AudioPath: path, Language: r.FormValue("language"), Force: force})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	write(w, http.StatusOK, result)
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
