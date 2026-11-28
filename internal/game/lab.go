package game

import (
	"context"
	"fmt"
	"math/rand"
	"preferans/config"
	"time"
)

// The lab runs whole deals without any UI so that bot policies can be compared
// phase by phase (bidding, discard, defence, card play). It is used by
// cmd/botlab; nothing here is reachable from the running application.

// LabSearch holds the card-play search settings of one policy.
type LabSearch struct {
	Samples, ExactCards, Msec int
}

// LabContext is what a hook may look at when it decides. State is the
// authoritative state: an honest policy uses View (and Situation) only, a
// deliberately omniscient "oracle" policy used to measure headroom may use State.
type LabContext struct {
	State     *State
	View      View
	Seat      int
	Seed      int64
	Situation func() BotSituation
}

// LabHook returns the command for a decision, or false to use the default bot.
// The runner fills in ID, Seat and Revision, so a hook only sets the action,
// contract and cards.
type LabHook func(c LabContext) (Command, bool)

// LabPolicy is one bot configuration. Phase hooks are optional.
type LabPolicy struct {
	Name    string
	Search  LabSearch
	Bid     LabHook // auction and no-talon contract declaration
	Discard LabHook // discard and contract choice
	Whist   LabHook // defence: whist, pass, half, return, dealer whist, open/closed
	Sim     LabSim
}

// DefaultLabSearch returns the application's current card-play settings.
func DefaultLabSearch() LabSearch {
	return LabSearch{
		Samples:    config.Interface.LocalBotSearchSamples,
		ExactCards: config.Interface.LocalBotExactCards,
		Msec:       config.Interface.LocalBotSearchTimeMs,
	}
}

// labWhistFirst makes the first defender whist and leaves everything else to the
// default bot. It reproduces the harness of TestSelfPlayComparison, so that there
// is always a defence to play against.
func labWhistFirst(c LabContext) (Command, bool) {
	s := c.State
	if s.Stage == "defend" && len(s.Defenders) > 0 && s.Turn == s.Defenders[0] {
		return Command{Action: "whist"}, true
	}
	return Command{}, false
}

// labWhistAlways whists whenever whisting is a legal answer.
func labWhistAlways(c LabContext) (Command, bool) {
	for _, a := range c.View.Actions {
		if a == "whist" {
			return Command{Action: "whist"}, true
		}
	}
	return Command{}, false
}

// LabResult is the outcome of one deal. Nums are the application's result
// numerators for every seat (use Points for points).
type LabResult struct {
	Nums      []int
	Kind      string // pass, misere, contract or none
	Declarer  int
	Level     int
	Decisions int // card-play decisions searched
	Err       error
}

// labPoints converts one ResultNumerators entry to points. The numerators have the
// player count as their common denominator and count pool points in tens.
func labPoints(numerator, players int) float64 {
	return simPoints(numerator, players)
}

// Points is the result of one seat in points.
func (r LabResult) Points(seat int) float64 { return labPoints(r.Nums[seat], len(r.Nums)) }

// labKind classifies a finished deal.
func labKind(s *State) string {
	switch {
	case s.AllPass:
		return "pass"
	case s.Contract == nil:
		return "none"
	case s.Contract.Misere:
		return "misere"
	}
	return "contract"
}

func (p LabPolicy) search(situation BotSituation, seed int64) Command {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.Search.Msec)*time.Millisecond)
	defer cancel()
	command, _ := SearchBotCommand(ctx, situation, SearchOptions{Samples: p.Search.Samples, ExactCards: p.Search.ExactCards, Seed: seed})
	return command
}

// labDeck is the deal for a seed: the same shuffle as the tests use.
func labDeck(seed int64) []Card {
	r := rand.New(rand.NewSource(seed))
	d := make([]Card, 32)
	for i := range d {
		d[i] = Card(i)
	}
	r.Shuffle(32, func(i, j int) { d[i], d[j] = d[j], d[i] })
	return d
}

// PlayLabDeal plays one complete deal with a deck fixed by seed. policies has one
// entry per seat. Equal seeds and equal policies give equal deals.
func PlayLabDeal(players int, seed int64, policies []LabPolicy) LabResult {
	if len(policies) != players {
		return LabResult{Err: fmt.Errorf("need %d policies, got %d", players, len(policies))}
	}
	s, err := New(players, 30)
	if err != nil {
		return LabResult{Err: err}
	}
	for i := range s.Players {
		s.Players[i].Ready = true
	}
	if err := s.deal(labDeck(seed)); err != nil {
		return LabResult{Err: err}
	}
	var decisions int64
	for step := 0; step < 600; step++ {
		switch s.Stage {
		case "auction", "contract", "discard", "defend", "option", "return", "mode", "play", "trick", "dealer-choice", "dealer-whist", "catch":
		default:
			r := LabResult{Nums: s.ResultNumerators(), Kind: labKind(s), Declarer: s.Declarer, Decisions: int(decisions)}
			if s.Contract != nil {
				r.Level = s.Contract.Level
			}
			return r
		}
		var command Command
		if s.Stage == "trick" {
			command = Command{Seat: 0, Action: "collect"}
		} else {
			seat := s.Actor()
			v := s.View(seat)
			policy := policies[seat]
			state := s
			c := LabContext{State: state, View: v, Seat: seat, Seed: seed*1000 + decisions + 1,
				Situation: func() BotSituation { sit, _ := state.BotChoices(seat, nil); return sit }}
			var hook LabHook
			switch s.Stage {
			case "auction", "contract":
				hook = policy.Bid
			case "discard":
				hook = policy.Discard
			case "defend", "option", "return", "dealer-choice", "dealer-whist", "mode":
				hook = policy.Whist
			}
			chosen := false
			if hook != nil {
				command, chosen = hook(c)
			}
			if !chosen {
				if s.Stage == "play" && len(v.Legal) > 1 {
					decisions++
					command = policy.search(c.Situation(), seed*1000+decisions)
				} else {
					command = BotCommand(v)
				}
			}
			command.Seat = seat
		}
		command.ID = ID()
		command.Revision = s.Revision
		next, err := Apply(s, command, nil)
		if err != nil {
			return LabResult{Err: fmt.Errorf("deal %d, stage %s, action %s: %w", seed, s.Stage, command.Action, err)}
		}
		s = next
	}
	return LabResult{Err: fmt.Errorf("deal %d did not finish", seed)}
}
