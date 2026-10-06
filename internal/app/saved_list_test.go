package app

import (
	"preferans/internal/game"
	"testing"
)

func TestListOnlyUnfinishedHostedGames(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	for _, stage := range []string{"lobby", "play", "finished"} {
		s, _ := game.New(3, 30)
		s.Stage = stage
		if err := a.store.Save(s.ID, Saved{State: s, Table: s.ID}); err != nil {
			t.Fatal(err)
		}
	}
	guestID := game.ID()
	guest := Saved{Table: guestID, Seat: 1, Key: "rejoin-key", View: &game.View{ID: guestID, Stage: "play"}}
	if err := a.store.Save(guestID, guest); err != nil {
		t.Fatal(err)
	}
	pendingID := game.ID()
	if err := a.store.Save(pendingID, Saved{Table: pendingID, Seat: 1, Key: "pending-key"}); err != nil {
		t.Fatal(err)
	}
	saves, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(saves) != 2 {
		t.Fatalf("got %d saves", len(saves))
	}
	for _, s := range saves {
		if !s.Host || s.Stage == "finished" {
			t.Fatal("unwanted save visible")
		}
	}
	var retained Saved
	if err := a.store.Load(guestID, &retained); err != nil || retained.Key != "rejoin-key" {
		t.Fatal("guest key removed")
	}
}

func TestGuestLeaveRetainsSeatAndSave(t *testing.T) {
	host, guest := New(t.TempDir()), New(t.TempDir())
	defer host.Close()
	defer guest.Close()
	_ = host.Configure(nil)
	_ = guest.Configure(nil)
	if err := host.Create(3, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	connect(t, host, guest, 1)
	if _, err := host.FillBots(); err != nil {
		t.Fatal(err)
	}
	host.mu.Lock()
	for i := range host.saved.State.Players {
		host.saved.State.Players[i].Ready = true
	}
	err := host.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: host.saved.State.Revision, Action: "start"})
	host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	id := guest.Status().View.ID
	guest.Leave()
	eventually(t, func() bool { return !host.Status().Connected[1] })
	if !host.Status().Paused || guest.Status().Current {
		t.Fatal("guest leave did not pause host / exit guest")
	}
	var saved Saved
	if err := guest.store.Load(id, &saved); err != nil || saved.Key == "" {
		t.Fatal("rejoin key lost")
	}
	if list, err := guest.List(); err != nil || len(list) != 0 {
		t.Fatal("guest save visible")
	}
	host.mu.Lock()
	reserved := host.saved.Joined[1]
	host.mu.Unlock()
	if !reserved {
		t.Fatal("guest seat freed")
	}
	connect(t, host, guest, 1)
	if host.Status().Paused {
		t.Fatal("host stayed paused after rejoin")
	}
}
