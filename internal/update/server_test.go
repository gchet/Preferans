package update

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerServesManifestAndAPKOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "version.json"), []byte(`{"versionCode":2,"versionName":"0.1.2","apk":"Preferans.apk"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Preferans.apk"), []byte("apk-data"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Start(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	addr := s.Addr()
	addr = strings.Replace(addr, "0.0.0.0", "127.0.0.1", 1)

	// The test server uses a fixed port only to keep the public API small.
	for _, path := range []string{"/version.json", "/Preferans.apk", "/secret.txt"} {
		resp, err := http.Get("http://" + addr + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if path == "/secret.txt" {
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("secret status: %d", resp.StatusCode)
			}
		} else if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status: %d", path, resp.StatusCode)
		}
		if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("missing CORS header for %s", path)
		}
	}
	var m manifest
	b, err := os.ReadFile(filepath.Join(dir, "version.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &m); err != nil || m.APK != "Preferans.apk" {
		t.Fatal("bad test manifest")
	}
}
