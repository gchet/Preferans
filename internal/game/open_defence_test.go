package game

import "testing"

func TestMisereDefenceRevealedAfterDeclarersOpeningLead(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, talon := range []bool{true, false} {
			r := DefaultRules()
			r.MisereTalon = talon
			s := auctionWinner(t, n, r, Contract{Misere: true, Suit: 4, NoTalon: !talon})
			d := s.Declarer
			if talon {
				s = step(t, s, d, "declare", s.Bid, s.Hands[d][0], s.Hands[d][1])
			}
			a, b := s.Defenders[0], s.Defenders[1]
			s = step(t, s, a, "catch", nil)
			s = step(t, s, b, "trust", nil)
			if s.Turn != d || !s.Open {
				t.Fatal("expected declarer's first lead")
			}
			s = s.Clone()
			for seat := range s.Players {
				v := s.View(seat)
				for _, defender := range s.Defenders {
					want := 0
					if seat == defender {
						want = 10
					}
					if len(v.Players[defender].Cards) != want {
						t.Fatal("defence revealed before first card")
					}
				}
				if seat != d && len(v.PlayHand) != 0 {
					t.Fatal("other defender hand leaked through PlayHand")
				}
			}
			s = step(t, s, d, "play", nil, s.LegalCards()[0])
			if len(s.Trick) != 1 {
				t.Fatal("expected just one card on table")
			}
			for seat := range s.Players {
				for _, defender := range s.Defenders {
					if len(s.View(seat).Players[defender].Cards) != 10 {
						t.Fatal("defence still hidden after first card")
					}
				}
			}
			s.TrickNo = 1
			s.Trick = nil
			s.Turn = d
			if len(s.View(d).Players[a].Cards) != 10 {
				t.Fatal("later lead hid defence")
			}
			s.TrickNo = 0
			s.Declarer = b
			s.Turn = d
			if len(s.View(b).Players[a].Cards) != 10 {
				t.Fatal("defender's first lead must open defence immediately")
			}
		}
	}
}

func TestOpenDefenceRevealedAfterDeclarersOpeningLead(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, passedBot := range []bool{false, true} {
			s := auctionWinner(t, n, DefaultRules(), Contract{Level: 7, Suit: 1})
			d := s.Declarer
			s = step(t, s, d, "declare", s.Bid, s.Hands[d][0], s.Hands[d][1])
			w, p := s.Defenders[0], s.Defenders[1]
			s.Players[p].Bot = passedBot
			s = step(t, s, w, "whist", nil)
			s = step(t, s, p, "pass", nil)
			s = step(t, s, w, "whist", nil)
			s = step(t, s, w, "open", nil)
			if s.Turn != d || !s.Open {
				t.Fatal("expected declarer's opening lead in open defence")
			}
			for _, defender := range s.Defenders {
				if len(s.View(d).Players[defender].Cards) != 0 {
					t.Fatal("defender cards leaked before opening lead")
				}
			}
			for seat := range s.Players {
				v := s.View(seat)
				for other := range s.Players {
					if seat != other && len(v.Players[other].Cards) != 0 {
						t.Fatalf("seat %d sees seat %d before opening lead", seat, other)
					}
				}
				if len(v.Hand) != len(s.Hands[seat]) || len(v.Players[seat].Cards) != len(s.Hands[seat]) {
					t.Fatal("own hand hidden")
				}
			}
			restored := s.Clone()
			if len(restored.View(d).Players[w].Cards) != 0 {
				t.Fatal("restore exposed defence")
			}
			s = step(t, s, d, "play", nil, s.LegalCards()[0])
			for _, defender := range s.Defenders {
				for seat := range s.Players {
					if len(s.View(seat).Players[defender].Cards) != 10 {
						t.Fatal("defence not revealed to everyone immediately after first card")
					}
				}
			}
			// On subsequent leads the open hands must stay visible.
			s.TrickNo = 1
			s.Trick = nil
			s.Turn = d
			if len(s.View(d).Players[w].Cards) != 10 {
				t.Fatal("later lead hid defence")
			}
			// When a defender has the opening lead, there is no deferred reveal.
			s.TrickNo = 0
			s.Declarer = p
			s.Turn = d
			if len(s.View(p).Players[w].Cards) != 10 {
				t.Fatal("defender opening lead hid defence")
			}
		}
	}
}
