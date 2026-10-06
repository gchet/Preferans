package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func answer(w http.ResponseWriter, content, finish string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 12, "completion_tokens": 4}})
}
func TestRepairAndLimits(t *testing.T) {
	for _, bad := range []string{`{"move":99}`, `{"why":"missing index"}`, `{"move":0} extra`, `{"move":0,"unexpected":1}`, `{"move":0.5}`} {
		t.Run(bad, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fake-test-key" {
					t.Error("missing credential")
				}
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["max_tokens"] != float64(500) || body["temperature"] != float64(0) {
					t.Error("parameters changed")
				}
				if calls.Add(1) == 1 {
					answer(w, bad, "stop")
				} else {
					answer(w, `{"move":1,"why":"short"}`, "stop")
				}
			}))
			defer srv.Close()
			result, err := (Client{HTTP: srv.Client()}).Select(context.Background(), Model{URL: srv.URL, Model: "test", MaxTokens: 500}, "fake-test-key", "system", map[string]int{"seat": 1}, 2)
			if err != nil || result.Move != 1 || result.Attempts != 2 || result.PromptTokens != 24 {
				t.Fatalf("result %+v: %v", result, err)
			}
		})
	}
}
func TestDeadlineAndRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(429)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := (Client{HTTP: srv.Client()}).Select(ctx, Model{URL: srv.URL}, "", "system", nil, 1)
	if err != context.DeadlineExceeded || time.Since(start) > time.Second || calls.Load() != 1 {
		t.Fatalf("deadline ignored: %v %d", err, calls.Load())
	}
}
func TestTruncatedResponseGetsOneRepair(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); answer(w, `{"move":0}`, "length") }))
	defer srv.Close()
	_, err := (Client{HTTP: srv.Client()}).Select(context.Background(), Model{URL: srv.URL}, "", "system", nil, 1)
	if err == nil || calls.Load() != 2 {
		t.Fatalf("repair unbounded: %v %d", err, calls.Load())
	}
}
