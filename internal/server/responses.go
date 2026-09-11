package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/inference"
)

type responsesRequest struct {
	Model           string          `json:"model"`
	Instructions    json.RawMessage `json:"instructions"`
	Input           json.RawMessage `json:"input"`
	Stream          bool            `json:"stream"`
	Tools           []responsesTool `json:"tools"`
	MaxOutputTokens int             `json:"max_output_tokens"`
	Temperature     *float64        `json:"temperature"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Tools       []responsesTool `json:"tools"`
}

func (s *Server) responses(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request, err := parseResponsesRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	upstream, err := s.executeChat(r.Context(), request, requestStream(body))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if requestStream(body) {
		s.streamResponses(w, upstream, request)
		return
	}
	defer upstream.Body.Close()
	data, err := io.ReadAll(io.LimitReader(upstream.Body, 32<<20))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	result, err := inference.DecodeChatCompletion(data)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	write(w, http.StatusOK, responseObject(newProtocolID("resp"), request, result))
}

func requestStream(body []byte) bool {
	var envelope struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &envelope)
	return envelope.Stream
}

func parseResponsesRequest(data []byte) (inference.Request, error) {
	var wire responsesRequest
	if err := json.Unmarshal(data, &wire); err != nil {
		return inference.Request{}, fmt.Errorf("invalid Responses request: %w", err)
	}
	request := inference.Request{Model: strings.TrimSpace(wire.Model), MaxTokens: wire.MaxOutputTokens, Temperature: wire.Temperature}
	if request.Model == "" {
		return request, fmt.Errorf("model is required")
	}
	if len(wire.Instructions) > 0 && string(wire.Instructions) != "null" {
		text, err := protocolText(wire.Instructions)
		if err != nil {
			return request, fmt.Errorf("instructions: %w", err)
		}
		request.Instructions = text
	}
	input := strings.TrimSpace(string(wire.Input))
	if input == "" || input == "null" {
		return request, fmt.Errorf("input is required")
	}
	if strings.HasPrefix(input, `"`) {
		var text string
		if err := json.Unmarshal(wire.Input, &text); err != nil {
			return request, err
		}
		request.Messages = append(request.Messages, inference.Message{Role: "user", Content: []inference.ContentPart{{Type: "text", Text: text}}})
	} else {
		var items []struct {
			Type      string          `json:"type"`
			Role      string          `json:"role"`
			Content   json.RawMessage `json:"content"`
			ID        string          `json:"id"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments string          `json:"arguments"`
			Output    json.RawMessage `json:"output"`
			Namespace string          `json:"namespace"`
		}
		if err := json.Unmarshal(wire.Input, &items); err != nil {
			return request, fmt.Errorf("input must be text or an array of input items")
		}
		for _, item := range items {
			switch item.Type {
			case "", "message":
				parts, err := responsesContent(item.Content)
				if err != nil {
					return request, err
				}
				role := item.Role
				if role == "developer" {
					role = "system"
				}
				request.Messages = append(request.Messages, inference.Message{Role: role, Content: parts})
			case "function_call":
				name, err := responsesToolName(item.Namespace, item.Name)
				if err != nil {
					return request, fmt.Errorf("function_call: %w", err)
				}
				request.Messages = append(request.Messages, inference.Message{Role: "assistant", ToolCalls: []inference.ToolCall{{ID: item.CallID, Name: name, Arguments: item.Arguments}}})
			case "function_call_output":
				text, err := protocolText(item.Output)
				if err != nil {
					return request, fmt.Errorf("function_call_output: %w", err)
				}
				request.Messages = append(request.Messages, inference.Message{Role: "tool", ToolCallID: item.CallID, Content: []inference.ContentPart{{Type: "text", Text: text}}})
			default:
				return request, fmt.Errorf("unsupported Responses input item type %q", item.Type)
			}
		}
	}
	seen := map[string]bool{}
	for _, tool := range wire.Tools {
		switch tool.Type {
		case "function":
			if err := appendResponsesTool(&request, seen, "", tool); err != nil {
				return request, err
			}
		case "namespace":
			if strings.TrimSpace(tool.Name) == "" || len(tool.Tools) == 0 {
				return request, fmt.Errorf("namespace tool requires a name and nested tools")
			}
			for _, nested := range tool.Tools {
				if nested.Type != "function" {
					return request, fmt.Errorf("unsupported tool type %q in namespace %q", nested.Type, tool.Name)
				}
				if err := appendResponsesTool(&request, seen, tool.Name, nested); err != nil {
					return request, err
				}
			}
		case "web_search":
			// This is a provider-executed tool. Backpack cannot safely translate it
			// to a local model function, so leave it unavailable to the model.
		default:
			return request, fmt.Errorf("unsupported Responses tool type %q", tool.Type)
		}
	}
	return request, nil
}

func appendResponsesTool(request *inference.Request, seen map[string]bool, namespace string, tool responsesTool) error {
	name, err := responsesToolName(namespace, tool.Name)
	if err != nil {
		return err
	}
	if seen[name] {
		return fmt.Errorf("duplicate Responses tool %q", name)
	}
	seen[name] = true
	request.Tools = append(request.Tools, inference.Tool{Name: name, Namespace: namespace, SourceName: tool.Name, Description: tool.Description, Parameters: tool.Parameters})
	return nil
}

func responsesToolName(namespace, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("tool name is required")
	}
	if namespace == "" {
		return name, nil
	}
	value := namespace + "__" + name
	if len(value) <= 128 {
		return value, nil
	}
	digest := sha256.Sum256([]byte(value))
	return value[:111] + "_" + hex.EncodeToString(digest[:8]), nil
}

func responsesToolIdentity(request inference.Request, backendName string) (string, string) {
	for _, tool := range request.Tools {
		if tool.Name == backendName {
			return tool.Namespace, tool.SourceName
		}
	}
	return "", backendName
}

func responsesContent(raw json.RawMessage) ([]inference.ContentPart, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, err
		}
		return []inference.ContentPart{{Type: "text", Text: text}}, nil
	}
	var blocks []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL string `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("message content must be text or content blocks")
	}
	parts := make([]inference.ContentPart, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "input_text", "output_text", "text":
			parts = append(parts, inference.ContentPart{Type: "text", Text: block.Text})
		case "input_image":
			if !strings.HasPrefix(strings.ToLower(block.ImageURL), "data:image/") {
				return nil, fmt.Errorf("input_image must use an inline data:image URL")
			}
			parts = append(parts, inference.ContentPart{Type: "image_url", ImageURL: block.ImageURL})
		default:
			return nil, fmt.Errorf("unsupported Responses content type %q", block.Type)
		}
	}
	return parts, nil
}

func protocolText(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", fmt.Errorf("expected text or text blocks")
	}
	var values []string
	for _, block := range blocks {
		if block.Type != "text" && block.Type != "input_text" && block.Type != "output_text" {
			return "", fmt.Errorf("unsupported text block %q", block.Type)
		}
		values = append(values, block.Text)
	}
	return strings.Join(values, "\n"), nil
}

func responseObject(id string, request inference.Request, result inference.Result) map[string]any {
	output := make([]any, 0, len(result.ToolCalls)+1)
	if result.Text != "" {
		output = append(output, map[string]any{"id": newProtocolID("msg"), "type": "message", "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": result.Text, "annotations": []any{}}}})
	}
	for _, call := range result.ToolCalls {
		namespace, name := responsesToolIdentity(request, call.Name)
		item := map[string]any{"id": newProtocolID("fc"), "type": "function_call", "status": "completed", "call_id": call.ID, "name": name, "arguments": call.Arguments}
		if namespace != "" {
			item["namespace"] = namespace
		}
		output = append(output, item)
	}
	return map[string]any{"id": id, "object": "response", "created_at": time.Now().Unix(), "status": "completed", "model": request.Model, "output": output, "parallel_tool_calls": true, "usage": map[string]int{"input_tokens": result.Usage.InputTokens, "output_tokens": result.Usage.OutputTokens, "total_tokens": result.Usage.InputTokens + result.Usage.OutputTokens}}
}

func newProtocolID(prefix string) string {
	var value [12]byte
	_, _ = rand.Read(value[:])
	return prefix + "_" + hex.EncodeToString(value[:])
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, sequence *int, eventType string, fields map[string]any) error {
	fields["type"] = eventType
	fields["sequence_number"] = *sequence
	*sequence++
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

func (s *Server) streamResponses(w http.ResponseWriter, upstream *http.Response, request inference.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		upstream.Body.Close()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming is unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	id := newProtocolID("resp")
	sequence := 0
	base := map[string]any{"id": id, "object": "response", "created_at": time.Now().Unix(), "status": "in_progress", "model": request.Model, "output": []any{}}
	_ = writeSSE(w, flusher, &sequence, "response.created", map[string]any{"response": base})
	_ = writeSSE(w, flusher, &sequence, "response.in_progress", map[string]any{"response": base})
	messageID := newProtocolID("msg")
	textStarted := false
	textOutputIndex := -1
	nextOutputIndex := 0
	text := ""
	type streamedTool struct {
		Call        inference.ToolCall
		ItemID      string
		OutputIndex int
	}
	toolCalls := map[int]*streamedTool{}
	usage := inference.Usage{}
	finish := ""
	err := readChatStream(upstream, func(delta inference.Delta) error {
		if delta.Text != "" {
			if !textStarted {
				textStarted = true
				textOutputIndex = nextOutputIndex
				nextOutputIndex++
				item := map[string]any{"id": messageID, "type": "message", "status": "in_progress", "role": "assistant", "content": []any{}}
				if err := writeSSE(w, flusher, &sequence, "response.output_item.added", map[string]any{"output_index": textOutputIndex, "item": item}); err != nil {
					return err
				}
				if err := writeSSE(w, flusher, &sequence, "response.content_part.added", map[string]any{"item_id": messageID, "output_index": textOutputIndex, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}}); err != nil {
					return err
				}
			}
			text += delta.Text
			return writeSSE(w, flusher, &sequence, "response.output_text.delta", map[string]any{"item_id": messageID, "output_index": textOutputIndex, "content_index": 0, "delta": delta.Text})
		}
		if delta.ToolCall != nil {
			call := toolCalls[delta.ToolIndex]
			if call == nil {
				call = &streamedTool{Call: inference.ToolCall{ID: delta.ToolCall.ID, Name: delta.ToolCall.Name}, ItemID: newProtocolID("fc"), OutputIndex: nextOutputIndex}
				nextOutputIndex++
				toolCalls[delta.ToolIndex] = call
				namespace, name := responsesToolIdentity(request, call.Call.Name)
				item := map[string]any{"id": call.ItemID, "type": "function_call", "status": "in_progress", "call_id": call.Call.ID, "name": name, "arguments": ""}
				if namespace != "" {
					item["namespace"] = namespace
				}
				if err := writeSSE(w, flusher, &sequence, "response.output_item.added", map[string]any{"output_index": call.OutputIndex, "item": item}); err != nil {
					return err
				}
			}
			if call.Call.ID == "" {
				call.Call.ID = delta.ToolCall.ID
			}
			if call.Call.Name == "" {
				call.Call.Name = delta.ToolCall.Name
			}
			call.Call.Arguments += delta.ToolCall.Arguments
			if delta.ToolCall.Arguments != "" {
				return writeSSE(w, flusher, &sequence, "response.function_call_arguments.delta", map[string]any{"item_id": call.ItemID, "output_index": call.OutputIndex, "delta": delta.ToolCall.Arguments})
			}
		}
		if delta.Usage != nil {
			usage = *delta.Usage
		}
		if delta.FinishReason != "" {
			finish = delta.FinishReason
		}
		return nil
	})
	if err != nil {
		_ = writeSSE(w, flusher, &sequence, "response.failed", map[string]any{"response": map[string]any{"id": id, "object": "response", "status": "failed", "error": map[string]string{"message": err.Error(), "type": "runtime_error"}}})
		return
	}
	outputByIndex := make(map[int]any, len(toolCalls)+1)
	if textStarted {
		_ = writeSSE(w, flusher, &sequence, "response.output_text.done", map[string]any{"item_id": messageID, "output_index": textOutputIndex, "content_index": 0, "text": text})
		part := map[string]any{"type": "output_text", "text": text, "annotations": []any{}}
		_ = writeSSE(w, flusher, &sequence, "response.content_part.done", map[string]any{"item_id": messageID, "output_index": textOutputIndex, "content_index": 0, "part": part})
		item := map[string]any{"id": messageID, "type": "message", "status": "completed", "role": "assistant", "content": []any{part}}
		_ = writeSSE(w, flusher, &sequence, "response.output_item.done", map[string]any{"output_index": textOutputIndex, "item": item})
		outputByIndex[textOutputIndex] = item
	}
	for index := 0; index < len(toolCalls); index++ {
		call := toolCalls[index]
		if call == nil {
			continue
		}
		_ = writeSSE(w, flusher, &sequence, "response.function_call_arguments.done", map[string]any{"item_id": call.ItemID, "output_index": call.OutputIndex, "arguments": call.Call.Arguments})
		namespace, name := responsesToolIdentity(request, call.Call.Name)
		item := map[string]any{"id": call.ItemID, "type": "function_call", "status": "completed", "call_id": call.Call.ID, "name": name, "arguments": call.Call.Arguments}
		if namespace != "" {
			item["namespace"] = namespace
		}
		_ = writeSSE(w, flusher, &sequence, "response.output_item.done", map[string]any{"output_index": call.OutputIndex, "item": item})
		outputByIndex[call.OutputIndex] = item
	}
	output := make([]any, 0, len(outputByIndex))
	for index := 0; index < nextOutputIndex; index++ {
		if item, ok := outputByIndex[index]; ok {
			output = append(output, item)
		}
	}
	status := "completed"
	if finish == "length" {
		status = "incomplete"
	}
	response := map[string]any{"id": id, "object": "response", "created_at": time.Now().Unix(), "status": status, "model": request.Model, "output": output, "usage": map[string]int{"input_tokens": usage.InputTokens, "output_tokens": usage.OutputTokens, "total_tokens": usage.InputTokens + usage.OutputTokens}}
	_ = writeSSE(w, flusher, &sequence, "response.completed", map[string]any{"response": response})
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()
}
