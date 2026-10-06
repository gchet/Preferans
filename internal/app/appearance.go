package app

import "preferans/locales"

import (
	"os"
	"path/filepath"
	"preferans/internal/game"
	"runtime"
	"strings"
)

// Appearance is local to this device, never part of a table's shared state.
type Appearance struct {
	RoomServiceURL string          `json:"roomServiceURL,omitempty"`
	Language       string          `json:"language"`
	LocalOnly      bool            `json:"localOnly"`
	DebugAuction   *bool           `json:"debugAuction,omitempty"`
	DebugPlay      *bool           `json:"debugPlay,omitempty"`
	Debug          bool            `json:"debug"`
	PassPriceHint  *bool           `json:"passPriceHint,omitempty"`
	PlayerCount    int             `json:"playerCount,omitempty"`
	PoolTarget     int             `json:"poolTarget,omitempty"`
	DefaultRoom    string          `json:"defaultRoom"`
	TableRules     *game.Rules     `json:"tableRules,omitempty"`
	Name           string          `json:"name"`
	Deck           string          `json:"deck"`
	CardFrames     map[string]bool `json:"cardFrames,omitempty"`
	Size           string          `json:"size"`
	LargeIndices   bool            `json:"largeIndices"`
	Back           string          `json:"back"`
	LogDisabled    bool            `json:"logDisabled"`
	LogPath        string          `json:"logPath"`
	UpdateURL      string          `json:"updateURL"`
}

func (a *App) SetName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = locales.Text("go.internal.app.appearance.text001")
	}
	if len([]rune(name)) > 24 {
		return locales.Errorf("go.internal.app.appearance.text002")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.appearance
	p.Name = name
	if err := a.config.Save("appearance", p); err != nil {
		return err
	}
	a.appearance = p
	if a.rooms.config.Current == localRoomID {
		a.updateLocalRoomLocked()
	}
	return nil
}

func (p Appearance) valid() bool {
	if p.Language != "" && p.Language != "ru" && p.Language != "uk" && p.Language != "en" {
		return false
	}
	if p.PlayerCount != 0 && p.PlayerCount != 3 && p.PlayerCount != 4 {
		return false
	}
	if p.PoolTarget < 0 || p.PoolTarget > 1000 {
		return false
	}
	if p.TableRules != nil && p.TableRules.Validate() != nil {
		return false
	}
	switch p.Back {
	case "", "plaid", "diamonds", "ornament", "waves", "classic":
	default:
		return false
	}
	switch p.Deck {
	case "english", "atlas", "woodcut":
	default:
		return false
	}
	switch p.Size {
	case "normal", "large", "extra":
	default:
		return false
	}
	return true
}

func (a *App) SetAppearance(p Appearance) error {
	p.Debug = p.debugEnabled()
	p.LogPath = strings.TrimSpace(p.LogPath)
	if p.LogPath != "" && !filepath.IsAbs(p.LogPath) {
		return locales.Errorf("go.internal.app.appearance.text003")
	}
	p.migrateDeck()
	if !p.valid() {
		return locales.Errorf("go.internal.app.appearance.text004")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if p.Name == "" {
		p.Name = a.appearance.Name
	}
	// The connection endpoint is saved by SetRoomService independently.
	p.RoomServiceURL = a.appearance.RoomServiceURL
	if len([]rune(p.UpdateURL)) > 512 {
		return locales.Errorf("go.internal.app.appearance.text005")
	}
	path := a.logPath
	if runtime.GOOS == "windows" && !a.logOverride {
		path = p.LogPath
	}
	changeLog := path != a.logPath || p.LogDisabled != a.appearance.LogDisabled || (!p.LogDisabled && path != "" && a.logFile == nil)
	var next *os.File
	if changeLog && !p.LogDisabled && path != "" {
		var err error
		next, err = openLogFile(path)
		if err != nil {
			return err
		}
	}
	if err := a.config.Save("appearance", p); err != nil {
		if next != nil {
			_ = next.Close()
		}
		return err
	}
	if changeLog {
		old := a.logFile
		a.logFile, a.logPath = next, path
		if old != nil {
			_ = old.Close()
		}
		if next != nil {
			a.logLocked(locales.Text("go.internal.app.appearance.text006"), iceServerSummary(a.stun))
		}
	}
	if s := a.saved.State; s != nil && p.debugPhase(s.Stage) != a.appearance.debugPhase(s.Stage) {
		a.debugBotPermit = ""
		if p.debugPhase(s.Stage) {
			a.cancelAILocked()
		}
	}
	if p.LocalOnly != a.appearance.LocalOnly {
		if err := a.localPreferenceLocked(p.LocalOnly); err != nil {
			return err
		}
	}
	a.appearance = p
	if !p.Debug || (a.saved.State != nil && !p.debugPhase(a.saved.State.Stage)) {
		if a.ai.review != nil {
			a.cancelAILocked()
			a.broadcastLocked()
		}
		if err := a.closeDebugInspectionLocked(); err != nil {
			return err
		}
	}
	return nil
}

func (p Appearance) debugEnabled() bool {
	return p.debugPhase("auction") || p.debugPhase("play")
}

func (p Appearance) debugPhase(stage string) bool {
	var enabled *bool
	switch stage {
	case "auction", "discard", "contract", "defend", "option", "return", "mode", "catch", "dealer-choice", "dealer-whist":
		enabled = p.DebugAuction
	case "play", "trick":
		enabled = p.DebugPlay
	default:
		return false
	}
	if enabled == nil {
		return p.Debug
	}
	return *enabled
}

// Preserve all other preferences when an old deck is retired.
func (p *Appearance) migrateDeck() {
	switch p.Deck {
	case "jumbo":
		p.Deck = "english"
	case "modern", "russian", "european", "portraits":
		p.Deck = "atlas"
	}
}
