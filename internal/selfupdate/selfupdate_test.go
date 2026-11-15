package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hexSum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// releaseServer имитирует GitHub: API выпуска, файлы и SHA256SUMS.txt.
func releaseServer(t *testing.T, tag string, draft, prerelease bool, exe, apk []byte, mutate func(sums *string)) (*Client, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/owner/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"http://example/release","draft":%t,"prerelease":%t,"assets":[`+
			`{"name":"Preferans.exe","browser_download_url":%q},{"name":"Preferans.apk","browser_download_url":%q},{"name":"SHA256SUMS.txt","browser_download_url":%q}]}`,
			tag, draft, prerelease, srv.URL+"/dl/Preferans.exe", srv.URL+"/dl/Preferans.apk", srv.URL+"/dl/SHA256SUMS.txt")
	})
	mux.HandleFunc("/dl/Preferans.exe", func(w http.ResponseWriter, r *http.Request) { w.Write(exe) })
	mux.HandleFunc("/dl/Preferans.apk", func(w http.ResponseWriter, r *http.Request) { w.Write(apk) })
	mux.HandleFunc("/dl/SHA256SUMS.txt", func(w http.ResponseWriter, r *http.Request) {
		sums := hexSum(exe) + "  Preferans.exe\r\n" + hexSum(apk) + "  Preferans.apk\r\n" + strings.Repeat("0", 64) + "  LICENSE\r\n"
		if mutate != nil {
			mutate(&sums)
		}
		w.Write([]byte(sums))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New()
	c.API, c.Repository = srv.URL, "owner/repo"
	c.AllowHost = func(*url.URL) bool { return true }
	return c, srv
}

func TestNewer(t *testing.T) {
	cases := []struct {
		cur, cand string
		want      bool
	}{
		{"2.0.3", "2.0.4", true}, {"2.0.3", "2.1.0", true}, {"2.0.3", "3.0.0", true}, {"v2.0.3", "2.0.4", true},
		{"2.0.3", "2.0.3", false}, {"2.0.4", "2.0.3", false}, {"2.10.0", "2.9.9", false}, {"2.9.9", "2.10.0", true},
		{"dev", "2.0.4", false}, {"", "2.0.4", false}, {"2.0.3", "dev", false}, {"2.0", "2.0.4", false},
	}
	for _, c := range cases {
		if got := Newer(c.cur, c.cand); got != c.want {
			t.Errorf("Newer(%q,%q)=%v", c.cur, c.cand, got)
		}
	}
}

func TestLatestReadsReleaseAndChecksums(t *testing.T) {
	exe, apk := []byte("exe-bytes"), []byte("apk-bytes")
	c, srv := releaseServer(t, "v2.0.9", false, false, exe, apk, nil)
	r, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Tag != "v2.0.9" || r.Version != "2.0.9" || r.ExeSHA256 != hexSum(exe) || r.APKSHA256 != hexSum(apk) || r.ExeURL != srv.URL+"/dl/Preferans.exe" {
		t.Fatalf("%+v", r)
	}
}

func TestLatestRejectsDraftPrereleaseAndBadReleases(t *testing.T) {
	for name, tc := range map[string]struct {
		tag        string
		draft, pre bool
		mutate     func(*string)
	}{
		"draft":      {"v2.0.9", true, false, nil},
		"prerelease": {"v2.0.9", false, true, nil},
		"bad tag":    {"nightly", false, false, nil},
		"no sums":    {"v2.0.9", false, false, func(s *string) { *s = strings.Repeat("0", 64) + "  LICENSE\n" }},
	} {
		c, _ := releaseServer(t, tc.tag, tc.draft, tc.pre, []byte("e"), []byte("a"), tc.mutate)
		if _, err := c.Latest(context.Background()); !errors.Is(err, ErrBad) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestDownloadVerifiesChecksumAndLeavesNoPartFile(t *testing.T) {
	exe := []byte(strings.Repeat("X", 200000))
	c, srv := releaseServer(t, "v2.0.9", false, false, exe, []byte("a"), nil)
	dir := t.TempDir()
	dest := filepath.Join(dir, "Preferans.exe.new")
	var last, total int64
	if err := c.Download(context.Background(), srv.URL+"/dl/Preferans.exe", dest, hexSum(exe), func(d, tt int64) { last, total = d, tt }); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != string(exe) || last != int64(len(exe)) {
		t.Fatal("файл скачан неверно", last, total)
	}
	bad := filepath.Join(dir, "bad.new")
	if err := c.Download(context.Background(), srv.URL+"/dl/Preferans.exe", bad, strings.Repeat("0", 64), nil); !errors.Is(err, ErrHash) {
		t.Fatalf("ожидалась ErrHash: %v", err)
	}
	if _, err := os.Stat(bad); err == nil {
		t.Fatal("файл с неверной суммой остался")
	}
	if _, err := os.Stat(bad + ".part"); err == nil {
		t.Fatal("остался .part")
	}
}

func TestDownloadAllowsOnlyGitHubHTTPS(t *testing.T) {
	c := New()
	for _, raw := range []string{"http://github.com/x", "https://evil.example/x", "https://github.com.evil.example/x", "file:///c:/x", "ftp://github.com/x"} {
		u, _ := url.Parse(raw)
		if c.allowed(u) {
			t.Errorf("%s разрешён", raw)
		}
	}
	for _, raw := range []string{"https://github.com/gchet/Preferans/releases/download/v2.0.3/Preferans.exe", "https://release-assets.githubusercontent.com/x", "https://objects.githubusercontent.com/y", "https://api.github.com/repos/x"} {
		u, _ := url.Parse(raw)
		if !c.allowed(u) {
			t.Errorf("%s запрещён", raw)
		}
	}
	if err := c.Download(context.Background(), "https://evil.example/Preferans.exe", filepath.Join(t.TempDir(), "x"), strings.Repeat("0", 64), nil); !errors.Is(err, ErrHost) {
		t.Fatalf("%v", err)
	}
}

func TestReplaceExecutableKeepsOldAndRestoresOnFailure(t *testing.T) {
	dir := t.TempDir()
	cur, fresh := filepath.Join(dir, "Preferans.exe"), filepath.Join(dir, "Preferans.exe.new")
	os.WriteFile(cur, []byte("old"), 0o755)
	os.WriteFile(fresh, []byte("new"), 0o755)
	if err := ReplaceExecutable(cur, fresh); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(cur); string(b) != "new" {
		t.Fatal("новая версия не на месте")
	}
	if b, _ := os.ReadFile(cur + ".old"); string(b) != "old" {
		t.Fatal("прежняя версия не сохранена")
	}
	CleanupOld(cur)
	if _, err := os.Stat(cur + ".old"); err == nil {
		t.Fatal(".old не удалён")
	}
	// нет скачанного файла: прежний возвращается на место
	if err := ReplaceExecutable(cur, filepath.Join(dir, "missing.new")); err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if b, _ := os.ReadFile(cur); string(b) != "new" {
		t.Fatal("после неудачи прежний файл не восстановлен")
	}
}
