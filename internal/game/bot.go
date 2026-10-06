package game

// BotCommand sees the same filtered view as a person, never the authoritative state.
func BotCommand(v View) Command {
	c := Command{ID: ID(), Seat: v.Seat, Revision: v.Revision}
	has := func(a string) bool {
		for _, x := range v.Actions {
			if x == a {
				return true
			}
		}
		return false
	}
	switch {
	case has("dealer-skip"):
		// Keep the dealer's optional defence conservative by default.
		c.Action = "dealer-skip"
	case has("catch"):
		c.Action = "catch"
		for p, d := range v.Defence {
			if p != v.Seat && d == 2 {
				c.Action = "trust"
				break
			}
		}
	case has("reject-claim"):
		// Full claim search runs asynchronously; only trivial concessions are accepted here.
		c.Action = "reject-claim"
		if v.Claim != nil && v.Contract != nil && ((!v.Contract.Misere && *v.Claim == 0) || (v.Contract.Misere && *v.Claim == 10-v.TrickNo)) {
			c.Action = "accept-claim"
		}
	case has("collect"):
		c.Action = "collect"
	case has("next"):
		c.Action = "next"
	case has("ready"):
		c.Action = "ready"
	case has("bid"):
		c.Action = "pass"
		if v.DebugBots != "" && v.Players[v.Seat].Bot {
			if v.DebugBots == "misere" {
				chosen := -1
				for offset := 1; offset <= len(v.Players); offset++ {
					seat := (v.Dealer + offset) % len(v.Players)
					if len(v.Players) == 4 && seat == v.Dealer {
						continue
					}
					if v.Players[seat].Bot {
						chosen = seat
						break
					}
				}
				if chosen == v.Seat {
					for _, b := range v.Contracts {
						if b.Misere {
							bb := b
							c.Action = "bid"
							c.Contract = &bb
							break
						}
					}
				}
			}
			return c
		}
		c = botBid(v, c)
	case has("declare-no-talon"):
		c.Action = "declare-no-talon"
		for _, b := range v.Contracts {
			if c.Contract == nil || handTricks(v.Hand, b.Suit) > handTricks(v.Hand, c.Contract.Suit) {
				b := b
				c.Contract = &b
			}
		}
	case has("declare"):
		c = botDiscard(v, c)
	case has("open"):
		c.Action = "open"
	case has("whist"):
		c.Action = "whist"
		if v.Stage == "defend" && has("pass") {
			c.Action = "pass"
		}
	case has("play"):
		c.Action = "play"
		if len(v.Legal) > 0 {
			best := v.Legal[0]
			for _, x := range v.Legal {
				if x.Rank() < best.Rank() {
					best = x
				}
			}
			c.Cards = []Card{best}
		}
	}
	return c
}
