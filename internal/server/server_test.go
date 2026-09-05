package server

import (
	"bytes"
	"context"
	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
	"github.com/backpack-run/backpack-runtime/internal/sessions"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
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
func (f fakeSessions) Transcribe(_ context.Context, model, computeName string, request backruntime.TranscriptionRequest) (*backruntime.Transcription, error) {
	data, err := os.ReadFile(request.AudioPath)
	if err != nil {
		return nil, err
	}
	return &backruntime.Transcription{Text: string(data), Model: model, Language: request.Language}, nil
}
func (f fakeSessions) Synthesize(_ context.Context, model, computeName string, request backruntime.SpeechRequest) (*backruntime.Speech, error) {
	return &backruntime.Speech{Audio: []byte("RIFF-test"), Format: "wav", SampleRate: 24000, Model: model}, nil
}

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

func TestAudioTranscriptionUsesMultipartContract(t *testing.T) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "sample.wav")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, "spoken words")
	_ = form.WriteField("model", "whisper-large-v3-turbo")
	_ = form.WriteField("language", "en")
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	s := Server{Audio: fakeSessions{}}
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, request)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"text":"spoken words"`) {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestSpeechUsesGenericAudioContract(t *testing.T) {
	s := Server{Speech: fakeSessions{}}
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"kokoro-82m","input":"Hello","voice":"af_heart","format":"wav"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, request)
	if w.Code != http.StatusOK || w.Body.String() != "RIFF-test" || w.Header().Get("Content-Type") != "audio/wav" {
		t.Fatalf("status=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
	}
}
