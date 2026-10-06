package game

import (
	"encoding/json"
	"testing"
)

func TestNoTalonBidOrderAndValidity(t *testing.T) {
	contracts := Contracts()
	if len(contracts) != 28 {
		t.Fatal("missing new contracts")
	}
	for i, contract := range contracts {
		if !contract.Valid() || contract.Rank() != i {
			t.Fatalf("wrong rank/validity: %d %+v rank=%d", i, contract, contract.Rank())
		}
	}
	for _, contract := range []Contract{{Level: 6, Suit: 0, NoTalon: true}, {Level: 10, Suit: 4, NoTalon: true}} {
		if contract.Valid() {
			t.Fatal("unexpected no-talon level accepted")
		}
	}
}
func TestOrdinaryMisereAlwaysTakesTalon(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, allowed := range []bool{false, true} {
			r := DefaultRules()
			r.AllowNoTalon = &allowed
			r.MisereTalon = false
			s := auctionWinner(t, n, r, Contract{Misere: true, Suit: 4})
			if s.Stage != "discard" || len(s.Hands[s.Declarer]) != 12 {
				t.Fatal("ordinary misere skipped the talon")
			}
			s = step(t, s, s.Declarer, "declare", s.Bid, s.Hands[s.Declarer][0], s.Hands[s.Declarer][1])
			if s.Stage != "catch" || len(s.Discard) != 2 {
				t.Fatal("misere discard missing")
			}
			if s.Contract.NoTalon {
				t.Fatal("ordinary misere changed into no-talon")
			}
		}
	}
}
func TestNoTalonAuctionLadderAndHands(t *testing.T) {
	for _, n := range []int{3, 4} {
		r := DefaultRules()
		yes := true
		r.AllowNoTalon = &yes
		s := testState(t, n)
		s.Rules = &r
		s = step(t, s, 0, "start", nil)
		a := s.Actor()
		b := s.next(a)
		s = step(t, s, a, "bid", &Contract{Misere: true, Suit: 4})
		s = step(t, s, b, "bid", &Contract{Level: 9, Suit: 4})
		s = step(t, s, s.Actor(), "pass", nil)
		if s.Actor() != a {
			t.Fatal("wrong next bidder")
		}
		s = step(t, s, a, "bid", &Contract{Misere: true, Suit: 4, NoTalon: true})
		s = step(t, s, b, "bid", &Contract{Level: 9, Suit: -1, NoTalon: true})
		s = step(t, s, a, "pass", nil)
		if s.Stage != "contract" || len(s.Hands[b]) != 10 || len(s.Discard) != 0 || !s.Contract.NoTalon {
			t.Fatal("no-talon nine took/discarded cards")
		}
		for seat := range s.Players {
			if len(s.View(seat).Talon) != 0 {
				t.Fatal("unused talon revealed")
			}
		}
	}
}
func TestNoTalonBidsDisabledAndCannotBeDeclaredAfterPickup(t *testing.T) {
	r := DefaultRules()
	no := false
	r.AllowNoTalon = &no
	s := testState(t, 3)
	s.Rules = &r
	s = step(t, s, 0, "start", nil)
	for _, contract := range []Contract{{Misere: true, Suit: 4, NoTalon: true}, {Level: 9, Suit: 0, NoTalon: true}} {
		if _, err := Apply(s, Command{ID: ID(), Seat: s.Actor(), Revision: s.Revision, Action: "bid", Contract: &contract}, nil); err == nil {
			t.Fatal("disabled bid accepted")
		}
	}
	yes := true
	r.AllowNoTalon = &yes
	s = auctionWinner(t, 3, r, Contract{Level: 9, Suit: 0})
	b := Contract{Level: 9, Suit: 0, NoTalon: true}
	if _, err := Apply(s, Command{ID: ID(), Seat: s.Actor(), Revision: s.Revision, Action: "declare", Contract: &b, Cards: s.Hands[s.Actor()][:2]}, nil); err == nil {
		t.Fatal("declared no-talon after pickup")
	}
}
func TestLegacyNoTalonSaveMigration(t *testing.T) {
	r := DefaultRules()
	r.MisereTalon = false
	s := testState(t, 3)
	s.Rules = &r
	s.Stage = "catch"
	s.Contract = &Contract{Misere: true, Suit: 4}
	s.Bid = s.Contract
	data, _ := json.Marshal(s)
	var restored State
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if !restored.Contract.NoTalon || !restored.Bid.NoTalon {
		t.Fatal("legacy no-talon party changed meaning")
	}
}

func TestNoTalonContinuationOwnership(t *testing.T) {
	for _, n := range []int{3, 4} {
		s := testState(t, n)
		r := DefaultRules()
		yes := true
		r.AllowNoTalon = &yes
		s.Rules = &r
		s = step(t, s, 0, "start", nil)
		reject := func(state *State, b Contract) {
			t.Helper()
			if _, err := Apply(state, Command{ID: ID(), Seat: state.Actor(), Revision: state.Revision, Action: "bid", Contract: &b}, nil); err == nil {
				t.Fatalf("illegal continuation accepted: %+v", b)
			}
		}
		for _, b := range []Contract{{Misere: true, Suit: 4, NoTalon: true}, {Level: 9, Suit: -1, NoTalon: true}} {
			reject(s, b)
		}
		for _, b := range s.View(s.Actor()).Contracts {
			if b.NoTalon {
				t.Fatal("initial no-talon button exposed")
			}
		}
		first := s.Actor()
		s = step(t, s, first, "bid", &Contract{Misere: true, Suit: 4})
		reject(s, Contract{Misere: true, Suit: 4})
		reject(s, Contract{Misere: true, Suit: 4, NoTalon: true})
		reject(s, Contract{Level: 9, Suit: 0, NoTalon: true})
		choices := s.View(s.Actor()).Contracts
		generic := 0
		for _, b := range choices {
			if b.Misere {
				t.Fatal("another bidder can bid misere")
			}
			if b.NoTalon {
				generic++
				if b.Suit != -1 {
					t.Fatal("no-talon nine has auction suit")
				}
			}
		}
		if generic != 1 {
			t.Fatal("missing single generic nine")
		}
		disabled := s.Clone()
		no := false
		disabled.Rules.AllowNoTalon = &no
		reject(disabled, Contract{Level: 9, Suit: -1, NoTalon: true})
		s = step(t, s, s.Actor(), "bid", &Contract{Level: 9, Suit: 0})
		reject(s, Contract{Misere: true, Suit: 4, NoTalon: true})
		s = step(t, s, s.Actor(), "pass", nil)
		reject(s, Contract{Level: 9, Suit: -1, NoTalon: true})
		for _, b := range s.View(first).Contracts {
			if b.Misere && !b.NoTalon {
				t.Fatal("ordinary misere not replaced")
			}
		}
		s = step(t, s, first, "bid", &Contract{Misere: true, Suit: 4, NoTalon: true})
	}
}

func TestNoTalonNineDeclarationPreservesAuctionSuit(t *testing.T) {
	for _, n := range []int{3, 4} {
		for floor := 0; floor <= 4; floor++ {
			s := testState(t, n)
			r := DefaultRules()
			yes := true
			r.AllowNoTalon = &yes
			s.Rules = &r
			s = step(t, s, 0, "start", nil)
			a := s.Actor()
			b := s.next(a)
			s = step(t, s, a, "bid", &Contract{Misere: true, Suit: 4})
			s = step(t, s, b, "bid", &Contract{Level: 9, Suit: floor})
			s = step(t, s, s.Actor(), "pass", nil)
			s = step(t, s, a, "bid", &Contract{Misere: true, Suit: 4, NoTalon: true})
			s = step(t, s, b, "bid", &Contract{Level: 9, Suit: -1, NoTalon: true})
			s = step(t, s, a, "pass", nil)
			view := s.View(b)
			if s.Stage != "contract" || len(view.Contracts) != 5-floor || len(view.Hand) != 10 || len(view.Talon) != 0 {
				t.Fatal("wrong declaration view")
			}
			_, commands := s.BotChoices(b, nil)
			if len(commands) != 5-floor {
				t.Fatal("model choices differ from declaration")
			}
			for suit := -1; suit <= 4; suit++ {
				contract := Contract{Level: 9, Suit: suit, NoTalon: true}
				next, err := Apply(s, Command{ID: ID(), Seat: b, Revision: s.Revision, Action: "declare-no-talon", Contract: &contract}, nil)
				if suit < floor {
					if err == nil {
						t.Fatal("lowered announced suit")
					}
					continue
				}
				if err != nil || next.Stage != "defend" || next.Contract.Suit != suit || len(next.Discard) != 0 || len(next.Hands[b]) != 10 {
					t.Fatal("no-talon declaration failed", err)
				}
			}
			bot := BotCommand(view)
			if _, err := Apply(s, bot, nil); err != nil {
				t.Fatal("local bot cannot declare", err)
			}
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			var restored State
			if err = json.Unmarshal(data, &restored); err != nil || len(restored.View(b).Contracts) != 5-floor {
				t.Fatal("declaration lost in save", err)
			}
		}
	}
}
