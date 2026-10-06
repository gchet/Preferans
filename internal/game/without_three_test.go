package game

import "testing"

func TestWithoutThreeBeforeDiscard(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, level := range []int{6, 7, 8} {
			s := step(t, testState(t, n), 0, "start", nil)
			s = step(t, s, s.Actor(), "bid", &Contract{Level: level, Suit: 0})
			for s.Stage == "auction" {
				s = step(t, s, s.Actor(), "pass", nil)
			}
			d := s.Declarer
			s.PassStreak = 2
			if len(s.Hands[d]) != 12 {
				t.Fatal("expected hand before discard")
			}
			available := false
			for _, a := range s.View(d).Actions {
				available = available || a == "without-three"
			}
			if available != (level <= 7) {
				t.Fatal("incorrect availability")
			}
			c := Command{ID: ID(), Seat: d, Revision: s.Revision, Action: "without-three"}
			c.Seat = (d + 1) % n
			if _, err := Apply(s, c, nil); err == nil {
				t.Fatal("other player surrendered")
			}
			c.Seat = d
			next, err := Apply(s, c, nil)
			if level == 8 {
				if err == nil {
					t.Fatal("eight allowed")
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if next.Stage != "round" || len(next.History) != 1 || next.Mountain[d] != 12*(level-5) || next.PassStreak != 2 {
				t.Fatal("incorrect result")
			}
			for i := range next.Players {
				if next.Pool[i] != 0 {
					t.Fatal("pool awarded")
				}
				for _, w := range next.Whists[i] {
					if w != 0 {
						t.Fatal("whists awarded")
					}
				}
			}
			again, err := Apply(next, c, nil)
			if err != nil || len(again.History) != 1 {
				t.Fatal("duplicate surrender")
			}
		}
	}
}
