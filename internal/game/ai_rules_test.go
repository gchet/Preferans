package game

import (
	"preferans/config"
	"strings"
	"testing"
)

func TestAIRulesDescribeSelectedAgreements(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		for _, n := range []int{3, 4} {
			s := testState(t, n)
			s.Target = 20
			defaults := s.RuleExplanation(language)
			r := DefaultRules()
			no := false
			r.HereConvention = "seniority"
			r.Whist = "greedy"
			r.Responsibility = "full"
			r.PassPrices = []int{3, 6, 12}
			r.PassExit = 8
			r.PassExitWhistedOnly = &no
			r.PassExitSpecial = &no
			r.FinishAfterPassExit = &no
			r.Stalingrad = false
			r.TenCheck = false
			r.MisereTalon = false
			r.DealerBonus = false
			r.TalonPenalty = "mountain"
			r.EndCondition = "time"
			r.Minutes = 45
			s.Rules = &r
			changed := s.RuleExplanation(language)
			if len(defaults) != len(changed) {
				t.Fatal("explanation count changed")
			}
			for i, line := range changed {
				if line == "" || strings.ContainsAny(line, "{}") {
					t.Fatalf("missing resource or placeholder: %q", line)
				}
				// Dealer rewhist is an unconditional rule, not a configurable agreement.
				if n == 4 && line == config.AIAgreement("dealer_whist", language, nil) {
					continue
				}
				if line == defaults[i] {
					t.Fatalf("agreement %d did not reflect change", i)
				}
			}
			if !strings.Contains(strings.Join(changed, "\n"), "3 → 6 → 12") {
				t.Fatal("pass prices missing")
			}
			if !strings.Contains(changed[len(changed)-2], "45") {
				t.Fatal("time limit missing")
			}
		}
	}
}

func TestBotContextIncludesExpandedRules(t *testing.T) {
	s := testState(t, 3)
	if err := s.deal(fixedDeck(1)); err != nil {
		t.Fatal(err)
	}
	situation, _ := s.BotChoices(s.Actor(), nil)
	if len(situation.RuleExplanation) == 0 {
		t.Fatal("no rule explanation in request")
	}
}
