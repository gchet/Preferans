package game

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Lab adapters: the simulation-based decisions live in bot_sim.go (they are part of
// the application's bot); here they are exposed as per-phase hooks for botlab.

// LabSim holds the parameters of the simulation-based policies.
type LabSim = SimOptions

// DefaultLabSim returns the starting parameters.
func DefaultLabSim() LabSim { return DefaultSimOptions() }

func labBidSim(opt *LabSim) LabHook {
	return func(c LabContext) (Command, bool) { return simBid(context.Background(), opt, c.View, c.Seed) }
}

func labDiscardSim(opt *LabSim) LabHook {
	return func(c LabContext) (Command, bool) { return simDiscard(context.Background(), opt, c.View, c.Seed) }
}

func labWhistSim(opt *LabSim) LabHook {
	return func(c LabContext) (Command, bool) { return simWhist(context.Background(), opt, c.View, c.Seed) }
}

// ParseLabPolicy reads "key=value,key=value". Keys:
//
//	samples, exact, ms   card-play search settings
//	whist=base|first|always|sim
//	bid=sim              bidding by simulation
//	trump=sim            discard and final contract by simulation
//	smart                shorthand for bid=sim,trump=sim,whist=sim
//	sims, topk           simulation samples per decision, discard candidates
//	margin, passvalue    bid when the best contract beats passvalue+margin points
//	whistmin             whist when it beats passing by this many points
//	belief, beliefshift  weight sampled declarer hands by how well they fit the bid (sigma, centre shift)
//	partner=pass|whist   what the first defender assumes about the other defender (default whist)
//
// Unknown keys are an error, so a typo cannot silently turn into a comparison of
// identical bots.
func ParseLabPolicy(spec string) (LabPolicy, error) {
	p := LabPolicy{Name: spec, Search: DefaultLabSearch(), Sim: DefaultLabSim()}
	if strings.TrimSpace(spec) == "" || spec == "base" {
		p.Name = "base"
		return p, nil
	}
	var bidSim, trumpSim, whistSim bool
	for _, kv := range strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ':' }) {
		key, value, _ := strings.Cut(strings.TrimSpace(kv), "=")
		switch key {
		case "smart":
			bidSim, trumpSim, whistSim = true, true, true
			continue
		case "bid", "trump":
			if value != "sim" && value != "base" {
				return p, fmt.Errorf("%s=%q: expected sim or base", key, value)
			}
			if key == "bid" {
				bidSim = value == "sim"
			} else {
				trumpSim = value == "sim"
			}
			continue
		case "partner":
			if value != "pass" && value != "whist" {
				return p, fmt.Errorf("partner=%q: expected pass or whist", value)
			}
			p.Sim.PartnerPass = value == "pass"
			continue
		case "whist":
			switch value {
			case "base":
			case "first":
				p.Whist = labWhistFirst
			case "always":
				p.Whist = labWhistAlways
			case "sim":
				whistSim = true
			default:
				return p, fmt.Errorf("whist=%q: expected base, first, always or sim", value)
			}
			continue
		}
		number, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return p, fmt.Errorf("unknown policy key %q in %q (or %s=%q is not a number)", key, spec, key, value)
		}
		switch key {
		case "samples":
			p.Search.Samples = int(number)
		case "exact":
			p.Search.ExactCards = int(number)
		case "ms":
			p.Search.Msec = int(number)
		case "sims":
			p.Sim.Samples = int(number)
		case "topk":
			p.Sim.TopK = int(number)
		case "margin":
			p.Sim.Margin = number
		case "passvalue":
			p.Sim.PassValue = number
		case "whistmin":
			p.Sim.WhistMin = number
		case "belief":
			p.Sim.Belief = number
		case "beliefshift":
			p.Sim.BeliefShift = number
		default:
			return p, fmt.Errorf("unknown policy key %q in %q", key, spec)
		}
	}
	sim := &p.Sim
	if bidSim {
		p.Bid = labBidSim(sim)
	}
	if trumpSim {
		p.Discard = labDiscardSim(sim)
	}
	if whistSim {
		p.Whist = labWhistSim(sim)
	}
	return p, nil
}
