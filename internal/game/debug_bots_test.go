package game

import "testing"

func TestDebugBotAuctionModes(t *testing.T) {
	for _, n := range []int{3, 4} {
		for _, mode := range []string{"pass", "misere"} {
			for dealer := 0; dealer < n; dealer++ {
				s := testState(t, n)
				s.Players[0].Ready = false
				for i := 1; i < n; i++ {
					s.Players[i].Bot = true
				}
				cmd := Command{ID: ID(), Seat: 0, Revision: s.Revision, Action: "debug-bots", DebugBots: mode}
				var err error
				s, err = Apply(s, cmd, nil)
				if err != nil {
					t.Fatal(err)
				}
				s = s.Clone()
				s.Dealer = dealer
				s.Players[0].Ready = true
				if s.View(0).DebugBots != mode {
					t.Fatal("mode not saved/shared")
				}
				s = step(t, s, 0, "start", nil)
				for s.Stage == "auction" {
					c := BotCommand(s.View(s.Actor()))
					if !s.Players[s.Actor()].Bot {
						c = Command{ID: ID(), Seat: s.Actor(), Revision: s.Revision, Action: "pass"}
					}
					s, err = Apply(s, c, nil)
					if err != nil {
						t.Fatal(err)
					}
				}
				if mode == "pass" && !s.AllPass {
					t.Fatal("bots did not pass")
				}
				if mode == "misere" && (s.Bid == nil || !s.Bid.Misere || !s.Players[s.Declarer].Bot || (n == 4 && s.Declarer == s.Dealer)) {
					t.Fatal("bot misere not declared")
				}
			}
		}
	}
}

func TestDebugModeOnlyHostBeforeReady(t *testing.T) {
	s := testState(t, 3)
	c := Command{ID: ID(), Seat: 0, Revision: s.Revision, Action: "debug-bots", DebugBots: "pass"}
	if _, err := Apply(s, c, nil); err == nil {
		t.Fatal("ready host changed mode")
	}
	s.Players[0].Ready = false
	c.Seat = 1
	if _, err := Apply(s, c, nil); err == nil {
		t.Fatal("guest changed mode")
	}
	c.Seat = 0
	c.DebugBots = "bad"
	if _, err := Apply(s, c, nil); err == nil {
		t.Fatal("invalid mode")
	}
}
