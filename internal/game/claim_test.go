package game

import (
	"reflect"
	"testing"
)

func claimState(t *testing.T, n int, contract Contract) *State {
	s := auctionWinner(t, n, DefaultRules(), contract)
	s = step(t, s, s.Declarer, "declare", &contract, s.Hands[s.Declarer][0], s.Hands[s.Declarer][1])
	s.Stage = "play"
	s.Turn = s.Declarer
	s.Open = true
	s.Controller = s.Defenders[0]
	s.Defence[s.Defenders[0]] = 2
	s.Defence[s.Defenders[1]] = 1
	return s
}

func TestClaimMidTrickRejectAndAccept(t *testing.T) {
	for _, n := range []int{3, 4} {
		s := claimState(t, n, Contract{Level: 7, Suit: 1})
		s.TrickNo = 6
		s.Taken[s.Declarer] = 4
		s.Taken[s.Controller] = 2
		s.Trick = []Play{{Seat: s.Controller, Card: s.Hands[s.Controller][0]}}
		original := s.Clone()
		s = stepClaim(t, s, s.Declarer, "claim", 3)
		if _, err := Apply(s, Command{ID: ID(), Seat: s.Declarer, Revision: s.Revision, Action: "accept-claim"}, nil); err == nil {
			t.Fatal("self acceptance")
		}
		if _, err := Apply(s, Command{ID: ID(), Seat: s.Controller, Revision: s.Revision, Action: "play", Cards: s.Hands[s.Controller][:1]}, nil); err == nil {
			t.Fatal("play while pending")
		}
		s = step(t, s, s.Controller, "reject-claim", nil)
		if s.Turn != original.Turn || !reflect.DeepEqual(s.Trick, original.Trick) || !reflect.DeepEqual(s.Hands, original.Hands) || !s.canClaim() {
			t.Fatal("rejection changed play")
		}
		s = stepClaim(t, s, s.Declarer, "claim", 3)
		s = step(t, s, s.Controller, "accept-claim", nil)
		if s.Stage != "round" || s.Taken[s.Declarer] != 7 || s.Taken[s.Controller] != 3 || len(s.Trick) != 0 || len(s.History) != 1 || s.Pool[s.Declarer] != 4 {
			t.Fatal("incorrect claim settlement")
		}
	}
}

func stepClaim(t *testing.T, s *State, seat int, action string, tricks int) *State {
	t.Helper()
	next, err := Apply(s, Command{ID: ID(), Seat: seat, Revision: s.Revision, Action: action, Tricks: tricks}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestNoWhistClaimNeedsBothOpponents(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, b := range []Contract{{Level: 10, Suit: 4}, {Misere: true, Suit: 4}} {
			s := claimState(t, n, b)
			s.Defence = make([]int, n)
			s.Controller = -1
			amount := 10
			if b.Misere {
				amount = 0
			}
			s = stepClaim(t, s, s.Declarer, "claim", amount)
			reviewers := s.claimReviewers()
			for _, p := range reviewers {
				if len(s.View(p).Actions) != 2 {
					t.Fatal("reviewer missing controls")
				}
			}
			s = step(t, s, reviewers[1], "accept-claim", nil)
			if s.Stage != "play" || s.Claim == nil || len(s.History) != 0 {
				t.Fatal("single approval finished deal")
			}
			restored := s.Clone()
			s = step(t, s, reviewers[0], "reject-claim", nil)
			if s.Claim != nil || s.Turn != s.Declarer {
				t.Fatal("refusal did not resume")
			}
			s = step(t, restored, reviewers[0], "accept-claim", nil)
			if s.Stage != "round" || s.Taken[s.Declarer] != amount || len(s.History) != 1 {
				t.Fatal("two approvals did not finish")
			}
			want := 10
			if !b.Misere {
				want = 10
			}
			if s.Pool[s.Declarer] != want {
				t.Fatal("incorrect no-whist score")
			}
		}
	}
}

func TestClaimEligibilityAndBounds(t *testing.T) {
	s := claimState(t, 4, Contract{Level: 7, Suit: 1})
	for _, amount := range []int{-1, 11} {
		if _, err := Apply(s, Command{ID: ID(), Seat: s.Declarer, Revision: s.Revision, Action: "claim", Tricks: amount}, nil); err == nil {
			t.Fatal("invalid amount")
		}
	}
	s.Defence[s.Defenders[1]] = 2
	if s.canClaim() {
		t.Fatal("two whisters allowed")
	}
	s.Defence[s.Defenders[1]] = 1
	s.Turn = s.Controller
	if s.canClaim() {
		t.Fatal("claim out of turn")
	}
}
