package inference

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatCompletionsBodyPreservesNeutralToolLoop(t *testing.T) {
	request := Request{
		Model:        "coder",
		Instructions: "Be precise",
		Messages: []Message{
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Name: "read_file", Arguments: `{"path":"a.txt"}`}}},
			{Role: "tool", ToolCallID: "call_1", Content: []ContentPart{{Type: "text", Text: "contents"}}},
		},
		Tools: []Tool{{Name: "read_file", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	body, err := request.ChatCompletionsBody(true)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{`"role":"system"`, `"tool_call_id":"call_1"`, `"tool_calls"`, `"name":"read_file"`, `"stream":true`} {
		if !strings.Contains(text, required) {
			t.Fatalf("missing %s in %s", required, text)
		}
	}
}

func TestDecodeChatChunkPreservesFragmentedToolArguments(t *testing.T) {
	chunk, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 2, "id": "call_2", "function": map[string]any{"name": "write_file", "arguments": `{"path":"a`}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	deltas, err := DecodeChatChunk(chunk)
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 1 || deltas[0].ToolCall == nil || deltas[0].ToolIndex != 2 || deltas[0].ToolCall.Arguments != `{"path":"a` {
		t.Fatalf("unexpected deltas %#v", deltas)
	}
}
