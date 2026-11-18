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
				for j, w := range next.Whists[i] {
					// Four players: the talon is thrown in the dealer's face, so the declarer is credited against him.
					want := 0
					if n == 4 && i == d && j == next.Dealer && d != next.Dealer {
						want = 8 * (level - 5)
					}
					if w != want {
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

// Four players: leaving without three throws the talon in the dealer's face automatically.
func TestWithoutThreeThrowsTalonInDealersFace(t *testing.T) {
	for _, mode := range []string{"whists", "mountain"} {
		for _, level := range []int{6, 7} {
			r := DefaultRules()
			r.TalonPenalty = mode
			s := auctionWinner(t, 4, r, Contract{Level: level, Suit: 1})
			if s.Declarer == s.Dealer {
				continue
			}
			declarer, dealer := s.Declarer, s.Dealer
			s = step(t, s, s.Actor(), "without-three", nil)
			l := s.History[len(s.History)-1]
			if !s.TalonShown {
				t.Fatalf("%s %d: talon not thrown", mode, level)
			}
			penalty := 4 * (level - 5)
			if mode == "whists" && l.Whists[declarer][dealer] != 2*penalty || mode == "mountain" && l.Mountain[dealer] != penalty {
				t.Fatalf("%s %d: ledger %+v", mode, level, l)
			}
			if l.Mountain[declarer] != 3*penalty {
				t.Fatalf("%s %d: declarer mountain %v", mode, level, l.Mountain)
			}
		}
	}
	s := auctionWinner(t, 3, DefaultRules(), Contract{Level: 6, Suit: 1})
	s = step(t, s, s.Actor(), "without-three", nil)
	if s.TalonShown {
		t.Fatal("three players have no talon penalty")
	}
}
