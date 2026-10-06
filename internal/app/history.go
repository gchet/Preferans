package app

import "preferans/locales"

import (
	"preferans/internal/game"
	"sort"
	"time"
)

// Called with mu held. Save clock transitions, rather than writing every tick.
func (a *App) syncPauseLocked() bool {
	s := a.saved.State
	if s == nil {
		return false
	}
	paused := a.pausedLocked()
	if (s.PausedAt > 0) == paused {
		return false
	}
	sv := a.saved
	sv.State = s.Clone()
	sv.State.SetPaused(paused, time.Now().Unix())
	if err := a.store.Save(sv.Table, sv); err != nil {
		a.lastError = locales.Text("go.internal.app.history.text001") + err.Error()
		return false
	}
	a.saved = sv
	return true
}

func savedFinished(s Saved) bool {
	return s.State != nil && s.State.Stage == "finished" || s.View != nil && s.View.Stage == "finished"
}
func savedResult(s Saved) *game.View {
	if s.State != nil {
		v := s.State.View(s.Seat)
		return &v
	}
	return s.View
}

type HistoryInfo struct {
	ID         string   `json:"id"`
	Names      []string `json:"names"`
	PlayerIDs  []string `json:"playerIDs"`
	Bots       []bool   `json:"bots"`
	Results    []int    `json:"results"`
	Round      int      `json:"round"`
	FinishedAt int64    `json:"finishedAt"`
	StartedAt  int64    `json:"startedAt"`
}

func (a *App) History() ([]HistoryInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ids, err := a.store.IDs()
	if err != nil {
		return nil, err
	}
	out := []HistoryInfo{}
	for _, id := range ids {
		var s Saved
		if a.store.Load(id, &s) != nil || !savedFinished(s) || s.Room != a.rooms.config.Current {
			continue
		}
		v := savedResult(s)
		item := HistoryInfo{ID: id, Results: v.Results, Round: v.Round, FinishedAt: v.FinishedAt, StartedAt: v.StartedAt}
		for i, p := range v.Players {
			item.Names = append(item.Names, p.Name)
			playerID := ""
			if i < len(s.PlayerIDs) {
				playerID = s.PlayerIDs[i]
			}
			item.PlayerIDs = append(item.PlayerIDs, playerID)
			item.Bots = append(item.Bots, p.Bot)
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FinishedAt > out[j].FinishedAt })
	return out, nil
}
func (a *App) HistoryResult(id string) (*game.View, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var s Saved
	if err := a.store.Load(id, &s); err != nil {
		return nil, err
	}
	if !savedFinished(s) {
		return nil, locales.Errorf("go.internal.app.history.text002")
	}
	if s.Room != a.rooms.config.Current {
		return nil, locales.Errorf("go.internal.app.history.text003")
	}
	return savedResult(s), nil
}
