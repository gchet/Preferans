package game

import (
	"context"
	"math"
	"math/bits"
	"math/rand"
	"sort"
)

// Simulation-based decisions of the local bot. Bidding, the discard with the final
// contract, and (for experiments) the whist decision are estimated by playing
// sampled deals out with the search engine's rollout heuristic and scoring them
// with the application's real scorer. Everything here uses the player-visible
// View only; hidden hands are sampled, never read.

// SimOptions holds the parameters of the simulation-based decisions.
type SimOptions struct {
	Samples   int     // sampled deals per decision
	Margin    float64 // bid only when the best contract beats passing by this many points
	PassValue float64 // value of passing (the expected result of an all-pass deal)
	TopK      int     // discard candidates evaluated by simulation
	WhistMin  float64 // whist when its value beats passing by this many points
	// Belief makes sampled declarer hands count the more, the closer their own
	// handTricks estimate is to the contract level the declarer actually bid
	// (a gaussian with this sigma; zero samples hands uniformly). BeliefShift moves
	// the centre: the expected estimate is level+BeliefShift.
	Belief      float64
	BeliefShift float64
	// PartnerPass makes the first defender assume that the other defender passes
	// (what a bot does against humans); by default it assumes the partner whists.
	PartnerPass bool
}

// DefaultSimOptions returns the parameters that the lab found to be a plateau.
func DefaultSimOptions() SimOptions {
	return SimOptions{Samples: 60, Margin: 0, PassValue: 0, TopK: 12, WhistMin: 0}
}

// simPoints converts one ResultNumerators entry to points. The numerators have the
// player count as their common denominator and count pool points in tens.
func simPoints(numerator, players int) float64 {
	return float64(numerator) / float64(10*players)
}

type simScoreKey struct {
	contract Contract
	declarer int
	taken    [4]int8
	defence  [4]int8
}

// simEvaluator scores and plays out sampled deals for one decision.
type simEvaluator struct {
	v      View
	n      int
	active [3]int
	rng    *rand.Rand
	cache  map[simScoreKey][4]float64
}

func newSimEvaluator(v View, seed int64) *simEvaluator {
	e := &simEvaluator{v: v, n: len(v.Players), rng: rand.New(rand.NewSource(seed)), cache: map[simScoreKey][4]float64{}}
	at := 0
	for offset := 1; offset <= e.n; offset++ {
		seat := (v.Dealer + offset) % e.n
		if e.n == 4 && seat == v.Dealer {
			continue
		}
		e.active[at] = seat
		at++
	}
	return e
}

// score returns every seat's points for a played deal.
func (e *simEvaluator) score(c Contract, declarer int, taken [4]int, defence [4]int) [4]float64 {
	key := simScoreKey{contract: c, declarer: declarer}
	for i := 0; i < 4; i++ {
		key.taken[i], key.defence[i] = int8(taken[i]), int8(defence[i])
	}
	if r, ok := e.cache[key]; ok {
		return r
	}
	n := e.n
	v := e.v
	s := &State{Players: make([]Player, n), Rules: &v.Rules, Target: v.Target, Stage: "play",
		Contract: &c, Declarer: declarer, Dealer: v.Dealer, PassPrice: v.PassPrice, TrickNo: 10,
		Taken: append([]int(nil), taken[:n]...), Defence: append([]int(nil), defence[:n]...),
		Pool: make([]int, n), Mountain: make([]int, n), Whists: matrix(n)}
	for _, seat := range e.active {
		if seat != declarer {
			s.Defenders = append(s.Defenders, seat)
		}
	}
	s.score()
	var r [4]float64
	for i, value := range s.ResultNumerators() {
		r[i] = simPoints(value, n)
	}
	e.cache[key] = r
	return r
}

// playout plays a deal to the end with the rollout heuristic for every seat and
// returns the tricks taken by each seat. Play starts with the first active seat
// after the dealer, exactly as the engine does.
func (e *simEvaluator) playout(hands [4]uint32, c Contract, declarer int) [4]int {
	p := searchPosition{dealer: -1, declarer: declarer, turn: e.active[0], contract: c, active: e.active, hands: hands}
	if e.n == 4 {
		p.dealer = e.v.Dealer
	}
	for p.trickNo < 10 {
		moves := p.legal()
		if moves == 0 {
			break
		}
		p.play(p.rolloutCard(moves, e.rng))
	}
	return p.taken
}

func simMask(cards []Card) uint32 {
	var m uint32
	for _, c := range cards {
		m |= uint32(1) << c
	}
	return m
}

// unknown lists the cards that are not in the given mask, shuffled.
func (e *simEvaluator) unknown(known uint32) []Card {
	var out []Card
	for c := 0; c < 32; c++ {
		if known&(uint32(1)<<c) == 0 {
			out = append(out, Card(c))
		}
	}
	e.rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// simSeats puts the given hands at the non-self active seats in play order.
func (e *simEvaluator) seats(self int, selfHand uint32, others ...uint32) [4]uint32 {
	var hands [4]uint32
	hands[self] = selfHand
	at := 0
	for _, seat := range e.active {
		if seat != self {
			hands[seat] = others[at]
			at++
		}
	}
	return hands
}

// bestDiscard drops the two cards of a 12-card hand that leave the best handTricks
// estimate for the trump.
func bestDiscard(hand []Card, trump int) uint32 {
	best, bi, bj := math.Inf(-1), 0, 1
	rest := make([]Card, 0, len(hand)-2)
	for i := 0; i < len(hand); i++ {
		for j := i + 1; j < len(hand); j++ {
			rest = rest[:0]
			for k, c := range hand {
				if k != i && k != j {
					rest = append(rest, c)
				}
			}
			if value := handTricks(rest, trump); value > best {
				best, bi, bj = value, i, j
			}
		}
	}
	var mask uint32
	for k, c := range hand {
		if k != bi && k != bj {
			mask |= uint32(1) << c
		}
	}
	return mask
}

var simBothWhist = [4]int{2, 2, 2, 2}

func simCards(mask uint32) []Card {
	var out []Card
	for c := 0; c < 32; c++ {
		if mask&(uint32(1)<<c) != 0 {
			out = append(out, Card(c))
		}
	}
	return out
}

// simBeliefWeight says how plausible a declarer's final hand is, given that the
// declarer bid this contract: bidders bid roughly what their hand is worth, so a
// hand far weaker (or far stronger) than the contract is unlikely.
func simBeliefWeight(opt *SimOptions, hand []Card, c Contract) float64 {
	if opt.Belief <= 0 || c.Suit < 0 || c.Suit > 4 {
		return 1
	}
	d := handTricks(hand, c.Suit) - (float64(c.Level) + opt.BeliefShift)
	return math.Exp(-d * d / (2 * opt.Belief * opt.Belief))
}

// SimBotCommand is the application's entry point: it decides bidding and the
// discard with the final contract by simulation. The bool is false when the
// stage is not covered (misere keeps the application's rules), when the context
// ends first, or when simulation has nothing better than the default bot; the
// caller then uses BotCommand. The int is the number of samples evaluated.
func SimBotCommand(ctx context.Context, v View, seed int64, samples int) (Command, int, bool) {
	opt := DefaultSimOptions()
	if samples > 0 {
		opt.Samples = samples
	}
	var command Command
	var ok bool
	switch v.Stage {
	case "auction":
		command, ok = simBid(ctx, &opt, v, seed)
	case "discard":
		command, ok = simDiscard(ctx, &opt, v, seed)
	}
	if !ok || ctx.Err() != nil {
		return Command{}, 0, false
	}
	return command, opt.Samples, true
}

// SimWhistCommand decides whist or pass in the defence stage by simulation. The
// sampled declarer hands are weighted by how well they fit the contract. The lab
// found this about as good as always passing, so the application uses it only when
// the host switches the bot's forced pass off. The bool is false when the stage is
// not covered or the context ends; the caller then passes.
func SimWhistCommand(ctx context.Context, v View, seed int64, samples int) (Command, int, bool) {
	opt := DefaultSimOptions()
	opt.Samples = 100
	opt.Belief = 0.7
	if samples > opt.Samples {
		opt.Samples = samples
	}
	command, ok := simWhist(ctx, &opt, v, seed)
	if !ok || ctx.Err() != nil {
		return Command{}, 0, false
	}
	return command, opt.Samples, true
}

func simBid(ctx context.Context, opt *SimOptions, v View, seed int64) (Command, bool) {
	if v.Stage != "auction" || len(v.Hand) != 10 {
		return Command{}, false
	}
	e := newSimEvaluator(v, seed)
	me := v.Seat
	mine := simMask(v.Hand)
	// tricks[sample][suit][noTalon] for the bidder
	type tricks [5][2][4]int
	samples := make([]tricks, 0, opt.Samples)
	for s := 0; s < opt.Samples; s++ {
		if ctx.Err() != nil {
			return Command{}, false
		}
		u := e.unknown(mine)
		talon, a, b := u[0:2], u[2:12], u[12:22]
		var t tricks
		for suit := 0; suit < 5; suit++ {
			for noTalon := 0; noTalon < 2; noTalon++ {
				hand := mine
				if noTalon == 0 {
					hand = bestDiscard(append(append([]Card(nil), v.Hand...), talon...), suit)
				}
				hands := e.seats(me, hand, simMask(a), simMask(b))
				t[suit][noTalon] = e.playout(hands, Contract{Level: 6, Suit: suit, NoTalon: noTalon == 1}, me)
			}
		}
		samples = append(samples, t)
	}
	value := func(contract Contract, suit int) float64 {
		noTalon := 0
		if contract.NoTalon {
			noTalon = 1
		}
		total := 0.0
		for _, t := range samples {
			cc := contract
			cc.Suit = suit
			total += e.score(cc, me, t[suit][noTalon], simBothWhist)[me]
		}
		return total / float64(len(samples))
	}
	best := math.Inf(-1)
	var chosen *Contract
	for _, contract := range v.Contracts {
		if contract.Misere {
			continue
		}
		ct := contract
		val := math.Inf(-1)
		if contract.Suit >= 0 && contract.Suit <= 4 {
			val = value(contract, contract.Suit)
		} else {
			for suit := 0; suit < 5; suit++ {
				val = math.Max(val, value(contract, suit))
			}
		}
		if val > best {
			best, chosen = val, &ct
		}
	}
	if chosen != nil && best > opt.PassValue+opt.Margin {
		return Command{Action: "bid", Contract: chosen}, true
	}
	// Simulation covers ordinary contracts only: the misere rule and passing stay with BotCommand.
	return Command{}, false
}

// simDiscard chooses the discard and the final contract by simulation.
func simDiscard(ctx context.Context, opt *SimOptions, v View, seed int64) (Command, bool) {
	if v.Stage != "discard" || v.Bid == nil || v.Bid.Misere || len(v.Hand) != 12 {
		return Command{}, false
	}
	contracts := v.Contracts
	if len(contracts) == 0 {
		contracts = []Contract{*v.Bid}
	}
	type candidate struct {
		i, j     int
		contract Contract
		pre      float64
	}
	var cands []candidate
	for i := 0; i < len(v.Hand); i++ {
		for j := i + 1; j < len(v.Hand); j++ {
			if v.TalonShown && simMask([]Card{v.Hand[i], v.Hand[j]}) != simMask(v.Talon) {
				continue
			}
			hand := make([]Card, 0, 10)
			for k, card := range v.Hand {
				if k != i && k != j {
					hand = append(hand, card)
				}
			}
			for _, contract := range contracts {
				if contract.Misere || contract.Rank() < v.Bid.Rank() || contract.Suit < 0 || contract.Suit > 4 {
					continue
				}
				tricks := handTricks(hand, contract.Suit)
				deficit := math.Max(0, float64(contract.Level)+0.25-tricks)
				cands = append(cands, candidate{i, j, contract, tricks + float64(contract.Level)*0.1 - deficit*4})
			}
		}
	}
	if len(cands) == 0 {
		return Command{}, false
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].pre > cands[b].pre })
	if len(cands) > opt.TopK {
		cands = cands[:opt.TopK]
	}
	e := newSimEvaluator(v, seed)
	me := v.Seat
	mine := simMask(v.Hand)
	splits := make([][2]uint32, opt.Samples)
	for s := range splits {
		u := e.unknown(mine)
		splits[s] = [2]uint32{simMask(u[0:10]), simMask(u[10:20])}
	}
	type key struct {
		i, j, suit int
	}
	played := map[key][][4]int{}
	bestValue := math.Inf(-1)
	best := cands[0]
	for _, cand := range cands {
		if ctx.Err() != nil {
			return Command{}, false
		}
		k := key{cand.i, cand.j, cand.contract.Suit}
		results, ok := played[k]
		if !ok {
			var hand uint32
			for idx, card := range v.Hand {
				if idx != cand.i && idx != cand.j {
					hand |= uint32(1) << card
				}
			}
			for _, split := range splits {
				hands := e.seats(me, hand, split[0], split[1])
				results = append(results, e.playout(hands, Contract{Level: 6, Suit: cand.contract.Suit}, me))
			}
			played[k] = results
		}
		total := 0.0
		for _, taken := range results {
			total += e.score(cand.contract, me, taken, simBothWhist)[me]
		}
		if mean := total / float64(len(results)); mean > bestValue {
			bestValue, best = mean, cand
		}
	}
	contract := best.contract
	return Command{Action: "declare", Contract: &contract, Cards: []Card{v.Hand[best.i], v.Hand[best.j]}}, true
}

// simWhist decides whist or pass in the "defend" stage by simulation. The lab
// found no gain yet (the sampled hands ignore what the bidding revealed), so the
// application does not use it.
func simWhist(ctx context.Context, opt *SimOptions, v View, seed int64) (Command, bool) {
	if v.Stage != "defend" || v.Contract == nil || v.Contract.Misere || len(v.Hand) != 10 {
		return Command{}, false
	}
	canWhist, canPass := false, false
	for _, a := range v.Actions {
		canWhist = canWhist || a == "whist"
		canPass = canPass || a == "pass"
	}
	if !canWhist || !canPass {
		return Command{}, false
	}
	e := newSimEvaluator(v, seed)
	me, declarer := v.Seat, v.Declarer
	partner := -1
	for _, seat := range e.active {
		if seat != me && seat != declarer {
			partner = seat
		}
	}
	if partner < 0 || v.Contract.Suit < 0 || v.Contract.Suit > 4 {
		return Command{}, false
	}
	mine := simMask(v.Hand)
	partnerDefence := 2
	if opt.PartnerPass {
		partnerDefence = 1
	}
	if len(v.Defence) > partner && v.Defence[partner] == 1 {
		partnerDefence = 1
	} else if len(v.Defence) > partner && v.Defence[partner] == 2 {
		partnerDefence = 2
	}
	var whist, pass, weight float64
	for s := 0; s < opt.Samples; s++ {
		if ctx.Err() != nil {
			return Command{}, false
		}
		u := e.unknown(mine)
		var hands [4]uint32
		hands[me] = mine
		hands[partner] = simMask(u[0:10])
		hands[declarer] = simMask(u[10:20])
		w := simBeliefWeight(opt, simCards(hands[declarer]), *v.Contract)
		if w < 1e-6 {
			continue
		}
		taken := e.playout(hands, *v.Contract, declarer)
		var def [4]int
		def[partner] = partnerDefence
		def[me] = 2
		whist += w * e.score(*v.Contract, declarer, taken, def)[me]
		def[me] = 1
		pass += w * e.score(*v.Contract, declarer, taken, def)[me]
		weight += w
	}
	if weight == 0 {
		return Command{}, false
	}
	if (whist-pass)/weight > opt.WhistMin {
		return Command{Action: "whist"}, true
	}
	return Command{Action: "pass"}, true
}

var _ = bits.TrailingZeros32
