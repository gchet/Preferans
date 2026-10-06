package game

import (
	"encoding/json"
	"fmt"
)

type AuctionCall struct {
	Seat     int       `json:"seat"`
	Action   string    `json:"action"`
	Contract *Contract `json:"contract,omitempty"`
}
type BotOption struct {
	Move     int       `json:"move"`
	Action   string    `json:"action"`
	Cards    []Card    `json:"cards,omitempty"`
	Contract *Contract `json:"contract,omitempty"`
	Tricks   int       `json:"tricks,omitempty"`
	Label    string    `json:"label"`
}
type BotSituation struct {
	RuleExplanation []string      `json:"ruleExplanation"`
	View            View          `json:"view"`
	Auction         []AuctionCall `json:"auction"`
	PlayedTricks    [][]Play      `json:"playedTricks"`
	VoidSuits       map[int][]int `json:"voidSuits"`
	KnownDiscard    []Card        `json:"knownDiscard,omitempty"`
	Choices         []BotOption   `json:"legal_actions"`
	SelectedDiscard []Card        `json:"selectedDiscard,omitempty"`
}

// BotChoices enumerates verified commands. Discard and contract are two
// selections in a single decision deadline to avoid a huge Cartesian prompt.
func (s *State) BotChoices(seat int, selected []Card) (BotSituation, []Command) {
	v := s.View(seat)
	// Prior deal ledgers add tokens without helping the current decision;
	// current pool, mountain, whists and final balances remain available.
	v.History = nil
	situation := BotSituation{View: v, Auction: s.Auction, PlayedTricks: s.PlayedTricks, VoidSuits: map[int][]int{}, SelectedDiscard: selected}
	situation.RuleExplanation = s.RuleExplanation("ru")
	if seat == s.Declarer || s.TalonShown {
		situation.KnownDiscard = append([]Card(nil), s.Discard...)
	}
	tricks := append(append([][]Play(nil), s.PlayedTricks...), s.Trick)
	for _, trick := range tricks {
		if len(trick) == 0 {
			continue
		}
		lead := trick[0].Card.Suit()
		for _, play := range trick[1:] {
			if play.Seat < 0 || (len(s.Players) == 4 && s.AllPass && play.Seat == s.Dealer) || play.Card.Suit() == lead {
				continue
			}
			addVoid := func(suit int) {
				for _, known := range situation.VoidSuits[play.Seat] {
					if known == suit {
						return
					}
				}
				situation.VoidSuits[play.Seat] = append(situation.VoidSuits[play.Seat], suit)
			}
			addVoid(lead)
			if s.Contract != nil && !s.Contract.Misere && s.Contract.Suit < 4 && play.Card.Suit() != s.Contract.Suit {
				addVoid(s.Contract.Suit)
			}
		}
	}
	var commands []Command
	add := func(c Command, label string) {
		c.ID = ID()
		c.Seat = seat
		c.Revision = s.Revision
		c.Round = s.Round
		if _, err := Apply(s, c, nil); err != nil {
			return
		}
		situation.Choices = append(situation.Choices, BotOption{Move: len(commands), Action: c.Action, Cards: c.Cards, Contract: c.Contract, Tricks: c.Tricks, Label: label})
		commands = append(commands, c)
	}
	for _, action := range v.Actions {
		switch action {
		case "bid", "declare-no-talon":
			for _, b := range v.Contracts {
				add(Command{Action: action, Contract: &b}, b.String())
			}
		case "play":
			for _, card := range v.Legal {
				add(Command{Action: action, Cards: []Card{card}}, card.String())
			}
		case "claim":
			for n := 0; n <= 10-s.TrickNo; n++ {
				add(Command{Action: action, Tricks: n}, fmt.Sprintf("claim %d more tricks", n))
			}
		case "declare":
			if len(selected) == 2 || s.TalonShown {
				cards := selected
				if s.TalonShown {
					cards = v.Talon
				}
				for _, b := range v.Contracts {
					add(Command{Action: action, Cards: cards, Contract: &b}, b.String())
				}
			} else {
				for i := 0; i < len(v.Hand); i++ {
					for j := i + 1; j < len(v.Hand); j++ {
						add(Command{Action: action, Cards: []Card{v.Hand[i], v.Hand[j]}, Contract: v.Bid}, v.Hand[i].String()+" + "+v.Hand[j].String())
					}
				}
			}
		case "show-talon":
			if !v.TalonShown && len(selected) == 0 {
				add(Command{Action: action}, action)
			}
		default:
			if len(selected) == 0 {
				add(Command{Action: action}, action)
			}
		}
	}
	// Own/open hands are the only card-bearing sources in this snapshot.
	b, _ := json.Marshal(situation)
	var detached BotSituation
	_ = json.Unmarshal(b, &detached)
	return detached, commands
}
