package app

import (
	"preferans/internal/game"
	"testing"
)

func TestDebugInspectionPrivacyPauseAndPreferences(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	if a.Status().Appearance.Debug {
		t.Fatal("debug enabled by default")
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
	a.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if a.ToggleDebugInspection(1) == nil {
		t.Fatal("inspection permitted without debug")
	}
	p := a.Status().Appearance
	p.Debug = true
	if err = a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"auction", "play", "trick"} {
		a.mu.Lock()
		a.saved.State.Stage = stage
		a.mu.Unlock()
		if err = a.ToggleDebugInspection(1); err != nil {
			t.Fatal(stage, err)
		}
		v := a.Status().View
		if !v.ManualPaused || v.DebugBotSeat == nil || *v.DebugBotSeat != 1 || len(v.Players[1].Cards) != 10 {
			t.Fatal("inspection did not pause/reveal", stage)
		}
		a.mu.Lock()
		peer := a.viewWithAILocked(2)
		regular := a.saved.State.View(0)
		a.mu.Unlock()
		if peer.DebugBotSeat != nil || len(peer.Players[1].Cards) != 0 || len(regular.Players[1].Cards) != 0 {
			t.Fatal("debug cards leaked to peer or regular bot context")
		}
		if err = a.ToggleDebugInspection(2); err != nil {
			t.Fatal(err)
		}
		if *a.Status().View.DebugBotSeat != 2 {
			t.Fatal("switching inspection failed")
		}
		if err = a.ToggleDebugInspection(2); err != nil {
			t.Fatal(err)
		}
		v = a.Status().View
		if v.ManualPaused || v.DebugBotSeat != nil || len(v.Players[2].Cards) != 0 {
			t.Fatal("closing did not resume and hide")
		}
	}
	v := a.Status().View
	if err = a.Command(game.Command{ID: game.ID(), Revision: v.Revision, Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	if err = a.ToggleDebugInspection(1); err != nil {
		t.Fatal(err)
	}
	if err = a.ToggleDebugInspection(1); err != nil {
		t.Fatal(err)
	}
	if !a.Status().View.ManualPaused {
		t.Fatal("preexisting pause was cleared")
	}
	v = a.Status().View
	if err = a.Command(game.Command{ID: game.ID(), Revision: v.Revision, Action: "resume-play"}); err != nil {
		t.Fatal(err)
	}
	if err = a.ToggleDebugInspection(1); err != nil {
		t.Fatal(err)
	}
	p.Debug = false
	if err = a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	if a.Status().View.ManualPaused || a.Status().View.DebugBotSeat != nil {
		t.Fatal("disabling debug retained inspection pause")
	}
}
