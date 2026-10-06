package game

import (
	"context"
	"math/bits"
)

// ClaimBotCommand proves a proposed distribution using only visible hands.
// A timeout or incomplete information keeps the conservative refusal.
func ClaimBotCommand(ctx context.Context, v View) (Command, SearchStats) {
	c := Command{ID: ID(), Seat: v.Seat, Revision: v.Revision, Action: "reject-claim"}
	stats := SearchStats{}
	if v.Claim == nil || v.Contract == nil || v.AllPass {
		return c, stats
	}
	remaining := 10 - v.TrickNo
	if *v.Claim < 0 || *v.Claim > remaining {
		return c, stats
	}
	if (!v.Contract.Misere && *v.Claim == 0) || (v.Contract.Misere && *v.Claim == remaining) {
		c.Action = "accept-claim"
		return c, stats
	}
	n := len(v.Players)
	if (n != 3 && n != 4) || v.Declarer < 0 || v.Declarer >= n || v.Turn < 0 || v.Turn >= n || len(v.Trick) > 2 {
		return c, stats
	}
	p := searchPosition{declarer: v.Declarer, turn: v.Turn, trickNo: v.TrickNo, contract: *v.Contract}
	copy(p.taken[:], v.Taken)
	copy(p.trick[:], v.Trick)
	p.trickLen = len(v.Trick)
	occupied := uint32(0)
	count := 0
	for offset := 1; offset <= n; offset++ {
		seat := (v.Dealer + offset) % n
		if n == 4 && seat == v.Dealer {
			continue
		}
		p.active[count] = seat
		count++
		cards := v.Players[seat].Cards
		if seat == v.Seat {
			cards = v.Hand
		}
		mask := cardMask(cards)
		if len(cards) != v.Players[seat].Count || bits.OnesCount32(mask) != len(cards) || mask&occupied != 0 {
			return c, stats
		}
		p.hands[seat] = mask
		occupied |= mask
	}
	for _, play := range v.Trick {
		mask := cardMask([]Card{play.Card})
		if mask == 0 || occupied&mask != 0 {
			return c, stats
		}
		occupied |= mask
	}
	if p.remaining()+p.trickLen != remaining*3 {
		return c, stats
	}
	solver := claimSolver{ctx: ctx, target: p.taken[p.declarer] + *v.Claim, misere: v.Contract.Misere, memo: map[claimKey]bool{}}
	accepted, complete := solver.prove(p)
	stats.Nodes = solver.nodes
	if complete && accepted {
		c.Action = "accept-claim"
	}
	return c, stats
}

type claimKey struct {
	hands                 [4]uint32
	trick                 [3]Play
	turn, trickLen, taken int
}
type claimSolver struct {
	ctx    context.Context
	target int
	misere bool
	nodes  int
	memo   map[claimKey]bool
}

// Declarer maximizes normal tricks and minimizes misere tricks; defenders
// cooperate in the opposite direction. Beneficial concessions are accepted.
func (s *claimSolver) prove(p searchPosition) (bool, bool) {
	s.nodes++
	if s.ctx.Err() != nil {
		return false, false
	}
	taken := p.taken[p.declarer]
	if s.misere {
		if taken > s.target {
			return false, true
		}
		if taken+10-p.trickNo <= s.target {
			return true, true
		}
	} else {
		if taken >= s.target {
			return true, true
		}
		if taken+10-p.trickNo < s.target {
			return false, true
		}
	}
	if p.trickNo == 10 {
		return s.misere, true
	}
	key := claimKey{hands: p.hands, turn: p.turn, trickLen: p.trickLen, taken: taken}
	copy(key.trick[:], p.trick[:p.trickLen])
	if answer, ok := s.memo[key]; ok {
		return answer, true
	}
	moves := p.legal()
	if moves == 0 {
		return false, false
	}
	declarerTurn := p.turn == p.declarer
	result := !declarerTurn
	others := uint32(0)
	for seat, hand := range p.hands {
		if seat != p.turn {
			others |= hand
		}
	}
	for _, play := range p.trick[:p.trickLen] {
		others |= uint32(1) << play.Card
	}
	previous := Card(-1)
	for moves != 0 {
		card := Card(bits.Len32(moves) - 1)
		moves &^= uint32(1) << card
		// Adjacent ranks with no intervening opponent/trick card are equivalent.
		if previous >= 0 && previous.Suit() == card.Suit() {
			between := ((uint32(1) << previous) - 1) &^ ((uint32(1) << (card + 1)) - 1)
			if others&between == 0 {
				continue
			}
		}
		previous = card
		next := p
		next.play(card)
		answer, complete := s.prove(next)
		if !complete {
			return false, false
		}
		if answer == declarerTurn {
			result = answer
			break
		}
	}
	// Keep the cache bounded on tablets; uncached branches remain exact.
	if len(s.memo) < 65536 {
		s.memo[key] = result
	}
	return result, true
}
