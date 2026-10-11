package game

import (
	"context"
	"fmt"
	"math"
	"os"
	"preferans/config"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSelfPlayComparison measures one local-bot search setting against another in
// whole deals. It is skipped unless SELFPLAY_DEALS is set, e.g. in PowerShell:
//
//	$env:SELFPLAY_DEALS = "200"
//	$env:SELFPLAY_X = "samples=300"      # settings: samples, exact, ms
//	$env:SELFPLAY_Y = "samples=120"
//	go test ./internal/game -run TestSelfPlayComparison -v -count=1
//
// Optional: SELFPLAY_PLAYERS (3 or 4), SELFPLAY_FIRST (first deal number),
// SELFPLAY_WORKERS. Bidding, discarding and whisting use the plain heuristics for
// everybody; only the card play differs. Every deal is played twice: X at seat 0
// against two Y bots, then Y at seat 0 against two X bots. The report is the mean
// paired difference of seat 0's result (in points) with its standard error, so a
// positive mean favours X. Compare mean/se: below about 2 the difference is noise.

type selfPlaySetting struct {
	name                 string
	samples, exact, msec int
}

func parseSelfPlaySetting(spec string) selfPlaySetting {
	s := selfPlaySetting{name: spec, samples: config.Interface.LocalBotSearchSamples, exact: config.Interface.LocalBotExactCards, msec: config.Interface.LocalBotSearchTimeMs}
	for _, kv := range strings.Split(spec, ":") {
		key, value, _ := strings.Cut(kv, "=")
		n, _ := strconv.Atoi(value)
		switch key {
		case "samples":
			s.samples = n
		case "exact":
			s.exact = n
		case "ms":
			s.msec = n
		}
	}
	return s
}

func (s selfPlaySetting) command(situation BotSituation, seed int64) Command {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.msec)*time.Millisecond)
	defer cancel()
	command, _ := SearchBotCommand(ctx, situation, SearchOptions{Samples: s.samples, ExactCards: s.exact, Seed: seed})
	return command
}

type selfPlayResult struct {
	nums []int
	kind string
	err  error
}

func selfPlayDeal(players int, seed int64, settings []selfPlaySetting) selfPlayResult {
	s, err := New(players, 30)
	if err != nil {
		return selfPlayResult{err: err}
	}
	for i := range s.Players {
		s.Players[i].Ready = true
	}
	if err := s.deal(fixedDeck(seed)); err != nil {
		return selfPlayResult{err: err}
	}
	kind, decision := "", int64(0)
	for step := 0; step < 600; step++ {
		switch s.Stage {
		case "auction", "contract", "discard", "defend", "option", "return", "mode", "play", "trick", "dealer-choice", "dealer-whist", "catch":
		default:
			return selfPlayResult{nums: s.ResultNumerators(), kind: kind}
		}
		if s.Stage == "play" && kind == "" {
			if s.AllPass {
				kind = "pass"
			} else {
				kind = "contract"
			}
		}
		var command Command
		if s.Stage == "trick" {
			command = Command{ID: ID(), Seat: 0, Revision: s.Revision, Action: "collect"}
		} else {
			seat := s.Actor()
			v := s.View(seat)
			switch {
			case s.Stage == "defend" && len(s.Defenders) > 0 && s.Turn == s.Defenders[0]:
				command = Command{ID: ID(), Seat: seat, Revision: s.Revision, Action: "whist"}
			case s.Stage == "play" && len(v.Legal) > 1:
				decision++
				situation, _ := s.BotChoices(seat, nil)
				command = settings[seat].command(situation, seed*1000+decision)
			default:
				command = BotCommand(v)
			}
		}
		next, err := Apply(s, command, nil)
		if err != nil {
			return selfPlayResult{err: fmt.Errorf("deal %d, stage %s, action %s: %w", seed, s.Stage, command.Action, err)}
		}
		s = next
	}
	return selfPlayResult{err: fmt.Errorf("deal %d did not finish", seed)}
}

func TestSelfPlayComparison(t *testing.T) {
	deals, _ := strconv.Atoi(os.Getenv("SELFPLAY_DEALS"))
	if deals == 0 {
		t.Skip("set SELFPLAY_DEALS to run")
	}
	players, _ := strconv.Atoi(os.Getenv("SELFPLAY_PLAYERS"))
	if players == 0 {
		players = 3
	}
	first, _ := strconv.Atoi(os.Getenv("SELFPLAY_FIRST"))
	workers, _ := strconv.Atoi(os.Getenv("SELFPLAY_WORKERS"))
	if workers == 0 {
		workers = 4
	}
	config.Interface.LocalBotExactCardsLimit = 40
	x, y := parseSelfPlaySetting(os.Getenv("SELFPLAY_X")), parseSelfPlaySetting(os.Getenv("SELFPLAY_Y"))

	type job struct {
		seed int64
		swap bool
	}
	jobs := make(chan job)
	results := map[job]selfPlayResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				settings := make([]selfPlaySetting, players)
				others, seat0 := y, x
				if j.swap {
					others, seat0 = x, y
				}
				for i := range settings {
					settings[i] = others
				}
				settings[0] = seat0
				result := selfPlayDeal(players, j.seed, settings)
				mu.Lock()
				results[j] = result
				mu.Unlock()
			}
		}()
	}
	for d := 0; d < deals; d++ {
		seed := int64(first + d + 1)
		jobs <- job{seed, false}
		jobs <- job{seed, true}
	}
	close(jobs)
	wg.Wait()

	diffs := map[string][]float64{}
	for d := 0; d < deals; d++ {
		seed := int64(first + d + 1)
		a, b := results[job{seed, false}], results[job{seed, true}]
		if a.err != nil || b.err != nil {
			t.Fatalf("deal %d: %v %v", seed, a.err, b.err)
		}
		diff := float64(a.nums[0]-b.nums[0]) / 10
		diffs["all"] = append(diffs["all"], diff)
		diffs[a.kind] = append(diffs[a.kind], diff)
	}
	fmt.Printf("X=%s  Y=%s  players=%d\n", x.name, y.name, players)
	var kinds []string
	for kind := range diffs {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		values := diffs[kind]
		mean := 0.0
		for _, value := range values {
			mean += value
		}
		mean /= float64(len(values))
		variance := 0.0
		for _, value := range values {
			variance += (value - mean) * (value - mean)
		}
		se := 0.0
		if len(values) > 1 {
			se = math.Sqrt(variance / float64(len(values)-1) / float64(len(values)))
		}
		fmt.Printf("%-9s n=%4d  mean(X-Y)=%+.2f  se=%.2f\n", kind, len(values), mean, se)
	}
}
