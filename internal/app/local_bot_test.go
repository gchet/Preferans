package app

import (
	"preferans/internal/game"
	"testing"
	"time"
)

func TestLocalSearchAppliesOnceAndRespectsPause(t *testing.T) {
	for _, pause := range []bool{false, true} {
		a := New(t.TempDir())
		if err := a.Create(3, 30, "Host", true); err != nil {
			t.Fatal(err)
		}
		a.mu.Lock()
		for seat := range a.saved.State.Players {
			a.saved.State.Players[seat].Bot = false
			a.saved.State.Players[seat].Ready = true
			a.connected[seat] = true
		}
		err := a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: a.saved.State.Revision, Action: "start"})
		for err == nil && a.saved.State.Stage == "auction" {
			s := a.saved.State
			err = a.commandLocked(game.Command{ID: game.ID(), Seat: s.Actor(), Revision: s.Revision, Action: "pass"})
		}
		if err != nil {
			a.mu.Unlock()
			a.Close()
			t.Fatal(err)
		}
		// Pick a hand with at least two legal options before starting the async worker.
		for len(a.saved.State.LegalCards()) < 2 {
			err = a.commandLocked(game.BotCommand(a.saved.State.View(a.saved.State.Actor())))
			if err != nil {
				a.mu.Unlock()
				a.Close()
				t.Fatal(err)
			}
		}
		s := a.saved.State
		a.startLocalSearchLocked(s, s.Actor())
		if a.ai.pending == nil {
			a.mu.Unlock()
			a.Close()
			t.Fatal("search not started")
		}
		if pause {
			err = a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: s.Revision, Action: "pause"})
			if err != nil {
				a.mu.Unlock()
				a.Close()
				t.Fatal(err)
			}
		}
		revision := a.saved.State.Revision
		a.mu.Unlock()
		eventually(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.ai.pending == nil })
		a.mu.Lock()
		got := a.saved.State.Revision
		a.mu.Unlock()
		if pause && got != revision {
			a.Close()
			t.Fatal("applied search during pause")
		}
		if !pause && got != revision+1 {
			a.Close()
			t.Fatal("search not applied exactly once")
		}
		time.Sleep(30 * time.Millisecond)
		a.mu.Lock()
		got = a.saved.State.Revision
		a.mu.Unlock()
		if !pause && got != revision+1 {
			a.Close()
			t.Fatal("search action repeated")
		}
		a.Close()
	}
}

func TestLocalBotRespondsToClaimAsynchronously(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	if err := a.Create(3, 30, "Host", true); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	s := a.saved.State
	s.Stage = "play"
	s.Round = 1
	s.TrickNo = 9
	s.Turn = 0
	s.Declarer = 0
	s.Contract = &game.Contract{Level: 8, Suit: 0}
	s.Open = true
	s.Controller = 1
	s.Defenders = []int{1, 2}
	s.Defence = []int{0, 2, 1}
	s.Hands = [][]game.Card{{7}, {0}, {6}}
	s.Taken = []int{8, 1, 0}
	s.Players[0].Bot = false
	for seat := range s.Players {
		a.connected[seat] = true
	}
	err := a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: s.Revision, Action: "claim", Tricks: 1})
	if err == nil {
		a.localBotLocked(a.saved.State, 1, "")
	}
	pending := a.ai.pending != nil
	a.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Fatal("claim solver was not scheduled")
	}
	eventually(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.ai.pending == nil })
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.saved.State.Stage != "round" || a.saved.State.Taken[0] != 9 {
		t.Fatal("bot did not accept proven claim")
	}
}
