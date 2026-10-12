package app

import (
	"preferans/internal/game"
	"testing"
	"time"
)

// prepareTrick starts a game with bots until a trick is waiting to be collected.
// Bots never whist, so a deal that gets a contract ends without any trick; such a
// deal is simply replaced by a new one.
func prepareTrick(t *testing.T) (*App, uint64) {
	for attempt := 0; attempt < 60; attempt++ {
		a := New(t.TempDir())
		if err := a.Create(3, 30, "Host", true); err != nil {
			t.Fatal(err)
		}
		a.mu.Lock()
		for i := range a.saved.State.Players {
			a.saved.State.Players[i].Bot = false
			a.saved.State.Players[i].Ready = true
		}
		err := a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: a.saved.State.Revision, Action: "start"})
		for step := 0; err == nil && a.saved.State.Stage != "trick" && step < 100; step++ {
			v := a.saved.State.View(a.saved.State.Actor())
			err = a.commandLocked(game.BotCommand(v))
		}
		stage := a.saved.State.Stage
		revision := a.saved.State.Revision
		a.connected[1] = false
		a.mu.Unlock()
		if err == nil && stage == "trick" {
			return a, revision
		}
		a.Close()
	}
	t.Fatal("no deal reached a trick")
	return nil, 0
}

func TestAutomaticTrickCollectionWaitsForConnection(t *testing.T) {
	a, revision := prepareTrick(t)
	defer a.Close()
	time.Sleep(2800 * time.Millisecond)
	if a.Status().View.Revision != revision {
		t.Fatal("collected while a human was disconnected")
	}
	a.mu.Lock()
	a.connected[1] = true
	a.mu.Unlock()
	eventually(t, func() bool { return a.Status().View.Revision > revision })
	if a.Status().View.TrickNo != 1 {
		t.Fatal("trick was not collected exactly once")
	}
}
