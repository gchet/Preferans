package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSavedWindowsLogPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows saved path")
	}
	dir := t.TempDir()
	a := New(filepath.Join(dir, "config"))
	defer a.Close()
	p := a.Status().Appearance
	first, second := filepath.Join(dir, "first.log"), filepath.Join(dir, "sub", "second.log")
	p.LogPath = first
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	p.LogPath = second
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.logLocked("new-file-marker")
	a.mu.Unlock()
	b, _ := os.ReadFile(first)
	if strings.Contains(string(b), "new-file-marker") {
		t.Fatal("old file still active")
	}
	b, _ = os.ReadFile(second)
	if !strings.Contains(string(b), "new-file-marker") {
		t.Fatal("new file not active")
	}
	p.LogPath = filepath.Join(second, "invalid.log")
	if err := a.SetAppearance(p); err == nil {
		t.Fatal("file used as directory accepted")
	}
	if a.Status().Appearance.LogPath != second || a.Status().LogPath != second {
		t.Fatal("failed change lost prior path")
	}
	a.Close()
	a = New(filepath.Join(dir, "config"))
	defer a.Close()
	if a.Status().LogPath != second {
		t.Fatal("path not restored")
	}
	p = a.Status().Appearance
	p.LogDisabled = true
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(second)
	a.mu.Lock()
	a.logLocked("disabled-marker")
	a.mu.Unlock()
	after, _ := os.ReadFile(second)
	if string(before) != string(after) {
		t.Fatal("disabled file changed")
	}
	override := filepath.Join(dir, "cli.log")
	if err := a.SetLogFile(override); err != nil {
		t.Fatal(err)
	}
	p.LogDisabled = false
	p.LogPath = first
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	if a.Status().LogPath != override || !a.Status().LogPathOverride {
		t.Fatal("CLI priority lost")
	}
	a.mu.Lock()
	a.logLocked("override-marker")
	a.mu.Unlock()
	b, _ = os.ReadFile(override)
	if !strings.Contains(string(b), "override-marker") {
		t.Fatal("override not writing")
	}
}
