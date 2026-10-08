package game

import "preferans/locales"

import (
	crand "crypto/rand"

	"math/big"
	"strings"
	"time"
)

// Apply is transactional: invalid commands never alter the supplied state.
func Apply(s *State, c Command, deck []Card) (*State, error) {
	if c.Seat < 0 || c.Seat >= len(s.Players) || c.ID == "" || len(c.ID) > 128 {
		return nil, locales.Errorf("go.internal.game.engine.text001")
	}
	if owner, ok := s.Applied[c.ID]; ok {
		if owner != c.Seat {
			return nil, locales.Errorf("go.internal.game.engine.text002")
		}
		return s, nil
	}
	// Both defenders may answer concurrently. Accept their independent choice
	// only within the same deal while selection is still open.
	catchChoice := s.Stage == "catch" && c.Round == s.Round && c.Round > 0 && c.Revision <= s.Revision && (c.Action == "catch" || c.Action == "trust")
	if c.Revision != s.Revision && !catchChoice {
		return nil, locales.Errorf("go.internal.game.engine.text003")
	}
	t := s.Clone()
	if err := t.apply(c, deck); err != nil {
		return nil, err
	}
	if t.Stage == "finished" && s.Stage != "finished" {
		t.FinishedAt = time.Now().Unix()
	}
	t.Revision++
	t.Applied[c.ID] = c.Seat
	return t, nil
}
func (s *State) apply(c Command, deck []Card) error {
	if c.Action == "pause" || c.Action == "resume-play" {
		if s.Stage == "lobby" || s.Stage == "finished" || s.Players[c.Seat].Bot {
			return locales.Errorf("go.internal.game.engine.text004")
		}
		if s.ManualPaused && c.Seat != s.PausedBy {
			return locales.Errorf("go.game.pause_owner_only")
		}
		if c.Action == "resume-play" && !s.ManualPaused {
			return locales.Errorf("go.internal.game.engine.text004")
		}
		s.ManualPaused = c.Action == "pause"
		if s.ManualPaused {
			s.PausedBy = c.Seat
		}
		s.SetPaused(s.ManualPaused, time.Now().Unix())
		return nil
	}
	if s.PausedAt > 0 {
		return locales.Errorf("go.internal.game.engine.text005")
	}
	rules := s.Agreements()
	if c.Action == "debug-bots" {
		if c.Seat != 0 || s.Stage != "lobby" || s.Players[0].Ready {
			return locales.Errorf("go.internal.game.engine.text006")
		}
		if c.DebugBots != "" && c.DebugBots != "pass" && c.DebugBots != "misere" {
			return locales.Errorf("go.internal.game.engine.text007")
		}
		s.DebugBots = c.DebugBots
		return nil
	}
	if c.Action == "rules" {
		if c.Seat != 0 || s.Stage != "lobby" || s.Round != 0 {
			return locales.Errorf("go.internal.game.engine.text008")
		}
		if c.Rules == nil {
			return locales.Errorf("go.internal.game.engine.text009")
		}
		if err := c.Rules.Validate(); err != nil {
			return err
		}
		if c.Target < 1 || c.Target > 1000 {
			return locales.Errorf("go.internal.game.engine.text010")
		}
		s.Rules = c.Rules
		s.Target = c.Target
		for i := range s.Players {
			s.Players[i].Ready = s.Players[i].Bot
		}
		return nil
	}
	if c.Action == "ready" && s.Stage == "lobby" {
		s.Players[c.Seat].Ready = !s.Players[c.Seat].Ready
		if name := strings.TrimSpace(c.Name); name != "" {
			if len([]rune(name)) > 24 {
				return locales.Errorf("go.internal.game.engine.text011")
			}
			s.Players[c.Seat].Name = name
		}
		return nil
	}
	if c.Action == "start" && s.Stage == "lobby" && c.Seat == 0 {
		for _, p := range s.Players {
			if !p.Ready {
				return locales.Errorf("go.internal.game.engine.text012")
			}
		}
		s.StartedAt = time.Now().Unix()
		if rules.EndCondition == "time" {
			s.Deadline = s.StartedAt + int64(rules.Minutes)*60
		}
		return s.deal(deck)
	}
	if c.Action == "expire" && c.Seat == 0 && s.Stage == "round" && s.FinishDue() {
		s.Stage = "finished"
		return nil
	}
	if c.Action == "next" && s.Stage == "round" && c.Seat == 0 {
		if s.FinishDue() {
			s.Stage = "finished"
			return nil
		}
		s.Dealer = (s.Dealer + 1) % len(s.Players)
		return s.deal(deck)
	}
	if c.Action == "collect" && s.Stage == "trick" {
		s.PlayedTricks = append(s.PlayedTricks, append([]Play(nil), s.Trick...))
		s.LastTrick = append([]Play(nil), s.Trick...)
		s.TrickNo++
		if s.TrickNo == 10 {
			s.score()
			return nil
		}
		leader := s.Winner
		if s.AllPass && s.TrickNo <= 2 {
			leader = s.Active()[0]
		}
		s.beginTrick(leader)
		return nil
	}
	if c.Seat != s.Actor() && !(s.Stage == "catch" && s.isCatcher(c.Seat)) && !(s.Stage == "play" && s.canRespondToClaim(c.Seat) && (c.Action == "accept-claim" || c.Action == "reject-claim")) {
		return locales.Errorf("go.internal.game.engine.text013")
	}
	switch s.Stage {
	case "auction":
		if c.Action == "pass" {
			s.Passed[c.Seat] = true
		} else if c.Action == "bid" && c.Contract != nil {
			b := *c.Contract
			misereSeat := s.misereBidder()
			upgradingMisere := b.Misere && b.NoTalon && misereSeat == c.Seat
			invalidNoTalon := b.NoTalon && (!rules.AllowsNoTalonBids() || misereSeat < 0 ||
				(b.Misere && !upgradingMisere) || (!b.Misere && (misereSeat == c.Seat || b.Suit != -1)))
			if !b.Valid() || invalidNoTalon || (b.Misere && !b.NoTalon && misereSeat >= 0) ||
				(b.Misere && s.Spoken[c.Seat] && !upgradingMisere) || (!b.Misere && s.PassStreak > 0 && b.Level < rules.PassExit) {
				return locales.Errorf("go.internal.game.engine.text014")
			}
			if s.Bid != nil && (b.Rank() < s.Bid.Rank() || (b.Rank() == s.Bid.Rank() && !s.canRepeatBid(c.Seat))) {
				return locales.Errorf("go.internal.game.engine.text015")
			}
			if s.Bid != nil && b.Rank() == s.Bid.Rank() && rules.HereConvention != "seniority" && s.HereOwner == nil {
				owner := c.Seat
				s.HereOwner = &owner
			}
			s.Bid = &b
			s.Declarer = c.Seat
		} else {
			return locales.Errorf("go.internal.game.engine.text016")
		}
		s.Auction = append(s.Auction, AuctionCall{Seat: c.Seat, Action: c.Action, Contract: c.Contract})
		s.Spoken[c.Seat] = true
		left := 0
		for _, p := range s.Active() {
			if !s.Passed[p] {
				left++
			}
		}
		if left == 0 {
			s.AllPass = true
			s.PassStreak++
			s.PassPrice = rules.PassPrices[min(s.PassStreak-1, len(rules.PassPrices)-1)]
			s.beginTrick(s.Active()[0])
			return nil
		}
		if left == 1 && s.Bid != nil {
			if s.Bid.NoTalon {
				b := *s.Bid
				s.Contract = &b
				s.Defenders = []int{s.next(s.Declarer), s.next(s.next(s.Declarer))}
				s.Defence = make([]int, len(s.Players))
				if b.Misere {
					s.beginCatching()
				} else {
					s.Stage = "contract"
					s.Turn = s.Declarer
				}
				return nil
			}
			s.Stage = "discard"
			s.Turn = s.Declarer
			s.Hands[s.Declarer] = append(s.Hands[s.Declarer], s.Talon...)
			sortHand(s.Hands[s.Declarer])
			return nil
		}
		p := s.next(c.Seat)
		for s.Passed[p] {
			p = s.next(p)
		}
		s.Turn = p
		return nil
	case "contract":
		if c.Action != "declare-no-talon" || c.Contract == nil || len(c.Cards) != 0 {
			return locales.Errorf("go.internal.game.engine.text020")
		}
		b := *c.Contract
		if !b.Valid() || !b.NoTalon || b.Misere || b.Level != 9 || b.Suit < s.noTalonNineFloor(c.Seat) {
			return locales.Errorf("go.internal.game.engine.text021")
		}
		s.Contract = &b
		s.Stage = "defend"
		s.Turn = s.Defenders[0]
		return nil
	case "discard":
		if c.Action == "show-talon" {
			if len(s.Players) != 4 || s.Bid == nil || s.Bid.Misere || s.Bid.NoTalon || s.Declarer == s.Dealer {
				return locales.Errorf("go.internal.game.engine.text017")
			}
			s.TalonShown = !s.TalonShown
			return nil
		}
		if c.Action == "without-three" {
			if s.Bid == nil || s.Bid.Misere || (s.Bid.Level != 6 && s.Bid.Level != 7) {
				return locales.Errorf("go.internal.game.engine.text018")
			}
			b := *s.Bid
			s.Contract = &b
			n := len(s.Players)
			l := Ledger{Round: s.Round, Label: b.String() + locales.Text("go.internal.game.engine.text019"), Pool: make([]int, n), Mountain: make([]int, n), Whists: matrix(n)}
			l.Mountain[s.Declarer] = 3 * 4 * (b.Level - 5)
			s.recordScore(l)
			return nil
		}
		if c.Action == "declare" && len(s.Players) == 4 && s.TalonShown {
			c.Cards = append([]Card{}, s.Talon...)
		}
		if c.Action != "declare" || c.Contract == nil || len(c.Cards) != 2 || c.Cards[0] == c.Cards[1] {
			return locales.Errorf("go.internal.game.engine.text020")
		}
		b := *c.Contract
		if !b.Valid() || b.NoTalon || b.Misere != s.Bid.Misere || b.Rank() < s.Bid.Rank() {
			return locales.Errorf("go.internal.game.engine.text021")
		}
		for _, card := range c.Cards {
			if !remove(&s.Hands[c.Seat], card) {
				return locales.Errorf("go.internal.game.engine.text022")
			}
		}
		s.Discard = append([]Card{}, c.Cards...)
		s.Contract = &b
		s.Defenders = []int{s.next(s.Declarer), s.next(s.next(s.Declarer))}
		s.Defence = make([]int, len(s.Players))
		if b.Misere {
			s.beginCatching()
			return nil
		}
		if b.Level == 10 && rules.TenCheck {
			s.Open = true
			s.beginTrick(s.Active()[0])
			return nil
		}
		if b.Level == 6 && b.Suit == 0 && rules.Stalingrad {
			for _, p := range s.Defenders {
				s.Defence[p] = 2
			}
			s.beginTrick(s.Active()[0])
			return nil
		}
		s.Stage = "defend"
		s.Turn = s.Defenders[0]
		return nil
	case "catch":
		return s.chooseCatcher(c)
	case "defend":
		switch c.Action {
		case "pass":
			s.Defence[c.Seat] = 1
		case "whist":
			s.Defence[c.Seat] = 2
		case "half":
			if s.Contract.Level > 7 || s.Defence[s.Defenders[0]] != 1 || c.Seat != s.Defenders[1] {
				return locales.Errorf("go.internal.game.engine.text023")
			}
			s.HalfSeat = c.Seat
			s.Stage = "return"
			s.Turn = s.Defenders[0]
			return nil
		default:
			return locales.Errorf("go.internal.game.engine.text024")
		}
		if c.Seat == s.Defenders[0] {
			s.Turn = s.Defenders[1]
			return nil
		}
		if s.Defence[s.Defenders[0]] == 2 && s.Defence[s.Defenders[1]] == 1 && s.Contract.Level <= 7 {
			s.Stage = "option"
			s.Turn = s.Defenders[0]
			return nil
		}
		if s.Defence[s.Defenders[0]] == 1 && s.Defence[s.Defenders[1]] == 1 {
			s.HalfSeat = -1
			if s.Contract.Level <= 7 {
				// On six/seven the first defender may change a pass into a whist.
				s.Stage = "return"
				s.Turn = s.Defenders[0]
				return nil
			}
			if s.offerDealerWhist() {
				return nil
			}
			s.resolveDefence()
			return nil
		}
		s.resolveDefence()
		return nil
	case "option":
		if c.Action == "half" {
			s.HalfSeat = c.Seat
			s.Stage = "return"
			s.Turn = s.Defenders[1]
			return nil
		}
		if c.Action != "whist" {
			return locales.Errorf("go.internal.game.engine.text025")
		}
		s.resolveDefence()
		return nil
	case "return":
		if c.Action == "pass" {
			if s.HalfSeat >= 0 {
				s.Defence[s.HalfSeat] = 3
			}
			s.Defence[c.Seat] = 1
			if s.offerDealerWhist() {
				return nil
			}
			s.score()
			return nil
		}
		if c.Action != "whist" || s.Contract.Level > 7 {
			return locales.Errorf("go.internal.game.engine.text026")
		}
		if s.HalfSeat >= 0 {
			s.Defence[s.HalfSeat] = 1
		}
		s.Defence[c.Seat] = 2
		s.resolveDefence()
		return nil
	case "dealer-choice":
		if c.Action == "dealer-skip" {
			s.score()
			return nil
		}
		index := -1
		if c.Action == "dealer-first" {
			index = 0
		} else if c.Action == "dealer-second" {
			index = 1
		}
		if index < 0 {
			return locales.Errorf("go.dealer_whist.choice")
		}
		seat := s.Defenders[index]
		s.DealerLook = &seat
		s.Stage = "dealer-whist"
		return nil
	case "dealer-whist":
		if c.Action == "pass" {
			s.DealerLook = nil
			s.score()
			return nil
		}
		if c.Action != "whist" {
			return locales.Errorf("go.internal.game.engine.text026")
		}
		s.DealerWhist = true
		if s.HalfSeat >= 0 {
			s.Defence[s.HalfSeat] = 1
			s.HalfSeat = -1
		}
		s.Defence[s.Dealer] = 2
		s.Controller = s.Dealer
		s.Open = true
		s.beginTrick(s.Active()[0])
		return nil
	case "mode":
		if c.Action != "open" && c.Action != "closed" {
			return locales.Errorf("go.internal.game.engine.text027")
		}
		s.Open = c.Action == "open"
		s.beginTrick(s.Active()[0])
		return nil
	case "play":
		if s.Claim != nil {
			switch c.Action {
			case "reject-claim":
				s.Claim = nil
				s.ClaimAccepted = nil
				return nil
			case "accept-claim":
				s.ClaimAccepted = append(s.ClaimAccepted, c.Seat)
				if s.claimResponder() >= 0 {
					return nil
				}
				s.Taken[s.Declarer] += *s.Claim
				remainingSeat := s.claimReviewers()[0]
				if s.DealerWhist {
					remainingSeat = s.Defenders[0]
				}
				s.Taken[remainingSeat] += 10 - s.TrickNo - *s.Claim
				s.Claim = nil
				s.ClaimAccepted = nil
				s.Trick = nil
				s.TrickNo = 10
				for i := range s.Hands {
					s.Hands[i] = nil
				}
				s.score()
				return nil
			default:
				return locales.Errorf("go.internal.game.engine.text028")
			}
		}
		if c.Action == "claim" {
			if !s.canClaim() || c.Tricks < 0 || c.Tricks > 10-s.TrickNo {
				return locales.Errorf("go.internal.game.engine.text029")
			}
			n := c.Tricks
			s.Claim = &n
			s.DeclarerShown = true
			s.ClaimAccepted = nil
			return nil
		}
		if c.Action != "play" || len(c.Cards) != 1 {
			return locales.Errorf("go.internal.game.engine.text030")
		}
		card := c.Cards[0]
		ok := false
		for _, v := range s.LegalCards() {
			if v == card {
				ok = true
			}
		}
		if !ok {
			return locales.Errorf("go.internal.game.engine.text031")
		}
		remove(&s.Hands[s.Turn], card)
		s.LastTrick = nil
		s.Trick = append(s.Trick, Play{s.Turn, card})
		if s.Contract != nil && s.Contract.Misere && s.Turn == s.Declarer {
			s.MiserePlayed = append(s.MiserePlayed, card)
		}
		count := len(s.Trick)
		if s.AllPass && s.TrickNo < 2 {
			count--
		}
		if count == 3 {
			s.Winner = s.trickWinner()
			s.Taken[s.Winner]++
			s.Stage = "trick"
		} else {
			s.Turn = s.next(s.Turn)
		}
		return nil
	}
	return locales.Errorf("go.internal.game.engine.text032")
}
func remove(h *[]Card, c Card) bool {
	for i, v := range *h {
		if v == c {
			*h = append((*h)[:i], (*h)[i+1:]...)
			return true
		}
	}
	return false
}
func (s *State) deal(deck []Card) error {
	s.Auction = nil
	s.PlayedTricks = nil
	s.LastTrick = nil
	s.HereOwner = nil
	s.DeclarerShown = false
	s.Claim = nil
	s.ClaimAccepted = nil
	if deck == nil {
		deck = make([]Card, 32)
		for i := range deck {
			deck[i] = Card(i)
		}
		for i := 31; i > 0; i-- {
			j, e := crand.Int(crand.Reader, big.NewInt(int64(i+1)))
			if e != nil {
				return e
			}
			deck[i], deck[j.Int64()] = deck[j.Int64()], deck[i]
		}
	}
	if len(deck) != 32 {
		return locales.Errorf("go.internal.game.engine.text033")
	}
	seen := map[Card]bool{}
	for _, c := range deck {
		if c < 0 || c >= 32 || seen[c] {
			return locales.Errorf("go.internal.game.engine.text034")
		}
		seen[c] = true
	}
	n := len(s.Players)
	s.Round++
	s.Stage = "auction"
	s.Turn = s.Active()[0]
	s.Hands = make([][]Card, n)
	for i, p := range s.Active() {
		s.Hands[p] = append([]Card{}, deck[i*10:i*10+10]...)
		sortHand(s.Hands[p])
	}
	s.Talon = append([]Card{}, deck[30:]...)
	s.Discard = nil
	s.MisereCards, s.MiserePlayed = nil, nil
	s.TalonShown = false
	s.Passed = make([]bool, n)
	s.Spoken = make([]bool, n)
	s.Bid = nil
	s.Contract = nil
	s.Declarer = -1
	s.Defenders = nil
	s.Defence = nil
	s.Open = false
	s.Controller = -1
	s.HalfSeat = -1
	s.DealerLook = nil
	s.DealerWhist = false
	s.AllPass = false
	s.Trick = nil
	s.TrickNo = 0
	s.Taken = make([]int, n)
	s.Winner = -1
	return nil
}
func (s *State) resolveDefence() {
	ws := []int{}
	for _, p := range s.Defenders {
		if s.Defence[p] == 2 {
			ws = append(ws, p)
		}
	}
	if len(ws) == 0 {
		s.score()
		return
	}
	if len(ws) == 1 {
		s.Controller = ws[0]
		s.Stage = "mode"
		s.Turn = ws[0]
		return
	}
	s.beginTrick(s.Active()[0])
}

// In four-player games, the dealer may act after both defenders finally decline.
func (s *State) offerDealerWhist() bool {
	if len(s.Players) != 4 || s.Contract == nil || s.Contract.Misere {
		return false
	}
	for _, seat := range s.Defenders {
		if s.Defence[seat] != 1 && s.Defence[seat] != 3 {
			return false
		}
	}
	s.Stage = "dealer-choice"
	s.Turn = s.Dealer
	return true
}
func (s *State) beginTrick(leader int) {
	s.Trick = nil
	s.Stage = "play"
	s.Turn = leader
	if s.AllPass && s.TrickNo < 2 {
		p := -1
		if len(s.Players) == 4 {
			p = s.Dealer
		}
		s.Trick = append(s.Trick, Play{p, s.Talon[s.TrickNo]})
	}
}
func (s *State) LegalCards() []Card {
	if s.Stage != "play" {
		return nil
	}
	h := s.Hands[s.Turn]
	if len(s.Trick) == 0 {
		return append([]Card{}, h...)
	}
	lead := s.Trick[0].Card.Suit()
	var suit, trump []Card
	for _, c := range h {
		if c.Suit() == lead {
			suit = append(suit, c)
		}
		if s.Contract != nil && !s.Contract.Misere && c.Suit() == s.Contract.Suit {
			trump = append(trump, c)
		}
	}
	if len(suit) > 0 {
		return suit
	}
	if len(trump) > 0 {
		return trump
	}
	return append([]Card{}, h...)
}
func (s *State) trickWinner() int {
	lead := s.Trick[0].Card.Suit()
	best := -1
	winner := -1
	for _, p := range s.Trick {
		if p.Seat < 0 {
			continue
		}
		v := p.Card.Rank()
		if p.Card.Suit() == lead {
			v += 10
		} else {
			v = -1
		}
		if s.Contract != nil && !s.Contract.Misere && p.Card.Suit() == s.Contract.Suit {
			v = p.Card.Rank() + 30
		}
		if winner < 0 || v > best {
			best = v
			winner = p.Seat
		}
	}
	return winner
}
