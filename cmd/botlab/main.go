// botlab is the offline laboratory for the Preferans bots: it plays whole deals
// headlessly, compares bot configurations and keeps every run's parameters and
// results under .lab/.
//
//	botlab eval   -players 3 -deals 500 -x "whist=first,samples=300" -y "whist=first" -name card-play-300
//	botlab report
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"preferans/config"
	"preferans/internal/game"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"text/tabwriter"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "eval":
		err = runEval(os.Args[2:])
	case "report":
		err = runReport(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "botlab:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: botlab <command> [flags]

commands:
  eval     play paired deals, X at seat 0 against two Y bots and the other way round
  report   print the table of all recorded runs

eval flags: -players -deals -first -workers -x -y -name -dir
policy spec: comma-separated key=value, keys: samples, exact, ms, whist=base|first|always
`)
}

type stat struct {
	Kind string  `json:"kind"`
	N    int     `json:"n"`
	Mean float64 `json:"mean"`
	SE   float64 `json:"se"`
}

type runParams struct {
	Command     string    `json:"command"`
	Name        string    `json:"name"`
	Players     int       `json:"players"`
	Deals       int       `json:"deals"`
	First       int       `json:"first"`
	Workers     int       `json:"workers"`
	X           string    `json:"x"`
	Y           string    `json:"y"`
	Commit      string    `json:"commit"`
	GoVersion   string    `json:"goVersion"`
	Started     time.Time `json:"started"`
	Seconds     float64   `json:"seconds"`
	Finished    int       `json:"finishedDeals"`
	Interrupted bool      `json:"interrupted"`
	Unit        string    `json:"unit"`
}

func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	commit := strings.TrimSpace(string(out))
	if exec.Command("git", "diff", "--quiet").Run() != nil {
		commit += "+dirty"
	}
	return commit
}

func runEval(args []string) error {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	players := fs.Int("players", 3, "number of players (3 or 4)")
	deals := fs.Int("deals", 200, "number of deals; each is played twice")
	first := fs.Int("first", 0, "deals use seeds first+1 .. first+deals")
	workers := fs.Int("workers", max(1, runtime.NumCPU()/2), "parallel games; keep it below the core count, the search is time-limited")
	xSpec := fs.String("x", "base", "policy X")
	ySpec := fs.String("y", "base", "policy Y")
	name := fs.String("name", "", "run name (default: eval-<time>)")
	dir := fs.String("dir", ".lab", "lab directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *workers < 1 {
		return fmt.Errorf("-workers must be at least 1")
	}
	if *deals < 1 {
		return fmt.Errorf("-deals must be at least 1")
	}
	if *players != 3 && *players != 4 {
		return fmt.Errorf("-players must be 3 or 4")
	}
	config.Interface.LocalBotExactCardsLimit = 40
	x, err := game.ParseLabPolicy(*xSpec)
	if err != nil {
		return err
	}
	y, err := game.ParseLabPolicy(*ySpec)
	if err != nil {
		return err
	}
	started := time.Now()
	runName := *name
	if runName == "" {
		runName = "eval-" + started.Format("20060102-150405")
	}
	runDir := filepath.Join(*dir, "runs", runName)
	if _, err := os.Stat(runDir); err == nil {
		return fmt.Errorf("run %q already exists in %s; choose another -name", runName, runDir)
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	logFile, err := os.Create(filepath.Join(runDir, "log.txt"))
	if err != nil {
		return err
	}
	defer logFile.Close()
	logf := func(format string, a ...any) {
		line := fmt.Sprintf(format, a...)
		fmt.Println(line)
		fmt.Fprintln(logFile, line)
	}
	params := runParams{Command: "eval", Name: runName, Players: *players, Deals: *deals, First: *first, Workers: *workers,
		X: *xSpec, Y: *ySpec, Commit: gitCommit(), GoVersion: runtime.Version(), Started: started, Unit: "points = numerator/(10*players)"}
	writeJSON(filepath.Join(runDir, "params.json"), params)
	logf("run %s: X=%q Y=%q players=%d deals=%d workers=%d commit=%s", runName, *xSpec, *ySpec, *players, *deals, *workers, params.Commit)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	type job struct {
		seed int64
		swap bool
	}
	jobs := make(chan job)
	results := map[job]game.LabResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var done atomic.Int64
	total := int64(*deals) * 2
	step := max(1, total/20)
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				policies := make([]game.LabPolicy, *players)
				others, seat0 := y, x
				if j.swap {
					others, seat0 = x, y
				}
				for i := range policies {
					policies[i] = others
				}
				policies[0] = seat0
				result := game.PlayLabDeal(*players, j.seed, policies)
				mu.Lock()
				results[j] = result
				mu.Unlock()
				if n := done.Add(1); n%step == 0 || n == total {
					logf("  %d/%d games, %s elapsed", n, total, time.Since(started).Round(time.Second))
				}
			}
		}()
	}
feed:
	for d := 0; d < *deals; d++ {
		seed := int64(*first + d + 1)
		for _, swap := range []bool{false, true} {
			select {
			case jobs <- job{seed, swap}:
			case <-ctx.Done():
				break feed
			}
		}
	}
	close(jobs)
	wg.Wait()
	params.Interrupted = ctx.Err() != nil

	diffs := map[string][]float64{}
	var failures []string
	for d := 0; d < *deals; d++ {
		seed := int64(*first + d + 1)
		a, aok := results[job{seed, false}]
		b, bok := results[job{seed, true}]
		if !aok || !bok {
			continue
		}
		if a.Err != nil || b.Err != nil {
			failures = append(failures, fmt.Sprintf("seed %d: %v %v", seed, a.Err, b.Err))
			continue
		}
		diff := a.Points(0) - b.Points(0)
		diffs["all"] = append(diffs["all"], diff)
		diffs[a.Kind] = append(diffs[a.Kind], diff)
		params.Finished++
	}
	var stats []stat
	for kind, values := range diffs {
		stats = append(stats, summarize(kind, values))
	}
	sort.Slice(stats, func(i, j int) bool {
		if (stats[i].Kind == "all") != (stats[j].Kind == "all") {
			return stats[i].Kind == "all"
		}
		return stats[i].Kind < stats[j].Kind
	})
	logf("X=%s  Y=%s  players=%d", *xSpec, *ySpec, *players)
	for _, s := range stats {
		logf("%-9s n=%5d  mean(X-Y)=%+.3f  se=%.3f", s.Kind, s.N, s.Mean, s.SE)
	}
	for i, f := range failures {
		if i == 3 {
			logf("... and %d more failed deals", len(failures)-3)
			break
		}
		logf("FAILED %s", f)
	}
	params.Seconds = time.Since(started).Seconds()
	writeJSON(filepath.Join(runDir, "params.json"), params)
	writeJSON(filepath.Join(runDir, "metrics.json"), stats)
	if err := appendIndex(*dir, params, stats); err != nil {
		return err
	}
	if params.Interrupted {
		logf("interrupted: results cover %d finished deal pairs", params.Finished)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d deals failed", len(failures))
	}
	return nil
}

func summarize(kind string, values []float64) stat {
	s := stat{Kind: kind, N: len(values)}
	for _, v := range values {
		s.Mean += v
	}
	s.Mean /= float64(len(values))
	if len(values) > 1 {
		variance := 0.0
		for _, v := range values {
			variance += (v - s.Mean) * (v - s.Mean)
		}
		s.SE = math.Sqrt(variance / float64(len(values)-1) / float64(len(values)))
	}
	return s
}

func writeJSON(path string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		err = os.WriteFile(path, data, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "botlab: cannot write", path+":", err)
	}
}

var indexHeader = []string{"time", "name", "command", "players", "deals", "x", "y", "all_mean", "all_se", "commit"}

func appendIndex(dir string, p runParams, stats []stat) error {
	path := filepath.Join(dir, "index.csv")
	_, statErr := os.Stat(path)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if statErr != nil {
		w.Write(indexHeader)
	}
	var all stat
	for _, s := range stats {
		if s.Kind == "all" {
			all = s
		}
	}
	w.Write([]string{p.Started.Format("2006-01-02 15:04:05"), p.Name, p.Command, fmt.Sprint(p.Players), fmt.Sprint(p.Finished),
		p.X, p.Y, fmt.Sprintf("%+.3f", all.Mean), fmt.Sprintf("%.3f", all.SE), p.Commit})
	w.Flush()
	return w.Error()
}

func runReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	dir := fs.String("dir", ".lab", "lab directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(*dir, "index.csv"))
	if err != nil {
		return fmt.Errorf("no runs recorded yet (%v)", err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush()
}
