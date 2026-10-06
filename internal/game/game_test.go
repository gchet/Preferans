package game

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"testing"
)

func testState(t *testing.T, n int) *State {
	t.Helper()
	s, e := New(n, 30)
	if e != nil {
		t.Fatal(e)
	}
	for i := range s.Players {
		s.Players[i].Ready = true
	}
	return s
}
func fixedDeck(seed int64) []Card {
	r := rand.New(rand.NewSource(seed))
	d := make([]Card, 32)
	for i := range d {
		d[i] = Card(i)
	}
	r.Shuffle(32, func(i, j int) { d[i], d[j] = d[j], d[i] })
	return d
}
func step(t *testing.T, s *State, seat int, action string, c *Contract, cards ...Card) *State {
	t.Helper()
	n, e := Apply(s, Command{ID: ID(), Seat: seat, Revision: s.Revision, Action: action, Contract: c, Cards: cards}, nil)
	if e != nil {
		t.Fatalf("%s round=%d stage=%s turn=%d: %v", action, s.Round, s.Stage, s.Turn, e)
	}
	return n
}
func passRound(t *testing.T, s *State) *State {
	t.Helper()
	for s.Stage == "auction" {
		s = step(t, s, s.Turn, "pass", nil)
	}
	for s.Stage == "play" || s.Stage == "trick" {
		if s.Stage == "trick" {
			s = step(t, s, 0, "collect", nil)
		} else {
			s = step(t, s, s.Actor(), "play", nil, s.LegalCards()[0])
		}
	}
	return s
}
func TestPassProgressionAndExit(t *testing.T) {
	for _, n := range []int{3, 4} {
		s := testState(t, n)
		s.Target = 1000
		for i, want := range []int{2, 4, 8, 8, 8} {
			if e := s.deal(fixedDeck(int64(i))); e != nil {
				t.Fatal(e)
			}
			if i > 0 {
				// The bidding dialog receives this list; every six must be
				// disabled and every seven available at the start of bidding.
				available := s.View(s.Actor()).Contracts
				sevens := 0
				for _, b := range available {
					if !b.Misere && b.Level < 7 {
						t.Fatalf("dialog offers %v after all-pass (%d players)", b, n)
					}
					if !b.Misere && b.Level == 7 {
						sevens++
					}
				}
				if sevens != 5 {
					t.Fatalf("dialog offers %d sevens, want all five", sevens)
				}
				low := Contract{Level: 6, Suit: 0}
				if _, e := Apply(s, Command{ID: ID(), Seat: s.Turn, Revision: s.Revision, Action: "bid", Contract: &low}, nil); e == nil {
					t.Fatal("six allowed after all-pass")
				}
				seven := Contract{Level: 7, Suit: 0}
				if _, e := Apply(s, Command{ID: ID(), Seat: s.Turn, Revision: s.Revision, Action: "bid", Contract: &seven}, nil); e != nil {
					t.Fatal(e)
				}
			}
			s = passRound(t, s)
			if s.PassPrice != want {
				t.Fatalf("price %d want %d", s.PassPrice, want)
			}
			l := s.History[len(s.History)-1]
			minimum := s.Taken[0]
			for _, taken := range s.Taken {
				minimum = min(minimum, taken)
			}
			for p, k := range s.Taken {
				if l.Mountain[p] != (k-minimum)*want {
					t.Fatal("wrong mountain")
				}
				if k == 0 && l.Pool[p] != want {
					t.Fatal("zero trick bonus")
				}
			}
		}
		if e := s.deal(fixedDeck(9)); e != nil {
			t.Fatal(e)
		}
		seven := Contract{Level: 7, Suit: 0}
		s = step(t, s, s.Turn, "bid", &seven)
		s = step(t, s, s.Turn, "pass", nil)
		s = step(t, s, s.Turn, "pass", nil)
		s = step(t, s, s.Turn, "declare", &seven, s.Hands[s.Turn][0], s.Hands[s.Turn][1])
		if s.PassStreak == 0 {
			t.Fatal("declaration must not reset pass series")
		}
	}
}
func TestTransactionalAndDedup(t *testing.T) {
	s := testState(t, 3)
	before, _ := json.Marshal(s)
	c := Command{ID: "start", Seat: 0, Revision: 0, Action: "start"}
	next, e := Apply(s, c, fixedDeck(1))
	if e != nil {
		t.Fatal(e)
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("mutated source")
	}
	same, e := Apply(next, c, nil)
	if e != nil || same != next {
		t.Fatal("duplicate was applied")
	}
	bad := Command{ID: "bad", Seat: next.Turn, Revision: 0, Action: "pass"}
	if _, e = Apply(next, bad, nil); e == nil {
		t.Fatal("stale accepted")
	}
	bad.Revision = next.Revision
	bad.Action = "play"
	if _, e = Apply(next, bad, nil); e == nil {
		t.Fatal("invalid phase accepted")
	}
}
func TestLegalFollowAndTrump(t *testing.T) {
	s := testState(t, 3)
	s.Stage = "play"
	s.Turn = 0
	s.Contract = &Contract{Level: 6, Suit: 1}
	s.Trick = []Play{{1, Card(7)}}
	s.Hands = [][]Card{{0, 8, 16}, nil, nil}
	if !reflect.DeepEqual(s.LegalCards(), []Card{0}) {
		t.Fatal(s.LegalCards())
	}
	s.Hands[0] = []Card{8, 16}
	if !reflect.DeepEqual(s.LegalCards(), []Card{8}) {
		t.Fatal(s.LegalCards())
	}
	s.Contract = &Contract{Suit: 4, Misere: true}
	if len(s.LegalCards()) != 2 {
		t.Fatal("trump on misere")
	}
}
func TestTalonWinnerAndThirdLead(t *testing.T) {
	s := testState(t, 4)
	s.Stage = "play"
	s.AllPass = true
	s.Talon = []Card{7, 15}
	s.Hands = [][]Card{{0, 1, 2}, {3, 4, 5}, {6, 8, 9}, nil}
	s.Taken = make([]int, 4)
	s.beginTrick(0)
	s = step(t, s, 0, "play", nil, 0)
	s = step(t, s, 1, "play", nil, 3)
	s = step(t, s, 2, "play", nil, 6)
	if s.Winner != 3 {
		t.Fatal("dealer must win ace")
	}
	s = step(t, s, 0, "collect", nil)
	if s.Turn != 0 {
		t.Fatal("second lead")
	}
	s = step(t, s, 0, "play", nil, 1)
	s = step(t, s, 1, "play", nil, 4)
	s = step(t, s, 2, "play", nil, 8)
	s = step(t, s, 0, "collect", nil)
	if s.Turn != 0 || len(s.Trick) != 0 {
		t.Fatal("third lead")
	}
}
func TestPrivacy(t *testing.T) {
	s := testState(t, 4)
	if e := s.deal(fixedDeck(1)); e != nil {
		t.Fatal(e)
	}
	for p := range s.Players {
		v := s.View(p)
		if len(v.Talon) > 0 {
			t.Fatal("talon leaked during auction")
		}
		for other, sv := range v.Players {
			if other != p && len(sv.Cards) > 0 {
				t.Fatal("hidden hand leaked")
			}
		}
	}
	s.Declarer = 0
	s.Open = true
	s.Stage = "play"
	s.Turn = 1
	s.Controller = 2
	s.Contract = &Contract{Level: 7, Suit: 0}
	v := s.View(2)
	if len(v.Players[0].Cards) > 0 || len(v.PlayHand) != 10 {
		t.Fatal("open whist visibility")
	}
	if len(s.View(3).Players[0].Cards) > 0 {
		t.Fatal("dealer sees declarer")
	}
}
func scoreState(t *testing.T, n, level int) *State {
	s := testState(t, n)
	s.Round = 1
	s.Contract = &Contract{Level: level, Suit: 1}
	s.Declarer = 0
	s.Defenders = []int{1, 2}
	s.Defence = make([]int, n)
	s.Defence[1] = 2
	s.Defence[2] = 2
	s.Taken = make([]int, n)
	s.TrickNo = 10
	s.Trick = []Play{{0, 0}}
	s.Talon = []Card{0, 1}
	return s
}
func TestScores(t *testing.T) {
	for level := 6; level <= 10; level++ {
		s := scoreState(t, 3, level)
		s.Taken = []int{level, 10 - level, 0}
		s.score()
		if s.Pool[0] != 2*(level-5) {
			t.Fatal("pool")
		}
		s = scoreState(t, 3, level)
		s.Taken = []int{level - 1, 11 - level, 0}
		s.score()
		if s.Mountain[0] != 4*(level-5) {
			t.Fatal("declarer mountain")
		}
	}
	s := scoreState(t, 3, 6)
	s.Defence[2] = 1
	s.Taken = []int{5, 5, 0}
	s.score()
	if s.Whists[1][0] != 14 || s.Whists[2][0] != 14 || s.Mountain[0] != 4 {
		t.Fatalf("gentlemen: %+v", s.History)
	}
	s = scoreState(t, 3, 6)
	s.Taken = []int{6, 1, 3}
	s.score()
	if s.Mountain[1] != 0 {
		t.Fatal("defenders fulfilled their joint obligation")
	}
	s = scoreState(t, 3, 8)
	s.Taken = []int{10, 0, 0}
	s.score()
	if s.Mountain[1] != 0 || s.Mountain[2] != 6 {
		t.Fatal("last whister liability")
	}
	s = scoreState(t, 3, 10)
	s.Taken = []int{9, 1, 0}
	s.score()
	if s.Whists[1][0] != 0 || s.Mountain[1] != 0 || s.Mountain[0] != 20 {
		t.Fatal("ten no whist")
	}
}

func TestJointWhistLiability(t *testing.T) {
	for _, level := range []int{6, 7} {
		obligation := 4
		if level == 7 {
			obligation = 2
		}
		for a := 0; a <= 10; a++ {
			for b := 0; b <= 10-a; b++ {
				s := scoreState(t, 3, level)
				s.Taken = []int{10 - a - b, a, b}
				s.score()
				want := max(0, obligation-a-b) * 2 * (level - 5)
				if s.Mountain[1]+s.Mountain[2] != want {
					t.Fatalf("%d: defenders %d/%d, penalty %v, want total %d", level, a, b, s.Mountain, want)
				}
			}
		}
	}
}
func TestBonus(t *testing.T) {
	for _, x := range []struct {
		cards []Card
		want  int
	}{{[]Card{7, 0}, 1}, {[]Card{7, 6}, 2}, {[]Card{7, 15}, 3}, {[]Card{5, 6}, 1}, {[]Card{5, 14}, 0}} {
		if Bonus(x.cards) != x.want {
			t.Fatal(x)
		}
	}
	s := scoreState(t, 4, 7)
	s.Talon = []Card{7, 15}
	s.Taken = []int{6, 2, 2, 0}
	s.score()
	if s.Whists[3][0] != 24 {
		t.Fatal("bonus on failed contract")
	}
}
func TestMisereAndHalf(t *testing.T) {
	s := scoreState(t, 3, 6)
	s.Contract = &Contract{Suit: 4, Misere: true}
	s.Taken = []int{2, 4, 4}
	s.score()
	if s.Mountain[0] != 40 {
		t.Fatal("misere")
	}
	s = scoreState(t, 3, 6)
	s.Stage = "return"
	s.Turn = 2
	s.HalfSeat = 1
	s.Trick = nil
	s.TrickNo = 0
	s = step(t, s, 2, "pass", nil)
	if s.Pool[0] != 2 || s.Whists[1][0] != 8 {
		t.Fatal("half")
	}
}
func TestStalingradAndReturn(t *testing.T) {
	s := testState(t, 3)
	_ = s.deal(fixedDeck(5))
	b := Contract{Level: 6, Suit: 0}
	s = step(t, s, s.Turn, "bid", &b)
	s = step(t, s, s.Turn, "pass", nil)
	s = step(t, s, s.Turn, "pass", nil)
	s = step(t, s, s.Turn, "declare", &b, s.Hands[s.Turn][0], s.Hands[s.Turn][1])
	if s.Stage != "play" || s.Defence[1] != 2 || s.Defence[2] != 2 {
		t.Fatal("stalingrad")
	}
	s = scoreState(t, 3, 7)
	s.Stage = "return"
	s.Turn = 2
	s.HalfSeat = 1
	s = step(t, s, 2, "whist", nil)
	if s.Stage != "mode" || s.Controller != 2 || s.Defence[1] != 1 {
		t.Fatal("return whist")
	}
}
func TestBotsCompleteGames(t *testing.T) {
	for _, n := range []int{3, 4} {
		for seed := int64(0); seed < 8; seed++ {
			s := testState(t, n)
			s.Target = 5
			_ = s.deal(fixedDeck(seed))
			steps := 0
			for s.Stage != "finished" && steps < 12000 {
				steps++
				switch s.Stage {
				case "round":
					s.Dealer = (s.Dealer + 1) % n
					if e := s.deal(fixedDeck(seed + int64(steps))); e != nil {
						t.Fatal(e)
					}
				case "trick":
					s = step(t, s, 0, "collect", nil)
				default:
					c := BotCommand(s.View(s.Actor()))
					next, e := Apply(s, c, nil)
					if e != nil {
						t.Fatalf("bot n=%d seed=%d stage=%s: %v", n, seed, s.Stage, e)
					}
					s = next
				}
				if s.Stage == "play" || s.Stage == "trick" {
					seen := map[Card]bool{}
					for _, hand := range s.Hands {
						for _, card := range hand {
							if seen[card] {
								t.Fatal("duplicate")
							}
							seen[card] = true
						}
					}
				}
				sum := 0
				for _, r := range s.ResultNumerators() {
					sum += r
				}
				if sum != 0 {
					t.Fatal("nonzero sum")
				}
			}
			if s.Stage != "finished" {
				t.Fatal("bots stalled")
			}
		}
	}
}
