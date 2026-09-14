package cloud

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/config"
)

const DefaultBaseURL = "https://api.backpack.run"

var allowedInferencePaths = map[string]bool{
	"/v1/chat/completions": true,
	"/v1/responses":        true,
	"/v1/messages":         true,
}

type Model struct {
	ID            string         `json:"id"`
	Object        string         `json:"object"`
	OwnedBy       string         `json:"owned_by"`
	DisplayName   string         `json:"display_name"`
	Capabilities  []string       `json:"capabilities"`
	ContextWindow int            `json:"context_window"`
	Pricing       map[string]any `json:"pricing,omitempty"`
	Qualification map[string]any `json:"qualification,omitempty"`
	Status        string         `json:"status"`
}

func (m Model) HasCapability(want string) bool {
	for _, capability := range m.Capabilities {
		if strings.EqualFold(strings.TrimSpace(capability), want) {
			return true
		}
	}
	return false
}

type Client struct {
	BaseURL string
	HTTP    *http.Client
	Store   *CredentialStore

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

func New(paths config.Paths) (*Client, error) {
	baseURL := DefaultBaseURL
	if override := strings.TrimSpace(os.Getenv("BACKPACK_CLOUD_URL")); override != "" {
		baseURL = override
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid BACKPACK_CLOUD_URL")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return nil, fmt.Errorf("Backpack Cloud URL must use HTTPS (HTTP is allowed only for loopback testing)")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Scale-to-zero GPU workers can require several minutes to load model
	// weights. Keep a finite bound, but do not fail before a qualified cold
	// start has had time to return its first response headers.
	transport.ResponseHeaderTimeout = 5 * time.Minute
	transport.TLSHandshakeTimeout = 15 * time.Second
	return &Client{
		BaseURL: strings.TrimRight(parsed.String(), "/"),
		HTTP: &http.Client{
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return fmt.Errorf("Backpack Cloud redirects are not accepted")
			},
		},
		Store: NewCredentialStore(paths),
	}, nil
}

func IsModel(name string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(name)), ":cloud")
}

func isLoopbackHost(host string) bool {
	return strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1"
}

type LoginPrompt struct {
	UserCode        string
	VerificationURL string
	ExpiresIn       int
	PollInterval    int
}

type deviceStartResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	PollInterval    int    `json:"poll_interval"`
}

type deviceStatusResponse struct {
	Status       string `json:"status"`
	DeviceKeyID  string `json:"device_key_id"`
	PollInterval int    `json:"poll_interval"`
}

func (c *Client) Login(ctx context.Context, name string, prompt func(LoginPrompt) error) error {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate device key: %w", err)
	}
	defer zero(privateKey)
	if strings.TrimSpace(name) == "" {
		name = "Backpack CLI"
	}
	if len([]rune(name)) > 80 {
		return fmt.Errorf("device name must not exceed 80 characters")
	}
	var started deviceStartResponse
	err = c.postJSON(ctx, "/api/auth/device/start", map[string]any{
		"public_key": base64.RawURLEncoding.EncodeToString(publicKey),
		"name":       name,
		"platform":   map[string]string{"os": runtime.GOOS, "arch": runtime.GOARCH},
	}, &started)
	if err != nil {
		return err
	}
	if started.DeviceCode == "" || started.UserCode == "" || started.ExpiresIn <= 0 || started.PollInterval <= 0 {
		return fmt.Errorf("Backpack Cloud returned an invalid device authorization")
	}
	verificationURL, err := verificationURL(started.VerificationURI, started.UserCode)
	if err != nil {
		return err
	}
	if prompt != nil {
		if err = prompt(LoginPrompt{UserCode: started.UserCode, VerificationURL: verificationURL, ExpiresIn: started.ExpiresIn, PollInterval: started.PollInterval}); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(time.Duration(started.ExpiresIn) * time.Second)
	interval := time.Duration(started.PollInterval) * time.Second
	for {
		if !time.Now().Before(deadline) {
			return fmt.Errorf("Backpack Cloud device authorization expired; run `backpack login` again")
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		var status deviceStatusResponse
		if err = c.postJSON(ctx, "/api/auth/device/status", map[string]string{"device_code": started.DeviceCode}, &status); err != nil {
			return err
		}
		switch status.Status {
		case "pending":
			if status.PollInterval > 0 {
				interval = time.Duration(status.PollInterval) * time.Second
			}
		case "approved":
			if status.DeviceKeyID == "" {
				return fmt.Errorf("Backpack Cloud approval omitted the device key ID")
			}
			if err = c.Store.Save(Credentials{DeviceKeyID: status.DeviceKeyID, PrivateKey: privateKey}); err != nil {
				return fmt.Errorf("save Backpack Cloud device credential: %w", err)
			}
			c.clearToken()
			return nil
		case "denied":
			return fmt.Errorf("Backpack Cloud device authorization was denied")
		case "expired":
			return fmt.Errorf("Backpack Cloud device authorization expired; run `backpack login` again")
		default:
			return fmt.Errorf("Backpack Cloud returned unknown device status %q", status.Status)
		}
	}
}

func verificationURL(raw, code string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.Scheme != "https" {
		return "", fmt.Errorf("Backpack Cloud returned an unsafe verification URL")
	}
	query := parsed.Query()
	query.Set("code", code)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (c *Client) AuthStatus() (source string, deviceKeyID string, err error) {
	if strings.TrimSpace(os.Getenv("BACKPACK_API_KEY")) != "" {
		return "api-key-environment", "", nil
	}
	credentials, err := c.Store.Load()
	if errors.Is(err, os.ErrNotExist) {
		return "none", "", nil
	}
	if err != nil {
		return "invalid", "", err
	}
	zero(credentials.PrivateKey)
	return credentials.Storage, credentials.DeviceKeyID, nil
}

func (c *Client) Logout() error {
	c.clearToken()
	return c.Store.Delete()
}

func (c *Client) Models(ctx context.Context) ([]Model, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	response, err := c.doAuthenticated(ctx, request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return nil, responseError(response)
	}
	var result struct {
		Data []Model `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Backpack Cloud models: %w", err)
	}
	return result.Data, nil
}

func (c *Client) ResolveModel(ctx context.Context, name string) (Model, error) {
	models, err := c.Models(ctx)
	if err != nil {
		return Model{}, err
	}
	for _, model := range models {
		if strings.EqualFold(model.ID, strings.TrimSpace(name)) {
			if model.Status != "available" {
				return Model{}, fmt.Errorf("Backpack Cloud model %q is %s", model.ID, model.Status)
			}
			return model, nil
		}
	}
	available := make([]string, 0, len(models))
	for _, model := range models {
		if model.Status == "available" {
			available = append(available, model.ID)
		}
	}
	return Model{}, fmt.Errorf("cloud model %q is not enabled; available: %s", name, strings.Join(available, ", "))
}

func (c *Client) Inference(ctx context.Context, path string, body []byte, headers http.Header) (*http.Response, error) {
	if !allowedInferencePaths[path] {
		return nil, fmt.Errorf("unsupported Backpack Cloud inference path %q", path)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	for _, name := range []string{"Accept", "Anthropic-Version", "Anthropic-Beta", "X-Request-ID", "User-Agent"} {
		if value := headers.Get(name); value != "" && !strings.ContainsAny(value, "\r\n") {
			request.Header.Set(name, value)
		}
	}
	return c.doAuthenticated(ctx, request)
}

func (c *Client) doAuthenticated(ctx context.Context, request *http.Request) (*http.Response, error) {
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	return c.HTTP.Do(request)
}

func (c *Client) token(ctx context.Context) (string, error) {
	if apiKey := strings.TrimSpace(os.Getenv("BACKPACK_API_KEY")); apiKey != "" {
		if strings.ContainsAny(apiKey, "\r\n\x00") {
			return "", fmt.Errorf("BACKPACK_API_KEY contains invalid characters")
		}
		return apiKey, nil
	}
	c.mu.Lock()
	if c.accessToken != "" && time.Now().Add(30*time.Second).Before(c.expiresAt) {
		token := c.accessToken
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()
	credentials, err := c.Store.Load()
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("Backpack Cloud is not authenticated; run `backpack login` or set BACKPACK_API_KEY for non-interactive use")
	}
	if err != nil {
		return "", fmt.Errorf("load Backpack Cloud credential: %w", err)
	}
	defer zero(credentials.PrivateKey)
	var challenge struct {
		Challenge      string `json:"challenge"`
		SignatureInput string `json:"signature_input"`
		ExpiresIn      int    `json:"expires_in"`
	}
	if err = c.postJSON(ctx, "/api/auth/device/challenge", map[string]string{"device_key_id": credentials.DeviceKeyID}, &challenge); err != nil {
		return "", err
	}
	if challenge.Challenge == "" || challenge.SignatureInput == "" {
		return "", fmt.Errorf("Backpack Cloud returned an invalid device challenge")
	}
	signature := ed25519.Sign(credentials.PrivateKey, []byte(challenge.SignatureInput))
	var access struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	err = c.postJSON(ctx, "/api/auth/device/token", map[string]string{
		"device_key_id": credentials.DeviceKeyID,
		"challenge":     challenge.Challenge,
		"signature":     base64.RawURLEncoding.EncodeToString(signature),
	}, &access)
	zero(signature)
	if err != nil {
		return "", err
	}
	if access.AccessToken == "" || access.TokenType != "Bearer" || access.ExpiresIn <= 0 {
		return "", fmt.Errorf("Backpack Cloud returned an invalid access token")
	}
	c.mu.Lock()
	c.accessToken = access.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(access.ExpiresIn) * time.Second)
	c.mu.Unlock()
	return access.AccessToken, nil
}

func (c *Client) postJSON(ctx context.Context, path string, input, output any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("Backpack Cloud request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return responseError(response)
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output); err != nil {
		return fmt.Errorf("decode Backpack Cloud response: %w", err)
	}
	return nil
}

type APIError struct {
	StatusCode int
	Type       string
	Message    string
	RequestID  string
}

func (e *APIError) Error() string {
	message := fmt.Sprintf("Backpack Cloud returned HTTP %d", e.StatusCode)
	if e.Type != "" {
		message += " (" + e.Type + ")"
	}
	if e.Message != "" {
		message += ": " + e.Message
	}
	if e.RequestID != "" {
		message += " [request " + e.RequestID + "]"
	}
	return message
}

func responseError(response *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &envelope)
	return &APIError{StatusCode: response.StatusCode, Type: safeText(envelope.Error.Type, 80), Message: safeText(envelope.Error.Message, 500), RequestID: safeText(response.Header.Get("X-Request-ID"), 100)}
}

func safeText(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}

func (c *Client) clearToken() {
	c.mu.Lock()
	c.accessToken = ""
	c.expiresAt = time.Time{}
	c.mu.Unlock()
}

func zero(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
