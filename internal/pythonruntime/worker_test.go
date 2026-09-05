package pythonruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWorkerProtocolHealthAndStructuredError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			_ = json.NewEncoder(w).Encode(Health{ProtocolVersion: 1, Engine: "test", Status: "ready"})
		case "/v1/infer":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid input"})
		}
	}))
	defer server.Close()
	client := NewWorkerClient(server.URL)
	if err := client.WaitReady(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.Infer(context.Background(), map[string]string{"x": "y"}, nil); err == nil || !strings.Contains(err.Error(), "invalid input") {
		t.Fatalf("structured error %v", err)
	}
}

func TestWorkerProtocolRejectsMismatchAndTimesOut(t *testing.T) {
	mismatch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Health{ProtocolVersion: 2, Status: "ready"})
	}))
	defer mismatch.Close()
	if err := NewWorkerClient(mismatch.URL).WaitReady(context.Background()); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("mismatch error %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := NewWorkerClient("http://127.0.0.1:1").WaitReady(ctx); err == nil {
		t.Fatal("unavailable worker did not time out")
	}
}

func TestEnvironmentDefinitionsAreEngineDrivenAndPinned(t *testing.T) {
	qwen := definitions["qwen-asr"]
	kokoro := definitions["kokoro"]
	if qwen.Version != "0.0.6" || qwen.Python != "3.11" || kokoro.Version != "0.9.4" || kokoro.Python != "3.12" {
		t.Fatalf("definitions qwen=%#v kokoro=%#v", qwen, kokoro)
	}
	data, err := assets.ReadFile("requirements/" + qwen.Requirements)
	if err != nil || !strings.Contains(string(data), "qwen-asr==0.0.6") || !strings.Contains(string(data), "torch==2.13.0+cpu") {
		t.Fatalf("qwen lock: %v %q", err, data)
	}
}
