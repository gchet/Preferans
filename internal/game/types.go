package game

import "preferans/locales"

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

const RulesVersion = "leningrad-1"

type Card int

func (c Card) Suit() int { return int(c) / 8 }
func (c Card) Rank() int { return int(c) % 8 }
func (c Card) String() string {
	return []string{"7", "8", "9", "10", locales.Text("go.internal.game.types.text001"), locales.Text("go.internal.game.types.text002"), locales.Text("go.internal.game.types.text003"), locales.Text("go.internal.game.types.text004")}[c.Rank()] + []string{"♠", "♣", "♦", "♥"}[c.Suit()]
}

type Contract struct {
	NoTalon bool `json:"noTalon,omitempty"`
	Level   int  `json:"level"`
	Suit    int  `json:"suit"`
	Misere  bool `json:"misere"`
}

func (c Contract) Rank() int {
	if c.Misere {
		if c.NoTalon {
			return 21
		}
		return 15
	}
	r := (c.Level-6)*5 + c.Suit
	if c.Level >= 9 {
		r++
	}
	if c.Level == 9 && c.NoTalon {
		return 22
	}
	if c.Level == 10 {
		r += 2
	}
	return r
}
func (c Contract) Valid() bool {
	if c.NoTalon && !c.Misere && c.Level != 9 {
		return false
	}
	if c.NoTalon && c.Level == 9 && !c.Misere && c.Suit == -1 {
		return true
	}
	return (c.Misere && c.Level == 0 && c.Suit == 4) || (!c.Misere && c.Level >= 6 && c.Level <= 10 && c.Suit >= 0 && c.Suit <= 4)
}
func (c Contract) String() string {
	if c.NoTalon && !c.Misere && c.Suit == -1 {
		return locales.Text("go.contract.nine_no_talon")
	}
	if c.NoTalon {
		ordinary := c
		ordinary.NoTalon = false
		return ordinary.String() + locales.Text("go.contract.no_talon_suffix")
	}
	if c.Misere {
		return locales.Text("go.internal.game.types.text005")
	}
	return fmt.Sprintf("%d%s", c.Level, []string{"♠", "♣", "♦", "♥", locales.Text("go.internal.game.types.text006")}[c.Suit])
}
func Contracts() []Contract {
	var cs []Contract
	for n := 6; n <= 10; n++ {
		if n == 9 {
			cs = append(cs, Contract{Suit: 4, Misere: true})
		}
		for s := 0; s < 5; s++ {
			cs = append(cs, Contract{Level: n, Suit: s})
		}
		if n == 9 {
			cs = append(cs, Contract{Misere: true, Suit: 4, NoTalon: true})
			cs = append(cs, Contract{Level: 9, Suit: -1, NoTalon: true})
		}
	}
	return cs
}

type Player struct {
	BotModel string `json:"botModel,omitempty"`
	Name     string `json:"name"`
	Bot      bool   `json:"bot"`
	Ready    bool   `json:"ready"`
}
type Play struct {
	Seat int  `json:"seat"`
	Card Card `json:"card"`
}
type Ledger struct {
	AmnestyTricks int     `json:"amnestyTricks,omitempty"`
	Amnesty       int     `json:"amnesty,omitempty"`
	Round         int     `json:"round"`
	Label         string  `json:"label"`
	Pool          []int   `json:"pool"`
	Mountain      []int   `json:"mountain"`
	Whists        [][]int `json:"whists"`
}
type State struct {
	Auction       []AuctionCall  `json:"auction,omitempty"`
	PlayedTricks  [][]Play       `json:"playedTricks,omitempty"`
	LastTrick     []Play         `json:"lastTrick,omitempty"`
	HereOwner     *int           `json:"hereOwner,omitempty"`
	MisereCards   []Card         `json:"misereCards,omitempty"`
	MiserePlayed  []Card         `json:"miserePlayed,omitempty"`
	ManualPaused  bool           `json:"manualPaused,omitempty"`
	PausedBy      int            `json:"pausedBy"`
	PausedAt      int64          `json:"pausedAt,omitempty"`
	PauseSeconds  int64          `json:"pauseSeconds,omitempty"`
	FinishedAt    int64          `json:"finishedAt,omitempty"`
	StartedAt     int64          `json:"startedAt,omitempty"`
	DebugBots     string         `json:"debugBots,omitempty"`
	ClaimAccepted []int          `json:"claimAccepted,omitempty"`
	Claim         *int           `json:"claim,omitempty"`
	DeclarerShown bool           `json:"declarerShown,omitempty"`
	Deadline      int64          `json:"deadline,omitempty"`
	Rules         *Rules         `json:"rules,omitempty"`
	Version       string         `json:"version"`
	ID            string         `json:"id"`
	Revision      uint64         `json:"revision"`
	Players       []Player       `json:"players"`
	Target        int            `json:"target"`
	Stage         string         `json:"stage"`
	Dealer        int            `json:"dealer"`
	Round         int            `json:"round"`
	Turn          int            `json:"turn"`
	Hands         [][]Card       `json:"hands"`
	Talon         []Card         `json:"talon"`
	TalonShown    bool           `json:"talonShown,omitempty"`
	Discard       []Card         `json:"discard"`
	Passed        []bool         `json:"passed"`
	Spoken        []bool         `json:"spoken"`
	Bid           *Contract      `json:"bid"`
	Declarer      int            `json:"declarer"`
	Contract      *Contract      `json:"contract"`
	Defenders     []int          `json:"defenders"`
	Defence       []int          `json:"defence"`
	Open          bool           `json:"open"`
	Controller    int            `json:"controller"`
	HalfSeat      int            `json:"halfSeat"`
	DealerLook    *int           `json:"dealerLook,omitempty"`
	DealerWhist   bool           `json:"dealerWhist,omitempty"`
	AllPass       bool           `json:"allPass"`
	PassStreak    int            `json:"passStreak"`
	PassPrice     int            `json:"passPrice"`
	Trick         []Play         `json:"trick"`
	TrickNo       int            `json:"trickNo"`
	Winner        int            `json:"winner"`
	Taken         []int          `json:"taken"`
	Pool          []int          `json:"pool"`
	Mountain      []int          `json:"mountain"`
	Whists        [][]int        `json:"whists"`
	History       []Ledger       `json:"history"`
	Applied       map[string]int `json:"applied"`
}
type Command struct {
	Round     int       `json:"round,omitempty"`
	DebugBots string    `json:"debugBots,omitempty"`
	Tricks    int       `json:"tricks,omitempty"`
	Rules     *Rules    `json:"rules,omitempty"`
	Target    int       `json:"target,omitempty"`
	ID        string    `json:"id"`
	Seat      int       `json:"seat"`
	Revision  uint64    `json:"revision"`
	Action    string    `json:"action"`
	Cards     []Card    `json:"cards,omitempty"`
	Contract  *Contract `json:"contract,omitempty"`
	Name      string    `json:"name,omitempty"`
}

// Preserve already-started saves made when the legacy switch skipped all talons.
// New ordinary miseres always have a two-card discard and do not match this case.
func (s *State) UnmarshalJSON(data []byte) error {
	type plain State
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = State(decoded)
	if s.Rules != nil && s.Rules.AllowNoTalon == nil && !s.Rules.MisereTalon && s.Contract != nil && s.Contract.Misere && !s.Contract.NoTalon && len(s.Discard) == 0 && s.Stage != "discard" && s.Stage != "auction" && s.Stage != "lobby" {
		contract := *s.Contract
		contract.NoTalon = true
		s.Contract = &contract
		if s.Bid != nil && s.Bid.Misere {
			bid := *s.Bid
			bid.NoTalon = true
			s.Bid = &bid
		}
	}
	return nil
}

func ID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func New(n, target int) (*State, error) {
	if (n != 3 && n != 4) || target < 1 || target > 1000 {
		return nil, locales.Errorf("go.internal.game.types.text007")
	}
	s := &State{Version: RulesVersion, ID: ID(), Players: make([]Player, n), Target: target, Stage: "lobby", Dealer: n - 1, Turn: 0, Declarer: -1, Controller: -1, Applied: map[string]int{}, Pool: make([]int, n), Mountain: make([]int, n), Whists: make([][]int, n)}
	for i := range s.Players {
		s.Players[i].Name = locales.Format("go.internal.game.types.text008", i+1)
		s.Whists[i] = make([]int, n)
	}
	return s, nil
}
func (s *State) Clone() *State {
	b, _ := json.Marshal(s)
	var t State
	_ = json.Unmarshal(b, &t)
	return &t
}
func (s *State) Active() []int {
	var a []int
	for i := 1; i <= len(s.Players); i++ {
		p := (s.Dealer + i) % len(s.Players)
		if len(s.Players) == 3 || p != s.Dealer {
			a = append(a, p)
		}
	}
	return a
}
func (s *State) next(p int) int {
	a := s.Active()
	for i, v := range a {
		if v == p {
			return a[(i+1)%3]
		}
	}
	return a[0]
}
func (s *State) priority(p int) int {
	for i, v := range s.Active() {
		if v == p {
			return i
		}
	}
	return 99
}

// Only two remaining bidders can use "Here". The through convention gives
// priority to the first caller for the rest of this auction, including raises.
func (s *State) canRepeatBid(seat int) bool {
	if s.Stage != "auction" || s.Bid == nil || s.Actor() != seat || s.Passed[seat] || seat == s.Declarer {
		return false
	}
	remaining := 0
	for _, p := range s.Active() {
		if !s.Passed[p] {
			remaining++
		}
	}
	if remaining != 2 {
		return false
	}
	if s.Agreements().HereConvention == "seniority" {
		return s.priority(seat) < s.priority(s.Declarer)
	}
	if (!s.Bid.Misere && s.Bid.Level == 6 && s.Bid.Suit == 0) || (s.HereOwner != nil && *s.HereOwner != seat) {
		return false
	}
	// Use physical seats: next() skips the dealer in a four-player game.
	between := (s.Declarer + 1) % len(s.Players)
	if between == seat {
		return false
	}
	for p := between; p != seat; p = (p + 1) % len(s.Players) {
		if !(len(s.Players) == 4 && p == s.Dealer) && !s.Passed[p] {
			return false
		}
	}
	return true
}
func (s *State) Actor() int {
	if s.Stage == "play" && s.Claim != nil {
		return s.claimResponder()
	}
	if s.Stage == "play" && s.Open && s.Controller >= 0 && s.Turn != s.Declarer {
		return s.Controller
	}
	return s.Turn
}
func sortHand(h []Card) { sort.Slice(h, func(i, j int) bool { return h[i] < h[j] }) }
