package app

import (
	"context"
	"encoding/json"
	"preferans/config"
	"preferans/internal/ai"
	"strings"
	"time"
)

func (a *App) AskAIReview(id, question string) (string, error) {
	if !config.AIEnabled {
		return "", aiError("build-disabled")
	}
	question = strings.TrimSpace(question)
	if question == "" || len([]rune(question)) > 2000 {
		return "", aiError("question")
	}
	a.mu.Lock()
	review := a.ai.review
	if review == nil || review.ID != id || !a.appearance.Debug || !a.ai.settings.ReviewResponses {
		a.mu.Unlock()
		return "", aiError("review-missing")
	}
	if review.QuestionBusy || len(review.Chat) >= 20 {
		a.mu.Unlock()
		return "", aiError("question-limit")
	}
	model, ok := a.modelLocked(review.Model.ID)
	if !ok {
		a.mu.Unlock()
		return "", aiError("model-missing")
	}
	key := ""
	if model.KeyID != "" {
		if a.secretCipher == nil {
			a.mu.Unlock()
			return "", aiError("key-load")
		}
		var err error
		key, err = a.secretCipher.Open(a.ai.encrypted[model.KeyID])
		if err != nil {
			a.mu.Unlock()
			return "", aiError("key-load")
		}
	}
	// Only the exact decision context is included; no current hidden state is added.
	data, _ := json.Marshal(map[string]any{"decision": review.Summary, "why": review.Why, "exchanges": review.Exchanges, "error": review.Error})
	messages := []ai.ChatMessage{{Role: "system", Content: config.AIDiscussionPrompt(review.Language)}, {Role: "user", Content: string(data)}}
	for _, message := range review.Chat {
		messages = append(messages, ai.ChatMessage{Role: message.Role, Content: message.Text})
	}
	messages = append(messages, ai.ChatMessage{Role: "user", Content: question})
	review.Chat = append(review.Chat, AIChatMessage{Role: "user", Text: question})
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(a.ai.settings.TimeoutSeconds)*time.Second)
	review.QuestionBusy = true
	review.chatCancel = cancel
	client := a.ai.client
	a.mu.Unlock()
	answer, err := client.Explain(ctx, model, key, messages)
	cancel()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ai.review != review {
		return "", aiError("review-missing")
	}
	review.QuestionBusy = false
	review.chatCancel = nil
	if err != nil {
		return "", aiError("discussion-failed")
	}
	if key != "" {
		answer = strings.ReplaceAll(answer, key, "[redacted]")
	}
	review.Chat = append(review.Chat, AIChatMessage{Role: "assistant", Text: answer})
	return answer, nil
}
