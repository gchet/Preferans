package game

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPassAmnestyFromCurrentTricks(t *testing.T) {
	cases := []struct {
		mountain, taken, delta []int
		price, cancelled       int
	}{
		{[]int{6, 10, 20}, []int{5, 1, 4}, []int{32, 0, 24}, 8, 1},
		{[]int{0, 0, 0}, []int{5, 1, 4}, []int{32, 0, 24}, 8, 1},
		{[]int{10, 20, 30}, []int{3, 3, 4}, []int{0, 0, 2}, 2, 3},
		{[]int{4, 4, 4}, []int{0, 4, 6}, []int{0, 16, 24}, 4, 0},
		{[]int{8, 12, 16, 20}, []int{2, 3, 3, 2}, []int{0, 8, 8, 0}, 8, 2},
		{[]int{8, 12, 16, 20}, []int{3, 3, 4, 0}, []int{24, 24, 32, 0}, 8, 0},
	}
	for _, tc := range cases {
		s := testState(t, len(tc.mountain))
		s.Mountain = append([]int(nil), tc.mountain...)
		s.Talon = []Card{0, 8}
		s.AllPass = true
		s.PassPrice = tc.price
		s.PassStreak = 1
		s.Round = 1
		s.TrickNo = 10
		s.Taken = append([]int(nil), tc.taken...)
		expected := s.Clone()
		want := append([]int(nil), tc.mountain...)
		for seat, taken := range tc.taken {
			expected.Mountain[seat] += taken * tc.price
			want[seat] += tc.delta[seat]
			if taken == 0 {
				expected.Pool[seat] += tc.price
			}
		}
		s.score()
		if !reflect.DeepEqual(s.Mountain, want) {
			t.Fatalf("mountain=%v want=%v", s.Mountain, want)
		}
		ledger := s.History[0]
		if ledger.AmnestyTricks != tc.cancelled || ledger.Amnesty != tc.cancelled*tc.price || !reflect.DeepEqual(ledger.Mountain, tc.delta) {
			t.Fatalf("incorrect ledger: %+v", ledger)
		}
		if !reflect.DeepEqual(s.Taken, tc.taken) {
			t.Fatal("actual trick counts changed")
		}
		if !reflect.DeepEqual(s.Pool, expected.Pool) {
			t.Fatal("amnesty granted a zero-trick pool bonus")
		}
		if !reflect.DeepEqual(s.ResultNumerators(), expected.ResultNumerators()) {
			t.Fatal("uniform amnesty changed settlement")
		}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var restored State
		if err = json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		if restored.History[0].AmnestyTricks != tc.cancelled || !reflect.DeepEqual(restored.View(0).Mountain, want) {
			t.Fatal("amnesty not preserved")
		}
	}
}

func TestAmnestyDoesNotApplyToOrdinaryGame(t *testing.T) {
	s := claimState(t, 3, Contract{Level: 8, Suit: 4})
	s.Mountain = []int{10, 20, 30}
	s.Taken[s.Declarer] = 8
	s.TrickNo = 10
	before := append([]int(nil), s.Mountain...)
	s.score()
	if s.History[0].Amnesty != 0 {
		t.Fatal("ordinary game received amnesty")
	}
	for seat, mountain := range s.Mountain {
		if mountain < before[seat] {
			t.Fatal("ordinary mountain reduced")
		}
	}
}
