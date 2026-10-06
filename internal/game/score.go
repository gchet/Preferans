package game

import "preferans/locales"

func matrix(n int) [][]int {
	m := make([][]int, n)
	for i := range m {
		m[i] = make([]int, n)
	}
	return m
}
func Bonus(t []Card) int {
	if len(t) != 2 {
		return 0
	}
	a, b := t[0], t[1]
	if a.Rank() == 7 && b.Rank() == 7 {
		return 3
	}
	if a.Suit() == b.Suit() {
		if a.Rank()+b.Rank() == 13 {
			return 2
		}
		if (a.Rank() == 6 && b.Rank() == 5) || (a.Rank() == 5 && b.Rank() == 6) {
			return 1
		}
	}
	if a.Rank() == 7 || b.Rank() == 7 {
		return 1
	}
	return 0
}
func (s *State) score() {
	rules := s.Agreements()
	n := len(s.Players)
	l := Ledger{Round: s.Round, Pool: make([]int, n), Mountain: make([]int, n), Whists: matrix(n)}
	if s.AllPass {
		// Cancel the common number of tricks in this deal, including the dealer.
		minimum := 0
		if len(s.Taken) == n && n > 0 {
			minimum = s.Taken[0]
			for _, taken := range s.Taken {
				minimum = min(minimum, taken)
			}
		}
		l.AmnestyTricks = minimum
		l.Amnesty = minimum * s.PassPrice
		l.Label = locales.Format("go.internal.game.score.text001", s.PassPrice)
		for p, t := range s.Taken {
			l.Mountain[p] = (t - minimum) * s.PassPrice
			if t == 0 {
				l.Pool[p] = s.PassPrice
			}
		}
	} else {
		c := *s.Contract
		d := s.Declarer
		l.Label = c.String()
		if c.Misere {
			if s.Taken[d] == 0 {
				l.Pool[d] = 10
			} else {
				l.Mountain[d] = s.Taken[d] * 20
			}
		} else {
			base := 2 * (c.Level - 5)
			rate := 2 * base
			unplayed := s.TrickNo == 0 && len(s.Trick) == 0
			short := c.Level - s.Taken[d]
			if unplayed {
				short = 0
			}
			if short <= 0 {
				l.Pool[d] = base
			} else {
				l.Mountain[d] = short * rate
			}
			if n == 4 && rules.DealerBonus && !c.NoTalon {
				l.Whists[s.Dealer][d] += Bonus(s.Talon) * rate
			}
			if c.Level != 10 || !rules.TenCheck {
				var ws []int
				for _, p := range s.Defenders {
					if s.Defence[p] == 2 {
						ws = append(ws, p)
					}
					if s.Defence[p] == 3 {
						ob := 4
						if c.Level == 7 {
							ob = 2
						}
						l.Whists[p][d] += ob / 2 * rate
					}
				}
				if s.DealerWhist {
					ws = []int{s.Dealer}
				}
				if !unplayed {
					if len(ws) == 1 {
						a := ws[0]
						b := s.Defenders[0]
						if b == a {
							b = s.Defenders[1]
						}
						total := s.Taken[a] + s.Taken[b]
						if s.DealerWhist {
							total = s.Taken[s.Defenders[0]] + s.Taken[s.Defenders[1]]
							l.Whists[a][d] += (total + 2*max(0, short)) * rate
						} else if short > 0 && rules.Whist == "gentleman" && s.HalfSeat != b {
							v := (total + 2*short) * rate / 2
							l.Whists[a][d] += v
							l.Whists[b][d] += v
						} else {
							l.Whists[a][d] += total * rate
							if short > 0 {
								l.Whists[a][d] += short * rate
								if s.HalfSeat == b {
									l.Whists[a][d] += short * rate
								} else {
									l.Whists[b][d] += short * rate
								}
							}
						}
						ob := 1
						if c.Level == 6 {
							ob = 4
						}
						if c.Level == 7 {
							ob = 2
						}
						if total < ob {
							l.Mountain[a] += (ob - total) * base
						}
					}
					if len(ws) == 2 {
						for _, p := range ws {
							l.Whists[p][d] += s.Taken[p] * rate
							if short > 0 {
								l.Whists[p][d] += short * rate
							}
						}
						if c.Level <= 7 {
							ob := 1
							if c.Level == 6 {
								ob = 2
							}
							deficit := max(0, 2*ob-s.Taken[ws[0]]-s.Taken[ws[1]])
							for _, p := range ws {
								if s.Taken[p] < ob {
									l.Mountain[p] += min(ob-s.Taken[p], deficit) * base
								}
							}
						} else {
							if s.Taken[ws[0]]+s.Taken[ws[1]] < 1 {
								l.Mountain[ws[1]] += base
							}
						}
					}
				}
			}
		}
	}
	if rules.Responsibility == "full" && !s.AllPass && s.Contract != nil && !s.Contract.Misere {
		for _, p := range s.Defenders {
			l.Mountain[p] *= 2
		}
		if s.DealerWhist {
			l.Mountain[s.Dealer] *= 2
		}
	}
	if !s.AllPass && s.Contract != nil && l.Pool[s.Declarer] > 0 {
		played := s.TrickNo == 10
		whisted := s.DealerWhist
		for _, p := range s.Defenders {
			whisted = whisted || s.Defence[p] == 2
		}
		if s.Contract.Misere || (s.Contract.Level == 10 && rules.TenCheck) {
			if played && rules.SpecialExitsPasses() {
				s.PassStreak = 0
			}
		} else if !rules.RequiresWhistedExit() || (played && whisted) {
			s.PassStreak = 0
		}
	}
	s.recordScore(l)
}

func (s *State) recordScore(l Ledger) {
	n := len(s.Players)
	rules := s.Agreements()
	if s.TalonShown && !s.AllPass && s.Contract != nil && !s.Contract.Misere && s.Declarer != s.Dealer {
		if rules.TalonPenalty == "mountain" {
			l.Mountain[s.Dealer] += 4 * (s.Contract.Level - 5)
		} else {
			l.Whists[s.Declarer][s.Dealer] += 2 * 4 * (s.Contract.Level - 5)
		}
	}
	for i := 0; i < n; i++ {
		s.Pool[i] += l.Pool[i]
		s.Mountain[i] += l.Mountain[i]
		for j := 0; j < n; j++ {
			s.Whists[i][j] += l.Whists[i][j]
		}
	}
	s.History = append(s.History, l)
	s.Stage = "round"
	if s.FinishDue() {
		s.Stage = "finished"
	}
}

// ResultNumerators returns exact scores with the common denominator player count.
// Centering pool and mountain values avoids rounding during settlement.
func (s *State) ResultNumerators() []int {
	n := len(s.Players)
	out := make([]int, n)
	total := 0
	for i := 0; i < n; i++ {
		total += s.Mountain[i] - 2*s.Pool[i]
	}
	for i := 0; i < n; i++ {
		out[i] = 10 * (total - n*(s.Mountain[i]-2*s.Pool[i]))
		for j := 0; j < n; j++ {
			out[i] += n * (s.Whists[i][j] - s.Whists[j][i])
		}
	}
	return out
}
