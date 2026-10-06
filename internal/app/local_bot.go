package app

import (
	"context"
	"encoding/binary"
	"hash/fnv"
	"preferans/config"
	"preferans/internal/game"
	"preferans/locales"
	"time"
)

func (a *App) startLocalSearchLocked(s *game.State, seat int) {
	if a.ai.pending != nil {
		return
	}
	snapshot := s.Clone()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Interface.LocalBotSearchTimeMs)*time.Millisecond)
	request := &aiRequest{table: s.ID, revision: s.Revision, seat: seat, cancel: cancel, started: time.Now()}
	a.ai.pending = request
	a.broadcastLocked()
	go func() {
		defer cancel()
		situation, _ := snapshot.BotChoices(seat, nil)
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(snapshot.ID))
		var revision [8]byte
		binary.LittleEndian.PutUint64(revision[:], snapshot.Revision)
		_, _ = hash.Write(revision[:])
		command, stats := game.SearchBotCommand(ctx, situation, game.SearchOptions{
			Samples: config.Interface.LocalBotSearchSamples, ExactCards: config.Interface.LocalBotExactCards, Seed: int64(hash.Sum64())})
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.ai.pending != request {
			return
		}
		a.ai.pending = nil
		current := a.saved.State
		if current == nil || current.ID != request.table || current.Revision != request.revision || current.Actor() != seat || a.pausedLocked() {
			return
		}
		// Deadline exhaustion still yields the best completed search batch.
		if snapshot.Claim != nil {
			result := locales.Text("go.bot.claim_rejected")
			if command.Action == "accept-claim" {
				result = locales.Text("go.bot.claim_accepted")
			}
			a.logLocked("%s", locales.Format("go.bot.claim_search", current.Players[seat].Name, result, stats.Nodes, stats.Elapsed.Milliseconds()))
		} else {
			a.logLocked("%s", locales.Format("go.bot.local_search", current.Players[seat].Name, stats.Samples, stats.Nodes, stats.Elapsed.Milliseconds()))
		}
		if err := a.commandLocked(command); err != nil {
			// A conservative, legal fallback prevents a bot from stalling a party.
			if err = a.commandLocked(game.BotCommand(current.View(seat))); err != nil {
				a.lastError = err.Error()
			}
		}
	}()
}
