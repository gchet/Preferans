package game

import (
	"context"
	"math"
	"testing"
	"time"
)

// Bidding and the discard chosen by simulation must be accepted by the engine, and
// SearchBotCommand must actually use the simulation for those stages.
func TestSimBotCommandsAreLegal(t *testing.T) {
	simulated := 0
	for _, players := range []int{3, 4} {
		for seed := int64(1); seed <= 6; seed++ {
			s, err := New(players, 30)
			if err != nil {
				t.Fatal(err)
			}
			for i := range s.Players {
				s.Players[i].Ready = true
			}
			if err := s.deal(fixedDeck(seed)); err != nil {
				t.Fatal(err)
			}
			for step := 0; step < 600; step++ {
				switch s.Stage {
				case "auction", "contract", "discard", "defend", "option", "return", "mode", "play", "trick", "dealer-choice", "dealer-whist", "catch":
				default:
					step = 600
					continue
				}
				var command Command
				if s.Stage == "trick" {
					command = Command{Seat: 0, Action: "collect"}
				} else {
					seat := s.Actor()
					v := s.View(seat)
					command = BotCommand(v)
					if v.Stage == "auction" || v.Stage == "discard" || v.Stage == "defend" {
						situation, _ := s.BotChoices(seat, nil)
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						found, stats := SearchBotCommand(ctx, situation, SearchOptions{SimSamples: 8, OwnWhist: true, Seed: seed*1000 + int64(step)})
						cancel()
						if stats.Samples > 0 {
							simulated++
							command = found
						}
					}
					command.Seat = seat
				}
				command.ID = ID()
				command.Revision = s.Revision
				next, err := Apply(s, command, nil)
				if err != nil {
					t.Fatalf("%d players, seed %d, stage %s, action %s: %v", players, seed, s.Stage, command.Action, err)
				}
				s = next
			}
		}
	}
	if simulated == 0 {
		t.Fatal("simulation never decided a bid or discard")
	}
}

// A cancelled context must fall back to the default bot instead of returning a
// half-evaluated decision.
func TestSimBotCommandHonoursCancellation(t *testing.T) {
	s, err := New(3, 30)
	if err != nil {
		t.Fatal(err)
	}
	for i := range s.Players {
		s.Players[i].Ready = true
	}
	if err := s.deal(fixedDeck(1)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, ok := SimBotCommand(ctx, s.View(s.Actor()), 1, 8); ok {
		t.Fatal("cancelled context still produced a simulated decision")
	}
}

// A declarer hand that is worth exactly what the bid says gets full weight; one
// that is off by two tricks gets exp(-2) with sigma 1; no belief means no weighting.
func TestSimBeliefWeight(t *testing.T) {
	hand := []Card{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	contract := Contract{Level: 7, Suit: 0}
	estimate := handTricks(hand, contract.Suit)
	off := &SimOptions{Belief: 1}
	if got := simSimBelief(off, hand, contract); got != math.Exp(-math.Pow(estimate-7, 2)/2) {
		t.Fatalf("weight %v does not follow the gaussian around the level", got)
	}
	centred := &SimOptions{Belief: 1, BeliefShift: estimate - 7}
	if got := simSimBelief(centred, hand, contract); got != 1 {
		t.Fatalf("centred weight = %v, want 1", got)
	}
	shifted := &SimOptions{Belief: 1, BeliefShift: estimate - 7 + 2}
	if got := simSimBelief(shifted, hand, contract); math.Abs(got-math.Exp(-2)) > 1e-12 {
		t.Fatalf("shifted weight = %v, want exp(-2)", got)
	}
	if got := simSimBelief(&SimOptions{}, hand, contract); got != 1 {
		t.Fatalf("weight without belief = %v, want 1", got)
	}
}

func simSimBelief(opt *SimOptions, hand []Card, c Contract) float64 {
	return simBeliefWeight(opt, hand, c)
}
