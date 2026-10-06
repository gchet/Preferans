package game

import "preferans/locales"

import (
	"time"
)

// Rules are owned by the host and frozen when the first deal starts.
// A nil rules pointer in older saves means the original agreements.
type Rules struct {
	AllowNoTalon        *bool  `json:"allowNoTalon,omitempty"`
	FinishAfterPassExit *bool  `json:"finishAfterPassExit,omitempty"`
	HereConvention      string `json:"hereConvention,omitempty"`
	PassExitWhistedOnly *bool  `json:"passExitWhistedOnly,omitempty"`
	PassExitSpecial     *bool  `json:"passExitSpecial,omitempty"`
	TalonPenalty        string `json:"talonPenalty,omitempty"`
	Whist               string `json:"whist"`
	Responsibility      string `json:"responsibility"`
	PassPrices          []int  `json:"passPrices"`
	PassExit            int    `json:"passExit"`
	Stalingrad          bool   `json:"stalingrad"`
	TenCheck            bool   `json:"tenCheck"`
	MisereTalon         bool   `json:"misereTalon"`
	DealerBonus         bool   `json:"dealerBonus"`
	EndCondition        string `json:"endCondition"`
	Minutes             int    `json:"minutes"`
}

// Legacy settings now enable the separate no-talon bids, never change misere.
func (r Rules) AllowsNoTalonBids() bool {
	if r.AllowNoTalon != nil {
		return *r.AllowNoTalon
	}
	return !r.MisereTalon
}

func (r Rules) RequiresWhistedExit() bool {
	return r.PassExitWhistedOnly == nil || *r.PassExitWhistedOnly
}
func (r Rules) SpecialExitsPasses() bool { return r.PassExitSpecial == nil || *r.PassExitSpecial }

func DefaultRules() Rules {
	return Rules{HereConvention: "through", Whist: "gentleman", Responsibility: "half", PassPrices: []int{2, 4, 8}, PassExit: 7, Stalingrad: true, TenCheck: true, MisereTalon: true, DealerBonus: true, EndCondition: "pool", Minutes: 60}
}
func (r Rules) Validate() error {
	if r.HereConvention != "" && r.HereConvention != "through" && r.HereConvention != "seniority" {
		return locales.Errorf("go.rules.here_invalid")
	}
	if r.TalonPenalty != "" && r.TalonPenalty != "whists" && r.TalonPenalty != "mountain" {
		return locales.Errorf("go.internal.game.rules.text001")
	}
	if r.Whist != "gentleman" && r.Whist != "greedy" {
		return locales.Errorf("go.internal.game.rules.text002")
	}
	if r.Responsibility != "half" && r.Responsibility != "full" {
		return locales.Errorf("go.internal.game.rules.text003")
	}
	if len(r.PassPrices) < 1 || len(r.PassPrices) > 10 {
		return locales.Errorf("go.internal.game.rules.text004")
	}
	for i, v := range r.PassPrices {
		if v < 1 || v > 100 || (i > 0 && v < r.PassPrices[i-1]) {
			return locales.Errorf("go.internal.game.rules.text005")
		}
	}
	if r.PassExit < 6 || r.PassExit > 8 {
		return locales.Errorf("go.internal.game.rules.text006")
	}
	if r.EndCondition != "pool" && r.EndCondition != "time" {
		return locales.Errorf("go.internal.game.rules.text007")
	}
	if r.Minutes < 1 || r.Minutes > 1440 {
		return locales.Errorf("go.internal.game.rules.text008")
	}
	return nil
}
func (s *State) Agreements() Rules {
	if s.Rules == nil {
		return DefaultRules()
	}
	return *s.Rules
}

func (s *State) TimeExpired() bool {
	now := time.Now().Unix()
	if s.PausedAt > 0 {
		now = s.PausedAt
	}
	return s.Agreements().EndCondition == "time" && s.Deadline > 0 && now >= s.Deadline
}

// A failed contract does not end a pass series, even after the party limit.
func (s *State) FinishDue() bool {
	rules := s.Agreements()
	if (rules.FinishAfterPassExit == nil || *rules.FinishAfterPassExit) && s.PassStreak > 0 {
		return false
	}
	sum := 0
	for _, value := range s.Pool {
		sum += value
	}
	return (rules.EndCondition == "pool" && sum >= s.Target*len(s.Players)) || s.TimeExpired()
}

// SetPaused freezes the clock; resuming moves the deadline by the pause length.
// The timestamps are saved and shared, so reloads do not restart a pause.
func (s *State) SetPaused(paused bool, now int64) bool {
	if paused && s.PausedAt == 0 {
		s.PausedAt = now
		return true
	}
	if !paused && s.PausedAt > 0 {
		elapsed := max(int64(0), now-s.PausedAt)
		s.PauseSeconds += elapsed
		if s.Deadline > 0 {
			s.Deadline += elapsed
		}
		s.PausedAt = 0
		return true
	}
	return false
}
