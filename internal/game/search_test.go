package game

import (
	"context"
	"math/bits"
	"math/rand"
	"reflect"
	"testing"
)

func searchTestDeal(t *testing.T, n int, contract *Contract) *State {
	t.Helper()
	var s *State
	if contract == nil {
		s = testState(t, n)
		if err := s.deal(fixedDeck(17)); err != nil {
			t.Fatal(err)
		}
		for s.Stage == "auction" {
			s = step(t, s, s.Actor(), "pass", nil)
		}
	} else {
		s = auctionWinner(t, n, DefaultRules(), *contract)
		for s.Stage != "play" {
			c := BotCommand(s.View(s.Actor()))
			var err error
			s, err = Apply(s, c, nil)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	return s
}
func positionFromState(s *State) searchPosition {
	p := searchPosition{dealer: -1, declarer: s.Declarer, turn: s.Turn, trickNo: s.TrickNo, allPass: s.AllPass}
	if len(s.Players) == 4 {
		p.dealer = s.Dealer
	}
	if s.Contract != nil {
		p.contract = *s.Contract
	}
	copy(p.active[:], s.Active())
	copy(p.taken[:], s.Taken)
	copy(p.talon[:], s.Talon)
	copy(p.trick[:], s.Trick)
	p.trickLen = len(s.Trick)
	for seat, hand := range s.Hands {
		p.hands[seat] = cardMask(hand)
	}
	return p
}
func TestSearchSimulationMatchesEngineAndScore(t *testing.T) {
	contracts := []*Contract{nil, {Misere: true, Suit: 4}, {Level: 6, Suit: 0}, {Level: 7, Suit: 3}, {Level: 8, Suit: 4}, {Level: 10, Suit: 2}}
	for _, n := range []int{3, 4} {
		for _, contract := range contracts {
			s := searchTestDeal(t, n, contract)
			initial := s.View(s.Actor())
			p := positionFromState(s)
			rng := rand.New(rand.NewSource(42))
			for s.Stage == "play" {
				legal := s.LegalCards()
				if cardMask(legal) != p.legal() {
					t.Fatalf("legal mismatch n=%d round=%d", n, p.trickNo)
				}
				card := legal[rng.Intn(len(legal))]
				p.play(card)
				s = step(t, s, s.Actor(), "play", nil, card)
				if s.Stage == "trick" {
					s = step(t, s, 0, "collect", nil)
				}
				if p.trickNo != s.TrickNo || !reflect.DeepEqual(p.taken[:n], s.Taken) {
					t.Fatal("trick winner/count mismatch")
				}
				if s.Stage == "play" && (p.turn != s.Turn || (len(s.Trick) != p.trickLen || (p.trickLen > 0 && !reflect.DeepEqual(p.trick[:p.trickLen], s.Trick)))) {
					t.Fatalf("turn/talon mismatch: %+v %+v", p, s.Trick)
				}
			}
			engine := searchEngine{view: initial}
			result := engine.terminal(p)
			actual := s.ResultNumerators()
			for seat, score := range actual {
				if result[seat] != float64(score)/float64(n) {
					t.Fatalf("score mismatch n=%d contract=%+v seat=%d", n, contract, seat)
				}
			}
		}
	}
}

func TestSearchDealsRespectVisibleCardsAndVoids(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, contract := range []*Contract{nil, {Level: 6, Suit: 1}, {Misere: true, Suit: 4}} {
			s := searchTestDeal(t, n, contract)
			rng := rand.New(rand.NewSource(25))
			for s.Stage == "play" {
				situation, _ := s.BotChoices(s.Actor(), nil)
				sampler, ok := newSearchSampler(situation)
				if !ok {
					t.Fatalf("valid deal rejected n=%d trick=%d contract=%+v", n, s.TrickNo, contract)
				}
				p, ok := sampler.sample(rng)
				if !ok {
					t.Fatal("sampling failed")
				}
				used := uint32(0)
				for seat, player := range situation.View.Players {
					if bits.OnesCount32(p.hands[seat]) != player.Count || used&p.hands[seat] != 0 {
						t.Fatal("duplicate cards or wrong hand size")
					}
					used |= p.hands[seat]
					if len(player.Cards) > 0 && p.hands[seat] != cardMask(player.Cards) {
						t.Fatal("visible hand changed")
					}
					for _, suit := range situation.VoidSuits[seat] {
						if p.hands[seat]&(uint32(255)<<(8*suit)) != 0 {
							t.Fatal("known void violated")
						}
					}
				}
				command, stats := SearchBotCommand(context.Background(), situation, SearchOptions{Samples: 2, ExactCards: 3, Seed: 25})
				if len(situation.View.Legal) > 1 && stats.Samples == 0 {
					t.Fatal("search did not complete any sample")
				}
				var err error
				s, err = Apply(s, command, nil)
				if err != nil {
					t.Fatal(err)
				}
				if s.Stage == "trick" {
					s = step(t, s, 0, "collect", nil)
				}
			}
		}
	}
}
func TestSearchCannotUseAuthoritativeHiddenHands(t *testing.T) {
	s := searchTestDeal(t, 3, &Contract{Level: 6, Suit: 2})
	first, _ := s.BotChoices(s.Actor(), nil)
	other := s.Clone()
	a, b := s.next(s.Actor()), s.next(s.next(s.Actor()))
	other.Hands[a], other.Hands[b] = other.Hands[b], other.Hands[a]
	second, _ := other.BotChoices(other.Actor(), nil)
	options := SearchOptions{Samples: 8, ExactCards: 3, Seed: 47}
	one, _ := SearchBotCommand(context.Background(), first, options)
	two, _ := SearchBotCommand(context.Background(), second, options)
	if !reflect.DeepEqual(one.Cards, two.Cards) {
		t.Fatal("hidden hands influenced search")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fallback, stats := SearchBotCommand(ctx, first, options)
	if stats.Samples != 0 || !reflect.DeepEqual(fallback.Cards, BotCommand(first.View).Cards) {
		t.Fatal("cancelled search did not use safe fallback")
	}
}

func TestSearchSavesContractInsteadOfDiscardingWinningAce(t *testing.T) {
	contract := Contract{Level: 6, Suit: 4}
	v := View{Seat: 0, Actor: 0, Turn: 0, Dealer: 2, Declarer: 0, Stage: "play", TrickNo: 8, Contract: &contract,
		Rules: DefaultRules(), Hand: []Card{7, 24}, PlayHand: []Card{7, 24}, Legal: []Card{7, 24}, Actions: []string{"play"},
		Taken: []int{5, 2, 1}, Defence: []int{0, 2, 2}, HalfSeat: -1,
		Players: []SeatView{{Count: 2, Cards: []Card{7, 24}}, {Count: 2, Cards: []Card{0, 1}}, {Count: 2, Cards: []Card{30, 31}}}}
	situation := BotSituation{View: v, KnownDiscard: []Card{8, 9}}
	used := cardMask([]Card{7, 24, 0, 1, 30, 31, 8, 9})
	var played []Play
	for card := Card(0); card < 32; card++ {
		if used&(uint32(1)<<card) == 0 {
			played = append(played, Play{Seat: len(played) % 3, Card: card})
		}
	}
	for i := 0; i < len(played); i += 3 {
		situation.PlayedTricks = append(situation.PlayedTricks, played[i:i+3])
	}
	if BotCommand(v).Cards[0] != 24 {
		t.Fatal("baseline no longer demonstrates the losing move")
	}
	command, stats := SearchBotCommand(context.Background(), situation, SearchOptions{Samples: 1, ExactCards: 6, Seed: 1})
	if stats.Samples != 1 || len(command.Cards) != 1 || command.Cards[0] != 7 {
		t.Fatalf("did not preserve the winning ace: %+v %+v", command, stats)
	}
}

func TestSearchSamplingUsesConditionalDealProbabilities(t *testing.T) {
	// Three feasible assignments; card 0 belongs to hand 0 in exactly one.
	// Choosing between its two available hands with probability 1/2 is biased.
	d := &searchSampler{unknown: []Card{0, 1, 2}, completions: map[uint32]float64{}}
	d.capacity[0], d.capacity[1], d.capacity[5] = 1, 1, 1
	d.allowed[0] = 3
	d.allowed[1] = 33
	d.allowed[2] = 35
	if d.completionCount(0, d.capacity) != 3 {
		t.Fatal("incorrect count of feasible deals")
	}
	rng := rand.New(rand.NewSource(5))
	firstHand := 0
	for i := 0; i < 12000; i++ {
		p, ok := d.sample(rng)
		if !ok {
			t.Fatal("feasible deal rejected")
		}
		if p.hands[0]&1 != 0 {
			firstHand++
		}
	}
	if firstHand < 3800 || firstHand > 4200 {
		t.Fatalf("biased conditional sampling: %d/12000, want about 4000", firstHand)
	}
}

func TestSearchSamplingFourMissingSuitCards(t *testing.T) {
	// Four specified unseen cards among twenty cards, ten slots in each hand.
	// Combined splits: 4:0 = 420/4845, 3:1 = 2400/4845, 2:2 = 2025/4845.
	d := &searchSampler{completions: map[uint32]float64{}}
	d.capacity[0], d.capacity[1] = 10, 10
	for card := Card(0); card < 20; card++ {
		d.unknown = append(d.unknown, card)
		d.allowed[card] = 3
	}
	if d.completionCount(0, d.capacity) != 184756 {
		t.Fatal("expected C(20,10) equally likely hand assignments")
	}
	rng := rand.New(rand.NewSource(73))
	var splits [3]int
	for i := 0; i < 6000; i++ {
		p, ok := d.sample(rng)
		if !ok {
			t.Fatal("sampling failed")
		}
		count := bits.OnesCount32(p.hands[0] & 15)
		splits[min(count, 4-count)]++
	}
	expected := [3]float64{420.0 / 4845, 2400.0 / 4845, 2025.0 / 4845}
	for i, count := range splits {
		frequency := float64(count) / 6000
		if frequency < expected[i]-0.025 || frequency > expected[i]+0.025 {
			t.Fatalf("split %d biased: %.3f, expected %.3f", i, frequency, expected[i])
		}
	}
}
