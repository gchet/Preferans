package app

import (
	"preferans/internal/game"
	"testing"
	"time"
)

func TestHistoryRetainsHostAndGuestResults(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	s, _ := game.New(4, 30)
	s.Stage = "finished"
	s.FinishedAt = time.Now().Unix()
	s.StartedAt = s.FinishedAt - 2700
	s.Round = 7
	s.Pool[0] = 10
	s.Mountain[1] = 4
	for _, host := range []bool{true, false} {
		copy := s.Clone()
		copy.ID = game.ID()
		saved := Saved{Table: copy.ID, Seat: 1, PlayerIDs: []string{"host", "guest", "", ""}}
		if host {
			saved.State = copy
		} else {
			v := copy.View(1)
			saved.View = &v
		}
		if err := a.store.Save(copy.ID, saved); err != nil {
			t.Fatal(err)
		}
		v, err := a.HistoryResult(copy.ID)
		if err != nil || v.Stage != "finished" || len(v.Players) != 4 || v.Round != 7 || v.StartedAt != s.StartedAt || v.FinishedAt != s.FinishedAt {
			t.Fatalf("result: %v", err)
		}
	}
	entries, err := a.History()
	if err != nil || len(entries) != 2 {
		t.Fatalf("history: %v %+v", err, entries)
	}
	for _, entry := range entries {
		if len(entry.PlayerIDs) != 4 || entry.PlayerIDs[1] != "guest" || len(entry.Bots) != 4 {
			t.Fatal("history identities lost")
		}
		want := s.View(1).Results
		if len(entry.Results) != len(want) {
			t.Fatal("history results lost")
		}
		for i, value := range want {
			if entry.Results[i] != value {
				t.Fatal("history results changed")
			}
		}
		if entry.StartedAt != s.StartedAt || entry.FinishedAt != s.FinishedAt {
			t.Fatal("history dates lost")
		}
	}
	saves, _ := a.List()
	if len(saves) != 0 {
		t.Fatal("history mixed with resumable saves")
	}
	if a.Status().Current {
		t.Fatal("history opened a table")
	}
}

func TestPoolTargetSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	p := a.Status().Appearance
	p.PoolTarget = 50
	p.PlayerCount = 4
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b := New(dir)
	defer b.Close()
	if b.Status().Appearance.PoolTarget != 50 {
		t.Fatal("target lost")
	}
	if b.Status().Appearance.PlayerCount != 4 {
		t.Fatal("player count lost")
	}
}

func TestHistoryIsScopedToCurrentRoom(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	ids := map[string]string{}
	for _, room := range []string{"room-a", "room-b", ""} {
		s, _ := game.New(3, 30)
		s.Stage = "finished"
		ids[room] = s.ID
		if err := a.store.Save(s.ID, Saved{State: s, Table: s.ID, Room: room}); err != nil {
			t.Fatal(err)
		}
	}
	for _, room := range []string{"room-a", "room-b"} {
		a.mu.Lock()
		a.rooms.config.Current = room
		a.mu.Unlock()
		items, err := a.History()
		if err != nil || len(items) != 1 || items[0].ID != ids[room] {
			t.Fatalf("wrong history for %s: %+v %v", room, items, err)
		}
		for other, id := range ids {
			_, err := a.HistoryResult(id)
			if (err == nil) != (other == room) {
				t.Fatal("cross-room result access")
			}
		}
	}
}

func TestGuestPauseIsSharedAndSurvivesRestore(t *testing.T) {
	host, guest := New(t.TempDir()), New(t.TempDir())
	defer host.Close()
	defer guest.Close()
	_ = host.Configure(nil)
	_ = guest.Configure(nil)
	if err := host.Create(3, 30, "Ведущий", false); err != nil {
		t.Fatal(err)
	}
	connect(t, host, guest, 1)
	_, err := host.FillBots()
	if err != nil {
		t.Fatal(err)
	}
	host.mu.Lock()
	r := game.DefaultRules()
	r.EndCondition = "time"
	host.saved.State.Rules = &r
	for i := range host.saved.State.Players {
		host.saved.State.Players[i].Ready = true
	}
	err = host.commandLocked(game.Command{ID: game.ID(), Revision: host.saved.State.Revision, Action: "start"})
	host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return guest.Status().View.Stage == "auction" })
	v := guest.Status().View
	send(t, []*App{host, guest}, 1, game.Command{ID: game.ID(), Revision: v.Revision, Action: "pause"})
	for _, a := range []*App{host, guest} {
		st := a.Status()
		if !st.Paused || !st.View.ManualPaused || st.View.PausedBy != 1 {
			t.Fatal("pause not shared")
		}
	}
	before := host.Status().View
	time.Sleep(1400 * time.Millisecond)
	after := host.Status().View
	if after.Revision != before.Revision || after.Deadline != before.Deadline || after.PausedAt != before.PausedAt {
		t.Fatal("game advanced while paused")
	}
	host.mu.Lock()
	var restored Saved
	err = host.store.Load(after.ID, &restored)
	host.mu.Unlock()
	if err != nil || !restored.State.ManualPaused || restored.State.PausedBy != 1 {
		t.Fatal("pause not persisted")
	}
	send(t, []*App{host, guest}, 1, game.Command{ID: game.ID(), Revision: guest.Status().View.Revision, Action: "resume-play"})
	after = host.Status().View
	if host.Status().Paused || guest.Status().Paused || after.Deadline <= before.Deadline {
		t.Fatal("resume did not restore shared clock")
	}
}
