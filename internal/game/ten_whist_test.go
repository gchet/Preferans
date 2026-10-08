package game

import (
	"slices"
	"testing"
)

func TestTenWhistAndClaim(t *testing.T) {
	for _, n := range []int{3, 4} {
		r := DefaultRules()
		r.TenCheck = false
		r.DealerBonus = false
		b := Contract{Level: 10, Suit: 4}
		s := auctionWinner(t, n, r, b)
		s = step(t, s, s.Declarer, "declare", &b, s.Hands[s.Declarer][0], s.Hands[s.Declarer][1])
		if s.Stage != "defend" || len(s.History) != 0 {
			t.Fatal("whisted ten must wait for defenders")
		}
		passed := step(t, s, s.Actor(), "pass", nil)
		passed = step(t, passed, passed.Actor(), "pass", nil)
		if n == 4 {
			if passed.Stage != "dealer-choice" || passed.Actor() != passed.Dealer {
				t.Fatal("four-player dealer should be offered the ten after both defenders pass")
			}
			passed = step(t, passed, passed.Dealer, "dealer-skip", nil)
		}
		if passed.Stage != "round" || passed.Pool[passed.Declarer] != 10 {
			t.Fatal("declined ten was not scored")
		}
		s = step(t, s, s.Actor(), "whist", nil)
		s = step(t, s, s.Actor(), "pass", nil)
		if s.Stage != "mode" {
			t.Fatal("missing open/closed choice")
		}
		s = step(t, s, s.Actor(), "open", nil)
		s.Turn = s.Declarer
		if !slices.Contains(s.View(s.Declarer).Actions, "claim") {
			t.Fatal("missing claim action")
		}
		if !slices.Equal(s.claimReviewers(), []int{s.Controller}) {
			t.Fatal("single whister must decide")
		}
		s = stepClaim(t, s, s.Declarer, "claim", 9)
		s = step(t, s, s.Controller, "accept-claim", nil)
		if s.Stage != "round" || s.Mountain[s.Declarer] != 20 {
			t.Fatal("ten down one was not scored")
		}
		if s.Whists[s.Defenders[0]][s.Declarer]+s.Whists[s.Defenders[1]][s.Declarer] != 60 {
			t.Fatal("whisted ten must score tricks and consolation whists")
		}
	}
}

func TestCheckedTenOffersClaimOnLaterTurn(t *testing.T) {
	for _, n := range []int{3, 4} {
		s := claimState(t, n, Contract{Level: 10, Suit: 4})
		s.Defence = make([]int, n)
		s.Controller = -1
		s.TrickNo = 3
		s.Taken[s.Declarer] = 3
		if !slices.Contains(s.View(s.Declarer).Actions, "claim") {
			t.Fatal("missing later-turn claim")
		}
		s = stepClaim(t, s, s.Declarer, "claim", 7)
		reviewers := s.claimReviewers()
		s = step(t, s, reviewers[0], "accept-claim", nil)
		if s.Stage != "play" {
			t.Fatal("checked ten needs both approvals")
		}
		s = step(t, s, reviewers[1], "accept-claim", nil)
		if s.Stage != "round" || s.Pool[s.Declarer] != 10 {
			t.Fatal("checked ten claim not scored")
		}
	}
}
