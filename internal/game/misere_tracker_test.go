package game

import (
	"reflect"
	"testing"
)

func TestMisereTrackerDoesNotRevealDiscard(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, talon := range []bool{true, false} {
			r := DefaultRules()
			r.MisereTalon = talon
			s := auctionWinner(t, n, r, Contract{Misere: true, Suit: 4, NoTalon: !talon})
			original := append([]Card{}, s.Hands[s.Declarer]...)
			if talon {
				s = step(t, s, s.Declarer, "declare", s.Bid, original[0], original[1])
			} else {
				original = append(original, s.Talon...)
			}
			sortHand(original)
			if !reflect.DeepEqual(s.MisereCards, original) {
				t.Fatal("not the original complete hand")
			}
			if len(s.MisereCards) != 12 {
				t.Fatal("tracker must always contain twelve cards")
			}
			a, b := s.Defenders[0], s.Defenders[1]
			s = step(t, s, a, "catch", nil)
			s = step(t, s, b, "trust", nil)
			if !talon {
				old := s.Clone()
				old.MisereCards = append([]Card{}, s.Hands[s.Declarer]...)
				if !reflect.DeepEqual(old.View(a).MisereCards, original) {
					t.Fatal("old ten-card tracker was not upgraded")
				}
			}
			for _, seat := range []int{a, b} {
				v := s.View(seat)
				if !reflect.DeepEqual(v.MisereCards, original) || len(v.MiserePlayed) != 0 {
					t.Fatal("discard was marked or cards lost")
				}
				if len(v.Players[s.Declarer].Cards) != 0 {
					t.Fatal("exact remaining declarer hand leaked")
				}
			}
			if len(s.View(s.Declarer).MisereCards) != 0 {
				t.Fatal("tracker shown to declarer")
			}
			if n == 4 && len(s.View(s.Dealer).MisereCards) != 0 {
				t.Fatal("tracker shown to dealer")
			}
			for s.Stage == "play" {
				c := BotCommand(s.View(s.Actor()))
				var err error
				s, err = Apply(s, c, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			if s.Stage != "trick" || len(s.MiserePlayed) != 1 {
				t.Fatal("played declarer card not recorded")
			}
			for _, card := range s.Discard {
				if card == s.MiserePlayed[0] {
					t.Fatal("discard marked as played")
				}
			}
			s = step(t, s, 0, "collect", nil)
			s = s.Clone()
			if !reflect.DeepEqual(s.View(a).MiserePlayed, s.MiserePlayed) || len(s.MiserePlayed) != 1 {
				t.Fatal("mark lost after collecting/saving")
			}
			s.Stage = "round"
			s = step(t, s, 0, "next", nil)
			if len(s.MisereCards) != 0 || len(s.MiserePlayed) != 0 {
				t.Fatal("tracker leaked into next deal")
			}
		}
	}
}
