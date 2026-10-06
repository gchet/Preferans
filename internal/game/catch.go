package game

import "preferans/locales"

func (s *State) beginCatching() {
	s.MisereCards = append(append([]Card{}, s.Hands[s.Declarer]...), s.Discard...)
	if s.Contract.NoTalon {
		s.MisereCards = append(s.MisereCards, s.Talon...)
	}
	sortHand(s.MisereCards)
	s.MiserePlayed = nil
	s.Stage = "catch"
	s.Open = false
	s.Controller = -1
	s.Turn = s.Defenders[0]
}

func (s *State) isCatcher(seat int) bool {
	for _, p := range s.Defenders {
		if p == seat {
			return true
		}
	}
	return false
}

func (s *State) chooseCatcher(c Command) error {
	if !s.isCatcher(c.Seat) || (c.Action != "catch" && c.Action != "trust") {
		return locales.Errorf("go.internal.game.catch.text001")
	}
	s.Defence[c.Seat] = 1
	if c.Action == "catch" {
		s.Defence[c.Seat] = 2
	}
	a, b := s.Defenders[0], s.Defenders[1]
	if (s.Defence[a] == 2 && s.Defence[b] == 1) || (s.Defence[a] == 1 && s.Defence[b] == 2) {
		s.Controller = a
		if s.Defence[b] == 2 {
			s.Controller = b
		}
		s.Open = true
		s.beginTrick(s.Active()[0])
	} else {
		s.Turn = a
		if c.Seat == a {
			s.Turn = b
		}
	}
	return nil
}
