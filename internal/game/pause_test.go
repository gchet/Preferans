package game

import (
	"testing"
	"time"
)

func TestPauseClockAndCommands(t *testing.T) {
	s, _ := New(3, 30)
	r := DefaultRules()
	r.EndCondition = "time"
	s.Rules = &r
	s.Stage = "auction"
	now := time.Now().Unix()
	s.Deadline = now - 5 // Would be expired without a pause that started earlier.
	s.SetPaused(true, now-20)
	if s.TimeExpired() {
		t.Fatal("clock ran during pause")
	}
	if _, err := Apply(s, Command{ID: ID(), Seat: 0, Revision: s.Revision, Action: "pass"}, nil); err == nil {
		t.Fatal("move accepted on pause")
	}
	s.SetPaused(false, now)
	if s.Deadline != now+15 || s.PauseSeconds != 20 || s.PausedAt != 0 {
		t.Fatalf("bad resume: %+v", s)
	}
	next, err := Apply(s, Command{ID: ID(), Seat: 2, Revision: s.Revision, Action: "pause"}, nil)
	if err != nil || !next.ManualPaused || next.PausedBy != 2 {
		t.Fatalf("pause: %v", err)
	}
	if s.ManualPaused {
		t.Fatal("mutated input")
	}
	v := next.View(1)
	if !v.ManualPaused || v.PausedAt == 0 || v.PausedBy != 2 {
		t.Fatal("pause not shared in view")
	}
	for _, action := range []string{"resume-play", "pause"} {
		for _, seat := range []int{0, 1} {
			if _, err := Apply(next, Command{ID: ID(), Seat: seat, Revision: next.Revision, Action: action}, nil); err == nil {
				t.Fatalf("non-owner %d accepted %s", seat, action)
			}
		}
	}
	if !next.ManualPaused || next.PausedBy != 2 || next.PausedAt != v.PausedAt {
		t.Fatal("rejected command changed pause")
	}
	next, err = Apply(next, Command{ID: ID(), Seat: 2, Revision: next.Revision, Action: "resume-play"}, nil)
	if err != nil || next.ManualPaused || next.PausedAt != 0 {
		t.Fatalf("resume: %v", err)
	}
}
