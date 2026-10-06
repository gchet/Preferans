package game

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestRulesHostLobbyAndReadiness(t *testing.T) {
	s := testState(t, 4)
	r := DefaultRules()
	r.Stalingrad = false
	r.EndCondition = "time"
	r.Minutes = 15
	c := Command{ID: ID(), Action: "rules", Rules: &r, Target: 50, Revision: s.Revision}
	c.Seat = 1
	if _, e := Apply(s, c, nil); e == nil {
		t.Fatal("guest changed rules")
	}
	c.Seat = 0
	s.Players[2].Bot = true
	next, e := Apply(s, c, nil)
	if e != nil {
		t.Fatal(e)
	}
	if next.Target != 50 || next.Players[0].Ready || next.Players[1].Ready || !next.Players[2].Ready {
		t.Fatal("rules did not reset readiness")
	}
	b, _ := json.Marshal(next)
	var restored State
	if e = json.Unmarshal(b, &restored); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(restored.Agreements(), r) || !reflect.DeepEqual(restored.View(1).Rules, r) {
		t.Fatal("rules lost in save/view")
	}
	next.Stage = "play"
	c.ID = ID()
	c.Revision = next.Revision
	if _, e = Apply(next, c, nil); e == nil {
		t.Fatal("rules changed during game")
	}
	if !reflect.DeepEqual(s.Agreements(), DefaultRules()) {
		t.Fatal("legacy defaults changed")
	}
	r.PassPrices = []int{8, 2}
	if r.Validate() == nil {
		t.Fatal("invalid scale accepted")
	}
}

func TestCustomPassScaleAndExit(t *testing.T) {
	for _, n := range []int{3, 4} {
		s := testState(t, n)
		r := DefaultRules()
		r.PassPrices = []int{3, 6}
		r.PassExit = 8
		s.Rules = &r
		s = step(t, s, 0, "start", nil)
		for round := 0; round < 3; round++ {
			if round > 0 {
				s = step(t, s, 0, "next", nil)
				low := Contract{Level: 7, Suit: 4}
				if _, e := Apply(s, Command{ID: ID(), Seat: s.Actor(), Revision: s.Revision, Action: "bid", Contract: &low}, nil); e == nil {
					t.Fatal("low exit accepted")
				}
			}
			s = passRound(t, s)
			want := 3
			if round > 0 {
				want = 6
			}
			if s.PassPrice != want {
				t.Fatalf("price %d != %d", s.PassPrice, want)
			}
		}
	}
}

func auctionWinner(t *testing.T, n int, r Rules, b Contract) *State {
	s := testState(t, n)
	s.Rules = &r
	s = step(t, s, 0, "start", nil)
	if b.NoTalon && b.Misere {
		seat := s.Actor()
		s = step(t, s, seat, "bid", &Contract{Misere: true, Suit: 4})
		s = step(t, s, s.Actor(), "bid", &Contract{Level: 9, Suit: 0})
		s = step(t, s, s.Actor(), "pass", nil)
		s = step(t, s, seat, "bid", &b)
	} else {
		s = step(t, s, s.Actor(), "bid", &b)
	}
	for s.Stage == "auction" {
		s = step(t, s, s.Actor(), "pass", nil)
	}
	return s
}

func TestMisereWithoutTalon(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, take := range []bool{false, true} {
			r := DefaultRules()
			r.MisereTalon = take
			s := auctionWinner(t, n, r, Contract{Misere: true, Suit: 4, NoTalon: !take})
			if take {
				if s.Stage != "discard" || len(s.Hands[s.Declarer]) != 12 {
					t.Fatal("missing talon")
				}
			} else {
				if s.Stage != "catch" || len(s.Hands[s.Declarer]) != 10 || s.Open {
					t.Fatal("no-talon misere incorrect")
				}
				for seat := range s.Players {
					if len(s.View(seat).Talon) != 0 {
						t.Fatal("hidden talon leaked")
					}
				}
			}
		}
	}
}

func TestStalingradAndTenOptions(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, on := range []bool{false, true} {
			r := DefaultRules()
			r.Stalingrad = on
			r.TenCheck = on
			for _, level := range []int{6, 10} {
				b := Contract{Level: level, Suit: 0}
				s := auctionWinner(t, n, r, b)
				s = step(t, s, s.Actor(), "declare", &b, s.Hands[s.Declarer][0], s.Hands[s.Declarer][1])
				want := "play"
				if !on {
					want = "defend"
				}
				if s.Stage != want {
					t.Fatalf("n=%d level=%d on=%v: %s", n, level, on, s.Stage)
				}
				if level == 10 && !on && s.Pool[s.Declarer] != 0 {
					t.Fatal("ten credited before whisting")
				}
			}
		}
	}
}

func scoreFixture(t *testing.T, n int, r Rules, taken []int) *State {
	s := testState(t, n)
	s.Rules = &r
	s.Contract = &Contract{Level: 6, Suit: 1}
	s.Declarer = 0
	s.Defenders = []int{1, 2}
	s.Defence = make([]int, n)
	s.Defence[1] = 2
	s.Taken = taken
	s.TrickNo = 10
	s.Round = 1
	s.Talon = []Card{7, 15}
	return s
}
func TestWhistResponsibilityAndDealerBonus(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, gentle := range []bool{false, true} {
			r := DefaultRules()
			if !gentle {
				r.Whist = "greedy"
			}
			taken := make([]int, n)
			copy(taken, []int{5, 3, 2})
			s := scoreFixture(t, n, r, taken)
			s.score()
			a, b := 24, 4
			if gentle {
				a, b = 14, 14
			}
			if s.Whists[1][0] != a || s.Whists[2][0] != b {
				t.Fatalf("whists %v", s.Whists)
			}
		}
		for _, full := range []bool{false, true} {
			r := DefaultRules()
			if full {
				r.Responsibility = "full"
			}
			taken := make([]int, n)
			copy(taken, []int{8, 1, 1})
			s := scoreFixture(t, n, r, taken)
			s.score()
			want := 4
			if full {
				want = 8
			}
			if s.Mountain[1] != want {
				t.Fatal("responsibility ignored")
			}
		}
	}
	for _, on := range []bool{false, true} {
		r := DefaultRules()
		r.DealerBonus = on
		s := scoreFixture(t, 4, r, []int{6, 2, 2, 0})
		s.score()
		want := 0
		if on {
			want = 12
		}
		if s.Whists[3][0] != want {
			t.Fatal("dealer bonus ignored")
		}
	}
}

func TestTimedGameFinishesAfterDeal(t *testing.T) {
	for _, n := range []int{3, 4} {
		r := DefaultRules()
		finishImmediately := false
		r.FinishAfterPassExit = &finishImmediately
		r.EndCondition = "time"
		r.Minutes = 1
		s := testState(t, n)
		s.Rules = &r
		s = step(t, s, 0, "start", nil)
		if s.Deadline < time.Now().Unix()+55 {
			t.Fatal("timer did not start")
		}
		deadline := s.Deadline
		if s.Clone().Deadline != deadline {
			t.Fatal("timer lost in save")
		}
		s.Deadline = time.Now().Unix() - 1
		s = passRound(t, s)
		if s.Stage != "finished" {
			t.Fatal("expired game did not finish")
		}
		s = scoreFixture(t, n, r, make([]int, n))
		s.Deadline = time.Now().Unix() + 60
		s.Pool[0] = 10000
		s.score()
		if s.Stage != "round" {
			t.Fatal("pool ended timed game")
		}
		s.Deadline = time.Now().Unix() - 1
		s = step(t, s, 0, "next", nil)
		if s.Stage != "finished" || s.Round != 1 {
			t.Fatal("new deal after deadline")
		}
	}
}
