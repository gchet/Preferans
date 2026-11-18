package game

import "testing"

// Every player is credited for a set, the dealer of a four-player game included.
func TestSetCreditsEveryPlayer(t *testing.T) {
	const rate = 8 // seven: base 4, rate 8
	run := func(n int, mutate func(s *State)) *State {
		s := scoreState(t, n, 7)
		s.Taken = []int{5, 3, 2, 0}[:n]
		s.HalfSeat = -1
		mutate(s)
		s.score()
		return s
	}
	whists := func(s *State) []int {
		out := make([]int, len(s.Players))
		for p := range out {
			out[p] = s.History[0].Whists[p][0]
		}
		return out
	}
	same := func(got, want []int) bool {
		for i := range want {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}
	for _, half := range []bool{false, true} {
		// Gentleman: the whister's tricks and both set credits are summed and split in two.
		s := run(4, func(s *State) {
			s.Defence[1], s.Defence[2] = 2, 1
			if half {
				// The second defender chose "own", then the first one rewhisted.
				s.Defence[1], s.Defence[2], s.HalfSeat = 1, 2, 1
			}
		})
		want := []int{0, 36, 36, 2 * rate}
		if half {
			want = []int{0, 2 * rate, 7 * rate, 2 * rate}
		}
		if got := whists(s); !same(got, want) {
			t.Fatalf("gentleman, own=%t: %v want %v", half, got, want)
		}
		// Greedy: the whister keeps his tricks, each defender and the dealer get a set credit.
		g := run(4, func(s *State) {
			r := DefaultRules()
			r.Whist = "greedy"
			s.Rules = &r
			s.Defence[1], s.Defence[2] = 2, 1
			if half {
				s.Defence[1], s.Defence[2], s.HalfSeat = 1, 2, 1
			}
		})
		want = []int{0, 5*rate + 2*rate, 2 * rate, 2 * rate}
		if half {
			want = []int{0, 2 * rate, 5*rate + 2*rate, 2 * rate}
		}
		if got := whists(g); !same(got, want) {
			t.Fatalf("greedy, own=%t: %v want %v", half, got, want)
		}
	}
	both := run(4, func(s *State) {})
	if got := whists(both); !same(got, []int{0, 3*rate + 2*rate, 2*rate + 2*rate, 2 * rate}) {
		t.Fatalf("both whist: %v", got)
	}
	three := run(3, func(s *State) { s.Defence[1], s.Defence[2] = 2, 1 })
	if got := whists(three); !same(got, []int{0, 36, 36}) {
		t.Fatalf("three players: %v", got)
	}
	dealer := run(4, func(s *State) {
		s.Defence[1], s.Defence[2], s.Defence[3] = 1, 1, 2
		s.DealerWhist = true
	})
	if got := whists(dealer); !same(got, []int{0, 2 * rate, 2 * rate, (5 + 2) * rate}) {
		t.Fatalf("dealer whist: %v", got)
	}
	// No set, no credit.
	exact := run(4, func(s *State) { s.Taken = []int{7, 2, 1, 0}; s.Defence[1], s.Defence[2] = 2, 1 })
	if got := whists(exact); got[3] != 0 || got[2] != 0 {
		t.Fatalf("credit without a set: %v", got)
	}
}
