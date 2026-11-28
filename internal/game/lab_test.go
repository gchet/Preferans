package game

import "testing"

// The simulations must lead from the seat the engine leads from: play starts with
// s.Active()[0] (see beginTrick callers), which is the first seat after the dealer
// and, with four players, never the dealer.
func TestLabEvaluatorActiveMatchesEngine(t *testing.T) {
	for _, players := range []int{3, 4} {
		for dealer := 0; dealer < players; dealer++ {
			s, err := New(players, 30)
			if err != nil {
				t.Fatal(err)
			}
			s.Dealer = dealer
			want := s.Active()
			e := newSimEvaluator(s.View(0), 1)
			if len(want) != 3 {
				t.Fatalf("%d players: expected 3 active seats, got %v", players, want)
			}
			for i := range want {
				if e.active[i] != want[i] {
					t.Fatalf("%d players, dealer %d: active %v, engine %v", players, dealer, e.active, want)
				}
			}
		}
	}
}

func TestLabPoints(t *testing.T) {
	if got := labPoints(30, 3); got != 1 {
		t.Fatalf("30/3 players = %v, want 1", got)
	}
	if got := labPoints(-40, 4); got != -1 {
		t.Fatalf("-40/4 players = %v, want -1", got)
	}
	r := LabResult{Nums: []int{60, -30, -30}}
	if r.Points(0) != 2 || r.Points(1) != -1 {
		t.Fatalf("Points = %v %v", r.Points(0), r.Points(1))
	}
}

// Whole deals with every simulation policy must finish without illegal commands.
func TestLabSmartDealsFinish(t *testing.T) {
	smart, err := ParseLabPolicy("smart,sims=20,samples=20,ms=50")
	if err != nil {
		t.Fatal(err)
	}
	base, err := ParseLabPolicy("samples=20,ms=50")
	if err != nil {
		t.Fatal(err)
	}
	for _, players := range []int{3, 4} {
		for seed := int64(1); seed <= 4; seed++ {
			policies := make([]LabPolicy, players)
			for i := range policies {
				policies[i] = base
			}
			policies[0] = smart
			if r := PlayLabDeal(players, seed, policies); r.Err != nil {
				t.Fatalf("%d players, seed %d: %v", players, seed, r.Err)
			}
		}
	}
}

func TestParseLabPolicyRejectsUnknownKeys(t *testing.T) {
	for _, spec := range []string{"smrat", "bid=maybe", "whist=sometimes", "margin=x", "samples"} {
		if _, err := ParseLabPolicy(spec); err == nil {
			t.Errorf("%q: expected an error", spec)
		}
	}
}
