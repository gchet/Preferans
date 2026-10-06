package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"preferans/config"
	"preferans/internal/ai"
	"preferans/internal/game"
	"preferans/internal/secrets"
	"preferans/locales"
)

type AIKey struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	HasValue bool   `json:"hasValue"`
}
type AISettings struct {
	ReviewResponses bool       `json:"reviewResponses"`
	PromptLanguage  string     `json:"promptLanguage"`
	Models          []ai.Model `json:"models"`
	Keys            []AIKey    `json:"keys"`
	TimeoutSeconds  int        `json:"timeoutSeconds"`
}
type aiState struct {
	client    ai.Client
	transfer  *aiTransfer
	settings  AISettings
	encrypted map[string]string
	review    *AIReview
	pending   *aiRequest
}
type aiRequest struct {
	review   bool
	language string
	started  time.Time
	table    string
	revision uint64
	seat     int
	cancel   context.CancelFunc
}

func aiError(code string) error { key := "go.ai." + code; return locales.Errorf(key) }
func (a *App) initAI() {
	a.ai.settings = AISettings{PromptLanguage: "ru", Models: []ai.Model{}, Keys: []AIKey{}, TimeoutSeconds: 10}
	if !config.AIEnabled {
		return
	}
	var settings AISettings
	if a.config.Load("ai-settings", &settings) == nil {
		a.ai.settings = settings
	}
	if a.ai.settings.TimeoutSeconds < 1 || a.ai.settings.TimeoutSeconds > 120 {
		a.ai.settings.TimeoutSeconds = 10
	}
	if a.ai.settings.PromptLanguage != "en" {
		a.ai.settings.PromptLanguage = "ru"
	}
	validModels := []ai.Model{}
	for _, m := range a.ai.settings.Models {
		if err := m.Validate(); err != nil {
			a.lastError = aiError(err.Error()).Error()
			continue
		}
		validModels = append(validModels, m)
	}
	a.ai.settings.Models = validModels
	if a.ai.settings.Keys == nil {
		a.ai.settings.Keys = []AIKey{}
	}
	a.ai.encrypted = map[string]string{}
	if err := a.config.Load("ai-keys", &a.ai.encrypted); err != nil && !os.IsNotExist(err) {
		a.lastError = aiError("key-load").Error()
	}
	if a.ai.encrypted == nil {
		a.ai.encrypted = map[string]string{}
	}
}
func (a *App) SetSecretCipher(cipher secrets.Cipher) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.secretCipher = cipher
}
func (a *App) AISettings() AISettings {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.ai.settings
	s.Models = append([]ai.Model{}, s.Models...)
	s.Keys = append([]AIKey{}, s.Keys...)
	for i := range s.Keys {
		s.Keys[i].HasValue = a.ai.encrypted[s.Keys[i].ID] != ""
	}
	return s
}
func (a *App) modelLocked(id string) (ai.Model, bool) {
	if !config.AIEnabled {
		return ai.Model{}, false
	}
	for _, m := range a.ai.settings.Models {
		if m.ID == id {
			return m, true
		}
	}
	return ai.Model{}, false
}
func (a *App) SaveAISettings(settings AISettings) error {
	if !config.AIEnabled {
		return aiError("build-disabled")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if settings.TimeoutSeconds < 1 || settings.TimeoutSeconds > 120 {
		return aiError("timeout")
	}
	if settings.PromptLanguage != "en" && settings.PromptLanguage != "ru" && settings.PromptLanguage != "" {
		return aiError("prompt-language")
	}
	if settings.PromptLanguage == "" {
		settings.PromptLanguage = "ru"
	}
	aliases, ids := map[string]bool{}, map[string]bool{}
	for i, m := range settings.Models {
		m.Alias = strings.TrimSpace(m.Alias)
		m.Model = strings.TrimSpace(m.Model)
		m.URL = strings.TrimSpace(m.URL)
		if err := m.Validate(); err != nil {
			return aiError(err.Error())
		}
		alias := strings.ToLower(m.Alias)
		if aliases[alias] || ids[m.ID] {
			return aiError("duplicate")
		}
		aliases[alias] = true
		ids[m.ID] = true
		if m.KeyID != "" {
			found := false
			for _, key := range a.ai.settings.Keys {
				if key.ID == m.KeyID {
					found = true
				}
			}
			if !found {
				return aiError("key-missing")
			}
		}
		settings.Models[i] = m
	}
	settings.Keys = append([]AIKey{}, a.ai.settings.Keys...)
	if settings.Models == nil {
		settings.Models = []ai.Model{}
	}
	if err := a.config.Save("ai-settings", settings); err != nil {
		return err
	}
	a.ai.settings = settings
	a.cancelAILocked()
	if a.saved.State != nil {
		s := a.saved.State.Clone()
		changed := a.updateBotNamesLocked(s)
		if changed {
			s.Revision++
			sv := a.saved
			sv.State = s
			if err := a.store.Save(s.ID, sv); err != nil {
				return err
			}
			a.saved = sv
			a.broadcastLocked()
		}
	}
	if a.saved.State != nil {
		a.broadcastLocked()
	}
	return nil
}

// Key material never leaves this method through a status or settings response.
func (a *App) SaveAIKey(id, name, value string) (string, error) {
	if !config.AIEnabled {
		return "", aiError("build-disabled")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if name == "" || len([]rune(name)) > 40 || len(value) > 4096 || strings.ContainsAny(value, "\r\n") {
		return "", aiError("key-invalid")
	}
	if id == "" {
		id = game.ID()
	} else {
		found := false
		for _, k := range a.ai.settings.Keys {
			if k.ID == id {
				found = true
			}
		}
		if !found {
			return "", aiError("key-missing")
		}
	}
	for _, k := range a.ai.settings.Keys {
		if k.ID != id && strings.EqualFold(k.Name, name) {
			return "", aiError("duplicate")
		}
	}
	settings := a.ai.settings
	settings.Keys = append([]AIKey{}, settings.Keys...)
	values := map[string]string{}
	for k, v := range a.ai.encrypted {
		values[k] = v
	}
	if value != "" {
		if a.secretCipher == nil {
			return "", aiError("vault-unavailable")
		}
		sealed, err := a.secretCipher.Seal(value)
		if err != nil {
			return "", aiError("key-save")
		}
		values[id] = sealed
	} else if values[id] == "" {
		return "", aiError("key-invalid")
	}
	found := false
	for i, k := range settings.Keys {
		if k.ID == id {
			settings.Keys[i] = AIKey{ID: id, Name: name}
			found = true
		}
	}
	if !found {
		settings.Keys = append(settings.Keys, AIKey{ID: id, Name: name})
	}
	if err := a.config.Save("ai-keys", values); err != nil {
		return "", err
	}
	if err := a.config.Save("ai-settings", settings); err != nil {
		_ = a.config.Save("ai-keys", a.ai.encrypted)
		return "", err
	}
	a.ai.encrypted = values
	a.ai.settings = settings
	a.cancelAILocked()
	return id, nil
}
func (a *App) DeleteAIKey(id string) error {
	if !config.AIEnabled {
		return aiError("build-disabled")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, m := range a.ai.settings.Models {
		if m.KeyID == id {
			return aiError("key-used")
		}
	}
	settings := a.ai.settings
	settings.Keys = []AIKey{}
	for _, k := range a.ai.settings.Keys {
		if k.ID != id {
			settings.Keys = append(settings.Keys, k)
		}
	}
	values := map[string]string{}
	for k, v := range a.ai.encrypted {
		if k != id {
			values[k] = v
		}
	}
	if err := a.config.Save("ai-keys", values); err != nil {
		return err
	}
	if err := a.config.Save("ai-settings", settings); err != nil {
		_ = a.config.Save("ai-keys", a.ai.encrypted)
		return err
	}
	a.ai.settings = settings
	a.ai.encrypted = values
	a.cancelAILocked()
	return nil
}
func (a *App) updateBotNamesLocked(s *game.State) bool {
	changed := false
	for i, p := range s.Players {
		if !p.Bot {
			continue
		}
		name := locales.Format("go.internal.app.app.text008", i)
		if config.AIEnabled && p.BotModel != "" {
			if m, ok := a.modelLocked(p.BotModel); ok {
				name += " " + m.Alias
			} else {
				s.Players[i].BotModel = ""
				a.lastError = aiError("model-missing").Error()
				a.logLocked("%s", a.lastError)
				changed = true
			}
		}
		if p.Name != name {
			s.Players[i].Name = name
			changed = true
		}
	}
	return changed
}
func (a *App) AssignBot(seat int, model string) error {
	if !config.AIEnabled {
		return aiError("build-disabled")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.saved.State
	if s == nil || seat < 0 || seat >= len(s.Players) || !s.Players[seat].Bot || (s.Stage != "lobby" && !s.ManualPaused) {
		return aiError("assign-unavailable")
	}
	if model != "" {
		if _, ok := a.modelLocked(model); !ok {
			return aiError("model-missing")
		}
	}
	s = s.Clone()
	s.Players[seat].BotModel = model
	a.updateBotNamesLocked(s)
	s.Revision++
	sv := a.saved
	sv.State = s
	if err := a.store.Save(s.ID, sv); err != nil {
		return err
	}
	a.cancelAILocked()
	a.saved = sv
	a.broadcastLocked()
	return nil
}
func (a *App) cancelAILocked() {
	if a.ai.review != nil && a.ai.review.chatCancel != nil {
		a.ai.review.chatCancel()
	}
	a.ai.review = nil
	if a.ai.pending != nil {
		a.ai.pending.cancel()
		a.ai.pending = nil
	}
}
func (a *App) checkAIRequestLocked() {
	r := a.ai.pending
	s := a.saved.State
	if r != nil && (s == nil || s.ID != r.table || s.Revision != r.revision || s.Actor() != r.seat || a.pausedLocked()) {
		a.cancelAILocked()
	}
}
func (a *App) botActionLocked(s *game.State, seat int) {
	if a.appearance.debugPhase(s.Stage) && a.debugBotPermit != debugBotKey(s) {
		return
	}
	if !config.AIEnabled || a.rooms.config.Current == localRoomID {
		a.localBotLocked(s, seat, "")
		return
	}
	if a.ai.pending != nil {
		return
	}
	m, ok := a.modelLocked(s.Players[seat].BotModel)
	if !ok || (s.DebugBots != "" && s.Stage == "auction") {
		a.localBotLocked(s, seat, "")
		return
	}
	key := ""
	if m.KeyID != "" {
		if a.secretCipher == nil || a.ai.encrypted[m.KeyID] == "" {
			a.localBotLocked(s, seat, "key-unavailable")
			return
		}
		var err error
		key, err = a.secretCipher.Open(a.ai.encrypted[m.KeyID])
		if err != nil {
			a.localBotLocked(s, seat, "key-unavailable")
			return
		}
	}
	snapshot := s.Clone()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(a.ai.settings.TimeoutSeconds)*time.Second)
	request := &aiRequest{table: s.ID, revision: s.Revision, seat: seat, cancel: cancel, started: time.Now()}
	request.review = a.appearance.debugPhase(s.Stage) && a.ai.settings.ReviewResponses
	request.language = a.ai.settings.PromptLanguage
	a.ai.pending = request
	a.broadcastLocked()
	a.logLocked("%s", locales.Format("go.ai.request", s.Players[seat].Name, s.Stage))
	client := a.ai.client
	client.Trace = request.review
	client.PromptLanguage = request.language
	go a.chooseAI(ctx, request, snapshot, m, key, client)
}

func (a *App) viewWithAILocked(seat int) game.View {
	v := a.saved.State.View(seat)
	enabled := config.AIEnabled && a.rooms.config.Current != localRoomID
	v.HostAIEnabled = &enabled
	if seat == 0 {
		v.DebugWaitingSeat = a.debugWaitingLocked()
	}
	if review := a.ai.review; review != nil && review.table == v.ID {
		i := review.Seat
		v.AIReviewSeat = &i
	}
	inspection := a.debugInspection
	if seat == 0 && a.appearance.Debug && v.ManualPaused && inspection != nil && inspection.table == v.ID {
		i := inspection.seat
		v.DebugBotSeat = &i
		v.Players[i].Cards = append([]game.Card{}, a.saved.State.Hands[i]...)
	}
	r := a.ai.pending
	if r != nil && r.table == v.ID && r.revision == v.Revision && r.seat == v.Actor && !a.pausedLocked() {
		v.BotThinking = &game.BotThinking{Seat: r.seat, StartedAt: r.started.UnixMilli()}
	}
	return v
}

// A guest keeps the host capability in its saved view across reconnections.
// Older hosts omit the field and retain the previous local-build behaviour.
func (a *App) aiEnabledLocked() bool {
	if a.rooms.config.Current == localRoomID {
		return false
	}
	if !config.AIEnabled {
		return false
	}
	if a.saved.State != nil {
		return true
	}
	if r := a.rooms.room; r != nil {
		if r.Table != nil {
			if a.saved.View != nil && a.saved.View.ID == r.Table.ID && a.saved.View.HostAIEnabled != nil {
				return *a.saved.View.HostAIEnabled
			}
			for _, m := range r.Members {
				if m.ID == r.Table.Host && m.AIEnabled != nil {
					return *m.AIEnabled
				}
			}
		} else {
			// The member list preserves arrival order, independently of free slots.
			for _, m := range r.Members {
				if m.Online && m.Platform == "windows" && m.AIEnabled != nil {
					return *m.AIEnabled
				}
			}
			return true
		}
	}
	return a.saved.State != nil || a.saved.View == nil || a.saved.View.HostAIEnabled == nil || *a.saved.View.HostAIEnabled
}
func (a *App) localBotLocked(s *game.State, seat int, reason string) {
	if reason != "" {
		a.logLocked("%s", locales.Format("go.ai.fallback", s.Players[seat].Name, reason))
	}
	if s.Stage == "play" && (s.Claim != nil || len(s.View(seat).Legal) > 1) {
		a.startLocalSearchLocked(s, seat)
		return
	}
	c := game.BotCommand(s.View(seat))
	if c.Action != "" {
		if err := a.commandLocked(c); err != nil {
			a.lastError = err.Error()
		}
	}
}
func (a *App) chooseAI(ctx context.Context, r *aiRequest, s *game.State, m ai.Model, key string, client ai.Client) {
	started := time.Now()
	situation, commands := s.BotChoices(r.seat, nil)
	situation.RuleExplanation = s.RuleExplanation(r.language)
	var result ai.Result
	var err error
	if len(commands) == 0 {
		err = fmt.Errorf("no-actions")
	} else if len(commands) == 1 {
		result.Move = 0
	} else {
		result, err = client.Select(ctx, m, key, config.AIPrompt(s.Stage, r.language), situation, len(commands))
	}
	for i := range result.Exchanges {
		result.Exchanges[i].Phase = s.Stage
		result.Exchanges[i].SystemRU = config.AIPrompt(s.Stage, "ru")
	}
	var command game.Command
	if err == nil {
		command = commands[result.Move]
		if s.Stage == "discard" && command.Action == "declare" && !s.TalonShown {
			next, options := s.BotChoices(r.seat, command.Cards)
			next.RuleExplanation = s.RuleExplanation(r.language)
			var selection ai.Result
			if len(options) == 1 {
				selection.Move = 0
			} else if len(options) == 0 {
				err = fmt.Errorf("no-actions")
			} else {
				selection, err = client.Select(ctx, m, key, config.AIPrompt("contract", r.language), next, len(options))
			}
			for i := range selection.Exchanges {
				selection.Exchanges[i].Phase = "contract"
				selection.Exchanges[i].SystemRU = config.AIPrompt("contract", "ru")
			}
			result.Exchanges = append(result.Exchanges, selection.Exchanges...)
			result.PromptTokens += selection.PromptTokens
			result.CompletionTokens += selection.CompletionTokens
			result.Attempts += selection.Attempts
			if err == nil {
				command = options[selection.Move]
				result.Why = selection.Why
			}
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	defer r.cancel()
	if a.ai.pending != r {
		return
	}
	a.ai.pending = nil
	current := a.saved.State
	if current == nil || current.ID != r.table || current.Revision != r.revision || current.Actor() != r.seat || a.pausedLocked() {
		return
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if a.logFile != nil && !a.appearance.LogDisabled {
		why := result.Why
		if key != "" {
			why = strings.ReplaceAll(why, key, "[redacted]")
		}
		trace := map[string]any{"ai": m.Alias, "phase": s.Stage, "situation": situation, "command": command, "why": why, "elapsedMs": time.Since(started).Milliseconds(), "promptTokens": result.PromptTokens, "completionTokens": result.CompletionTokens, "attempts": result.Attempts}
		if err != nil {
			trace["error"] = err.Error()
		}
		data, _ := json.Marshal(trace)
		_, _ = fmt.Fprintln(a.logFile, string(data))
	}
	if r.review && a.appearance.debugPhase(current.Stage) && a.ai.settings.ReviewResponses && result.Attempts > 0 {
		if err != nil {
			command = game.BotCommand(current.View(r.seat))
		}
		a.holdAIReviewLocked(current, r, m, key, result, command, err, time.Since(started))
		return
	}
	if err != nil {
		a.localBotLocked(current, r.seat, err.Error())
		return
	}
	if err = a.commandLocked(command); err != nil {
		a.localBotLocked(current, r.seat, "engine-rejected")
		return
	}
	// Only safe counters and the action are logged. Explanations can expose hands.
	a.logLocked("%s", locales.Format("go.ai.response", m.Alias, command.Action, time.Since(started).Milliseconds(), result.PromptTokens, result.CompletionTokens, result.Attempts))
}
