package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesTranslatesToolsAndReturnsFunctionCall(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var request map[string]any
		if err := json.Unmarshal(data, &request); err != nil {
			t.Fatal(err)
		}
		messages := request["messages"].([]any)
		if messages[0].(map[string]any)["role"] != "system" || messages[len(messages)-1].(map[string]any)["tool_call_id"] != "call_previous" {
			t.Fatalf("unexpected translated messages: %s", data)
		}
		tools := request["tools"].([]any)
		function := tools[0].(map[string]any)["function"].(map[string]any)
		if function["name"] != "read_file" {
			t.Fatalf("unexpected translated tools: %s", data)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chat-1","model":"coder","choices":[{"finish_reason":"tool_calls","message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]}}],"usage":{"prompt_tokens":12,"completion_tokens":7}}`)
	}))
	defer child.Close()

	s := Server{Sessions: fakeSessions{child.URL}}
	body := `{"model":"coder","instructions":"Be precise","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Inspect a.txt"}]},{"type":"function_call_output","call_id":"call_previous","output":"old result"}],"tools":[{"type":"function","name":"read_file","description":"Read a file","parameters":{"type":"object","properties":{"path":{"type":"string"}}}}]}`
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"object":"response"`) || !strings.Contains(w.Body.String(), `"type":"function_call"`) || !strings.Contains(w.Body.String(), `"call_id":"call_1"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestResponsesStreamingEventSequence(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer child.Close()

	s := Server{Sessions: fakeSessions{child.URL}}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"coder","input":"hello","stream":true}`)))
	output := w.Body.String()
	for _, required := range []string{"response.created", "response.output_item.added", "response.content_part.added", "response.output_text.delta", "response.output_text.done", "response.output_item.done", "response.completed", "data: [DONE]"} {
		if !strings.Contains(output, required) {
			t.Fatalf("missing %q in %s", required, output)
		}
	}
	if strings.Index(output, "response.created") > strings.Index(output, "response.completed") {
		t.Fatal("Responses events are out of order")
	}
}

func TestResponsesFlattensAndRestoresNamespaceTools(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(data), `"name":"files__read"`) {
			t.Fatalf("namespace tool was not flattened: %s", data)
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"tool_calls","message":{"tool_calls":[{"id":"call_ns","function":{"name":"files__read","arguments":"{}"}}]}}]}`)
	}))
	defer child.Close()
	s := Server{Sessions: fakeSessions{child.URL}}
	w := httptest.NewRecorder()
	body := `{"model":"coder","input":"read","tools":[{"type":"namespace","name":"files","description":"Files","tools":[{"type":"function","name":"read","parameters":{"type":"object"}}]}]}`
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"namespace":"files"`) || !strings.Contains(w.Body.String(), `"name":"read"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAnthropicTranslatesToolResultAndReturnsToolUse(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(data), `"tool_call_id":"tool_old"`) || !strings.Contains(string(data), `"name":"write_file"`) {
			t.Fatalf("unexpected translated request: %s", data)
		}
		_, _ = io.WriteString(w, `{"id":"chat-2","model":"coder","choices":[{"finish_reason":"tool_calls","message":{"content":"","tool_calls":[{"id":"tool_new","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"b.txt\",\"text\":\"ok\"}"}}]}}],"usage":{"prompt_tokens":20,"completion_tokens":9}}`)
	}))
	defer child.Close()

	s := Server{Sessions: fakeSessions{child.URL}}
	body := `{"model":"coder","max_tokens":1024,"system":[{"type":"text","text":"Use tools"}],"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"tool_old","name":"read_file","input":{"path":"a.txt"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_old","content":"contents"}]}],"tools":[{"name":"write_file","description":"Write","input_schema":{"type":"object"}}]}`
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", strings.NewReader(body)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"type":"message"`) || !strings.Contains(w.Body.String(), `"type":"tool_use"`) || !strings.Contains(w.Body.String(), `"stop_reason":"tool_use"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAnthropicStreamingToolUseEvents(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"tool_1\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\"}}]}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"a.txt\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer child.Close()

	s := Server{Sessions: fakeSessions{child.URL}}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"coder","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"read"}]}`)))
	output := w.Body.String()
	for _, required := range []string{"message_start", "content_block_start", "input_json_delta", "content_block_stop", "message_delta", `"stop_reason":"tool_use"`, "message_stop"} {
		if !strings.Contains(output, required) {
			t.Fatalf("missing %q in %s", required, output)
		}
	}
}

func TestCompatibilityEndpointsRejectUnsupportedSemantics(t *testing.T) {
	s := Server{Sessions: fakeSessions{}}
	tests := []struct {
		path string
		body string
		want string
	}{
		{"/v1/responses", `{"model":"coder","input":"x","tools":[{"type":"computer","name":"desktop"}]}`, "unsupported Responses tool type"},
		{"/v1/messages", `{"model":"coder","max_tokens":10,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"https://example.com/a.png"}}]}]}`, "only inline base64"},
	}
	for _, test := range tests {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body)))
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), test.want) {
			t.Fatalf("path=%s status=%d body=%s", test.path, w.Code, w.Body.String())
		}
	}
}
