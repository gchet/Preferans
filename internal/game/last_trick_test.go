package game

import (
	"reflect"
	"testing"
)

func TestLastTrickPersistsUntilNextCardOrDeal(t *testing.T) {
	for _, n := range []int{3, 4} {
		s := testState(t, n)
		s = step(t, s, 0, "start", nil)
		for s.Stage == "auction" {
			s = step(t, s, s.Actor(), "pass", nil)
		}
		for s.Stage == "play" {
			s = step(t, s, s.Actor(), "play", nil, s.LegalCards()[0])
		}
		trick := append([]Play(nil), s.Trick...)
		s = step(t, s, 0, "collect", nil)
		s = s.Clone()
		for seat := range s.Players {
			if !reflect.DeepEqual(s.View(seat).LastTrick, trick) {
				t.Fatal("previous trick missing after restore")
			}
		}
		next := step(t, s, s.Actor(), "play", nil, s.LegalCards()[0])
		if len(next.LastTrick) != 0 {
			t.Fatal("previous trick survived the next player's card")
		}
		// The final trick must also remain available until a new deal.
		s.Stage = "trick"
		s.TrickNo = 9
		s.Trick = trick
		s = step(t, s, 0, "collect", nil)
		if !reflect.DeepEqual(s.LastTrick, trick) {
			t.Fatal("final trick was lost")
		}
		if err := s.deal(fixedDeck(1)); err != nil {
			t.Fatal(err)
		}
		if len(s.LastTrick) != 0 {
			t.Fatal("previous trick leaked into auction")
		}
	}
}
