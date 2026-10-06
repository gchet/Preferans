package app

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"preferans/internal/ai"
	"preferans/internal/game"
)

// Tests use a deterministic adapter only; production never falls back to this.
type testAICipher struct{}

func (testAICipher) Seal(value string) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte(value)), nil
}
func (testAICipher) Open(value string) (string, error) {
	b, e := base64.StdEncoding.DecodeString(value)
	return string(b), e
}
func TestAIRegistryAndEncryptedTransfer(t *testing.T) {
	dir := t.TempDir()
	host := New(dir)
	defer host.Close()
	host.SetSecretCipher(testAICipher{})
	key, err := host.SaveAIKey("", "Groq", "test-api-secret")
	if err != nil {
		t.Fatal(err)
	}
	m := ai.Model{ID: "m1", Alias: " qwen ", Provider: "Groq", URL: "https://api.groq.com/openai/v1/chat/completions", Model: "test", KeyID: key, MaxTokens: 500}
	if err = host.SaveAISettings(AISettings{Models: []ai.Model{m}, TimeoutSeconds: 10}); err != nil {
		t.Fatal(err)
	}
	if err = host.Create(3, 30, "Host", true); err != nil {
		t.Fatal(err)
	}
	if err = host.AssignBot(1, "m1"); err != nil {
		t.Fatal(err)
	}
	if host.Status().View.Players[1].Name != "Бот 1 qwen" {
		t.Fatal("alias not displayed")
	}
	id := host.Status().View.ID
	host.Leave()
	if err = host.Resume(id); err != nil {
		t.Fatal(err)
	}
	if host.Status().View.Players[1].BotModel != "m1" {
		t.Fatal("saved model assignment lost")
	}
	if err = host.DeleteAIKey(key); err == nil {
		t.Fatal("deleted used key")
	}
	data, _ := json.Marshal(host.AISettings())
	if strings.Contains(string(data), "test-api-secret") {
		t.Fatal("key leaked in API")
	}
	for _, file := range []string{"ai-settings.json", "ai-keys.json"} {
		data, _ := os.ReadFile(filepath.Join(dir, file))
		if strings.Contains(string(data), "test-api-secret") {
			t.Fatal("plaintext key on disk")
		}
	}
	server := httptest.NewServer(host.AITransferHandler())
	defer server.Close()
	code, err := host.StartAITransfer()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		t.Fatalf("expected six decimal digits, got %q", code)
	}
	// Leading zeroes must survive text input and the handshake unchanged.
	code = "000042"
	host.mu.Lock()
	host.ai.transfer.ID = transferID(code)
	host.mu.Unlock()
	guest := New(t.TempDir())
	defer guest.Close()
	guest.SetSecretCipher(testAICipher{})
	if guest.ImportAITransfer(server.URL, "000043") == nil {
		t.Fatal("wrong transfer code accepted")
	}
	if err = guest.ImportAITransfer(server.URL, code); err != nil {
		t.Fatal(err)
	}
	if len(guest.AISettings().Models) != 1 || guest.AISettings().Models[0].Alias != "qwen" {
		t.Fatal("models not transferred")
	}
	guest.mu.Lock()
	value, err := guest.secretCipher.Open(guest.ai.encrypted[key])
	guest.mu.Unlock()
	if value != "test-api-secret" || err != nil {
		t.Fatal("key not transferred")
	}
	if guest.ImportAITransfer(server.URL, code) == nil {
		t.Fatal("transfer replay accepted")
	}
	if err = host.SaveAISettings(AISettings{TimeoutSeconds: 10}); err != nil {
		t.Fatal(err)
	}
	if host.Status().View.Players[1].BotModel != "" {
		t.Fatal("deleted assignment retained")
	}
}
func TestAITransferGuessLimit(t *testing.T) {
	host := New(t.TempDir())
	defer host.Close()
	host.SetSecretCipher(testAICipher{})
	code, err := host.StartAITransfer()
	if err != nil {
		t.Fatal(err)
	}
	wrong := "000000"
	if code == wrong {
		wrong = "000001"
	}
	handler := host.AITransferHandler()
	for i := 0; i < 10; i++ {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ai-transfer/"+transferID(wrong), nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("wrong code response: %d", response.Code)
		}
	}
	host.mu.Lock()
	remaining := host.ai.transfer
	host.mu.Unlock()
	if remaining != nil {
		t.Fatal("transfer remained available after ten incorrect codes")
	}
}

func TestAIAsyncDecisionFallbackAndPause(t *testing.T) {
	for _, mode := range []string{"success", "invalid", "pause", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			a := New(t.TempDir())
			defer a.Close()
			var calls atomic.Int32
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				select {
				case started <- struct{}{}:
				default:
				}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				content := `{"move":0}`
				if mode == "invalid" {
					content = `{"move":9999}`
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
			}))
			defer server.Close()
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			m := ai.Model{ID: "m", Alias: "test", URL: server.URL, Model: "test", MaxTokens: 500}
			if err := a.SaveAISettings(AISettings{Models: []ai.Model{m}, TimeoutSeconds: 1}); err != nil {
				t.Fatal(err)
			}
			if err := a.Create(3, 30, "Host", true); err != nil {
				t.Fatal(err)
			}
			if err := a.AssignBot(1, "m"); err != nil {
				t.Fatal(err)
			}
			a.mu.Lock()
			s := a.saved.State
			s.Dealer = 0
			for i := range s.Players {
				s.Players[i].Ready = true
			}
			a.ai.client = ai.Client{HTTP: server.Client()}
			revision := s.Revision
			err := a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: revision, Action: "start"})
			a.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("no AI request")
			}
			view := a.Status().View
			if view.BotThinking == nil || view.BotThinking.Seat != 1 || view.BotThinking.StartedAt == 0 {
				t.Fatal("pending AI activity missing")
			}
			a.mu.Lock()
			guestView := a.viewWithAILocked(2)
			a.mu.Unlock()
			if guestView.BotThinking == nil || guestView.BotThinking.Seat != 1 {
				t.Fatal("guest cannot see AI activity")
			}
			revision = view.Revision
			if mode == "pause" {
				err = a.Command(game.Command{ID: game.ID(), Seat: 0, Revision: revision, Action: "pause"})
				if err != nil {
					t.Fatal(err)
				}
				revision = a.Status().View.Revision
			}
			if mode != "timeout" {
				close(release)
			}
			if mode == "pause" {
				time.Sleep(400 * time.Millisecond)
				if a.Status().View.BotThinking != nil {
					t.Fatal("AI activity retained during pause")
				}
				if a.Status().View.Revision != revision {
					t.Fatal("applied during pause")
				}
				return
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) && a.Status().View.Revision == revision {
				time.Sleep(10 * time.Millisecond)
			}
			if a.Status().View.Revision == revision {
				t.Fatal("bot stalled")
			}
			if mode == "invalid" && calls.Load() != 2 {
				t.Fatalf("repair count %d", calls.Load())
			}
		})
	}
}
