package server

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/backpack-run/backpack-runtime/internal/inference"
)

func (s *Server) executeChat(ctx context.Context, request inference.Request, stream bool) (*http.Response, error) {
	session, err := s.Sessions.Ensure(ctx, request.Model)
	if err != nil {
		return nil, err
	}
	endpoint, err := s.Sessions.Endpoint(session.ID)
	if err != nil {
		return nil, err
	}
	body, err := request.ChatCompletionsBody(stream)
	if err != nil {
		return nil, err
	}
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	upstream.Header.Set("Content-Type", "application/json")
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	response, err := client.Do(upstream)
	if err != nil {
		return nil, fmt.Errorf("runtime inference request failed: %w", err)
	}
	if response.StatusCode/100 != 2 {
		defer response.Body.Close()
		message, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return nil, fmt.Errorf("runtime inference returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	return response, nil
}

func readChatStream(response *http.Response, consume func(inference.Delta) error) error {
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if data == "[DONE]" {
					return nil
				}
				if data != "" {
					deltas, decodeErr := inference.DecodeChatChunk([]byte(data))
					if decodeErr != nil {
						return fmt.Errorf("decode runtime stream: %w", decodeErr)
					}
					for _, delta := range deltas {
						if consumeErr := consume(delta); consumeErr != nil {
							return consumeErr
						}
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
