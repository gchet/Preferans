package game

// misereBidder identifies the only player allowed to raise their misere.
func (s *State) misereBidder() int {
	for _, call := range s.Auction {
		if call.Contract != nil && call.Contract.Misere && !call.Contract.NoTalon {
			return call.Seat
		}
	}
	return -1
}

// noTalonNineFloor preserves the suit floor already announced by this bidder.
func (s *State) noTalonNineFloor(seat int) int {
	floor := 0
	for _, call := range s.Auction {
		if call.Seat == seat && call.Contract != nil && !call.Contract.Misere && !call.Contract.NoTalon && call.Contract.Level == 9 {
			floor = max(floor, call.Contract.Suit)
		}
	}
	return floor
}
