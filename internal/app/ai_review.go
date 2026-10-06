package app

import (
	"encoding/json"
	"preferans/config"
	"preferans/internal/ai"
	"preferans/internal/game"
	"preferans/locales"
	"strings"
	"time"
)

type AIReview struct {
	Chat             []AIChatMessage `json:"chat"`
	QuestionBusy     bool            `json:"questionBusy"`
	ID               string          `json:"id"`
	Seat             int             `json:"seat"`
	Bot              string          `json:"bot"`
	Model            ai.Model        `json:"model"`
	Phase            string          `json:"phase"`
	Language         string          `json:"language"`
	Summary          string          `json:"summary"`
	Why              string          `json:"why"`
	Error            string          `json:"error,omitempty"`
	ElapsedMs        int64           `json:"elapsedMs"`
	PromptTokens     int             `json:"promptTokens"`
	CompletionTokens int             `json:"completionTokens"`
	Exchanges        []ai.Exchange   `json:"exchanges"`
	table            string
	revision         uint64
	command          game.Command
	chatCancel       func()
}

type AIChatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

func commandSummary(c game.Command) string {
	var cards []string
	for _, card := range c.Cards {
		cards = append(cards, card.String())
	}
	details := strings.Join(cards, ", ")
	if c.Contract != nil {
		if details != "" {
			details += "; "
		}
		details += c.Contract.String()
	}
	if c.Action == "claim" {
		return locales.Format("go.ai.review.claim", c.Tricks)
	}
	key := "go.ai.action." + c.Action
	if details == "" {
		return locales.Text(key)
	}
	return locales.Format("go.ai.review.action", locales.Text(key), details)
}

func (a *App) holdAIReviewLocked(s *game.State, r *aiRequest, m ai.Model, key string, result ai.Result, command game.Command, err error, elapsed time.Duration) {
	m.KeyID = ""
	review := &AIReview{ID: game.ID(), Seat: r.seat, Bot: s.Players[r.seat].Name, Model: m, Phase: s.Stage, Language: r.language, Summary: commandSummary(command), Why: result.Why, ElapsedMs: elapsed.Milliseconds(), PromptTokens: result.PromptTokens, CompletionTokens: result.CompletionTokens, Exchanges: result.Exchanges}
	if err != nil {
		review.Error = err.Error()
	}
	// Trace bodies and model explanations are untrusted; redact the actual key.
	if key != "" {
		data, _ := json.Marshal(review)
		data = []byte(strings.ReplaceAll(string(data), key, "[redacted]"))
		_ = json.Unmarshal(data, review)
	}
	review.table = s.ID
	review.revision = s.Revision
	review.command = command
	a.ai.review = review
	a.broadcastLocked()
}

func (a *App) ContinueAIReview(id string) error {
	if !config.AIEnabled {
		return aiError("build-disabled")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	review := a.ai.review
	s := a.saved.State
	if review == nil || review.ID != id || s == nil || s.ID != review.table {
		return aiError("review-missing")
	}
	if s.Revision != review.revision || s.Actor() != review.Seat {
		if review.chatCancel != nil {
			review.chatCancel()
		}
		a.ai.review = nil
		a.broadcastLocked()
		return aiError("review-stale")
	}
	if s.ManualPaused {
		return aiError("review-paused")
	}
	for _, connected := range a.connected {
		if !connected {
			return aiError("review-paused")
		}
	}
	a.ai.review = nil
	if review.chatCancel != nil {
		review.chatCancel()
	}
	if err := a.commandLocked(review.command); err != nil {
		a.ai.review = review
		a.broadcastLocked()
		return err
	}
	return nil
}

func (a *App) aiReviewSnapshotLocked() *AIReview {
	if a.ai.review == nil {
		return nil
	}
	data, _ := json.Marshal(a.ai.review)
	var review AIReview
	_ = json.Unmarshal(data, &review)
	return &review
}
