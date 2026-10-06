package game

import (
	"reflect"
	"testing"
)

func TestBotOptionsRemainLegalAcrossCompleteDeals(t *testing.T) {
	for _, n := range []int{3, 4} {
		for seed := int64(0); seed < 8; seed++ {
			s := testState(t, n)
			if err := s.deal(fixedDeck(seed)); err != nil {
				t.Fatal(err)
			}
			for count := 0; count < 180 && s.Stage != "round" && s.Stage != "finished"; count++ {
				seat := s.Actor()
				before := s.Clone()
				_, choices := s.BotChoices(seat, nil)
				if len(choices) == 0 {
					t.Fatalf("no choices: %d %s", n, s.Stage)
				}
				for _, c := range choices {
					if _, err := Apply(s, c, nil); err != nil {
						t.Fatalf("illegal option: %s %v", s.Stage, err)
					}
				}
				if !reflect.DeepEqual(s, before) {
					t.Fatal("enumeration modified state")
				}
				command := BotCommand(s.View(seat))
				next, err := Apply(s, command, nil)
				if err != nil {
					t.Fatal(err)
				}
				s = next
			}
		}
	}
}
func TestBotContextDoesNotExposeHiddenCards(t *testing.T) {
	s := testState(t, 3)
	_ = s.deal(fixedDeck(1))
	v, choices := s.BotChoices(s.Actor(), nil)
	if len(v.View.Talon) > 0 || len(v.KnownDiscard) > 0 || len(choices) == 0 {
		t.Fatal("unknown talon leaked")
	}
	for i, p := range v.View.Players {
		if i != v.View.Seat && len(p.Cards) > 0 {
			t.Fatal("closed hand leaked")
		}
	}
	s = step(t, s, s.Actor(), "bid", &Contract{Level: 6, Suit: 0})
	s = step(t, s, s.Actor(), "pass", nil)
	s = step(t, s, s.Actor(), "pass", nil)
	v, choices = s.BotChoices(s.Actor(), nil)
	pairs := 0
	for _, c := range choices {
		if c.Action == "declare" {
			pairs++
			if len(c.Cards) != 2 {
				t.Fatal("bad discard")
			}
			_, contracts := s.BotChoices(s.Actor(), c.Cards)
			if len(contracts) == 0 {
				t.Fatal("no declaration options")
			}
		}
	}
	if pairs != 66 || len(v.View.Hand) != 12 {
		t.Fatalf("discard pairs %d", pairs)
	}
}
