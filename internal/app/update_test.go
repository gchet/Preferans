package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"preferans/internal/selfupdate"
)

func sumOf(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// githubStub отдаёт «последний выпуск» с заданной версией; badSum подменяет сумму exe в SHA256SUMS.txt.
func githubStub(t *testing.T, version string, exe, apk []byte, badSum bool, hits *int32) *selfupdate.Client {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/owner/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		fmt.Fprintf(w, `{"tag_name":"v%s","html_url":"http://example/release","draft":false,"prerelease":false,"assets":[`+
			`{"name":"Preferans.exe","browser_download_url":%q},{"name":"Preferans.apk","browser_download_url":%q},{"name":"SHA256SUMS.txt","browser_download_url":%q}]}`,
			version, srv.URL+"/Preferans.exe", srv.URL+"/Preferans.apk", srv.URL+"/SHA256SUMS.txt")
	})
	mux.HandleFunc("/Preferans.exe", func(w http.ResponseWriter, r *http.Request) { w.Write(exe) })
	mux.HandleFunc("/Preferans.apk", func(w http.ResponseWriter, r *http.Request) { w.Write(apk) })
	mux.HandleFunc("/SHA256SUMS.txt", func(w http.ResponseWriter, r *http.Request) {
		exeSum := sumOf(exe)
		if badSum {
			exeSum = strings.Repeat("0", 64)
		}
		fmt.Fprintf(w, "%s  Preferans.exe\n%s  Preferans.apk\n", exeSum, sumOf(apk))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := selfupdate.New()
	c.API, c.Repository = srv.URL, "owner/repo"
	c.AllowHost = func(*url.URL) bool { return true }
	return c
}

func supportUpdates(t *testing.T, install bool) {
	t.Helper()
	oldSupported, oldInstall, oldAI := updateSupported, selfInstallable, aiBuild
	updateSupported, selfInstallable, aiBuild = func() bool { return true }, func() bool { return install }, func() bool { return false }
	t.Cleanup(func() { updateSupported, selfInstallable, aiBuild = oldSupported, oldInstall, oldAI })
}

func TestCheckUpdateOffersOnlyNewerVersionAndRespectsSetting(t *testing.T) {
	supportUpdates(t, true)
	var hits int32
	a := New(t.TempDir())
	defer a.Close()
	a.updater = githubStub(t, "2.0.9", []byte("exe"), []byte("apk"), false, &hits)

	info, err := a.CheckUpdate("2.0.3")
	if err != nil || info == nil || info.State != "available" || info.Version != "2.0.9" || !info.CanInstall ||
		info.APKSHA256 != sumOf([]byte("apk")) || !strings.HasSuffix(info.APKURL, "/Preferans.apk") {
		t.Fatalf("%+v %v", info, err)
	}
	if st := a.Status().Update; st == nil || st.Version != "2.0.9" {
		t.Fatalf("состояние обновления не попало в Status: %+v", st)
	}
	for _, same := range []string{"2.0.9", "2.1.0", "dev", ""} {
		if i, _ := a.CheckUpdate(same); i != nil && same != "" && same != "dev" && i.Version == "2.0.9" && same == "2.0.9" {
			t.Fatalf("для %q предложена та же версия", same)
		}
	}
	// выключенное автоматическое обновление: к GitHub не обращаемся
	off := false
	p := a.Status().Appearance
	p.AutoUpdate = &off
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	before := atomic.LoadInt32(&hits)
	if i, _ := a.CheckUpdate("2.0.3"); i != nil || atomic.LoadInt32(&hits) != before {
		t.Fatal("выключенная настройка проигнорирована")
	}
}

func TestCheckUpdateSkipsAIBuildsAndUnsupportedPlatforms(t *testing.T) {
	supportUpdates(t, true)
	var hits int32
	a := New(t.TempDir())
	defer a.Close()
	a.updater = githubStub(t, "2.0.9", []byte("exe"), []byte("apk"), false, &hits)
	aiBuild = func() bool { return true } // сборка с ИИ: выпуск на GitHub собран без ИИ, обновлять её нельзя
	if info, _ := a.CheckUpdate("2.0.3"); info != nil {
		t.Fatal("сборку с ИИ нельзя обновлять выпуском без ИИ")
	}
	aiBuild = func() bool { return false }
	updateSupported = func() bool { return false }
	if info, _ := a.CheckUpdate("2.0.3"); info != nil || atomic.LoadInt32(&hits) != 0 {
		t.Fatal("на неподдерживаемой платформе GitHub не опрашивается")
	}
}

func TestCheckUpdateFailureOnlyGoesToLog(t *testing.T) {
	supportUpdates(t, true)
	a := New(t.TempDir())
	defer a.Close()
	c := selfupdate.New()
	c.API, c.Repository = "http://127.0.0.1:1", "owner/repo"
	c.AllowHost = func(*url.URL) bool { return true }
	a.updater = c
	if info, err := a.CheckUpdate("2.0.3"); info != nil || err != nil {
		t.Fatalf("сбой сети не должен быть ошибкой: %+v %v", info, err)
	}
	a.mu.Lock()
	logs := strings.Join(a.logs, "\n")
	a.mu.Unlock()
	if !strings.Contains(logs, "Проверка обновления не удалась") {
		t.Fatal("сбой не записан в журнал")
	}
}

func TestInstallUpdateReplacesExecutableAndRestarts(t *testing.T) {
	supportUpdates(t, true)
	dir := t.TempDir()
	exe := filepath.Join(dir, "Preferans.exe")
	os.WriteFile(exe, []byte("old-version"), 0o755)
	oldExec, oldRestart := executablePath, restartExecutable
	executablePath = func() (string, error) { return exe, nil }
	restarted := make(chan string, 1)
	restartExecutable = func(path string, args []string) error { restarted <- path; return nil }
	t.Cleanup(func() { executablePath, restartExecutable = oldExec, oldRestart })

	a := New(t.TempDir())
	defer a.Close()
	quit := make(chan struct{}, 1)
	a.quit = func() { quit <- struct{}{} }
	newExe := []byte(strings.Repeat("new-version", 5000))
	a.updater = githubStub(t, "2.0.9", newExe, []byte("apk"), false, nil)
	if err := a.InstallUpdate(); err == nil {
		t.Fatal("установка без найденного обновления должна быть отклонена")
	}
	if _, err := a.CheckUpdate("2.0.3"); err != nil {
		t.Fatal(err)
	}
	if err := a.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	if err := a.InstallUpdate(); err == nil && a.Status().Update.State == "downloading" {
		t.Log("повторный запуск отклонён или уже завершён")
	}
	select {
	case path := <-restarted:
		if path != exe {
			t.Fatal(path)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("перезапуск не выполнен")
	}
	select {
	case <-quit:
	case <-time.After(5 * time.Second):
		t.Fatal("старый процесс не завершается")
	}
	if b, _ := os.ReadFile(exe); string(b) != string(newExe) {
		t.Fatal("exe не заменён")
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "old-version" {
		t.Fatal("прежняя версия не сохранена")
	}
	if _, err := os.Stat(exe + ".new"); err == nil {
		t.Fatal("остался .new")
	}
}

func TestInstallUpdateRejectsCorruptedDownload(t *testing.T) {
	supportUpdates(t, true)
	dir := t.TempDir()
	exe := filepath.Join(dir, "Preferans.exe")
	os.WriteFile(exe, []byte("old-version"), 0o755)
	oldExec, oldRestart := executablePath, restartExecutable
	executablePath = func() (string, error) { return exe, nil }
	var restarts int32
	restartExecutable = func(string, []string) error { atomic.AddInt32(&restarts, 1); return nil }
	t.Cleanup(func() { executablePath, restartExecutable = oldExec, oldRestart })
	a := New(t.TempDir())
	defer a.Close()
	a.quit = func() { t.Error("quit при неудачном обновлении") }
	a.updater = githubStub(t, "2.0.9", []byte("new-version"), []byte("apk"), true, nil)
	if _, err := a.CheckUpdate("2.0.3"); err != nil {
		t.Fatal(err)
	}
	if err := a.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if st := a.Status().Update; st != nil && st.State == "error" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	st := a.Status().Update
	if st == nil || st.State != "error" || !strings.Contains(st.Error, "Контрольная сумма") {
		t.Fatalf("%+v", st)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old-version" || atomic.LoadInt32(&restarts) != 0 {
		t.Fatal("при неверной сумме программа не должна меняться или перезапускаться")
	}
	for _, leftover := range []string{exe + ".new", exe + ".new.part", exe + ".old"} {
		if _, err := os.Stat(leftover); err == nil {
			t.Fatal("остался", leftover)
		}
	}
}

func TestInstallUpdateIsUnavailableWhereSystemInstallerIsUsed(t *testing.T) {
	supportUpdates(t, false)
	a := New(t.TempDir())
	defer a.Close()
	if err := a.InstallUpdate(); err == nil || !strings.Contains(err.Error(), "системный установщик") {
		t.Fatalf("%v", err)
	}
}
