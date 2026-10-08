package game

import (
	"encoding/json"
	"reflect"
	"testing"
)

func dealerWhistChoice(t *testing.T, level int) *State {
	t.Helper()
	r := DefaultRules()
	r.Stalingrad = false
	r.TenCheck = false
	r.DealerBonus = false
	s := auctionWinner(t, 4, r, Contract{Level: level, Suit: 1})
	s = step(t, s, s.Actor(), "declare", s.Bid, s.Hands[s.Actor()][0], s.Hands[s.Actor()][1])
	s = step(t, s, s.Actor(), "pass", nil)
	s = step(t, s, s.Actor(), "pass", nil)
	if s.Stage == "return" {
		if s.Actor() != s.Defenders[0] {
			t.Fatal("first defender must retain the first chance to rewhist")
		}
		s = step(t, s, s.Actor(), "pass", nil)
	}
	if s.Stage != "dealer-choice" || s.Actor() != s.Dealer {
		t.Fatal("dealer should be offered after final defender passes")
	}
	return s
}

func TestDealerWhistSelectionAndPrivacy(t *testing.T) {
	for _, level := range []int{6, 7, 8, 9, 10} {
		for index, action := range []string{"dealer-first", "dealer-second"} {
			s := dealerWhistChoice(t, level)
			if s.Stage != "dealer-choice" || s.Actor() != s.Dealer {
				t.Fatal("dealer missing after three final passes")
			}
			if got := s.View(s.Dealer).Actions; !reflect.DeepEqual(got, []string{"dealer-skip", "dealer-first", "dealer-second"}) {
				t.Fatalf("choices: %v", got)
			}
			if _, err := Apply(s, Command{ID: ID(), Seat: s.Dealer, Revision: s.Revision, Action: "whist"}, nil); err == nil {
				t.Fatal("dealer whisted without selecting a hand")
			}
			s = step(t, s, s.Dealer, action, nil)
			chosen := s.Defenders[index]
			for seat := range s.Players {
				v := s.View(seat)
				for owner, player := range v.Players {
					visible := owner == seat || (seat == s.Dealer && owner == chosen)
					if (len(player.Cards) > 0) != (visible && len(s.Hands[owner]) > 0) {
						t.Fatalf("hand leakage: viewer %d, owner %d", seat, owner)
					}
				}
				if seat != s.Dealer && (len(v.Actions) != 0 || v.DealerLook != nil) {
					t.Fatal("dealer decision exposed to another participant")
				}
			}
			if !reflect.DeepEqual(s.View(s.Dealer).Actions, []string{"whist", "pass"}) {
				t.Fatal("dealer must only choose whist/pass after viewing")
			}
			for _, invalid := range []string{"half", "dealer-first", "dealer-second", "open", "closed"} {
				if _, err := Apply(s, Command{ID: ID(), Seat: s.Dealer, Revision: s.Revision, Action: invalid}, nil); err == nil {
					t.Fatalf("invalid dealer action accepted: %s", invalid)
				}
			}
			data, _ := json.Marshal(s)
			var restored State
			if err := json.Unmarshal(data, &restored); err != nil || restored.DealerLook == nil || *restored.DealerLook != chosen {
				t.Fatal("selected hand lost in save")
			}
			s = step(t, &restored, restored.Dealer, "whist", nil)
			if !s.DealerWhist || !s.Open || s.Controller != s.Dealer || s.Stage != "play" {
				t.Fatal("dealer did not take control of open defence")
			}
			for _, hand := range s.Defenders {
				s.Turn = hand
				if s.Actor() != s.Dealer || !reflect.DeepEqual(s.View(s.Dealer).PlayHand, s.Hands[hand]) {
					t.Fatal("dealer cannot play both hands")
				}
			}
		}
	}
}

func TestDealerMayDeclineBeforeOrAfterLooking(t *testing.T) {
	for _, look := range []bool{false, true} {
		s := dealerWhistChoice(t, 7)
		if look {
			s = step(t, s, s.Dealer, "dealer-first", nil)
			s = step(t, s, s.Dealer, "pass", nil)
		} else {
			s = step(t, s, s.Dealer, "dealer-skip", nil)
		}
		if s.Stage != "round" || s.DealerLook != nil || s.DealerWhist || len(s.History) != 1 {
			t.Fatal("dealer refusal must finish the conceded deal")
		}
		s = step(t, s, 0, "next", nil)
		if s.DealerLook != nil || s.DealerWhist {
			t.Fatal("dealer choice leaked into next deal")
		}
	}
}

func TestDealerWhistDelayedOpeningAndReset(t *testing.T) {
	s := dealerWhistChoice(t, 7)
	s = step(t, s, s.Dealer, "dealer-second", nil)
	s = step(t, s, s.Dealer, "whist", nil)
	if s.Turn != s.Declarer {
		t.Fatal("fixture must have declarer's opening lead")
	}
	for viewer := range s.Players {
		for _, defender := range s.Defenders {
			visible := viewer == defender || viewer == s.Dealer && defender == *s.DealerLook
			if (len(s.View(viewer).Players[defender].Cards) > 0) != visible {
				t.Fatal("defence opened before declarer's first card")
			}
		}
	}
	s = step(t, s, s.Declarer, "play", nil, s.LegalCards()[0])
	for viewer := range s.Players {
		for _, defender := range s.Defenders {
			if len(s.View(viewer).Players[defender].Cards) != 10 {
				t.Fatal("defence failed to open after declarer's card")
			}
		}
	}
	if err := s.deal(fixedDeck(2)); err != nil {
		t.Fatal(err)
	}
	if s.DealerLook != nil || s.DealerWhist || s.Controller != -1 || s.Open {
		t.Fatal("dealer whist state leaked into next deal")
	}
}

func TestDealerWhistControllerSurvivesSaveDuringPlay(t *testing.T) {
	s := dealerWhistChoice(t, 8)
	s = step(t, s, s.Dealer, "dealer-first", nil)
	s = step(t, s, s.Dealer, "whist", nil)
	s.Turn = s.Defenders[0]
	s.SetPaused(true, 100)

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var restored State
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	restored.SetPaused(false, 110)
	v := restored.View(restored.Dealer)
	if restored.Controller != restored.Dealer || !restored.DealerWhist || restored.Actor() != restored.Dealer {
		t.Fatal("dealer control was lost after restoring the saved game")
	}
	if !reflect.DeepEqual(v.Actions, []string{"play"}) || !reflect.DeepEqual(v.PlayHand, restored.Hands[restored.Turn]) {
		t.Fatalf("restored dealer cannot play defence: actions=%v playHand=%v", v.Actions, v.PlayHand)
	}
	card := restored.LegalCards()[0]
	if _, err := Apply(&restored, Command{ID: ID(), Seat: restored.Dealer, Revision: restored.Revision, Action: "play", Cards: []Card{card}}, nil); err != nil {
		t.Fatalf("dealer defensive play rejected after restore: %v", err)
	}
}

func TestDealerWhistEligibility(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, level := range []int{6, 7, 8, 9, 10} {
			r := DefaultRules()
			r.Stalingrad = false
			r.TenCheck = false
			s := auctionWinner(t, n, r, Contract{Level: level, Suit: 1})
			s = step(t, s, s.Actor(), "declare", s.Bid, s.Hands[s.Actor()][0], s.Hands[s.Actor()][1])
			s = step(t, s, s.Actor(), "pass", nil)
			s = step(t, s, s.Actor(), "pass", nil)
			if level <= 7 {
				s = step(t, s, s.Actor(), "pass", nil)
			}
			if (s.Stage == "dealer-choice") != (n == 4) {
				t.Fatalf("unexpected dealer eligibility: %d seats, level %d, stage %s", n, level, s.Stage)
			}
		}
	}
	s := dealerWhistChoice(t, 7)
	s.Stage, s.Turn = "defend", s.Defenders[1]
	s = step(t, s, s.Actor(), "half", nil)
	s = step(t, s, s.Actor(), "pass", nil)
	if s.Stage != "dealer-choice" || s.Actor() != s.Dealer {
		t.Fatal("half-whist must offer dealer defence after the first defender declines")
	}
}

func TestDealerWhistAfterHalfWhist(t *testing.T) {
	for _, level := range []int{6, 7} {
		for _, fromOption := range []bool{false, true} {
			r := DefaultRules()
			r.Stalingrad = false
			r.DealerBonus = false
			s := auctionWinner(t, 4, r, Contract{Level: level, Suit: 2})
			s = step(t, s, s.Actor(), "declare", s.Bid, s.Hands[s.Actor()][0], s.Hands[s.Actor()][1])
			first, second := s.Defenders[0], s.Defenders[1]
			half := second
			if fromOption {
				s = step(t, s, first, "whist", nil)
				s = step(t, s, second, "pass", nil)
				s = step(t, s, first, "half", nil)
				half = first
			} else {
				s = step(t, s, first, "pass", nil)
				s = step(t, s, second, "half", nil)
			}
			if s.Stage != "return" || s.Actor() == s.Dealer {
				t.Fatal("defender must have the chance to rewhist before the dealer")
			}
			s = step(t, s, s.Actor(), "pass", nil)
			if s.Stage != "dealer-choice" || s.Actor() != s.Dealer || s.Defence[half] != 3 {
				t.Fatal("dealer missing after pass and half-whist")
			}
			for _, refusal := range []string{"dealer-skip", "pass"} {
				declined := s
				if refusal == "pass" {
					declined = step(t, declined, declined.Dealer, "dealer-second", nil)
				}
				declined = step(t, declined, declined.Dealer, refusal, nil)
				want := 8
				if declined.Stage != "round" || declined.History[0].Whists[half][declined.Declarer] != want {
					t.Fatalf("dealer refusal must preserve the half-whist award: level=%d firstHalf=%t stage=%s whists=%v", level, fromOption, declined.Stage, declined.History[0].Whists)
				}
			}
			s = step(t, s, s.Dealer, "dealer-first", nil)
			s = step(t, s, s.Dealer, "whist", nil)
			if s.HalfSeat != -1 || s.Defence[half] != 1 || s.Controller != s.Dealer {
				t.Fatal("dealer whist must replace half-whist and control both hands")
			}
			s.TrickNo = 10
			s.Taken[s.Declarer] = level
			s.Taken[first] = 10 - level
			s.score()
			if s.History[0].Whists[half][s.Declarer] != 0 || s.History[0].Whists[s.Dealer][s.Declarer] != (10-level)*2*2*(level-5) {
				t.Fatal("only the dealer should receive defensive whists after rewhisting")
			}
		}
	}
}

func TestDealerWhistScoresAndClaims(t *testing.T) {
	for _, level := range []int{6, 7} {
		for _, taken := range []int{level - 1, level, 9} {
			for _, full := range []bool{false, true} {
				s := dealerWhistChoice(t, level)
				s = step(t, s, s.Dealer, "dealer-second", nil)
				s = step(t, s, s.Dealer, "whist", nil)
				if full {
					s.Rules.Responsibility = "full"
				} else {
					s.Rules.Responsibility = "half"
				}
				s.TrickNo = 10
				s.Taken[s.Declarer] = taken
				s.Taken[s.Defenders[0]] = 10 - taken
				s.score()
				l := s.History[0]
				base := 2 * (level - 5)
				expected := (10 - taken + 2*max(0, level-taken)) * 2 * base
				if l.Whists[s.Dealer][s.Declarer] != expected {
					t.Fatalf("dealer whists: %d, want %d", l.Whists[s.Dealer][s.Declarer], expected)
				}
				ob := 4
				if level == 7 {
					ob = 2
				}
				mountain := max(0, ob-(10-taken)) * base
				if full {
					mountain *= 2
				}
				if l.Mountain[s.Dealer] != mountain {
					t.Fatal("dealer responsibility incorrect")
				}
				for _, seat := range s.Defenders {
					if l.Whists[seat][s.Declarer] != 0 || l.Mountain[seat] != 0 {
						t.Fatal("passed defender received dealer's score")
					}
				}
			}
		}
	}
	s := dealerWhistChoice(t, 7)
	s = step(t, s, s.Dealer, "dealer-first", nil)
	s = step(t, s, s.Dealer, "whist", nil)
	s.Turn = s.Declarer
	if !s.canClaim() {
		t.Fatal("declarer cannot propose to dealer")
	}
	s = step(t, s, s.Declarer, "claim", nil)
	s = step(t, s, s.Dealer, "accept-claim", nil)
	if s.Stage != "round" || s.Taken[s.Dealer] != 0 || s.Taken[s.Defenders[0]] != 10 {
		t.Fatal("claim credited tricks to non-playing dealer")
	}
}

func TestBotDealerMaySkipAndDealCanFinish(t *testing.T) {
	s := dealerWhistChoice(t, 7)
	if got := BotCommand(s.View(s.Dealer)); got.Action != "dealer-skip" {
		t.Fatal("bot dealer has no safe choice")
	}
	_, choices := s.BotChoices(s.Dealer, nil)
	if len(choices) != 3 {
		t.Fatal("bot missing dealer choices")
	}
	s = step(t, s, s.Dealer, "dealer-first", nil)
	s = step(t, s, s.Dealer, "whist", nil)
	for count := 0; count < 50 && s.Stage != "round"; count++ {
		command := BotCommand(s.View(s.Actor()))
		var err error
		s, err = Apply(s, command, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if s.Stage != "round" || s.TrickNo != 10 {
		t.Fatal("dealer-controlled play cannot finish")
	}
}
