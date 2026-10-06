//go:build noai

package app

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"preferans/config"
	"preferans/internal/ai"
	"preferans/internal/storage"
	"testing"
)

func TestNoAIBuildBlocksModelsAndKeys(t *testing.T) {
	dir := t.TempDir()
	store := storage.Store{Dir: dir}
	settings := AISettings{Models: []ai.Model{{ID: "saved", Alias: "Saved model"}}, TimeoutSeconds: 10}
	if err := store.Save("ai-settings", settings); err != nil {
		t.Fatal(err)
	}
	if err := store.Save("ai-keys", map[string]string{"secret": "encrypted"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "ai-settings.json"))
	a := New(dir)
	defer a.Close()
	if config.AIEnabled || a.Status().AIEnabled || len(a.AISettings().Models) != 0 || len(a.ai.encrypted) != 0 {
		t.Fatal("disabled build loaded AI settings")
	}
	for _, action := range []string{"ai-settings", "ai-save", "ai-key-save", "ai-key-delete", "ai-assign", "ai-transfer-create", "ai-transfer-import", "ai-review-ask", "ai-review-continue"} {
		if _, err := a.RPC(Request{Action: action}); err == nil {
			t.Fatal("AI RPC allowed", action)
		}
	}
	if err := a.SaveAISettings(settings); err == nil {
		t.Fatal("direct settings write allowed")
	}
	if _, err := a.SaveAIKey("", "test", "secret"); err == nil {
		t.Fatal("direct key write allowed")
	}
	if err := a.DeleteAIKey("secret"); err == nil {
		t.Fatal("direct key deletion allowed")
	}
	if err := a.AssignBot(1, "saved"); err == nil {
		t.Fatal("direct model assignment allowed")
	}
	if _, err := a.StartAITransfer(); err == nil {
		t.Fatal("direct transfer allowed")
	}
	if err := a.ImportAITransfer("http://127.0.0.1:1", "123456"); err == nil {
		t.Fatal("direct import allowed")
	}
	if _, err := a.AskAIReview("id", "question"); err == nil {
		t.Fatal("direct model discussion allowed")
	}
	if err := a.ContinueAIReview("id"); err == nil {
		t.Fatal("direct review allowed")
	}
	reply := httptest.NewRecorder()
	a.AITransferHandler().ServeHTTP(reply, httptest.NewRequest("POST", "/ai-transfer/test", nil))
	if reply.Code != 404 {
		t.Fatal("transfer endpoint present")
	}
	a.ai.settings = settings
	if _, ok := a.modelLocked("saved"); ok {
		t.Fatal("persisted model bypassed build flag")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "ai-settings.json"))
	if string(before) != string(after) {
		t.Fatal("disabled build overwrote AI settings")
	}
	if err := a.Create(3, 30, "Host", true); err != nil {
		t.Fatal("ordinary bots unavailable", err)
	}
}
