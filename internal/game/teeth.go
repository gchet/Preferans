package game

import "preferans/locales"

// TeethMatrix returns a complete players x players matrix; old saves have none stored.
func (s *State) TeethMatrix() [][]int {
	n := len(s.Players)
	out := make([][]int, n)
	for i := range out {
		out[i] = make([]int, n)
		if i < len(s.Teeth) {
			copy(out[i], s.Teeth[i])
		}
	}
	return out
}

// drawTeeth adds or removes the sender's own teeth on another player.
// Nobody can touch teeth drawn by someone else.
func (s *State) drawTeeth(c Command) error {
	n := len(s.Players)
	if s.Stage == "lobby" || s.Players[c.Seat].Bot {
		return locales.Errorf("go.game.teeth_unavailable")
	}
	if c.Aim == nil || *c.Aim < 0 || *c.Aim >= n || *c.Aim == c.Seat || c.Delta == 0 || c.Delta < -MaxTeeth || c.Delta > MaxTeeth {
		return locales.Errorf("go.game.teeth_invalid")
	}
	matrix := s.TeethMatrix()
	count := matrix[c.Seat][*c.Aim] + c.Delta
	if count < 0 || count > MaxTeeth {
		return locales.Errorf("go.game.teeth_invalid")
	}
	matrix[c.Seat][*c.Aim] = count
	s.Teeth = matrix
	return nil
}
