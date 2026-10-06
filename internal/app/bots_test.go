package app

import (
	"os"
	"path/filepath"
	"preferans/internal/game"
	"testing"
)

func TestDeleteSave(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	if err := a.Create(3, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	id := a.Status().View.ID
	if err := a.DeleteSave(id); err == nil {
		t.Fatal("active save deleted")
	}
	a.Leave()
	if err := a.DeleteSave("../appearance"); err == nil {
		t.Fatal("path traversal allowed")
	}
	if err := a.DeleteSave(id); err != nil {
		t.Fatal(err)
	}
	saves, err := a.List()
	if err != nil || len(saves) != 0 {
		t.Fatal("save not deleted", err)
	}
	if err := a.Resume(id); err == nil {
		t.Fatal("deleted save resumed")
	}
}

func TestMixedBots(t *testing.T) {
	for _, n := range []int{3, 4} {
		t.Run(string(rune('0'+n)), func(t *testing.T) {
			host, guest := New(t.TempDir()), New(t.TempDir())
			defer host.Close()
			defer guest.Close()
			_ = host.Configure(nil)
			_ = guest.Configure(nil)
			if err := host.Create(n, 30, "Ведущий", false); err != nil {
				t.Fatal(err)
			}
			connect(t, host, guest, 1)
			if _, err := guest.FillBots(); err == nil {
				t.Fatal("guest may fill bots")
			}
			count, err := host.FillBots()
			if err != nil || count != n-2 {
				t.Fatalf("fill: %d %v", count, err)
			}
			eventually(t, func() bool { return guest.Status().View.Players[n-1].Bot })
			v := host.Status().View
			if v.Players[0].Bot || v.Players[1].Bot {
				t.Fatal("human replaced")
			}
			if count, err = host.FillBots(); err != nil || count != 0 {
				t.Fatal("not idempotent")
			}
			apps := []*App{host, guest}
			for i, a := range apps {
				send(t, apps, i, game.Command{ID: game.ID(), Revision: a.Status().View.Revision, Action: "ready"})
			}
			send(t, apps, 0, game.Command{ID: game.ID(), Revision: host.Status().View.Revision, Action: "start"})
			if _, err := host.FillBots(); err == nil {
				t.Fatal("allowed after start")
			}
		})
	}
}

func TestFillBotsProtectsReservedSeatsAndPersists(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	if err := a.Create(3, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.saved.Joined[1] = true
	a.mu.Unlock()
	if a.Status().FreeSeats != 1 {
		t.Fatal("reserved human counted as free")
	}
	count, err := a.FillBots()
	if err != nil || count != 1 {
		t.Fatalf("%d %v", count, err)
	}
	id := a.Status().View.ID
	if err := a.Resume(id); err != nil {
		t.Fatal(err)
	}
	s := a.Status()
	if s.View.Players[1].Bot || !s.View.Players[2].Bot || !s.Connected[2] {
		t.Fatal("composition not restored")
	}
}

func TestFillBotsSaveFailure(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	if err := a.Create(3, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	revision := a.Status().View.Revision
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	a.store.Dir = file
	if _, err := a.FillBots(); err == nil {
		t.Fatal("save failure ignored")
	}
	s := a.Status()
	if s.View.Revision != revision || s.View.Players[1].Bot || s.Connected[1] {
		t.Fatal("failed mutation committed")
	}
}
