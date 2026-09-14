package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

const maxCodexAppBodyBytes = int64(8 << 20)

const (
	defaultCodexChatGPTURL = "https://chatgpt.com/backend-api/codex/responses"
	defaultCodexOpenAIURL  = "https://api.openai.com/v1/responses"
)

var errNativeCodexAuth = errors.New("native OpenAI models require Codex authentication")

// decodeCodexAppRequest handles the compressed request transport used by the
// desktop app. Decoding stays at this integration boundary so normal API
// clients and RuntimeAdapters continue to exchange ordinary JSON.
func decodeCodexAppRequest(request *http.Request) error {
	encoding := strings.ToLower(strings.TrimSpace(request.Header.Get("Content-Encoding")))
	if encoding == "" || encoding == "identity" {
		return nil
	}
	if encoding != "zstd" {
		return fmt.Errorf("unsupported Codex App content encoding %q", encoding)
	}

	compressed, err := io.ReadAll(io.LimitReader(request.Body, maxCodexAppBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read compressed Codex App request: %w", err)
	}
	if int64(len(compressed)) > maxCodexAppBodyBytes {
		return fmt.Errorf("compressed Codex App request exceeds %d bytes", maxCodexAppBodyBytes)
	}
	decoder, err := zstd.NewReader(bytes.NewReader(compressed), zstd.WithDecoderMaxMemory(uint64(maxCodexAppBodyBytes)))
	if err != nil {
		return fmt.Errorf("decode Codex App zstd request: %w", err)
	}
	decompressed, err := io.ReadAll(io.LimitReader(decoder, maxCodexAppBodyBytes+1))
	decoder.Close()
	if err != nil {
		return fmt.Errorf("decompress Codex App request: %w", err)
	}
	if int64(len(decompressed)) > maxCodexAppBodyBytes {
		return fmt.Errorf("decompressed Codex App request exceeds %d bytes", maxCodexAppBodyBytes)
	}

	request.Body = io.NopCloser(bytes.NewReader(decompressed))
	request.ContentLength = int64(len(decompressed))
	request.Header.Del("Content-Encoding")
	request.Header.Set("Content-Type", "application/json")
	return nil
}

// proxyNativeCodexResponse preserves Codex's normal OpenAI/ChatGPT path for
// models that are not in Backpack's trusted catalog. Native credentials are
// forwarded only to fixed OpenAI HTTPS origins and never to Backpack Cloud.
func (s *Server) proxyNativeCodexResponse(w http.ResponseWriter, source *http.Request, body []byte) error {
	authorization := strings.TrimSpace(source.Header.Get("Authorization"))
	if authorization == "" || authorization == "Bearer "+s.CloudProxyToken {
		return fmt.Errorf("%w: sign in to Codex or configure an OpenAI API key", errNativeCodexAuth)
	}
	target := s.codexOpenAIURL
	if strings.TrimSpace(source.Header.Get("ChatGPT-Account-ID")) != "" {
		target = s.codexChatGPTURL
	}
	if target == "" {
		if strings.TrimSpace(source.Header.Get("ChatGPT-Account-ID")) != "" {
			target = defaultCodexChatGPTURL
		} else {
			target = defaultCodexOpenAIURL
		}
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && !isLoopbackURL(parsed)) {
		return fmt.Errorf("refusing unsafe native Codex upstream")
	}
	request, err := http.NewRequestWithContext(source.Context(), http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header = source.Header.Clone()
	for _, name := range []string{"Connection", "Content-Encoding", "Content-Length", "Host", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		request.Header.Del(name)
	}
	request.Header.Set("Content-Type", "application/json")

	client := s.Client
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.ResponseHeaderTimeout = 5 * time.Minute
		client = &http.Client{
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("native Codex upstream redirects are not accepted")
			},
		}
	}
	response, err := client.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return fmt.Errorf("native Codex upstream: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 == 3 {
		return fmt.Errorf("native Codex upstream redirects are not accepted")
	}
	for name, values := range response.Header {
		if strings.EqualFold(name, "Set-Cookie") || strings.EqualFold(name, "Connection") || strings.EqualFold(name, "Transfer-Encoding") {
			continue
		}
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	if strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		flusher, ok := w.(http.Flusher)
		if !ok {
			return nil
		}
		buffer := make([]byte, 32<<10)
		for {
			n, readErr := response.Body.Read(buffer)
			if n > 0 {
				if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
					return nil
				}
				flusher.Flush()
			}
			if readErr != nil {
				return nil
			}
		}
	}
	_, _ = io.Copy(w, response.Body)
	return nil
}

func isLoopbackURL(value *url.URL) bool {
	host := strings.TrimSpace(value.Hostname())
	return value.Scheme == "http" && (strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1")
}
