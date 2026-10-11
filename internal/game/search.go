package game

import (
	"context"
	"math"
	"math/bits"
	"math/rand"
	"preferans/config"
	"time"
)

type SearchOptions struct {
	Samples    int
	ExactCards int
	Seed       int64
}
type SearchStats struct {
	Samples int
	Nodes   int
	Elapsed time.Duration
}

// SearchBotCommand consumes a detached, player-visible snapshot only.
// Every candidate is evaluated on the same sampled deals. No hidden authoritative
// hands are accepted, and an inconsistent deal is rejected rather than relaxed.
func SearchBotCommand(ctx context.Context, situation BotSituation, options SearchOptions) (Command, SearchStats) {
	start := time.Now()
	stats := SearchStats{}
	v := situation.View
	if v.Claim != nil {
		command, result := ClaimBotCommand(ctx, v)
		result.Elapsed = time.Since(start)
		return command, result
	}
	command := BotCommand(v)
	if v.Stage != "play" || v.Claim != nil || len(v.Legal) < 2 {
		return command, stats
	}
	if options.Samples <= 0 {
		options.Samples = 96
	}
	options.ExactCards = min(config.Interface.LocalBotExactCardsLimit, max(0, options.ExactCards))
	rng := rand.New(rand.NewSource(options.Seed))
	sampler, ok := newSearchSampler(situation)
	if !ok {
		return command, stats
	}
	totals := make([]float64, len(v.Legal))
	search := searchEngine{ctx: ctx, options: options, rng: rng, view: v}
	for stats.Samples < options.Samples && ctx.Err() == nil {
		position, valid := sampler.sample(rng)
		if !valid {
			break
		}
		legal := position.legal()
		// Touching cards that no hand can separate give identical outcomes in this
		// sampled deal, so only one of them is searched and the value is shared.
		var rep [32]Card
		for card := range rep {
			rep[card] = Card(card)
		}
		position.reduceMoves(legal, &rep)
		values := make([]float64, len(v.Legal))
		var known [32]float64
		var done [32]bool
		complete := true
		for i, card := range v.Legal {
			if legal&(uint32(1)<<card) == 0 {
				complete = false
				break
			}
			r := rep[card]
			if !done[r] {
				p := position
				p.play(r)
				result, valid := search.rollout(p)
				if !valid {
					complete = false
					break
				}
				known[r], done[r] = search.objective(result, v.Seat), true
			}
			values[i] = known[r]
		}
		if !complete {
			break
		}
		for i, value := range values {
			totals[i] += value
		}
		stats.Samples++
	}
	if stats.Samples > 0 {
		leading := len(v.Trick) == 0
		best := 0
		for i := 1; i < len(totals); i++ {
			if totals[i] > totals[best]+1e-9 {
				best = i
			} else if math.Abs(totals[i]-totals[best]) < 1e-9 {
				// Equal value: lead the top of a sequence, otherwise play low.
				if (leading && v.Legal[i].Rank() > v.Legal[best].Rank()) || (!leading && v.Legal[i].Rank() < v.Legal[best].Rank()) {
					best = i
				}
			}
		}
		command.Cards = []Card{v.Legal[best]}
	}
	stats.Nodes = search.nodes
	stats.Elapsed = time.Since(start)
	return command, stats
}

type searchPosition struct {
	hands                           [4]uint32
	taken                           [4]int
	active                          [3]int
	dealer, declarer, turn, trickNo int
	trick                           [4]Play
	trickLen                        int
	talon                           [2]Card
	contract                        Contract
	allPass                         bool
}

func (p *searchPosition) next(seat int) int {
	for i, a := range p.active {
		if a == seat {
			return p.active[(i+1)%3]
		}
	}
	return p.active[0]
}
func (p *searchPosition) legal() uint32 {
	hand := p.hands[p.turn]
	if p.trickLen == 0 {
		return hand
	}
	follow := hand & (uint32(255) << (8 * p.trick[0].Card.Suit()))
	if follow != 0 {
		return follow
	}
	if !p.allPass && !p.contract.Misere && p.contract.Suit < 4 {
		trumps := hand & (uint32(255) << (8 * p.contract.Suit))
		if trumps != 0 {
			return trumps
		}
	}
	return hand
}
func (p *searchPosition) winner() int {
	best, winner := -1, -1
	lead := p.trick[0].Card.Suit()
	for _, play := range p.trick[:p.trickLen] {
		if play.Seat < 0 {
			continue
		}
		strength := -1
		if play.Card.Suit() == lead {
			strength = 10 + play.Card.Rank()
		}
		if !p.allPass && !p.contract.Misere && play.Card.Suit() == p.contract.Suit {
			strength = 30 + play.Card.Rank()
		}
		if winner < 0 || strength > best {
			best, winner = strength, play.Seat
		}
	}
	return winner
}
func (p *searchPosition) play(card Card) {
	p.hands[p.turn] &^= uint32(1) << card
	p.trick[p.trickLen] = Play{Seat: p.turn, Card: card}
	p.trickLen++
	required := 3
	if p.allPass && p.trickNo < 2 {
		required++
	}
	if p.trickLen < required {
		p.turn = p.next(p.turn)
		return
	}
	winner := p.winner()
	p.taken[winner]++
	p.trickNo++
	p.trickLen = 0
	p.turn = winner
	if p.allPass && p.trickNo <= 2 {
		p.turn = p.active[0]
	}
	if p.allPass && p.trickNo < 2 {
		seat := -1
		if p.dealer >= 0 {
			seat = p.dealer
		}
		p.trick[0] = Play{Seat: seat, Card: p.talon[p.trickNo]}
		p.trickLen = 1
	}
}
func (p *searchPosition) remaining() int {
	total := 0
	for _, hand := range p.hands {
		total += bits.OnesCount32(hand)
	}
	return total
}

type searchEngine struct {
	ctx     context.Context
	options SearchOptions
	rng     *rand.Rand
	view    View
	nodes   int
	cache   map[uint32][4]float64
}

func (e *searchEngine) objective(scores [4]float64, seat int) float64 {
	if e.view.AllPass || seat == e.view.Declarer {
		return scores[seat]
	}
	if e.view.DealerWhist {
		return scores[e.view.Dealer]
	}
	total := 0.0
	for i := range e.view.Players {
		if i != e.view.Declarer && !(len(e.view.Players) == 4 && i == e.view.Dealer) {
			total += scores[i]
		}
	}
	return total
}
func (e *searchEngine) terminal(p searchPosition) [4]float64 {
	// Use the application's actual scorer, including whist agreements, all-pass,
	// dealer bonuses and talon penalties. This synthetic state is never persisted.
	v := e.view
	n := len(v.Players)
	s := &State{Players: make([]Player, n), Rules: &v.Rules, Target: v.Target, Stage: "play",
		Contract: &p.contract, Declarer: v.Declarer, Dealer: v.Dealer, AllPass: v.AllPass,
		PassPrice: v.PassPrice, TrickNo: 10, Taken: append([]int(nil), p.taken[:n]...),
		Defence: append([]int(nil), v.Defence...), HalfSeat: v.HalfSeat, TalonShown: v.TalonShown, DealerWhist: v.DealerWhist,
		Talon: append([]Card(nil), v.Talon...), Pool: make([]int, n), Mountain: make([]int, n), Whists: matrix(n)}
	for _, seat := range p.active {
		if seat != v.Declarer {
			s.Defenders = append(s.Defenders, seat)
		}
	}
	s.score()
	nums := s.ResultNumerators()
	var result [4]float64
	for i, value := range nums {
		result[i] = float64(value) / float64(n)
	}
	return result
}
func (e *searchEngine) exact(p searchPosition) ([4]float64, bool) {
	e.nodes++
	if e.nodes%32 == 0 && e.ctx.Err() != nil {
		return [4]float64{}, false
	}
	if p.trickNo == 10 {
		return e.leaf(p), true
	}
	moves := p.reduceMoves(p.legal(), nil)
	best := math.Inf(-1)
	var chosen [4]float64
	for moves != 0 {
		card := Card(bits.TrailingZeros32(moves))
		moves &^= uint32(1) << card
		next := p
		next.play(card)
		result, ok := e.exact(next)
		if !ok {
			return result, false
		}
		value := e.objective(result, p.turn)
		if value > best {
			best, chosen = value, result
		}
	}
	return chosen, true
}
func (e *searchEngine) rollout(p searchPosition) ([4]float64, bool) {
	for p.trickNo < 10 {
		e.nodes++
		if e.ctx.Err() != nil {
			return [4]float64{}, false
		}
		if p.remaining() <= e.options.ExactCards {
			return e.exact(p)
		}
		moves := p.legal()
		if moves == 0 {
			return [4]float64{}, false
		}
		card := p.rolloutCard(moves, e.rng)
		p.play(card)
	}
	return e.leaf(p), true
}
func (p *searchPosition) rolloutCard(moves uint32, rng *rand.Rand) Card {
	best := Card(bits.TrailingZeros32(moves))
	bestValue := math.Inf(-1)
	for moves != 0 {
		card := Card(bits.TrailingZeros32(moves))
		moves &^= uint32(1) << card
		rank := float64(card.Rank())
		value := -rank
		if !p.allPass {
			if p.contract.Misere {
				if p.turn == p.declarer {
					value = -rank
				} else {
					value = rank
				}
			} else if p.trickLen > 0 {
				currentWinner := p.winner()
				next := *p
				next.trick[next.trickLen] = Play{Seat: p.turn, Card: card}
				next.trickLen++
				winner := next.winner()
				sameTeam := func(a, b int) bool { return (a == p.declarer) == (b == p.declarer) }
				if winner == p.turn && !sameTeam(currentWinner, p.turn) {
					value = 20 - rank
				}
			} else {
				length := bits.OnesCount32(p.hands[p.turn] & (uint32(255) << (8 * card.Suit())))
				value = float64(length) + rank
				if card.Rank() == 7 {
					value += 12
				}
			}
		} else if p.trickLen > 0 {
			next := *p
			next.trick[next.trickLen] = Play{Seat: p.turn, Card: card}
			next.trickLen++
			if next.winner() != p.turn {
				value = 20 + rank
			}
		}
		value += rng.Float64() * 0.15
		if value > bestValue {
			bestValue, best = value, card
		}
	}
	return best
}

// trickBias breaks ties between equal real scores in favour of safer positions:
// the declarer prefers overtricks (a margin against sampling error) and, on a
// misere, fewer tricks. It is far smaller than any real score difference.
const trickBias = 0.01

// leaf is terminal() with a tie-breaking margin, memoised by the tricks taken:
// the real scorer only depends on them and is expensive to run per leaf.
func (e *searchEngine) leaf(p searchPosition) [4]float64 {
	key := uint32(p.taken[0]) | uint32(p.taken[1])<<4 | uint32(p.taken[2])<<8 | uint32(p.taken[3])<<12
	if result, ok := e.cache[key]; ok {
		return result
	}
	result := e.terminal(p)
	if !p.allPass {
		if p.contract.Misere {
			result[p.declarer] -= trickBias * float64(p.taken[p.declarer])
		} else {
			for _, seat := range p.active {
				result[seat] += trickBias * float64(p.taken[seat])
			}
		}
	}
	if e.cache == nil {
		e.cache = map[uint32][4]float64{}
	}
	e.cache[key] = result
	return result
}

// reduceMoves drops legal cards that are interchangeable with a lower legal card:
// two cards of one suit held by the mover with no other remaining card (in any
// hand or in the current trick) ranked between them always lead to the same
// result. rep, when given, maps every dropped card to the card kept instead.
func (p *searchPosition) reduceMoves(moves uint32, rep *[32]Card) uint32 {
	all := p.hands[p.active[0]] | p.hands[p.active[1]] | p.hands[p.active[2]]
	for i := 0; i < p.trickLen; i++ {
		all |= uint32(1) << p.trick[i].Card
	}
	if p.allPass && p.trickNo < 2 {
		all |= uint32(1)<<p.talon[0] | uint32(1)<<p.talon[1]
	}
	kept := moves
	for suit := 0; suit < 4; suit++ {
		shift := uint(8 * suit)
		mine := (moves >> shift) & 255
		if mine == 0 {
			continue
		}
		remaining := ((all >> shift) & 255) | mine
		prevMine := false
		anchor := Card(0)
		for rank := 0; rank < 8; rank++ {
			bit := uint32(1) << uint(rank)
			if remaining&bit == 0 {
				continue
			}
			card := Card(suit*8 + rank)
			if mine&bit == 0 {
				prevMine = false
				continue
			}
			if prevMine {
				kept &^= uint32(1) << uint(card)
				if rep != nil {
					rep[card] = anchor
				}
			} else {
				anchor = card
			}
			prevMine = true
		}
	}
	return kept
}
