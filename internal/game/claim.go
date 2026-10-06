package game

func (s *State) canClaim() bool {
	if s.Stage != "play" || s.Claim != nil || s.AllPass || s.Contract == nil || s.Turn != s.Declarer {
		return false
	}
	if s.Contract.Misere || (s.Contract.Level == 10 && s.Agreements().TenCheck) {
		return true
	}
	if !s.Open || s.Controller < 0 {
		return false
	}
	if s.DealerWhist {
		return true
	}
	whisters := 0
	for _, p := range s.Defenders {
		if s.Defence[p] == 2 {
			whisters++
		}
	}
	return whisters == 1 && s.Defence[s.Controller] == 2
}

func (s *State) claimReviewers() []int {
	if s.Contract.Misere || (s.Contract.Level == 10 && s.Agreements().TenCheck) {
		var seats []int
		for _, p := range s.Active() {
			if p != s.Declarer {
				seats = append(seats, p)
			}
		}
		return seats
	}
	return []int{s.Controller}
}

func (s *State) claimResponder() int {
	for _, p := range s.claimReviewers() {
		accepted := false
		for _, a := range s.ClaimAccepted {
			if p == a {
				accepted = true
			}
		}
		if !accepted {
			return p
		}
	}
	return -1
}

func (s *State) canRespondToClaim(seat int) bool {
	if s.Claim == nil {
		return false
	}
	for _, p := range s.ClaimAccepted {
		if p == seat {
			return false
		}
	}
	for _, p := range s.claimReviewers() {
		if p == seat {
			return true
		}
	}
	return false
}
