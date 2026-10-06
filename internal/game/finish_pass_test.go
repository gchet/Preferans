package game

import (
	"fmt"
	"testing"
	"time"
)

func TestFinishWaitsForPassExit(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, mode := range []string{"pool", "time"} {
			t.Run(fmt.Sprintf("%s/%d", mode, n), func(t *testing.T) {
				s := testState(t, n)
				rules := DefaultRules()
				rules.EndCondition = mode
				s.Rules = &rules
				s = step(t, s, 0, "start", nil)
				s.Pool[0] = s.Target * n
				s.Deadline = time.Now().Unix() - 1
				s = passRound(t, s)
				if s.Stage != "round" || s.PassStreak == 0 || s.FinishDue() {
					t.Fatal("party finished during passes")
				}
				if _, err := Apply(s, Command{ID: ID(), Seat: 0, Revision: s.Revision, Action: "expire"}, nil); err == nil {
					t.Fatal("expiry bypassed pass rule")
				}
				s = step(t, s, 0, "next", nil)
				if s.Stage != "auction" {
					t.Fatal("next deal blocked after deadline")
				}
				for _, won := range []bool{false, true} {
					contract := claimState(t, n, Contract{Level: 7, Suit: 1})
					contract.Rules = &rules
					contract.PassStreak = s.PassStreak
					contract.Pool[0] = contract.Target * n
					contract.Deadline = time.Now().Unix() - 1
					contract.TrickNo = 10
					contract.Taken[contract.Declarer] = 6
					if won {
						contract.Taken[contract.Declarer] = 7
					}
					contract.Taken[contract.Controller] = 10 - contract.Taken[contract.Declarer]
					contract.score()
					if (contract.Stage == "finished") != won {
						t.Fatalf("won=%v stage=%s streak=%d", won, contract.Stage, contract.PassStreak)
					}
				}
				immediate := false
				rules.FinishAfterPassExit = &immediate
				s.Rules = &rules
				if !s.FinishDue() {
					t.Fatal("disabled rule still postpones completion")
				}
			})
		}
	}
}
