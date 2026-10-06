package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClearDiskLogAndContinueWriting(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	path := filepath.Join(t.TempDir(), "preferans.log")
	if err := a.SetLogFile(path); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.logLocked("before-clear")
	a.mu.Unlock()
	b, err := os.ReadFile(path)
	if err != nil || a.Status().LogSize != int64(len(b)) {
		t.Fatal("incorrect log size", err)
	}
	if _, err := a.RPC(Request{Action: "log-clear"}); err != nil {
		t.Fatal(err)
	}
	if s := a.Status(); s.LogSize != 0 || len(s.Logs) != 0 {
		t.Fatal("log not cleared", s.LogSize)
	}
	a.mu.Lock()
	a.logLocked("after-clear")
	a.mu.Unlock()
	b, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "after-clear") || strings.Contains(string(b), "before-clear") {
		t.Fatal("append after truncation failed", err, string(b))
	}
	if err := a.SetLogEnabled(false); err != nil {
		t.Fatal(err)
	}
	if a.Status().LogSize != int64(len(b)) {
		t.Fatal("disabled log size lost")
	}
	if err := a.ClearLogFile(); err != nil {
		t.Fatal(err)
	}
	if a.Status().LogSize != 0 {
		t.Fatal("disabled log not cleared")
	}
	if err := a.SetLogEnabled(true); err != nil {
		t.Fatal(err)
	}
	if a.Status().LogSize == 0 {
		t.Fatal("logging not re-enabled")
	}
}
