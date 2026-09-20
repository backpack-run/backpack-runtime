package server

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/backpack-run/backpack-runtime/internal/inference"
)

const claudeAppRouteModel = "claude-sonnet-5"

func (s *Server) claudeAppAuthorized(r *http.Request) bool {
	provided := r.PathValue("token")
	return s.CloudProxyToken != "" && len(provided) == len(s.CloudProxyToken) && subtle.ConstantTimeCompare([]byte(provided), []byte(s.CloudProxyToken)) == 1
}

func (s *Server) claudeAppRoute(r *http.Request) (string, int, error) {
	if !s.claudeAppAuthorized(r) {
		return "", 0, fmt.Errorf("valid Claude App loopback authorization is required")
	}
	model := strings.TrimSpace(r.PathValue("model"))
	if model == "" || !s.isBackpackModel(model) {
		return "", 0, fmt.Errorf("unknown Backpack model")
	}
	contextTokens, err := strconv.Atoi(r.PathValue("context"))
	if err != nil || contextTokens < 1024 || contextTokens > 2_000_000 {
		return "", 0, fmt.Errorf("invalid Claude App context window")
	}
	return model, contextTokens, nil
}

func (s *Server) claudeAppModels(w http.ResponseWriter, r *http.Request) {
	model, contextTokens, err := s.claudeAppRoute(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	write(w, http.StatusOK, map[string]any{
		"data": []any{map[string]any{
			"id": claudeAppRouteModel, "type": "model", "display_name": model,
			"created_at": "2026-01-01T00:00:00Z", "max_tokens": inference.AgentOutputTokenBudget(contextTokens),
			"anthropic_family_tier": "sonnet", "is_family_default": true,
		}},
		"first_id": claudeAppRouteModel, "last_id": claudeAppRouteModel, "has_more": false,
	})
}

func (s *Server) claudeAppCountTokens(w http.ResponseWriter, r *http.Request) {
	_, _, err := s.claudeAppRoute(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var payload any
	if err = json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid token-count request"))
		return
	}
	// A conservative tokenizer-independent estimate. The actual runtime remains
	// authoritative; this endpoint exists for Claude App's compaction planning.
	encoded, _ := json.Marshal(payload)
	inputTokens := (len(encoded) + 2) / 3
	if inputTokens < 1 {
		inputTokens = 1
	}
	write(w, http.StatusOK, map[string]int{"input_tokens": inputTokens})
}

func (s *Server) claudeAppMessages(w http.ResponseWriter, r *http.Request) {
	model, _, err := s.claudeAppRoute(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var payload map[string]json.RawMessage
	if err = json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid Claude App request"))
		return
	}
	modelJSON, _ := json.Marshal(model)
	payload["model"] = modelJSON
	rewritten, _ := json.Marshal(payload)
	cloned := r.Clone(r.Context())
	cloned.Header = r.Header.Clone()
	cloned.Header.Del("Cookie")
	cloned.Header.Del("Proxy-Authorization")
	cloned.Header.Set("Authorization", "Bearer "+s.CloudProxyToken)
	cloned.Body = io.NopCloser(bytes.NewReader(rewritten))
	cloned.ContentLength = int64(len(rewritten))
	s.anthropicMessages(w, cloned)
}
