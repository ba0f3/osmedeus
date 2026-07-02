package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/j3ssie/osmedeus/v5/internal/config"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (s *Service) chatCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if err := s.requireConfig(); err != nil {
		return "", err
	}
	if s.cfg.LLM.GetProviderCount() == 0 {
		return "", fmt.Errorf("no LLM providers configured")
	}

	provider := s.cfg.LLM.GetCurrentProvider()
	if provider == nil {
		return "", fmt.Errorf("no LLM providers available")
	}

	model := provider.Model
	maxTokens := s.cfg.LLM.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	temperature := s.cfg.LLM.Temperature
	if temperature <= 0 {
		temperature = 0.2
	}

	reqBody := chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	timeout := 120 * time.Second
	if s.cfg.LLM.Timeout != "" {
		if parsed, parseErr := time.ParseDuration(s.cfg.LLM.Timeout); parseErr == nil {
			timeout = parsed
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.BaseURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if provider.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+provider.AuthToken)
	}
	for key, value := range parseCustomHeaders(s.cfg.LLM.CustomHeaders) {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var response chatCompletionResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", fmt.Errorf("failed to parse LLM response: %w", err)
	}
	if resp.StatusCode >= 400 {
		if response.Error != nil {
			return "", fmt.Errorf("LLM API error: %s", response.Error.Message)
		}
		return "", fmt.Errorf("LLM API error: HTTP %d", resp.StatusCode)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}
	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("LLM returned empty content")
	}
	return content, nil
}

func parseCustomHeaders(raw string) map[string]string {
	headers := make(map[string]string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return headers
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return headers
}

func workflowGenerationSystemPrompt() string {
	return `You are an Osmedeus workflow generator. Output valid Osmedeus workflow YAML only.
Rules:
- Use kind: module or kind: flow
- Include name, description, and steps
- Use {{Target}} and {{Output}} template variables where appropriate
- Prefer existing module patterns (bash, function, parallel-steps, foreach)
- Do not wrap output in markdown fences
- Do not include commentary outside YAML`
}

func extractYAMLFromLLMOutput(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 2 {
			end := len(lines)
			for i := len(lines) - 1; i >= 1; i-- {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
					end = i
					break
				}
			}
			content = strings.Join(lines[1:end], "\n")
		}
	}
	return strings.TrimSpace(content)
}

func suggestedWorkflowName(purpose, targetType string) string {
	purpose = sanitizeSlug(purpose, "scan")
	targetType = sanitizeSlug(targetType, "target")
	return fmt.Sprintf("ai-%s-%s", purpose, targetType)
}

func sanitizeSlug(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ':
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}

// Ensure config import is used when building without LLM paths.
var _ = (*config.Config)(nil)
