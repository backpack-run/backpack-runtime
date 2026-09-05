package pythonruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type WorkerClient struct {
	BaseURL string
	HTTP    *http.Client
}
type Health struct {
	ProtocolVersion int      `json:"protocol_version"`
	Engine          string   `json:"engine"`
	Status          string   `json:"status"`
	Capabilities    []string `json:"capabilities"`
	ModelLoaded     bool     `json:"model_loaded"`
	Error           string   `json:"error,omitempty"`
}

func NewWorkerClient(endpoint string) *WorkerClient {
	return &WorkerClient{BaseURL: endpoint, HTTP: &http.Client{Timeout: 0}}
}
func (c *WorkerClient) WaitReady(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var health Health
		if err := c.do(ctx, http.MethodGet, "/v1/health", nil, &health); err == nil {
			if health.ProtocolVersion != protocolVersion {
				return fmt.Errorf("Python worker protocol %d is incompatible with runtime protocol %d", health.ProtocolVersion, protocolVersion)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Python worker readiness: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
func (c *WorkerClient) Load(ctx context.Context, modelDirectory string) error {
	return c.do(ctx, http.MethodPost, "/v1/load", map[string]string{"model_directory": modelDirectory}, nil)
}
func (c *WorkerClient) Infer(ctx context.Context, input, output any) error {
	return c.do(ctx, http.MethodPost, "/v1/infer", input, output)
}
func (c *WorkerClient) Unload(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/v1/unload", map[string]any{}, nil)
}
func (c *WorkerClient) Shutdown(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/v1/shutdown", map[string]any{}, nil)
}
func (c *WorkerClient) do(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
		var envelope struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &envelope) == nil && envelope.Error != "" {
			return fmt.Errorf("Python worker: %s", envelope.Error)
		}
		return fmt.Errorf("Python worker returned %s", res.Status)
	}
	if output != nil {
		if err = json.NewDecoder(io.LimitReader(res.Body, 512<<20)).Decode(output); err != nil {
			return fmt.Errorf("decode Python worker response: %w", err)
		}
	}
	return nil
}
