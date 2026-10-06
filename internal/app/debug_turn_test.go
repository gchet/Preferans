package app

import (
	"preferans/config"
	"preferans/internal/game"
	"testing"
	"time"
)

func debugTurnApp(t *testing.T, play bool) *App {
	t.Helper()
	a := New(t.TempDir())
	t.Cleanup(a.Close)
	p := a.Status().Appearance
	yes, no := true, false
	p.DebugAuction, p.DebugPlay = &yes, &no
	if play {
		p.DebugAuction, p.DebugPlay = &no, &yes
	}
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	if err := a.Create(3, 30, "Host", true); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	s := a.saved.State
	s.Dealer = 2
	for i := range s.Players {
		s.Players[i].Ready = true
	}
	err := a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: s.Revision, Action: "start"})
	if err == nil {
		s = a.saved.State
		if play {
			s.Stage, s.Turn, s.Declarer = "play", 1, 0
			s.Contract = &game.Contract{Level: 7, Suit: 1}
			s.Defenders, s.Defence = []int{1, 2}, []int{0, 2, 2}
			s.Revision++
		} else {
			err = a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: s.Revision, Action: "bid", Contract: &game.Contract{Level: 7, Suit: 1}})
		}
	}
	a.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestDebugBotWaitsAndHostMayChooseLegalAction(t *testing.T) {
	for _, play := range []bool{false, true} {
		a := debugTurnApp(t, play)
		v := a.Status().View
		if v.DebugWaitingSeat == nil || *v.DebugWaitingSeat != 1 {
			t.Fatal("bot did not wait")
		}
		time.Sleep(time.Duration(config.Interface.BotTickIntervalMs*3) * time.Millisecond)
		if a.Status().View.Revision != v.Revision {
			t.Fatal("bot moved before permission")
		}
		situation, err := a.DebugBotOptions(1, v.Revision, nil)
		if err != nil || len(situation.Choices) == 0 || len(situation.View.Hand) != 10 {
			t.Fatal("missing manual choices", err)
		}
		a.mu.Lock()
		peer := a.viewWithAILocked(2)
		wrongCard := a.saved.State.Hands[0][0]
		a.mu.Unlock()
		if peer.DebugWaitingSeat != nil || len(peer.Players[1].Cards) != 0 {
			t.Fatal("host debug data leaked")
		}
		if _, err = a.DebugBotOptions(2, v.Revision, nil); err == nil {
			t.Fatal("inspected nonacting bot")
		}
		if err = a.DebugBotContinue(1, v.Revision-1); err == nil {
			t.Fatal("continued stale turn")
		}
		command := game.Command{Seat: 1, Revision: v.Revision, Action: "pass"}
		if play {
			command.Action = "play"
			command.Cards = []game.Card{wrongCard}
			if err = a.DebugBotCommand(command); err == nil {
				t.Fatal("host played unowned card")
			}
			command.Cards = situation.Choices[0].Cards
		}
		if err = a.DebugBotCommand(command); err != nil {
			t.Fatal(err)
		}
		next := a.Status().View
		if next.Revision != v.Revision+1 || next.DebugWaitingSeat == nil || *next.DebugWaitingSeat != 2 {
			t.Fatal("next bot did not wait separately")
		}
		if err = a.DebugBotContinue(2, next.Revision); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(4 * time.Second)
		for a.Status().View.Revision == next.Revision && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if a.Status().View.Revision == next.Revision {
			t.Fatal("continued bot failed to act")
		}
		if err = a.DebugBotContinue(2, next.Revision); err == nil {
			t.Fatal("replayed permission accepted")
		}
	}
}

func TestDebugTurnPhasesPersistAndDisableReleasesBot(t *testing.T) {
	a := debugTurnApp(t, false)
	p := a.Status().Appearance
	if !p.debugPhase("auction") || p.debugPhase("play") {
		t.Fatal("phase flags not independent")
	}
	no := false
	p.DebugAuction = &no
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	if a.Status().View.DebugWaitingSeat != nil {
		t.Fatal("disabled debug still holds bot")
	}
	var restored Appearance
	if err := a.config.Load("appearance", &restored); err != nil || restored.debugEnabled() {
		t.Fatal("flags not persisted", err)
	}
	guest := New(t.TempDir())
	defer guest.Close()
	if _, err := guest.DebugBotOptions(1, 0, nil); err == nil {
		t.Fatal("guest can inspect bot")
	}
}

func TestDebugBotManualDiscardThenContract(t *testing.T) {
	a := debugTurnApp(t, false)
	a.mu.Lock()
	s := a.saved.State
	s.Stage, s.Turn, s.Declarer = "discard", 1, 1
	s.Bid = &game.Contract{Level: 6, Suit: 0}
	s.Hands[1] = append(s.Hands[1], s.Talon...)
	s.Revision++
	revision := s.Revision
	a.mu.Unlock()
	first, err := a.DebugBotOptions(1, revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	var cards []game.Card
	pairs := 0
	for _, option := range first.Choices {
		if option.Action == "declare" {
			pairs++
			cards = option.Cards
		}
	}
	if pairs != 66 {
		t.Fatal("discard choices missing", pairs)
	}
	second, err := a.DebugBotOptions(1, revision, cards)
	if err != nil {
		t.Fatal(err)
	}
	var command game.Command
	for _, option := range second.Choices {
		if option.Action == "declare" && option.Contract.Level == 7 && option.Contract.Suit == 1 {
			command = game.Command{Seat: 1, Revision: revision, Action: option.Action, Cards: option.Cards, Contract: option.Contract}
		}
	}
	if command.Contract == nil {
		t.Fatal("no contract choice after discard")
	}
	if err = a.DebugBotCommand(command); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.saved.State.Hands[1]) != 10 || a.saved.State.Contract.Level != 7 || len(a.saved.State.Discard) != 2 {
		t.Fatal("manual discard/contract not applied")
	}
}
