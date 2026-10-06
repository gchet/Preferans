// Package ai selects engine-provided actions through a chat-completions API.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"preferans/config"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Model struct {
	ID          string  `json:"id"`
	Alias       string  `json:"alias"`
	Provider    string  `json:"provider"`
	URL         string  `json:"url"`
	Model       string  `json:"model"`
	KeyID       string  `json:"keyID"`
	MaxTokens   int     `json:"maxTokens"`
	Temperature float64 `json:"temperature"`
}

var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (m Model) Validate() error {
	if !safeID.MatchString(m.ID) || len(m.ID) > 128 || strings.TrimSpace(m.Alias) == "" || len([]rune(m.Alias)) > 40 || strings.TrimSpace(m.Model) == "" || len(m.Model) > 200 {
		return errors.New("invalid-model")
	}
	u, err := url.Parse(m.URL)
	if err != nil || u.Hostname() == "" || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid-url")
	}
	if m.MaxTokens < 1 || m.MaxTokens > 131072 || math.IsNaN(m.Temperature) || math.IsInf(m.Temperature, 0) || m.Temperature < 0 || m.Temperature > 2 {
		return errors.New("invalid-parameters")
	}
	return nil
}

type Choice struct {
	Move int    `json:"move"`
	Why  string `json:"why,omitempty"`
}
type Result struct {
	Choice
	PromptTokens, CompletionTokens int
	Attempts                       int
	Exchanges                      []Exchange `json:"exchanges,omitempty"`
}
type Exchange struct {
	Phase    string              `json:"phase"`
	SystemRU string              `json:"systemRU"`
	Messages []map[string]string `json:"messages"`
	Response string              `json:"response"`
	Error    string              `json:"error,omitempty"`
}

type Client struct {
	HTTP           *http.Client
	Trace          bool
	PromptLanguage string
}

// Select shares one caller-supplied deadline across HTTP retries and repair.
func (c Client) Select(ctx context.Context, m Model, key, system string, situation any, count int) (Result, error) {
	var result Result
	b, err := json.Marshal(situation)
	if err != nil {
		return result, err
	}
	messages := []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": string(b)}}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	repaired := false
	for result.Attempts < 3 {
		result.Attempts++
		payload := map[string]any{"model": m.Model, "messages": messages, "max_tokens": m.MaxTokens, "temperature": m.Temperature, "response_format": map[string]string{"type": "json_object"}}
		if c.Trace {
			result.Exchanges = append(result.Exchanges, Exchange{Messages: append([]map[string]string{}, messages...)})
		}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, "POST", m.URL, bytes.NewReader(body))
		if err != nil {
			return result, errors.New("request-failed")
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			return result, errors.New("network-error")
		}
		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			if c.Trace {
				result.Exchanges[len(result.Exchanges)-1].Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
			delay := time.Second
			if seconds, e := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64); e == nil && seconds > 0 {
				delay = time.Duration(seconds * float64(time.Second))
			} else if date, e := http.ParseTime(resp.Header.Get("Retry-After")); e == nil {
				delay = max(time.Until(date), time.Millisecond)
			}
			resp.Body.Close()
			if result.Attempts >= 3 {
				return result, fmt.Errorf("HTTP %d", resp.StatusCode)
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return result, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if resp.StatusCode != 200 {
			if c.Trace {
				result.Exchanges[len(result.Exchanges)-1].Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
			resp.Body.Close()
			return result, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		var wire struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Finish string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				Prompt     int `json:"prompt_tokens"`
				Completion int `json:"completion_tokens"`
			} `json:"usage"`
		}
		data, e := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
		resp.Body.Close()
		if e != nil || len(data) > 256*1024 {
			return result, errors.New("invalid-response")
		}
		valid := json.Unmarshal(data, &wire) == nil && len(wire.Choices) == 1
		if c.Trace {
			result.Exchanges[len(result.Exchanges)-1].Response = string(data)
		}
		result.PromptTokens += wire.Usage.Prompt
		result.CompletionTokens += wire.Usage.Completion
		if valid {
			var answer struct {
				Move *int   `json:"move"`
				Why  string `json:"why"`
			}
			decoder := json.NewDecoder(strings.NewReader(wire.Choices[0].Message.Content))
			decoder.DisallowUnknownFields()
			valid = wire.Choices[0].Finish == "stop" && decoder.Decode(&answer) == nil && decoder.Decode(new(any)) == io.EOF && answer.Move != nil && *answer.Move >= 0 && *answer.Move < count && len([]rune(answer.Why)) <= 240
			if valid {
				result.Choice = Choice{Move: *answer.Move, Why: answer.Why}
				return result, nil
			}
		}
		if repaired {
			return result, errors.New("invalid-answer")
		}
		repaired = true
		// Do not echo the provider's malformed response, which can be unbounded.
		messages = append(messages, map[string]string{"role": "user", "content": config.AIRepairPrompt(c.PromptLanguage, count-1)})
	}
	return result, errors.New("retry-limit")
}
