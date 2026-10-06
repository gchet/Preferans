package game

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestSettlementChatExamples(t *testing.T) {
	for _, tc := range []struct {
		mountain, want []int
	}{
		{[]int{8, 19, 14}, []int{92, -88, -4}},
		{[]int{8, 19, 14, 12}, []int{106, -134, -22, 50}},
	} {
		n := len(tc.mountain)
		s := &State{Players: make([]Player, n), Mountain: tc.mountain, Pool: make([]int, n), Whists: matrix(n)}
		// Net mutual whists: -26, +24, +2 (and 0 for the fourth seat).
		s.Whists[1][0], s.Whists[2][0] = 24, 2
		for i := range s.Pool {
			s.Pool[i] = 30
		}
		if got := s.ResultNumerators(); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("n=%d: got %v, want %v", n, got, tc.want)
		}
	}
}

func TestSettlementMatchesPairwiseWithAmnesty(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	for _, n := range []int{3, 4} {
		for trial := 0; trial < 1000; trial++ {
			s := &State{Players: make([]Player, n), Mountain: make([]int, n), Pool: make([]int, n), Whists: matrix(n)}
			g := make([]int, n)
			for i := range g {
				s.Mountain[i], s.Pool[i] = rng.Intn(300), rng.Intn(100)
				g[i] = s.Mountain[i] + 2*(30-s.Pool[i])
				for j := range g {
					if i != j {
						s.Whists[i][j] = rng.Intn(200)
					}
				}
			}
			minimum := g[0]
			for _, v := range g {
				minimum = min(minimum, v)
			}
			for i := range g {
				g[i] -= minimum
			}
			want := make([]int, n)
			for i := range g {
				for j := range g {
					if i != j {
						want[i] += 10*(g[j]-g[i]) + n*(s.Whists[i][j]-s.Whists[j][i])
					}
				}
			}
			got := s.ResultNumerators()
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("n=%d trial=%d: %v != %v", n, trial, got, want)
			}
			sum := 0
			for _, v := range got {
				sum += v
			}
			if sum != 0 {
				t.Fatal("nonzero total")
			}
		}
	}
}
