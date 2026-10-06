package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"preferans/internal/game"
)

func eventually(t *testing.T, f func() bool) {
	t.Helper()
	until := time.Now().Add(15 * time.Second)
	for time.Now().Before(until) {
		if f() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition timed out")
}
func connect(t *testing.T, host, client *App, seat int) {
	t.Helper()
	offer, e := host.Invite(seat)
	if e != nil {
		t.Fatal(e)
	}
	answer, e := client.Join(offer)
	if e != nil {
		t.Fatal(e)
	}
	if e = host.Answer(answer); e != nil {
		t.Fatal(e)
	}
	eventually(t, func() bool { s := client.Status(); return s.View != nil && host.Status().Connected[seat] })
}
func send(t *testing.T, apps []*App, seat int, c game.Command) {
	t.Helper()
	revision := apps[0].Status().View.Revision
	if e := apps[seat].Command(c); e != nil {
		t.Fatal(e)
	}
	eventually(t, func() bool {
		for _, a := range apps {
			v := a.Status().View
			if v == nil || v.Revision <= revision {
				return false
			}
		}
		return true
	})
}
func TestNetworkPartyAndHostRestore(t *testing.T) {
	for _, n := range []int{3, 4} {
		t.Run(string(rune('0'+n)), func(t *testing.T) {
			dir := t.TempDir()
			apps := make([]*App, n)
			for i := range apps {
				apps[i] = New(filepath.Join(dir, string(rune('0'+i))))
				_ = apps[i].Configure(nil)
			}
			defer func() {
				for _, a := range apps {
					a.Close()
				}
			}()
			if e := apps[0].Create(n, 1, "Ведущий", false); e != nil {
				t.Fatal(e)
			}
			id := apps[0].Status().View.ID
			for i := 1; i < n; i++ {
				connect(t, apps[0], apps[i], i)
			}
			// This test covers networking/restore, not breaking a pass series.
			// Permit finishing on pool so random all-pass deals cannot extend it indefinitely.
			rules := game.DefaultRules()
			no := false
			rules.FinishAfterPassExit = &no
			initial := apps[0].Status().View
			send(t, apps, 0, game.Command{ID: game.ID(), Seat: 0, Revision: initial.Revision, Action: "rules", Rules: &rules, Target: 1})
			for i, a := range apps {
				v := a.Status().View
				send(t, apps, i, game.Command{ID: game.ID(), Seat: i, Revision: v.Revision, Action: "ready"})
			}
			v := apps[0].Status().View
			send(t, apps, 0, game.Command{ID: game.ID(), Revision: v.Revision, Action: "start"})
			rev := apps[0].Status().View.Revision
			apps[0].Close()
			apps[0] = New(filepath.Join(dir, "0"))
			_ = apps[0].Configure(nil)
			if e := apps[0].Resume(id); e != nil {
				t.Fatal(e)
			}
			if !apps[0].Status().Paused {
				t.Fatal("not paused on restore")
			}
			for i := 1; i < n; i++ {
				connect(t, apps[0], apps[i], i)
			}
			eventually(t, func() bool {
				for _, a := range apps {
					if a.Status().Paused {
						return false
					}
				}
				return true
			})
			if apps[0].Status().View.Revision != rev {
				t.Fatal("restore changed revision")
			}
			for steps := 0; steps < 1000; steps++ {
				v = apps[0].Status().View
				if v.Stage == "finished" {
					return
				}
				seat := v.Actor
				if v.Stage == "trick" || v.Stage == "round" {
					seat = 0
				}
				cv := apps[seat].Status().View
				c := game.BotCommand(*cv)
				if c.Action == "" {
					t.Fatalf("no bot action at %s", v.Stage)
				}
				send(t, apps, seat, c)
			}
			t.Fatal("party did not finish")
		})
	}
}
func TestSaveFailureDoesNotCommit(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	if e := a.Create(3, 30, "Test", true); e != nil {
		t.Fatal(e)
	}
	v := a.Status().View
	file := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(file, []byte("x"), 0600)
	a.store.Dir = file
	if e := a.Command(game.Command{ID: game.ID(), Revision: v.Revision, Action: "ready"}); e == nil {
		t.Fatal("save failure ignored")
	}
	if a.Status().View.Revision != v.Revision {
		t.Fatal("unpersisted state committed")
	}
}
func TestLoopbackAPIRequiresToken(t *testing.T) {
	a := New(t.TempDir())
	url, stop, e := Serve(a, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}})
	if e != nil {
		t.Fatal(e)
	}
	defer stop()
	parts := strings.Split(url, "#")
	endpoint := strings.TrimSuffix(parts[0], "/") + "/api"
	for _, good := range []bool{false, true} {
		body, _ := json.Marshal(Request{Action: "status"})
		req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(body))
		if good {
			req.Header.Set("X-Preferans-Token", parts[1])
		}
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
		want := 403
		if good {
			want = 200
		}
		if resp.StatusCode != want {
			t.Fatal(resp.StatusCode)
		}
	}
}

func TestLogFileAppendsDiagnosticLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connection.log")
	a := New(filepath.Join(dir, "app"))
	defer a.Close()
	if err := a.SetLogFile(path); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.logLocked("тестовая запись %d", 7)
	a.mu.Unlock()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "тестовая запись 7") {
		t.Fatalf("log file does not contain diagnostic line: %q", string(b))
	}
	if err := a.SetLogEnabled(false); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.logLocked("не должна попасть")
	a.mu.Unlock()
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "не должна попасть") {
		t.Fatalf("disabled log still received a line: %q", string(b))
	}
	if err := a.SetLogEnabled(true); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.logLocked("снова включена")
	a.mu.Unlock()
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "снова включена") {
		t.Fatalf("re-enabled log did not receive a line: %q", string(b))
	}
}
