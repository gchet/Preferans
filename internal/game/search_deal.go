package game

import (
	"math/bits"
	"math/rand"
)

type searchSampler struct {
	completions map[uint32]float64
	base        searchPosition
	capacity    [6]int
	allowed     [32]uint8
	unknown     []Card
	talonSlots  []int
}

func cardMask(cards []Card) uint32 {
	var mask uint32
	for _, card := range cards {
		if card >= 0 && card < 32 {
			mask |= uint32(1) << card
		}
	}
	return mask
}

func newSearchSampler(situation BotSituation) (*searchSampler, bool) {
	v := situation.View
	n := len(v.Players)
	if n != 3 && n != 4 {
		return nil, false
	}
	d := &searchSampler{}
	d.base = searchPosition{dealer: -1, declarer: v.Declarer, turn: v.Turn, trickNo: v.TrickNo, allPass: v.AllPass}
	if v.Contract != nil {
		d.base.contract = *v.Contract
	}
	at := 0
	for offset := 1; offset <= n; offset++ {
		seat := (v.Dealer + offset) % n
		if n == 4 && seat == v.Dealer {
			continue
		}
		d.base.active[at] = seat
		at++
	}
	if n == 4 {
		d.base.dealer = v.Dealer
	}
	if v.TrickNo < 0 || v.TrickNo >= 10 || v.Turn < 0 || v.Turn >= n || len(v.Trick) > 3 {
		return nil, false
	}
	copy(d.base.taken[:], v.Taken)
	copy(d.base.trick[:], v.Trick)
	d.base.trickLen = len(v.Trick)
	used := uint32(0)
	for _, trick := range situation.PlayedTricks {
		for _, play := range trick {
			used |= cardMask([]Card{play.Card})
		}
	}
	for _, play := range v.Trick {
		used |= cardMask([]Card{play.Card})
	}
	// The deciding hand and any genuinely visible/control hand are fixed.
	for seat, player := range v.Players {
		cards := player.Cards
		if seat == v.Seat {
			cards = v.Hand
		}
		if seat == v.Turn && len(v.PlayHand) > 0 {
			cards = v.PlayHand
		}
		mask := cardMask(cards)
		if len(cards) > 0 {
			if bits.OnesCount32(mask) != player.Count || used&mask != 0 {
				return nil, false
			}
			d.base.hands[seat] = mask
			used |= mask
		} else {
			d.capacity[seat] = player.Count
		}
	}
	if v.AllPass {
		for i := 0; i < 2; i++ {
			if i < len(v.Talon) {
				d.base.talon[i] = v.Talon[i]
				used |= cardMask([]Card{v.Talon[i]})
			} else {
				d.capacity[4]++
				d.talonSlots = append(d.talonSlots, i)
			}
		}
	} else {
		discarded := cardMask(situation.KnownDiscard)
		if v.TalonShown {
			discarded |= cardMask(v.Talon)
		}
		if discarded&used != 0 || bits.OnesCount32(discarded) > 2 {
			return nil, false
		}
		used |= discarded
		d.capacity[5] = 2 - bits.OnesCount32(discarded)
	}
	tracker := cardMask(v.MisereCards)
	talon := cardMask(v.Talon)
	var forbidden [4]uint32
	for seat, suits := range situation.VoidSuits {
		if seat < 0 || seat >= n {
			continue
		}
		for _, suit := range suits {
			if suit >= 0 && suit < 4 {
				forbidden[seat] |= uint32(255) << (8 * suit)
			}
		}
	}
	for card := Card(0); card < 32; card++ {
		mask := uint32(1) << card
		if used&mask != 0 {
			continue
		}
		allow := uint8(0)
		for seat := 0; seat < 6; seat++ {
			if d.capacity[seat] == 0 {
				continue
			}
			if seat < 4 {
				if forbidden[seat]&mask != 0 {
					continue
				}
				// Public original talon cards can only remain with declarer or in discard.
				if !v.AllPass && talon&mask != 0 && seat != v.Declarer {
					continue
				}
				if tracker != 0 && v.Contract != nil && v.Contract.Misere {
					if (seat == v.Declarer) != (tracker&mask != 0) {
						continue
					}
				}
			}
			allow |= uint8(1) << seat
		}
		if allow == 0 {
			return nil, false
		}
		d.allowed[card] = allow
		d.unknown = append(d.unknown, card)
	}
	count := 0
	for _, capacity := range d.capacity {
		count += capacity
	}
	return d, count == len(d.unknown)
}

// completionCount counts unordered hand assignments satisfying all constraints.
// Weighting each branch by its completions samples the conditional uniform deal
// exactly, rather than biasing towards the first hand that still has room.
func (d *searchSampler) completionCount(index int, capacity [6]int) float64 {
	if index == len(d.unknown) {
		return 1
	}
	var key uint32
	for slot, count := range capacity {
		key |= uint32(count) << (4 * slot)
	}
	if value, ok := d.completions[key]; ok {
		return value
	}
	total := 0.0
	card := d.unknown[index]
	for slot := 0; slot < 6; slot++ {
		if capacity[slot] == 0 || d.allowed[card]&(uint8(1)<<slot) == 0 {
			continue
		}
		next := capacity
		next[slot]--
		total += d.completionCount(index+1, next)
	}
	d.completions[key] = total
	return total
}

func (d *searchSampler) sample(rng *rand.Rand) (searchPosition, bool) {
	p := d.base
	if d.completions == nil {
		d.completions = map[uint32]float64{}
	}
	capacity := d.capacity
	var allocated [6]uint32
	for index, card := range d.unknown {
		total := d.completionCount(index, capacity)
		if total == 0 {
			return p, false
		}
		choice := rng.Float64() * total
		selected := -1
		for slot := 0; slot < 6; slot++ {
			if capacity[slot] == 0 || d.allowed[card]&(uint8(1)<<slot) == 0 {
				continue
			}
			next := capacity
			next[slot]--
			weight := d.completionCount(index+1, next)
			if weight == 0 {
				continue
			}
			selected = slot
			if choice < weight {
				break
			}
			choice -= weight
		}
		if selected < 0 {
			return p, false
		}
		capacity[selected]--
		allocated[selected] |= uint32(1) << card
	}
	for seat := 0; seat < 4; seat++ {
		p.hands[seat] |= allocated[seat]
	}
	unseen := allocated[4]
	for _, index := range d.talonSlots {
		card := Card(bits.TrailingZeros32(unseen))
		unseen &^= uint32(1) << card
		p.talon[index] = card
	}
	return p, true
}
