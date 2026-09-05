package pythonworker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
)

func TestQwenAndKokoroUseCommonWorkerProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/infer" {
			http.NotFound(w, r)
			return
		}
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if _, ok := input["audio_path"]; ok {
			_ = json.NewEncoder(w).Encode(map[string]any{"text": "transcript", "language": "English"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"audio_base64": base64.StdEncoding.EncodeToString([]byte("RIFF-audio")), "format": "wav", "sample_rate": 24000})
	}))
	defer server.Close()
	session := &backruntime.Session{ModelID: "model", Endpoint: server.URL}
	qwen := &Adapter{Engine: "qwen-asr", Provides: []string{"transcription"}}
	transcript, err := qwen.TranscribeSession(context.Background(), session, backruntime.TranscriptionRequest{AudioPath: "sample.wav"})
	if err != nil || transcript.Text != "transcript" {
		t.Fatalf("transcription %#v error %v", transcript, err)
	}
	kokoro := &Adapter{Engine: "kokoro", Provides: []string{"speech"}}
	speech, err := kokoro.SynthesizeSession(context.Background(), session, backruntime.SpeechRequest{Input: "hello", Voice: "af_heart", Format: "wav", Speed: 1})
	if err != nil || string(speech.Audio) != "RIFF-audio" || speech.SampleRate != 24000 {
		t.Fatalf("speech %#v error %v", speech, err)
	}
}

func TestAdapterRejectsWrongCapability(t *testing.T) {
	session := &backruntime.Session{ModelID: "model", Endpoint: "http://127.0.0.1:1"}
	if _, err := (&Adapter{Engine: "kokoro"}).TranscribeSession(context.Background(), session, backruntime.TranscriptionRequest{}); err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("error %v", err)
	}
}
