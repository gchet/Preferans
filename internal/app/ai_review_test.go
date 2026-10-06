package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"preferans/internal/ai"
	"preferans/internal/game"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAIReviewHoldsActionAndDiscussionCannotPlay(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "fallback"}[invalid], func(t *testing.T) {
			a := New(t.TempDir())
			defer a.Close()
			var chatCalls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []ai.ChatMessage  `json:"messages"`
					Format   map[string]string `json:"response_format"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Error("bad payload")
					return
				}
				content := `{"move":0,"why":"Сохраняю старшие карты"}`
				if body.Format == nil {
					chatCalls.Add(1)
					last := body.Messages[len(body.Messages)-1].Content
					if !strings.Contains(last, "Почему") {
						t.Error("question missing")
					}
					if !strings.Contains(body.Messages[1].Content, "legal_actions") {
						t.Error("original situation missing from discussion")
					}
					if len(body.Messages) < 5 && chatCalls.Load() == 2 {
						t.Error("previous discussion missing")
					}
					content = "Этот вариант сохраняет старшие карты. Альтернатива рискованнее."
				} else {
					if !strings.Contains(body.Messages[0].Content, "Ты играешь") {
						t.Error("Russian prompt was not sent")
					}
					if invalid {
						content = `{"move":999999}`
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
			}))
			defer server.Close()
			p := a.Status().Appearance
			p.Debug = true
			if err := a.SetAppearance(p); err != nil {
				t.Fatal(err)
			}
			m := ai.Model{ID: "test", Alias: "test", URL: server.URL, Model: "test", MaxTokens: 500}
			if err := a.SaveAISettings(AISettings{Models: []ai.Model{m}, TimeoutSeconds: 1, ReviewResponses: true, PromptLanguage: "ru"}); err != nil {
				t.Fatal(err)
			}
			if err := a.Create(3, 30, "Host", true); err != nil {
				t.Fatal(err)
			}
			if err := a.AssignBot(1, "test"); err != nil {
				t.Fatal(err)
			}
			a.mu.Lock()
			s := a.saved.State
			s.Dealer = 0
			for i := range s.Players {
				s.Players[i].Ready = true
			}
			a.ai.client = ai.Client{HTTP: server.Client()}
			err := a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: s.Revision, Action: "start"})
			a.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			v := a.Status().View
			if err = a.DebugBotContinue(1, v.Revision); err != nil {
				t.Fatal(err)
			}
			var review *AIReview
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				review = a.Status().AIReview
				if review != nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if review == nil {
				t.Fatal("review missing")
			}
			revision := a.Status().View.Revision
			if !a.Status().Paused || review.Summary == "" || len(review.Exchanges) == 0 {
				t.Fatal("missing hold or traces")
			}
			time.Sleep(1100 * time.Millisecond)
			if a.Status().View.Revision != revision {
				t.Fatal("action executed without confirmation")
			}
			a.mu.Lock()
			peerView := a.viewWithAILocked(2)
			a.mu.Unlock()
			if peerView.AIReviewSeat == nil || *peerView.AIReviewSeat != 1 {
				t.Fatal("peer lacks review pause indication")
			}
			for i := 0; i < 2; i++ {
				answer, err := a.AskAIReview(review.ID, "Почему выбран этот вариант?")
				if err != nil || !strings.Contains(answer, "Альтернатива") {
					t.Fatal(answer, err)
				}
			}
			if a.Status().View.Revision != revision || len(a.Status().AIReview.Chat) != 4 {
				t.Fatal("discussion changed game or lost history")
			}
			if err = a.ContinueAIReview(review.ID); err != nil {
				t.Fatal(err)
			}
			if a.Status().View.Revision != revision+1 || a.Status().AIReview != nil {
				t.Fatal("confirmation did not execute once")
			}
			if a.ContinueAIReview(review.ID) == nil {
				t.Fatal("replayed confirmation")
			}
		})
	}
}
