package game

type SeatView struct {
	Player
	Count int    `json:"count"`
	Cards []Card `json:"cards,omitempty"`
}
type View struct {
	HostAIEnabled    *bool        `json:"hostAIEnabled,omitempty"`
	AIReviewSeat     *int         `json:"aiReviewSeat,omitempty"`
	DebugBotSeat     *int         `json:"debugBotSeat,omitempty"`
	DebugWaitingSeat *int         `json:"debugWaitingSeat,omitempty"`
	BotThinking      *BotThinking `json:"botThinking,omitempty"`
	LastTrick        []Play       `json:"lastTrick,omitempty"`
	MisereCards      []Card       `json:"misereCards,omitempty"`
	MiserePlayed     []Card       `json:"miserePlayed,omitempty"`
	ManualPaused     bool         `json:"manualPaused,omitempty"`
	PausedBy         int          `json:"pausedBy"`
	PausedAt         int64        `json:"pausedAt,omitempty"`
	PauseSeconds     int64        `json:"pauseSeconds,omitempty"`
	FinishedAt       int64        `json:"finishedAt,omitempty"`
	StartedAt        int64        `json:"startedAt,omitempty"`
	DebugBots        string       `json:"debugBots,omitempty"`
	Claim            *int         `json:"claim,omitempty"`
	Deadline         int64        `json:"deadline,omitempty"`
	Rules            Rules        `json:"rules"`
	ID               string       `json:"id"`
	Version          string       `json:"version"`
	Revision         uint64       `json:"revision"`
	Seat             int          `json:"seat"`
	Players          []SeatView   `json:"players"`
	Stage            string       `json:"stage"`
	Target           int          `json:"target"`
	Round            int          `json:"round"`
	Dealer           int          `json:"dealer"`
	Turn             int          `json:"turn"`
	Actor            int          `json:"actor"`
	Hand             []Card       `json:"hand"`
	Talon            []Card       `json:"talon,omitempty"`
	TalonShown       bool         `json:"talonShown,omitempty"`
	Trick            []Play       `json:"trick"`
	TrickNo          int          `json:"trickNo"`
	Taken            []int        `json:"taken"`
	Winner           int          `json:"winner"`
	Bid              *Contract    `json:"bid"`
	Contract         *Contract    `json:"contract"`
	Declarer         int          `json:"declarer"`
	Defence          []int        `json:"defence"`
	HalfSeat         int          `json:"halfSeat"`
	DealerLook       *int         `json:"dealerLook,omitempty"`
	DealerWhist      bool         `json:"dealerWhist,omitempty"`
	Passed           []bool       `json:"passed"`
	Open             bool         `json:"open"`
	AllPass          bool         `json:"allPass"`
	PassPrice        int          `json:"passPrice"`
	PassStreak       int          `json:"passStreak"`
	Actions          []string     `json:"actions"`
	Contracts        []Contract   `json:"contracts"`
	Legal            []Card       `json:"legal"`
	PlayHand         []Card       `json:"playHand"`
	Pool             []int        `json:"pool"`
	Mountain         []int        `json:"mountain"`
	Whists           [][]int      `json:"whists"`
	Results          []int        `json:"results"`
	History          []Ledger     `json:"history"`
}

// BotThinking is transient host activity, excluded from the saved game state.
type BotThinking struct {
	Seat      int   `json:"seat"`
	StartedAt int64 `json:"startedAt"`
}

func (s *State) View(seat int) View {
	v := View{ID: s.ID, Version: s.Version, Revision: s.Revision, Seat: seat, Stage: s.Stage, Target: s.Target, Round: s.Round, Dealer: s.Dealer, Turn: s.Turn, Actor: s.Actor(), Trick: s.Trick, TrickNo: s.TrickNo, Taken: s.Taken, Winner: s.Winner, Bid: s.Bid, Contract: s.Contract, Declarer: s.Declarer, Defence: s.Defence, Open: s.Open, AllPass: s.AllPass, PassPrice: s.PassPrice, PassStreak: s.PassStreak, Pool: s.Pool, Mountain: s.Mountain, Whists: s.Whists, Results: s.ResultNumerators(), History: s.History}
	v.LastTrick = append([]Play(nil), s.LastTrick...)
	v.Passed = s.Passed
	v.DebugBots = s.DebugBots
	v.Claim = s.Claim
	v.HalfSeat = s.HalfSeat
	v.DealerWhist = s.DealerWhist
	if seat == s.Dealer {
		v.DealerLook = s.DealerLook
	}
	v.TalonShown = s.TalonShown
	v.Rules = s.Agreements()
	v.Deadline = s.Deadline
	v.StartedAt = s.StartedAt
	if s.Contract != nil && s.Contract.Misere && s.isCatcher(seat) && (s.Stage == "play" || s.Stage == "trick") {
		v.MisereCards = append([]Card{}, s.MisereCards...)
		// Upgrade the first tracker format in existing no-talon misere saves.
		if s.Contract.NoTalon && len(v.MisereCards) == 10 {
			v.MisereCards = append(v.MisereCards, s.Talon...)
			sortHand(v.MisereCards)
		}
		v.MiserePlayed = append([]Card{}, s.MiserePlayed...)
	}
	v.PausedBy = s.PausedBy
	v.ManualPaused, v.PausedAt, v.PauseSeconds, v.FinishedAt = s.ManualPaused, s.PausedAt, s.PauseSeconds, s.FinishedAt
	// If the declarer has the opening lead, everyone sees only their own
	// hand until that card is played. Open defence is revealed to all seats
	// immediately afterwards, not after the whole trick.
	hideDefence := s.Stage == "play" && s.TrickNo == 0 && len(s.Trick) == 0 && s.Active()[0] == s.Declarer && s.Contract != nil && (s.Contract.Misere || s.Contract.Level < 10 || !s.Agreements().TenCheck)
	for p, player := range s.Players {
		sv := SeatView{Player: player}
		if len(s.Hands) > p {
			sv.Count = len(s.Hands[p])
			if p == seat || (seat == s.Dealer && s.DealerLook != nil && p == *s.DealerLook) || (p == s.Declarer && (s.DeclarerShown || s.Claim != nil)) || (s.Open && p != s.Declarer && !hideDefence) {
				sv.Cards = append([]Card{}, s.Hands[p]...)
			}
		}
		v.Players = append(v.Players, sv)
	}
	if len(s.Hands) > seat {
		v.Hand = append([]Card{}, s.Hands[seat]...)
	}
	if s.Contract != nil && !s.Contract.NoTalon {
		v.Talon = s.Talon
	}
	if s.AllPass {
		n := s.TrickNo + 1
		if n > 2 {
			n = 2
		}
		v.Talon = s.Talon[:n]
	}
	if s.Stage == "discard" {
		v.Talon = s.Talon
	}
	if s.Stage == "lobby" {
		v.Actions = []string{"ready"}
		if seat == 0 {
			v.Actions = append(v.Actions, "start")
		}
		return v
	}
	if s.Stage == "round" && seat == 0 {
		v.Actions = []string{"next"}
		return v
	}
	if s.Stage == "trick" {
		v.Actions = []string{"collect"}
		return v
	}
	if s.Stage == "play" && s.Claim != nil {
		if s.canRespondToClaim(seat) {
			v.Actions = []string{"accept-claim", "reject-claim"}
		}
		return v
	}
	if s.Stage == "catch" {
		if s.isCatcher(seat) {
			v.Actions = []string{"catch", "trust"}
		}
		return v
	}
	if s.Actor() != seat {
		return v
	}
	switch s.Stage {
	case "auction":
		v.Actions = []string{"pass", "bid"}
		for _, b := range Contracts() {
			c := Command{ID: "probe", Seat: seat, Revision: s.Revision, Action: "bid", Contract: &b}
			t := s.Clone()
			if t.apply(c, nil) == nil {
				v.Contracts = append(v.Contracts, b)
			}
		}
	case "contract":
		v.Actions = []string{"declare-no-talon"}
		for suit := s.noTalonNineFloor(seat); suit <= 4; suit++ {
			v.Contracts = append(v.Contracts, Contract{Level: 9, Suit: suit, NoTalon: true})
		}
	case "discard":
		v.Actions = []string{"declare"}
		if len(s.Players) == 4 && !s.Bid.Misere && s.Declarer != s.Dealer {
			v.Actions = append(v.Actions, "show-talon")
		}
		if !s.Bid.Misere && (s.Bid.Level == 6 || s.Bid.Level == 7) {
			v.Actions = append(v.Actions, "without-three")
		}
		for _, b := range Contracts() {
			if !b.NoTalon && b.Misere == s.Bid.Misere && b.Rank() >= s.Bid.Rank() {
				v.Contracts = append(v.Contracts, b)
			}
		}
	case "defend":
		v.Actions = []string{"pass", "whist"}
		if s.Contract.Level <= 7 && seat == s.Defenders[1] && s.Defence[s.Defenders[0]] == 1 {
			v.Actions = append(v.Actions, "half")
		}
	case "option":
		v.Actions = []string{"whist", "half"}
	case "return":
		v.Actions = []string{"pass", "whist"}
	case "dealer-choice":
		v.Actions = []string{"dealer-skip", "dealer-first", "dealer-second"}
	case "dealer-whist":
		v.Actions = []string{"whist", "pass"}
	case "mode":
		v.Actions = []string{"open", "closed"}
	case "play":
		if s.Claim != nil {
			v.Actions = []string{"accept-claim", "reject-claim"}
			break
		}
		v.Actions = []string{"play"}
		if s.canClaim() {
			v.Actions = append(v.Actions, "claim")
		}
		v.Legal = s.LegalCards()
		v.PlayHand = append([]Card{}, s.Hands[s.Turn]...)
	}
	return v
}
