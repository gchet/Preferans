package game

import (
	"context"
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
					if v.Stage == "auction" || v.Stage == "discard" {
						situation, _ := s.BotChoices(seat, nil)
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						found, stats := SearchBotCommand(ctx, situation, SearchOptions{SimSamples: 8, Seed: seed*1000 + int64(step)})
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
