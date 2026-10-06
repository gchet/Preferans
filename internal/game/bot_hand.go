package game

import "math"

// handTricks estimates protected honours, long trumps and short-side ruffs.
// It is deliberately conservative and uses no opponent's hidden cards.
func handTricks(hand []Card, trump int) float64 {
	var suits [4]uint32
	var lengths [4]int
	for _, card := range hand {
		suits[card.Suit()] |= uint32(1) << card.Rank()
		lengths[card.Suit()]++
	}
	estimate := 0.0
	for suit, mask := range suits {
		length := lengths[suit]
		top := 0
		for rank := 7; rank >= 0; rank-- {
			if mask&(uint32(1)<<rank) == 0 {
				break
			}
			top++
		}
		estimate += float64(top)
		for rank := 7 - top; rank >= 3; rank-- {
			if mask&(uint32(1)<<rank) == 0 {
				continue
			}
			protection := length - (7 - rank)
			if protection > 0 {
				estimate += math.Max(0, 0.75-float64(6-rank)*0.18)
			}
		}
		if suit == trump {
			estimate += math.Max(0, float64(length-top-2)) * 0.55
		}
	}
	if trump < 4 && lengths[trump] >= 4 {
		for suit, length := range lengths {
			if suit != trump && length <= 1 {
				estimate += float64(2-length) * 0.35
			}
		}
	}
	return math.Min(10, estimate)
}

// misereRisk measures holes below held high cards: low contiguous suits are safe.
func misereRisk(hand []Card) float64 {
	var suits [4]uint32
	for _, card := range hand {
		suits[card.Suit()] |= uint32(1) << card.Rank()
	}
	risk := 0.0
	for _, mask := range suits {
		missing := 0
		for rank := 0; rank < 8; rank++ {
			if mask&(uint32(1)<<rank) == 0 {
				missing++
				continue
			}
			risk += float64(missing) * float64(rank+1) / 8
		}
	}
	return risk
}
func botBid(v View, c Command) Command {
	c.Action = "pass"
	bestValue := math.Inf(-1)
	for _, contract := range v.Contracts {
		if contract.Misere {
			if misereRisk(v.Hand) < 1.6 && bestValue < 0.5 {
				b := contract
				c.Action = "bid"
				c.Contract = &b
				bestValue = 0.5
			}
			continue
		}
		estimate := handTricks(v.Hand, max(0, contract.Suit))
		if contract.Suit == -1 {
			for suit := 0; suit <= 4; suit++ {
				estimate = math.Max(estimate, handTricks(v.Hand, suit))
			}
		}
		if !contract.NoTalon {
			estimate += 0.65
		}
		if estimate < float64(contract.Level)+0.1 {
			continue
		}
		value := estimate - float64(contract.Level)*0.75
		if value > bestValue {
			b := contract
			c.Action = "bid"
			c.Contract = &b
			bestValue = value
		}
	}
	return c
}
func botDiscard(v View, c Command) Command {
	c.Action = "declare"
	if v.Bid == nil || len(v.Hand) < 2 {
		return c
	}
	contracts := v.Contracts
	if len(contracts) == 0 {
		contracts = []Contract{*v.Bid}
	}
	bestValue := math.Inf(-1)
	for i := 0; i < len(v.Hand); i++ {
		for j := i + 1; j < len(v.Hand); j++ {
			if v.TalonShown && cardMask([]Card{v.Hand[i], v.Hand[j]}) != cardMask(v.Talon) {
				continue
			}
			hand := make([]Card, 0, len(v.Hand)-2)
			for k, card := range v.Hand {
				if k != i && k != j {
					hand = append(hand, card)
				}
			}
			for _, contract := range contracts {
				if contract.Misere != v.Bid.Misere || contract.Rank() < v.Bid.Rank() {
					continue
				}
				value := 0.0
				if contract.Misere {
					value = -misereRisk(hand)
				} else {
					tricks := handTricks(hand, contract.Suit)
					deficit := math.Max(0, float64(contract.Level)+0.25-tricks)
					value = tricks + float64(contract.Level)*0.1 - deficit*4
				}
				if value > bestValue {
					b := contract
					c.Contract = &b
					c.Cards = []Card{v.Hand[i], v.Hand[j]}
					bestValue = value
				}
			}
		}
	}
	return c
}
