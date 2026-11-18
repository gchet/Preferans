package game

import "testing"

// With closed defence a human defender keeps his own hand, but a bot hand the dealer
// inspected and whisted on is played by the dealer.
func TestDealerPlaysInspectedBotHandInClosedWhist(t *testing.T) {
	for _, index := range []int{0, 1} {
		for _, open := range []bool{false, true} {
			s := dealerWhistChoice(t, 7)
			chosen, other := s.Defenders[index], s.Defenders[1-index]
			s.Players[chosen].Bot = true
			action, mode := "dealer-first", "closed"
			if index == 1 {
				action = "dealer-second"
			}
			if open {
				mode = "open"
			}
			s = step(t, s, s.Dealer, action, nil)
			s = step(t, s, s.Dealer, "whist", nil)
			s = step(t, s, s.Dealer, mode, nil)
			s.Turn = chosen
			if s.Actor() != s.Dealer {
				t.Fatalf("index=%d open=%t: bot kept playing the hand the dealer whisted on", index, open)
			}
			if v := s.View(s.Dealer); len(v.Actions) != 1 || v.Actions[0] != "play" || len(v.PlayHand) != len(s.Hands[chosen]) {
				t.Fatalf("dealer cannot play the inspected bot hand: %v", v.Actions)
			}
			s.Turn = other
			if want := other; !open && s.Actor() != want || open && s.Actor() != s.Dealer {
				t.Fatalf("index=%d open=%t: the other defender's control changed", index, open)
			}
		}
	}
	// A human defender plays his own hand in closed defence.
	s := dealerWhistChoice(t, 7)
	s = step(t, s, s.Dealer, "dealer-first", nil)
	s = step(t, s, s.Dealer, "whist", nil)
	s = step(t, s, s.Dealer, "closed", nil)
	s.Turn = s.Defenders[0]
	if s.Actor() != s.Defenders[0] {
		t.Fatal("human defender lost his hand in closed defence")
	}
}

// A whole closed-defence deal where the dealer whisted on a bot hand: the dealer must play that hand to the end.
func TestDealerPlaysInspectedBotHandWholeDeal(t *testing.T) {
	for deal := 0; deal < 20; deal++ {
		s := dealerWhistChoice(t, 7)
		chosen := s.Defenders[deal%2]
		for p := range s.Players {
			s.Players[p].Bot = p != s.Dealer
		}
		action := "dealer-first"
		if deal%2 == 1 {
			action = "dealer-second"
		}
		s = step(t, s, s.Dealer, action, nil)
		s = step(t, s, s.Dealer, "whist", nil)
		s = step(t, s, s.Dealer, "closed", nil)
		dealerPlays := 0
		for guard := 0; guard < 80 && s.Stage != "round" && s.Stage != "finished"; guard++ {
			actor := s.Actor()
			if s.Stage == "play" && s.Turn == chosen && actor != s.Dealer {
				t.Fatalf("deal %d: bot %d plays the hand the dealer whisted on", deal, actor)
			}
			var c Command
			if actor == s.Dealer {
				v := s.View(actor)
				if s.Stage == "trick" {
					c = Command{Action: "collect"}
				} else if len(v.Legal) > 0 && len(v.Actions) > 0 && v.Actions[0] == "play" {
					c = Command{Action: "play", Cards: []Card{v.Legal[0]}}
					dealerPlays++
				} else {
					t.Fatalf("deal %d: dealer stuck at %s: %v", deal, s.Stage, v.Actions)
				}
			} else if s.Stage == "trick" {
				c = Command{Action: "collect"}
			} else {
				c = BotCommand(s.View(actor))
			}
			c.ID, c.Seat, c.Revision = ID(), actor, s.Revision
			n, e := Apply(s, c, nil)
			if e != nil {
				t.Fatalf("deal %d %s seat %d %s: %v", deal, s.Stage, actor, c.Action, e)
			}
			s = n
		}
		if dealerPlays == 0 {
			t.Fatalf("deal %d: dealer never played the bot hand", deal)
		}
	}
}
