package game

import "testing"

func TestThreePlayerTalonCannotBeThrown(t *testing.T) {
	s := auctionWinner(t, 3, DefaultRules(), Contract{Level: 7, Suit: 1})
	for _, action := range s.View(s.Declarer).Actions {
		if action == "show-talon" {
			t.Fatal("three-player view offers talon throw")
		}
	}
	if _, err := Apply(s, Command{ID: ID(), Seat: s.Declarer, Revision: s.Revision, Action: "show-talon"}, nil); err == nil {
		t.Fatal("three-player talon throw accepted")
	}
}

func TestThrownTalonStaysVisibleThroughEntireDeal(t *testing.T) {
	s := auctionWinner(t, 4, DefaultRules(), Contract{Level: 7, Suit: 1})
	s = step(t, s, s.Declarer, "show-talon", nil)
	s = step(t, s, s.Declarer, "declare", s.Bid)
	for steps := 0; steps < 100; steps++ {
		for seat := range s.Players {
			v := s.View(seat)
			if !v.TalonShown || len(v.Talon) != 2 || v.Talon[0] != s.Talon[0] || v.Talon[1] != s.Talon[1] {
				t.Fatalf("open talon lost at stage %s for seat %d", s.Stage, seat)
			}
		}
		if s.Stage == "round" || s.Stage == "finished" {
			return
		}
		seat := s.Actor()
		if s.Stage == "trick" {
			seat = 0
		}
		c := BotCommand(s.View(seat))
		next, err := Apply(s, c, nil)
		if err != nil {
			t.Fatal(err)
		}
		s = next
	}
	t.Fatal("deal did not finish")
}

func TestThrownTalonToggleAndAutomaticDiscard(t *testing.T) {
	for _, penalty := range []string{"", "mountain"} {
		for _, abandon := range []bool{false, true} {
			s := auctionWinner(t, 4, DefaultRules(), Contract{Level: 7, Suit: 1})
			r := s.Agreements()
			r.TalonPenalty = penalty
			s.Rules = &r
			d := s.Declarer
			s = step(t, s, d, "show-talon", nil)
			s = step(t, s, d, "show-talon", nil)
			if s.TalonShown || len(s.Hands[d]) != 12 {
				t.Fatal("toggle did not restore normal discard")
			}
			if _, err := Apply(s, Command{ID: ID(), Seat: d, Revision: s.Revision, Action: "declare", Contract: s.Bid}, nil); err == nil {
				t.Fatal("normal declaration accepted without discard")
			}
			s = step(t, s, d, "show-talon", nil)
			if abandon {
				s = step(t, s, d, "without-three", nil)
			} else {
				s = step(t, s, d, "declare", &Contract{Level: 7, Suit: 1})
				if len(s.Hands[d]) != 10 || len(s.Discard) != 2 || s.Discard[0] != s.Talon[0] || s.Discard[1] != s.Talon[1] {
					t.Fatal("original talon not discarded")
				}
				s.Taken[d] = 7
				s.TrickNo = 10
				s.score()
			}
			if penalty == "mountain" {
				if s.Mountain[s.Dealer] != 8 || s.Whists[d][s.Dealer] != 0 {
					t.Fatal("incorrect dealer mountain")
				}
			} else if s.Whists[d][s.Dealer] != 16 {
				t.Fatal("incorrect dealer whists")
			}
		}
	}
}

func TestShownTalonScoresAgainstDealer(t *testing.T) {
	for _, n := range []int{4} {
		s := step(t, testState(t, n), 0, "start", nil)
		s = step(t, s, s.Actor(), "bid", &Contract{Level: 6, Suit: 0})
		for s.Stage == "auction" {
			s = step(t, s, s.Actor(), "pass", nil)
		}
		d := s.Declarer
		if _, err := Apply(s, Command{ID: ID(), Seat: s.Dealer, Revision: s.Revision, Action: "show-talon"}, nil); err == nil {
			t.Fatal("dealer revealed another player's talon")
		}
		s = step(t, s, d, "show-talon", nil)
		for p := range s.Players {
			if !s.View(p).TalonShown {
				t.Fatal("reveal not shared")
			}
		}
		s = step(t, s, d, "declare", &Contract{Level: 7, Suit: 0}, s.Hands[d][0], s.Hands[d][1])
		s.Taken[d] = 7
		s.TrickNo = 10
		mountain := s.Clone()
		rules := mountain.Agreements()
		rules.TalonPenalty = "mountain"
		mountain.Rules = &rules
		mountain.score()
		if mountain.Mountain[s.Dealer] != 8 || mountain.Whists[d][s.Dealer] != 0 {
			t.Fatal("mountain variant must replace whists with one trick penalty")
		}
		s.score()
		if s.Whists[d][s.Dealer] != 16 {
			t.Fatalf("bonus must use final game price: %v", s.Whists)
		}
		s = step(t, s, 0, "next", nil)
		if s.TalonShown {
			t.Fatal("reveal leaked into next deal")
		}
	}
}
