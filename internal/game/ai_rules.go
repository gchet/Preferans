package game

import (
	"preferans/config"
	"strconv"
	"strings"
)

// RuleExplanation describes only the selected agreements, without private cards.
func (s *State) RuleExplanation(language string) []string {
	r := s.Agreements()
	values := map[string]string{"exit": strconv.Itoa(r.PassExit), "target": strconv.Itoa(s.Target),
		"total": strconv.Itoa(s.Target * len(s.Players)), "players": strconv.Itoa(len(s.Players)), "minutes": strconv.Itoa(r.Minutes)}
	prices := make([]string, len(r.PassPrices))
	for i, price := range r.PassPrices {
		prices[i] = strconv.Itoa(price)
	}
	values["prices"] = strings.Join(prices, " → ")
	var result []string
	add := func(key string) { result = append(result, config.AIAgreement(key, language, values)) }
	choose := func(flag bool, yes, no string) {
		if flag {
			add(yes)
		} else {
			add(no)
		}
	}
	choose(r.HereConvention != "seniority", "here_through", "here_seniority")
	choose(r.Whist == "gentleman", "whist_gentleman", "whist_greedy")
	choose(r.Responsibility == "full", "responsibility_full", "responsibility_half")
	add("passes")
	choose(r.RequiresWhistedExit(), "exit_whisted", "exit_any")
	choose(r.SpecialExitsPasses(), "special_yes", "special_no")
	choose(r.Stalingrad, "stalingrad_yes", "stalingrad_no")
	choose(r.TenCheck, "ten_check", "ten_whist")
	choose(r.AllowsNoTalonBids(), "no_talon_bids_yes", "no_talon_bids_no")
	if len(s.Players) == 4 {
		add("dealer_whist")
		choose(r.DealerBonus, "bonus_yes", "bonus_no")
		choose(r.TalonPenalty == "mountain", "talon_mountain", "talon_whists")
	}
	choose(r.EndCondition == "time", "end_time", "end_pool")
	choose(r.FinishAfterPassExit == nil || *r.FinishAfterPassExit, "finish_exit", "finish_any")
	return result
}
