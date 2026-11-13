package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultLogIsUsedUntilOwnPathIsChosen(t *testing.T) {
	dir := t.TempDir()
	a := New(filepath.Join(dir, "config"))
	defer a.Close()
	def := filepath.Join(dir, "config", "preferans.log")
	if err := a.UseDefaultLog(def); err != nil {
		t.Fatal(err)
	}
	if a.Status().LogPath != def {
		t.Fatal("журнал по умолчанию не включён", a.Status().LogPath)
	}
	a.mu.Lock()
	a.logLocked("default-marker")
	a.mu.Unlock()
	if b, _ := os.ReadFile(def); !strings.Contains(string(b), "default-marker") {
		t.Fatal("в файл по умолчанию ничего не записано")
	}
	if runtime.GOOS != "windows" {
		return
	}
	own := filepath.Join(dir, "own.log")
	p := a.Status().Appearance
	p.LogPath = own
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.logLocked("own-marker")
	a.mu.Unlock()
	if b, _ := os.ReadFile(own); !strings.Contains(string(b), "own-marker") {
		t.Fatal("свой путь не используется")
	}
	p.LogPath = ""
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	if a.Status().LogPath != def {
		t.Fatal("пустое поле не вернуло файл по умолчанию", a.Status().LogPath)
	}
	a.mu.Lock()
	a.logLocked("back-marker")
	a.mu.Unlock()
	if b, _ := os.ReadFile(def); !strings.Contains(string(b), "back-marker") {
		t.Fatal("после возврата запись не идёт в файл по умолчанию")
	}
}

func TestCommandLineLogBeatsDefault(t *testing.T) {
	dir := t.TempDir()
	a := New(filepath.Join(dir, "config"))
	defer a.Close()
	cli := filepath.Join(dir, "cli.log")
	if err := a.SetLogFile(cli); err != nil {
		t.Fatal(err)
	}
	if err := a.UseDefaultLog(filepath.Join(dir, "config", "preferans.log")); err != nil {
		t.Fatal(err)
	}
	if a.Status().LogPath != cli {
		t.Fatal("путь из командной строки потерял приоритет", a.Status().LogPath)
	}
}

func TestDefaultLogRespectsDisabledSwitch(t *testing.T) {
	dir := t.TempDir()
	a := New(filepath.Join(dir, "config"))
	defer a.Close()
	if err := a.SetLogEnabled(false); err != nil {
		t.Fatal(err)
	}
	def := filepath.Join(dir, "config", "preferans.log")
	if err := a.UseDefaultLog(def); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(def); err == nil {
		t.Fatal("отключённый журнал создал файл")
	}
	if err := a.SetLogEnabled(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(def); err != nil {
		t.Fatal("после включения файл не создан", err)
	}
}

func TestLogRotatesWhenTooLarge(t *testing.T) {
	old := logMaxBytes
	logMaxBytes = 400
	defer func() { logMaxBytes = old }()
	dir := t.TempDir()
	a := New(filepath.Join(dir, "config"))
	defer a.Close()
	path := filepath.Join(dir, "preferans.log")
	if err := a.SetLogFile(path); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		a.mu.Lock()
		a.logLocked("строка журнала номер %d с заполнителем заполнителем", i)
		a.mu.Unlock()
	}
	previous, err := os.ReadFile(path + ".1")
	if err != nil || len(previous) == 0 {
		t.Fatal("прежняя часть журнала не сохранена", err)
	}
	current, _ := os.ReadFile(path)
	if int64(len(current)) > logMaxBytes+200 || !strings.Contains(string(current), "preferans.log.1") {
		t.Fatal("журнал не продолжен после ротации", len(current))
	}
	if !strings.Contains(string(previous)+string(current), "строка журнала номер 29") {
		t.Fatal("последняя строка потеряна")
	}
}