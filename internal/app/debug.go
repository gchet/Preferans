package app

import (
	"fmt"
	"preferans/internal/game"
	"preferans/locales"
)

func debugBotKey(s *game.State) string {
	return fmt.Sprintf("%s/%d", s.ID, s.Revision)
}

// Each permission covers exactly one revision. Hidden hands remain host-local.
func (a *App) debugWaitingLocked() *int {
	s := a.saved.State
	if s == nil || a.pausedLocked() || !a.appearance.debugPhase(s.Stage) || s.Stage == "trick" || a.debugBotPermit == debugBotKey(s) || a.ai.pending != nil || a.ai.review != nil {
		return nil
	}
	seat := s.Actor()
	if seat < 0 || !s.Players[seat].Bot {
		return nil
	}
	return &seat
}

func (a *App) DebugBotOptions(seat int, revision uint64, cards []game.Card) (game.BotSituation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	waiting := a.debugWaitingLocked()
	if waiting == nil || *waiting != seat || a.saved.State.Revision != revision {
		return game.BotSituation{}, locales.Errorf("go.debug.turn_unavailable")
	}
	if len(cards) != 0 && (len(cards) != 2 || a.saved.State.Stage != "discard") {
		return game.BotSituation{}, locales.Errorf("go.debug.turn_unavailable")
	}
	situation, _ := a.saved.State.BotChoices(seat, cards)
	return situation, nil
}

func (a *App) DebugBotContinue(seat int, revision uint64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	waiting := a.debugWaitingLocked()
	if waiting == nil || *waiting != seat || a.saved.State.Revision != revision {
		return locales.Errorf("go.debug.turn_unavailable")
	}
	a.debugBotPermit = debugBotKey(a.saved.State)
	a.broadcastLocked()
	return nil
}

func (a *App) DebugBotCommand(c game.Command) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	waiting := a.debugWaitingLocked()
	if waiting == nil || *waiting != c.Seat || a.saved.State.Revision != c.Revision {
		return locales.Errorf("go.debug.turn_unavailable")
	}
	// The ordinary engine validates ownership, turn, card suit and contract.
	c.ID = game.ID()
	c.Round = a.saved.State.Round
	return a.commandLocked(c)
}

type debugInspection struct {
	table  string
	seat   int
	resume bool
}

// Inspection is local to the host; hidden cards never enter peer views or AI prompts.
func (a *App) ToggleDebugInspection(seat int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.saved.State
	if s == nil || !a.appearance.debugPhase(s.Stage) || seat < 0 || seat >= len(s.Players) || !s.Players[seat].Bot || (s.Stage != "auction" && s.Stage != "play" && s.Stage != "trick") {
		return locales.Errorf("go.debug.unavailable")
	}
	if a.debugInspection != nil && a.debugInspection.table == s.ID {
		if a.debugInspection.seat == seat {
			return a.closeDebugInspectionLocked()
		}
		a.debugInspection.seat = seat
		return nil
	}
	resume := !s.ManualPaused
	if resume {
		if err := a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: s.Revision, Action: "pause"}); err != nil {
			return err
		}
	}
	a.cancelAILocked()
	a.debugInspection = &debugInspection{table: s.ID, seat: seat, resume: resume}
	return nil
}

func (a *App) closeDebugInspectionLocked() error {
	inspection := a.debugInspection
	if inspection == nil {
		return nil
	}
	s := a.saved.State
	if inspection.resume && s != nil && s.ID == inspection.table && s.ManualPaused && s.PausedBy == 0 {
		if err := a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: s.Revision, Action: "resume-play"}); err != nil {
			return err
		}
	}
	a.debugInspection = nil
	return nil
}
