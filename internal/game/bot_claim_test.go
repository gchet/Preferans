package game

import (
	"context"
	"math/bits"
	"math/rand"
	"testing"
	"time"
)

func TestLocalBotClaimAcceptance(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, misere := range []bool{false, true} {
			for _, declarerWins := range []bool{false, true} {
				contract := Contract{Level: 8, Suit: 4}
				if misere {
					contract = Contract{Misere: true, Suit: 4}
				}
				s := claimState(t, n, contract)
				s.TrickNo = 9
				s.Trick = nil
				s.Turn = s.Declarer
				s.Hands = make([][]Card, n)
				s.Hands[s.Declarer] = []Card{0}
				s.Hands[s.Defenders[0]] = []Card{6}
				s.Hands[s.Defenders[1]] = []Card{5}
				if declarerWins {
					s.Hands[s.Declarer] = []Card{7}
				}
				s.Taken = make([]int, n)
				s.Taken[s.Declarer] = 8
				s.Taken[s.Defenders[0]] = 1
				amount := 1
				if misere {
					amount = 0
				}
				s = stepClaim(t, s, s.Declarer, "claim", amount)
				for s.Claim != nil {
					seat := s.Actor()
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					command, stats := SearchBotCommand(ctx, BotSituation{View: s.View(seat)}, SearchOptions{})
					cancel()
					accept := declarerWins != misere
					if (command.Action == "accept-claim") != accept {
						t.Fatalf("n=%d misere=%v declarerWins=%v: %s (%d nodes)", n, misere, declarerWins, command.Action, stats.Nodes)
					}
					s = step(t, s, seat, command.Action, nil)
				}
				if (s.Stage == "round") != (declarerWins != misere) {
					t.Fatal("claim resolution did not finish/resume correctly")
				}
			}
		}
	}
}

func TestLocalBotClaimDuringTrickAndCancellation(t *testing.T) {
	s := claimState(t, 3, Contract{Level: 8, Suit: 0})
	s.TrickNo = 9
	s.Taken = []int{0, 0, 0}
	s.Taken[s.Declarer] = 8
	s.Taken[s.Controller] = 1
	s.Hands = make([][]Card, 3)
	s.Hands[s.Declarer] = []Card{7}
	s.Hands[s.Controller] = []Card{6}
	s.Trick = []Play{{Seat: s.Defenders[1], Card: 0}}
	s.Turn = s.Declarer
	s = stepClaim(t, s, s.Declarer, "claim", 1)
	v := s.View(s.Controller)
	command, _ := ClaimBotCommand(context.Background(), v)
	if command.Action != "accept-claim" {
		t.Fatalf("mid-trick winner ignored: %+v", v)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command, _ = ClaimBotCommand(ctx, v)
	if command.Action != "reject-claim" {
		t.Fatal("incomplete proof accepted")
	}
	v.Players[s.Declarer].Cards = nil
	command, _ = ClaimBotCommand(context.Background(), v)
	if command.Action != "reject-claim" {
		t.Fatal("used hidden cards")
	}
}

func TestClaimConcessions(t *testing.T) {
	for _, misere := range []bool{false, true} {
		contract := Contract{Level: 8, Suit: 4, Misere: misere}
		amount := 0
		if misere {
			amount = 4
		}
		v := View{Contract: &contract, Claim: &amount, TrickNo: 6}
		command, _ := ClaimBotCommand(context.Background(), v)
		if command.Action != "accept-claim" {
			t.Fatal("refused beneficial concession")
		}
	}
}

func TestClaimProofMatchesExhaustivePlay(t *testing.T) {
	rng := rand.New(rand.NewSource(20261002))
	var brute func(searchPosition) int
	brute = func(p searchPosition) int {
		if p.trickNo == 10 {
			return p.taken[p.declarer]
		}
		maximize := (p.turn == p.declarer) != p.contract.Misere
		best := 11
		if maximize {
			best = -1
		}
		moves := p.legal()
		for moves != 0 {
			card := Card(bits.TrailingZeros32(moves))
			moves &^= uint32(1) << card
			next := p
			next.play(card)
			value := brute(next)
			if maximize {
				best = max(best, value)
			} else {
				best = min(best, value)
			}
		}
		return best
	}
	for i := 0; i < 100; i++ {
		cards := rng.Perm(32)
		p := searchPosition{active: [3]int{0, 1, 2}, declarer: 0, turn: 0, trickNo: 8, contract: Contract{Level: 8, Suit: i % 5, Misere: i%2 == 0}}
		p.taken = [4]int{3, 3, 2, 0}
		for seat := 0; seat < 3; seat++ {
			p.hands[seat] = cardMask([]Card{Card(cards[seat*2]), Card(cards[seat*2+1])})
		}
		if i%3 == 0 {
			p.turn = 2
			p.play(Card(cards[4]))
		}
		value := brute(p)
		for amount := 0; amount <= 2; amount++ {
			solver := claimSolver{ctx: context.Background(), target: 3 + amount, misere: p.contract.Misere, memo: map[claimKey]bool{}}
			result, complete := solver.prove(p)
			expected := value >= solver.target
			if p.contract.Misere {
				expected = value <= solver.target
			}
			if !complete || result != expected {
				t.Fatalf("proof differs from full play: case=%d amount=%d value=%d", i, amount, value)
			}
		}
	}
}
