package game

import (
	"testing"
	"time"
)

func TestPartyDatesStartAtFirstDealAndPersistThroughFinish(t *testing.T) {
	for _, mode := range []string{"pool", "time"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := New(3, 30)
			r := DefaultRules()
			r.EndCondition = mode
			s.Rules = &r
			if s.StartedAt != 0 {
				t.Fatal("table creation started the game clock")
			}
			for i := range s.Players {
				s.Players[i].Ready = true
			}
			before := time.Now().Unix()
			s, err := Apply(s, Command{ID: ID(), Revision: s.Revision, Action: "start"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if s.StartedAt < before || s.StartedAt > time.Now().Unix() {
				t.Fatal("missing start date")
			}
			if mode == "time" && s.Deadline != s.StartedAt+int64(r.Minutes)*60 {
				t.Fatal("deadline and start disagree")
			}
			started := s.StartedAt
			// Force a timed finish at a round boundary, preserving the start timestamp.
			s.Rules.EndCondition = "time"
			s.Stage = "round"
			s.Deadline = time.Now().Unix() - 1
			s, err = Apply(s, Command{ID: ID(), Revision: s.Revision, Action: "expire"}, nil)
			if err != nil || s.Stage != "finished" || s.FinishedAt < started || s.StartedAt != started {
				t.Fatalf("bad finish: %v", err)
			}
			for seat := range s.Players {
				v := s.View(seat)
				if v.StartedAt != started || v.FinishedAt != s.FinishedAt {
					t.Fatal("dates not shared with all players")
				}
			}
		})
	}
}
