package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Explain uses a separate conversation and cannot apply or change game actions.
func (c Client) Explain(ctx context.Context, m Model, key string, messages []ChatMessage) (string, error) {
	payload := map[string]any{"model": m.Model, "messages": messages, "max_tokens": m.MaxTokens, "temperature": m.Temperature}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.URL, bytes.NewReader(body))
	if err != nil {
		return "", errors.New("request-failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client := c.HTTP
	if client == nil {
		client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("network-error")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	if err != nil || len(data) > 256*1024 {
		return "", errors.New("invalid-response")
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
	}
	if json.Unmarshal(data, &result) != nil || len(result.Choices) != 1 || result.Choices[0].Finish != "stop" {
		return "", errors.New("invalid-answer")
	}
	answer := strings.TrimSpace(result.Choices[0].Message.Content)
	if answer == "" || len([]rune(answer)) > 16000 {
		return "", errors.New("invalid-answer")
	}
	return answer, nil
}
