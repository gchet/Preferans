package app

import (
	"reflect"
	"testing"

	"preferans/internal/game"
)

func setDealerWhistPlayState(s *game.State, dealer int) {
	s.Stage = "play"
	s.Round = 1
	s.Dealer = dealer
	s.Turn = (dealer + 1) % 4
	s.Declarer = (dealer + 3) % 4
	s.Contract = &game.Contract{Level: 6, Suit: 1}
	s.Defenders = []int{(dealer + 1) % 4, (dealer + 2) % 4}
	s.Defence = make([]int, 4)
	s.Defence[dealer] = 2
	s.Defence[s.Defenders[0]] = 1
	s.Defence[s.Defenders[1]] = 1
	s.Open = true
	s.DealerWhist = true
	s.Controller = dealer
	s.Hands = make([][]game.Card, 4)
	card := game.Card(0)
	for seat := range s.Players {
		if seat == dealer {
			continue
		}
		s.Hands[seat] = make([]game.Card, 10)
		for i := range s.Hands[seat] {
			s.Hands[seat][i] = card
			card++
		}
	}
	s.Talon = []game.Card{30, 31}
	s.Taken = make([]int, 4)
}

func TestDealerWhistControlSurvivesReconnectPauseAndResume(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()

	s, err := game.New(4, 30)
	if err != nil {
		t.Fatal(err)
	}
	setDealerWhistPlayState(s, 1)
	for seat := range s.Players {
		s.Players[seat].Bot = seat >= 2
	}

	saved := Saved{Table: s.ID, State: s, Keys: []string{"host", "dealer", "bot-2", "bot-3"}, Joined: []bool{true, true, true, true}}
	if err := a.store.Save(s.ID, saved); err != nil {
		t.Fatal(err)
	}
	if err := a.Resume(s.ID); err != nil {
		t.Fatal(err)
	}

	// Recreate the pause/unpause transitions caused by a participant losing
	// and restoring their connection while the dealer is controlling defence.
	a.mu.Lock()
	a.connected[1] = true
	a.connected[2] = true
	a.connected[3] = false
	a.syncPauseLocked()
	a.connected[3] = true
	a.syncPauseLocked()
	v := a.viewWithAILocked(s.Dealer)
	if !a.saved.State.DealerWhist || a.saved.State.Controller != s.Dealer || v.Actor != s.Dealer {
		a.mu.Unlock()
		t.Fatal("dealer control was lost while reconnecting")
	}
	wantHand := append([]game.Card(nil), a.saved.State.Hands[s.Turn]...)
	if !reflect.DeepEqual(v.Actions, []string{"play"}) || !reflect.DeepEqual(v.PlayHand, wantHand) {
		a.mu.Unlock()
		t.Fatalf("dealer cannot resume defence: actions=%v playHand=%v", v.Actions, v.PlayHand)
	}
	move := a.saved.State.LegalCards()[0]
	cmd := game.Command{ID: game.ID(), Seat: s.Dealer, Revision: a.saved.State.Revision, Action: "play", Cards: []game.Card{move}}
	if err := a.commandLocked(cmd); err != nil {
		a.mu.Unlock()
		t.Fatalf("dealer's defensive play rejected after reconnect: %v", err)
	}
	a.mu.Unlock()
}

func TestDealerWhistControllerIsPersistedOnResume(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()

	s, err := game.New(4, 30)
	if err != nil {
		t.Fatal(err)
	}
	setDealerWhistPlayState(s, 0)
	for seat := range s.Players {
		s.Players[seat].Bot = seat > 0
	}
	if err := a.store.Save(s.ID, Saved{Table: s.ID, State: s, Keys: make([]string, 4), Joined: make([]bool, 4)}); err != nil {
		t.Fatal(err)
	}
	if err := a.Resume(s.ID); err != nil {
		t.Fatal(err)
	}
	v := a.Status().View
	if !v.DealerWhist || v.Actor != v.Dealer || !reflect.DeepEqual(v.Actions, []string{"play"}) {
		t.Fatalf("dealer controller missing from restored view: actor=%d dealer=%d actions=%v", v.Actor, v.Dealer, v.Actions)
	}
}
