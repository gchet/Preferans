package game

import "testing"

func TestPassExitAfterSettlement(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, tc := range []struct {
			name                                         string
			contract                                     Contract
			tricks                                       int
			played, whisted, onlyWhisted, special, reset bool
		}{
			{"won", Contract{Level: 7, Suit: 1}, 7, true, true, true, true, true},
			{"remise", Contract{Level: 7, Suit: 1}, 6, true, true, true, true, false},
			{"pass-pass", Contract{Level: 7, Suit: 1}, 0, false, false, true, true, false},
			{"pass-pass-permitted", Contract{Level: 7, Suit: 1}, 0, false, false, false, true, true},
			{"misere", Contract{Misere: true, Suit: 4}, 0, true, false, true, true, true},
			{"misere-disabled", Contract{Misere: true, Suit: 4}, 0, true, false, false, false, false},
			{"misere-remise", Contract{Misere: true, Suit: 4}, 1, true, false, true, true, false},
			{"ten", Contract{Level: 10, Suit: 1}, 10, true, false, true, true, true},
			{"ten-remise", Contract{Level: 10, Suit: 1}, 9, true, false, true, true, false},
			{"ten-unchecked", Contract{Level: 10, Suit: 1}, 0, false, false, false, true, false},
			{"ten-disabled", Contract{Level: 10, Suit: 1}, 10, true, false, true, false, false},
		} {
			t.Run(tc.name+string(rune('0'+n)), func(t *testing.T) {
				s := claimState(t, n, tc.contract)
				s.PassStreak = 2
				rules := s.Agreements()
				rules.PassExitWhistedOnly = &tc.onlyWhisted
				rules.PassExitSpecial = &tc.special
				s.Rules = &rules
				for _, p := range s.Defenders {
					s.Defence[p] = 1
				}
				if tc.whisted {
					s.Defence[s.Defenders[0]] = 2
				}
				s.Taken[s.Declarer] = tc.tricks
				s.TrickNo = 0
				if tc.played {
					s.TrickNo = 10
					s.Taken[s.Defenders[0]] = 10 - tc.tricks
				}
				s.score()
				if (s.PassStreak == 0) != tc.reset {
					t.Fatalf("streak=%d", s.PassStreak)
				}
			})
		}
	}
}

func TestClaimRevealsDeclarerUntilNextDeal(t *testing.T) {
	for _, n := range []int{3, 4} {
		s := claimState(t, n, Contract{Level: 7, Suit: 1})
		if len(s.View(s.Controller).Players[s.Declarer].Cards) != 0 {
			t.Fatal("premature exposure")
		}
		s = stepClaim(t, s, s.Declarer, "claim", 7)
		for seat := range s.Players {
			if len(s.View(seat).Players[s.Declarer].Cards) != len(s.Hands[s.Declarer]) {
				t.Fatal("claim hand hidden")
			}
		}
		s = step(t, s, s.Controller, "reject-claim", nil)
		if len(s.View(s.Controller).Players[s.Declarer].Cards) == 0 {
			t.Fatal("revealed hand hidden again")
		}
		if err := s.deal(fixedDeck(1)); err != nil {
			t.Fatal(err)
		}
		if s.DeclarerShown {
			t.Fatal("exposure persisted into new deal")
		}
	}
}
