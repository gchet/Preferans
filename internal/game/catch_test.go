package game

import "testing"

func TestChooseMisereController(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, talon := range []bool{false, true} {
			r := DefaultRules()
			r.MisereTalon = talon
			s := auctionWinner(t, n, r, Contract{Misere: true, Suit: 4, NoTalon: !talon})
			if talon {
				s = step(t, s, s.Declarer, "declare", s.Bid, s.Hands[s.Declarer][0], s.Hands[s.Declarer][1])
			}
			if s.Stage != "catch" || s.Open {
				t.Fatal("missing catcher selection")
			}
			a, b := s.Defenders[0], s.Defenders[1]
			for _, p := range []int{a, b} {
				if len(s.View(p).Actions) != 2 {
					t.Fatal("both defenders need buttons")
				}
			}
			if len(s.View(s.Declarer).Actions) != 0 {
				t.Fatal("declarer can choose catcher")
			}
			if _, err := Apply(s, Command{ID: ID(), Seat: s.Declarer, Revision: s.Revision, Action: "catch"}, nil); err == nil {
				t.Fatal("declarer selected catcher")
			}
			s = step(t, s, b, "catch", nil)
			if s.Stage != "catch" {
				t.Fatal("did not wait for second choice")
			}
			s = step(t, s, a, "catch", nil)
			if s.Stage != "catch" {
				t.Fatal("two catchers started play")
			}
			s = s.Clone()
			s = step(t, s, a, "trust", nil)
			if s.Stage != "play" || !s.Open || s.Controller != b {
				t.Fatal("wrong controller")
			}
			for _, hand := range []int{a, b} {
				s.Turn = hand
				s.Trick = nil
				v := s.View(b)
				if s.Actor() != b || len(v.PlayHand) != 10 || len(v.Legal) == 0 {
					t.Fatal("controller cannot play both hands")
				}
				if hand == a {
					if _, err := Apply(s, Command{ID: ID(), Seat: a, Revision: s.Revision, Action: "play", Cards: s.LegalCards()[:1]}, nil); err == nil {
						t.Fatal("trusted player played independently")
					}
				}
			}
		}
	}
}

func TestBothTrustAndBotsResolveCatcher(t *testing.T) {
	s := auctionWinner(t, 3, DefaultRules(), Contract{Misere: true, Suit: 4})
	s = step(t, s, s.Declarer, "declare", s.Bid, s.Hands[s.Declarer][0], s.Hands[s.Declarer][1])
	a, b := s.Defenders[0], s.Defenders[1]
	s = step(t, s, a, "trust", nil)
	c := Command{ID: ID(), Seat: b, Revision: s.Revision - 1, Round: s.Round, Action: "trust"}
	var err error
	s, err = Apply(s, c, nil)
	if err != nil {
		t.Fatal("concurrent choice rejected:", err)
	}
	if s.Stage != "catch" {
		t.Fatal("no catcher started play")
	}
	for i := 0; i < 3 && s.Stage == "catch"; i++ {
		c := BotCommand(s.View(s.Actor()))
		var err error
		s, err = Apply(s, c, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if s.Stage != "play" || s.Controller < 0 {
		t.Fatal("bots did not agree")
	}
}
