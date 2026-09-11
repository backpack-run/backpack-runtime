package inference

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Request is Backpack's protocol-neutral chat/tool inference contract. Public
// compatibility handlers translate into this type; runtime adapters continue
// to speak only their engine's native wire format.
type Request struct {
	Model        string
	Instructions string
	Messages     []Message
	Tools        []Tool
	MaxTokens    int
	Temperature  *float64
}

type Message struct {
	Role       string
	Content    []ContentPart
	ToolCalls  []ToolCall
	ToolCallID string
}

type ContentPart struct {
	Type     string
	Text     string
	ImageURL string
}

type Tool struct {
	Name        string
	Namespace   string
	SourceName  string
	Description string
	Parameters  json.RawMessage
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type Result struct {
	ID           string
	Model        string
	Text         string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        Usage
}

type Delta struct {
	Text         string
	ToolCall     *ToolCall
	ToolIndex    int
	FinishReason string
	Usage        *Usage
}

func (r Request) ChatCompletionsBody(stream bool) ([]byte, error) {
	if strings.TrimSpace(r.Model) == "" {
		return nil, fmt.Errorf("model is required")
	}
	messages := make([]map[string]any, 0, len(r.Messages)+1)
	if r.Instructions != "" {
		messages = append(messages, map[string]any{"role": "system", "content": r.Instructions})
	}
	for _, message := range r.Messages {
		wire := map[string]any{"role": message.Role}
		if message.ToolCallID != "" {
			wire["tool_call_id"] = message.ToolCallID
		}
		if len(message.Content) == 1 && message.Content[0].Type == "text" {
			wire["content"] = message.Content[0].Text
		} else {
			parts := make([]map[string]any, 0, len(message.Content))
			for _, part := range message.Content {
				switch part.Type {
				case "text":
					parts = append(parts, map[string]any{"type": "text", "text": part.Text})
				case "image_url":
					parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": part.ImageURL}})
				default:
					return nil, fmt.Errorf("unsupported content part %q", part.Type)
				}
			}
			wire["content"] = parts
		}
		if len(message.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				calls = append(calls, map[string]any{"id": call.ID, "type": "function", "function": map[string]string{"name": call.Name, "arguments": call.Arguments}})
			}
			wire["tool_calls"] = calls
		}
		messages = append(messages, wire)
	}
	payload := map[string]any{"model": r.Model, "messages": messages, "stream": stream}
	if stream {
		payload["stream_options"] = map[string]bool{"include_usage": true}
	}
	if r.MaxTokens > 0 {
		payload["max_tokens"] = r.MaxTokens
	}
	if r.Temperature != nil {
		payload["temperature"] = *r.Temperature
	}
	if len(r.Tools) > 0 {
		tools := make([]map[string]any, 0, len(r.Tools))
		for _, tool := range r.Tools {
			parameters := any(map[string]any{"type": "object"})
			if len(tool.Parameters) > 0 {
				if err := json.Unmarshal(tool.Parameters, &parameters); err != nil {
					return nil, fmt.Errorf("tool %q parameters: %w", tool.Name, err)
				}
			}
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": parameters}})
		}
		payload["tools"] = tools
	}
	return json.Marshal(payload)
}

func DecodeChatCompletion(data []byte) (Result, error) {
	var wire struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return Result{}, err
	}
	if len(wire.Choices) == 0 {
		return Result{}, fmt.Errorf("chat completion returned no choices")
	}
	result := Result{ID: wire.ID, Model: wire.Model, Text: wire.Choices[0].Message.Content, FinishReason: wire.Choices[0].FinishReason, Usage: Usage{InputTokens: wire.Usage.PromptTokens, OutputTokens: wire.Usage.CompletionTokens}}
	for _, call := range wire.Choices[0].Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
	}
	return result, nil
}

func DecodeChatChunk(data []byte) ([]Delta, error) {
	var wire struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Delta        struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	var deltas []Delta
	for _, choice := range wire.Choices {
		if choice.Delta.Content != "" || choice.FinishReason != "" {
			deltas = append(deltas, Delta{Text: choice.Delta.Content, FinishReason: choice.FinishReason})
		}
		for _, call := range choice.Delta.ToolCalls {
			deltas = append(deltas, Delta{ToolIndex: call.Index, ToolCall: &ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments}})
		}
	}
	if wire.Usage != nil {
		deltas = append(deltas, Delta{Usage: &Usage{InputTokens: wire.Usage.PromptTokens, OutputTokens: wire.Usage.CompletionTokens}})
	}
	return deltas, nil
}
