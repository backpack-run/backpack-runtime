package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Session struct {
	ID        string    `json:"id"`
	ModelID   string    `json:"model_id"`
	Runtime   string    `json:"runtime"`
	Compute   string    `json:"compute"`
	Endpoint  string    `json:"endpoint,omitempty"`
	Status    string    `json:"status"`
	PID       int       `json:"pid,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	LastError string    `json:"last_error,omitempty"`
}
type SessionOptions struct {
	ContextLength int `json:"context_length,omitempty"`
	GPULayers     any `json:"gpu_layers,omitempty"`
}
type CreateSessionRequest struct {
	Model   string         `json:"model"`
	Compute string         `json:"compute,omitempty"`
	Options SessionOptions `json:"options,omitempty"`
}

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(base string) *Client {
	return &Client{BaseURL: strings.TrimRight(base, "/"), HTTP: &http.Client{Timeout: 0}}
}
func (c *Client) Health(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/backpack/v1/health", nil)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return responseError(res)
	}
	return nil
}
func (c *Client) Sessions(ctx context.Context) ([]Session, error) {
	var out struct {
		Data []Session `json:"data"`
	}
	err := c.json(ctx, http.MethodGet, "/api/backpack/v1/sessions", nil, &out)
	return out.Data, err
}
func (c *Client) CreateSession(ctx context.Context, request CreateSessionRequest) (*Session, error) {
	var out Session
	err := c.json(ctx, http.MethodPost, "/api/backpack/v1/sessions", request, &out)
	return &out, err
}
func (c *Client) GetSession(ctx context.Context, id string) (*Session, error) {
	var out Session
	err := c.json(ctx, http.MethodGet, "/api/backpack/v1/sessions/"+id, nil, &out)
	return &out, err
}
func (c *Client) StopSession(ctx context.Context, id string) error {
	return c.json(ctx, http.MethodDelete, "/api/backpack/v1/sessions/"+id, nil, nil)
}
func (c *Client) Chat(ctx context.Context, model, prompt string, stream bool, onData func(string)) error {
	payload := map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": prompt}}, "stream": stream}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return responseError(res)
	}
	if !stream {
		var out struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err = json.NewDecoder(res.Body).Decode(&out); err != nil {
			return err
		}
		if len(out.Choices) == 0 {
			return fmt.Errorf("inference returned no choices")
		}
		onData(out.Choices[0].Message.Content)
		return nil
	}
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) == nil && len(chunk.Choices) > 0 {
			onData(chunk.Choices[0].Delta.Content)
		}
	}
	return scanner.Err()
}
func (c *Client) json(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, _ := json.Marshal(input)
		body = bytes.NewReader(data)
	}
	req, _ := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return responseError(res)
	}
	if output != nil && res.StatusCode != 204 {
		return json.NewDecoder(res.Body).Decode(output)
	}
	return nil
}
func responseError(res *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) == nil && envelope.Error.Message != "" {
		return fmt.Errorf("runtime API: %s", envelope.Error.Message)
	}
	return fmt.Errorf("runtime API returned %s", res.Status)
}
func (c *Client) WaitHealthy(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		check, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		err := c.Health(check)
		cancel()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
