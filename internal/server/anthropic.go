package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/backpack-run/backpack-runtime/internal/inference"
)

type anthropicRequest struct {
	Model       string             `json:"model"`
	System      json.RawMessage    `json:"system"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []anthropicTool    `json:"tools"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature"`
	Stream      bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func (s *Server) anthropicMessages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, err)
		return
	}
	request, stream, err := parseAnthropicRequest(body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, err)
		return
	}
	upstream, err := s.executeChat(r.Context(), request, stream)
	if err != nil {
		writeAnthropicError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if stream {
		s.streamAnthropic(w, upstream, request.Model)
		return
	}
	defer upstream.Body.Close()
	data, err := io.ReadAll(io.LimitReader(upstream.Body, 32<<20))
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, err)
		return
	}
	result, err := inference.DecodeChatCompletion(data)
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, err)
		return
	}
	write(w, http.StatusOK, anthropicResponse(request.Model, result))
}

func parseAnthropicRequest(data []byte) (inference.Request, bool, error) {
	var wire anthropicRequest
	if err := json.Unmarshal(data, &wire); err != nil {
		return inference.Request{}, false, fmt.Errorf("invalid Anthropic Messages request: %w", err)
	}
	request := inference.Request{Model: strings.TrimSpace(wire.Model), MaxTokens: wire.MaxTokens, Temperature: wire.Temperature}
	if request.Model == "" {
		return request, wire.Stream, fmt.Errorf("model is required")
	}
	if wire.MaxTokens <= 0 {
		return request, wire.Stream, fmt.Errorf("max_tokens must be greater than zero")
	}
	if len(wire.System) > 0 && string(wire.System) != "null" {
		text, err := protocolText(wire.System)
		if err != nil {
			return request, wire.Stream, fmt.Errorf("system: %w", err)
		}
		request.Instructions = text
	}
	for _, message := range wire.Messages {
		translated, err := translateAnthropicMessage(message)
		if err != nil {
			return request, wire.Stream, err
		}
		request.Messages = append(request.Messages, translated...)
	}
	for _, tool := range wire.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			return request, wire.Stream, fmt.Errorf("tool name is required")
		}
		request.Tools = append(request.Tools, inference.Tool{Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema})
	}
	return request, wire.Stream, nil
}

func translateAnthropicMessage(message anthropicMessage) ([]inference.Message, error) {
	if strings.TrimSpace(message.Role) != "user" && strings.TrimSpace(message.Role) != "assistant" {
		return nil, fmt.Errorf("unsupported Anthropic message role %q", message.Role)
	}
	if strings.HasPrefix(strings.TrimSpace(string(message.Content)), `"`) {
		var text string
		if err := json.Unmarshal(message.Content, &text); err != nil {
			return nil, err
		}
		return []inference.Message{{Role: message.Role, Content: []inference.ContentPart{{Type: "text", Text: text}}}}, nil
	}
	var blocks []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     json.RawMessage `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
		IsError   bool            `json:"is_error"`
		Source    struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
			URL       string `json:"url"`
		} `json:"source"`
	}
	if err := json.Unmarshal(message.Content, &blocks); err != nil {
		return nil, fmt.Errorf("message content must be text or content blocks")
	}
	var regular inference.Message
	regular.Role = message.Role
	var result []inference.Message
	flush := func() {
		if len(regular.Content) > 0 || len(regular.ToolCalls) > 0 {
			result = append(result, regular)
			regular = inference.Message{Role: message.Role}
		}
	}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			regular.Content = append(regular.Content, inference.ContentPart{Type: "text", Text: block.Text})
		case "tool_use":
			arguments := string(block.Input)
			if arguments == "" || arguments == "null" {
				arguments = "{}"
			}
			regular.ToolCalls = append(regular.ToolCalls, inference.ToolCall{ID: block.ID, Name: block.Name, Arguments: arguments})
		case "tool_result":
			flush()
			text, err := protocolText(block.Content)
			if err != nil {
				return nil, fmt.Errorf("tool_result: %w", err)
			}
			if block.IsError {
				text = "Tool error: " + text
			}
			result = append(result, inference.Message{Role: "tool", ToolCallID: block.ToolUseID, Content: []inference.ContentPart{{Type: "text", Text: text}}})
		case "image":
			if block.Source.Type != "base64" || block.Source.MediaType == "" || block.Source.Data == "" {
				return nil, fmt.Errorf("only inline base64 Anthropic image sources are supported")
			}
			regular.Content = append(regular.Content, inference.ContentPart{Type: "image_url", ImageURL: "data:" + block.Source.MediaType + ";base64," + block.Source.Data})
		default:
			return nil, fmt.Errorf("unsupported Anthropic content block %q", block.Type)
		}
	}
	flush()
	return result, nil
}

func anthropicResponse(model string, result inference.Result) map[string]any {
	content := make([]any, 0, len(result.ToolCalls)+1)
	if result.Text != "" {
		content = append(content, map[string]any{"type": "text", "text": result.Text})
	}
	for _, call := range result.ToolCalls {
		input := any(map[string]any{})
		if err := json.Unmarshal([]byte(call.Arguments), &input); err != nil {
			input = map[string]any{"_backpack_raw_arguments": call.Arguments}
		}
		content = append(content, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": input})
	}
	stopReason := "end_turn"
	if len(result.ToolCalls) > 0 {
		stopReason = "tool_use"
	} else if result.FinishReason == "length" {
		stopReason = "max_tokens"
	}
	return map[string]any{"id": newProtocolID("msg"), "type": "message", "role": "assistant", "content": content, "model": model, "stop_reason": stopReason, "stop_sequence": nil, "usage": map[string]int{"input_tokens": result.Usage.InputTokens, "output_tokens": result.Usage.OutputTokens}}
}

func writeAnthropicError(w http.ResponseWriter, status int, err error) {
	write(w, status, map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": err.Error()}})
}

func writeAnthropicSSE(w http.ResponseWriter, flusher http.Flusher, eventType string, fields map[string]any) error {
	fields["type"] = eventType
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (s *Server) streamAnthropic(w http.ResponseWriter, upstream *http.Response, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		upstream.Body.Close()
		writeAnthropicError(w, http.StatusInternalServerError, fmt.Errorf("streaming is unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	id := newProtocolID("msg")
	_ = writeAnthropicSSE(w, flusher, "message_start", map[string]any{"message": map[string]any{"id": id, "type": "message", "role": "assistant", "content": []any{}, "model": model, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]int{"input_tokens": 0, "output_tokens": 0}}})
	textIndex := -1
	text := ""
	nextIndex := 0
	toolIndices := map[int]int{}
	toolCalls := map[int]*inference.ToolCall{}
	usage := inference.Usage{}
	finishReason := ""
	err := readChatStream(upstream, func(delta inference.Delta) error {
		if delta.Text != "" {
			if textIndex < 0 {
				textIndex = nextIndex
				nextIndex++
				if err := writeAnthropicSSE(w, flusher, "content_block_start", map[string]any{"index": textIndex, "content_block": map[string]any{"type": "text", "text": ""}}); err != nil {
					return err
				}
			}
			text += delta.Text
			return writeAnthropicSSE(w, flusher, "content_block_delta", map[string]any{"index": textIndex, "delta": map[string]any{"type": "text_delta", "text": delta.Text}})
		}
		if delta.ToolCall != nil {
			call := toolCalls[delta.ToolIndex]
			if call == nil {
				call = &inference.ToolCall{ID: delta.ToolCall.ID, Name: delta.ToolCall.Name}
				toolCalls[delta.ToolIndex] = call
				toolIndices[delta.ToolIndex] = nextIndex
				nextIndex++
				if err := writeAnthropicSSE(w, flusher, "content_block_start", map[string]any{"index": toolIndices[delta.ToolIndex], "content_block": map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": map[string]any{}}}); err != nil {
					return err
				}
			}
			if call.ID == "" {
				call.ID = delta.ToolCall.ID
			}
			if call.Name == "" {
				call.Name = delta.ToolCall.Name
			}
			call.Arguments += delta.ToolCall.Arguments
			if delta.ToolCall.Arguments != "" {
				return writeAnthropicSSE(w, flusher, "content_block_delta", map[string]any{"index": toolIndices[delta.ToolIndex], "delta": map[string]any{"type": "input_json_delta", "partial_json": delta.ToolCall.Arguments}})
			}
		}
		if delta.Usage != nil {
			usage = *delta.Usage
		}
		if delta.FinishReason != "" {
			finishReason = delta.FinishReason
		}
		return nil
	})
	if err != nil {
		_ = writeAnthropicSSE(w, flusher, "error", map[string]any{"error": map[string]string{"type": "api_error", "message": err.Error()}})
		return
	}
	if textIndex >= 0 {
		_ = writeAnthropicSSE(w, flusher, "content_block_stop", map[string]any{"index": textIndex})
	}
	for index := 0; index < len(toolCalls); index++ {
		if _, ok := toolCalls[index]; ok {
			_ = writeAnthropicSSE(w, flusher, "content_block_stop", map[string]any{"index": toolIndices[index]})
		}
	}
	stopReason := "end_turn"
	if len(toolCalls) > 0 {
		stopReason = "tool_use"
	} else if finishReason == "length" {
		stopReason = "max_tokens"
	}
	_ = writeAnthropicSSE(w, flusher, "message_delta", map[string]any{"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil}, "usage": map[string]int{"output_tokens": usage.OutputTokens}})
	_ = writeAnthropicSSE(w, flusher, "message_stop", map[string]any{})
}
