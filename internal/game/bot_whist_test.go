package game

import "testing"

func TestBotPassesThenRewhists(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, secondChoice := range []string{"pass", "half", "whist"} {
			s := auctionWinner(t, n, DefaultRules(), Contract{Level: 7, Suit: 1})
			s = step(t, s, s.Actor(), "declare", s.Bid, s.Hands[s.Actor()][0], s.Hands[s.Actor()][1])
			first, second := s.Defenders[0], s.Defenders[1]
			command := BotCommand(s.View(first))
			if command.Action != "pass" {
				t.Fatalf("initial defence: %s", command.Action)
			}
			var err error
			s, err = Apply(s, command, nil)
			if err != nil {
				t.Fatal(err)
			}
			s = step(t, s, second, secondChoice, nil)
			if secondChoice == "whist" {
				if s.Stage != "mode" || s.Actor() != second {
					t.Fatal("partner whist must stand")
				}
				continue
			}
			if s.Stage != "return" || s.Actor() != first {
				t.Fatal("missing first defender's return")
			}
			command = BotCommand(s.View(first))
			if command.Action != "whist" {
				t.Fatal("bot must rewhist")
			}
			s, err = Apply(s, command, nil)
			if err != nil {
				t.Fatal(err)
			}
			if s.Stage != "mode" || s.Controller != first || s.Defence[second] != 1 {
				t.Fatal("rewhist must control defence")
			}
			s = step(t, s, first, "open", nil)
			s.Turn = second
			if s.Actor() != first {
				t.Fatal("rewhister must control both open hands")
			}
		}
	}
}

func TestFirstDefenderMayKeepPassAfterTwoPasses(t *testing.T) {
	for _, n := range []int{3, 4} {
		s := auctionWinner(t, n, DefaultRules(), Contract{Level: 8, Suit: 1})
		s = step(t, s, s.Actor(), "declare", s.Bid, s.Hands[s.Actor()][0], s.Hands[s.Actor()][1])
		s = step(t, s, s.Actor(), "pass", nil)
		s = step(t, s, s.Actor(), "pass", nil)
		if s.Stage != "return" || s.HalfSeat != -1 {
			t.Fatal("missing return after passes")
		}
		s = step(t, s, s.Actor(), "pass", nil)
		if s.Stage != "round" {
			t.Fatal("confirmed pass must finish the deal")
		}
	}
}

func TestBotPassesToExistingWhister(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, human := range []bool{true, false} {
			s := auctionWinner(t, n, DefaultRules(), Contract{Level: 8, Suit: 1})
			c := BotCommand(s.View(s.Actor()))
			var err error
			s, err = Apply(s, c, nil)
			if err != nil {
				t.Fatal(err)
			}
			first, second := s.Defenders[0], s.Defenders[1]
			s.Players[first].Bot = !human
			s.Players[second].Bot = true
			if got := BotCommand(s.View(first)).Action; got != "pass" {
				t.Fatalf("first defender: %s", got)
			}
			s = step(t, s, first, "whist", nil)
			c = BotCommand(s.View(second))
			if c.Action != "pass" {
				t.Fatalf("second defender: %s", c.Action)
			}
			s, err = Apply(s, c, nil)
			if err != nil {
				t.Fatal(err)
			}
			if s.Open || s.Stage != "mode" || s.Actor() != first {
				t.Fatal("single whister must choose open or closed defence")
			}
			v := s.View(first)
			if len(v.Actions) != 2 || v.Actions[0] != "open" || v.Actions[1] != "closed" {
				t.Fatal("missing defence mode choice")
			}
			closed := step(t, s, first, "closed", nil)
			closed.Turn = second
			if closed.Stage != "play" || closed.Open || closed.Actor() != second || len(closed.View(first).Players[second].Cards) != 0 {
				t.Fatal("closed defence must leave the bot's hand private and controlled by the bot")
			}
			if got := BotCommand(v).Action; got != "open" {
				t.Fatal("bot whister must choose open defence")
			}
			s = step(t, s, first, "open", nil)
			if !s.Open || s.Stage != "play" || s.Controller != first {
				t.Fatal("single whister must control both hands after choosing open defence")
			}
			s.Turn = second
			if s.Actor() != first || len(s.View(first).PlayHand) != 10 {
				t.Fatal("passed hand is not controlled by whister")
			}
		}
	}
}
