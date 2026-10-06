package app

import "preferans/locales"

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"preferans/config"
	"preferans/internal/game"
	"preferans/internal/network"
	"preferans/internal/rooms"
	"preferans/internal/secrets"
	"preferans/internal/storage"
)

type Saved struct {
	Pending   *game.Command `json:"pending,omitempty"`
	Room      string        `json:"room,omitempty"`
	PlayerIDs []string      `json:"playerIDs,omitempty"`
	State     *game.State   `json:"state,omitempty"`
	Keys      []string      `json:"keys,omitempty"`
	Joined    []bool        `json:"joined,omitempty"`
	Table     string        `json:"table,omitempty"`
	Seat      int           `json:"seat"`
	Key       string        `json:"key,omitempty"`
	View      *game.View    `json:"view,omitempty"`
}
type Packet struct {
	ConnectionReasons map[int]string `json:"connectionReasons,omitempty"`
	Ack               string         `json:"ack,omitempty"`
	Name              string         `json:"name,omitempty"`
	Voice             *VoiceMessage  `json:"voice,omitempty"`
	ProbeID           string         `json:"probeID,omitempty"`
	Payload           string         `json:"payload,omitempty"`
	Type              string         `json:"type"`
	Key               string         `json:"key,omitempty"`
	Nonce             string         `json:"nonce,omitempty"`
	Command           *game.Command  `json:"command,omitempty"`
	View              *game.View     `json:"view,omitempty"`
	Connected         []bool         `json:"connected,omitempty"`
	Error             string         `json:"error,omitempty"`
	Paused            bool           `json:"paused"`
}
type Link struct {
	ExitAck      chan struct{}
	LastSeen     time.Time
	AuthDeadline time.Time
	Probe        probeState
	Peer         *network.Peer
	Nonce        string
	Auth         bool
	Accepted     bool
	Status       string
}
type App struct {
	connectionReasons map[int]string
	debugInspection   *debugInspection
	debugBotPermit    string
	ai                aiState
	secretCipher      secrets.Cipher
	rooms             roomState
	roomControl       sync.Mutex
	voiceQueue        []VoiceEvent
	voiceSeq          uint64
	diagnostic        bool
	mu                sync.Mutex
	store             storage.Store
	config            storage.Store
	saved             Saved
	peers             map[int]*Link
	connected         []bool
	clientPaused      bool
	lastError         string
	logs              []string
	logFile           *os.File
	logPath           string
	logOverride       bool
	stun              []string
	appearance        Appearance
	closed            chan struct{}
	once              sync.Once
}
type Status struct {
	ConnectionReasons map[int]string `json:"connectionReasons"`
	AIEnabled         bool           `json:"aiEnabled"`
	AIReview          *AIReview      `json:"aiReview,omitempty"`
	ConnectionSetup   bool           `json:"connectionSetup"`
	LogPath           string         `json:"logPath"`
	LogSize           int64          `json:"logSize"`
	LogPathOverride   bool           `json:"logPathOverride"`
	Room              *rooms.Room    `json:"room,omitempty"`
	RoomMode          bool           `json:"roomMode"`
	RoomLocal         bool           `json:"roomLocal"`
	RoomSelf          string         `json:"roomSelf"`
	RoomError         string         `json:"roomError"`
	RoomOnline        bool           `json:"roomOnline"`
	FreeSeats         int            `json:"freeSeats"`
	Current           bool           `json:"current"`
	View              *game.View     `json:"view"`
	Connected         []bool         `json:"connected"`
	Paused            bool           `json:"paused"`
	Error             string         `json:"error"`
	Host              bool           `json:"host"`
	STUN              []string       `json:"stun"`
	Links             map[int]string `json:"links"`
	Appearance        Appearance     `json:"appearance"`
	Logs              []string       `json:"logs"`
}
type SaveInfo struct {
	ID    string   `json:"id"`
	Round int      `json:"round"`
	Stage string   `json:"stage"`
	Host  bool     `json:"host"`
	Names []string `json:"names"`
}

func New(dir string) *App {
	a := &App{store: storage.Store{Dir: filepath.Join(dir, "saves")}, config: storage.Store{Dir: dir}, peers: map[int]*Link{}, stun: []string{"stun:stun.l.google.com:19302", "stun:stun.cloudflare.com:3478"}, closed: make(chan struct{})}
	var urls []string
	if a.config.Load("settings", &urls) == nil {
		a.stun = urls
	}
	a.appearance = Appearance{Name: locales.Text("go.internal.app.app.text001"), Deck: "atlas", Size: "normal", LargeIndices: true, Back: "plaid"}
	var appearance Appearance
	if a.config.Load("appearance", &appearance) == nil {
		appearance.migrateDeck()
		if appearance.valid() {
			a.appearance = appearance
		}
	}
	if runtime.GOOS == "windows" {
		a.logPath = a.appearance.LogPath
		if a.logPath != "" && !a.appearance.LogDisabled {
			if err := a.enableLogFile(a.logPath); err != nil {
				a.lastError = err.Error()
			}
		}
	}
	a.initRooms()
	a.secretCipher = secrets.Platform()
	a.initAI()
	go a.botLoop()
	return a
}
func (a *App) Close() {
	a.once.Do(func() {
		close(a.closed)
		a.mu.Lock()
		a.cancelAILocked()
		a.rooms.generation++
		acks := a.notifyExitLocked()
		a.mu.Unlock()
		waitExitAcks(acks)
		a.mu.Lock()
		a.disconnectLocked()
		if a.logFile != nil {
			_ = a.logFile.Close()
			a.logFile = nil
		}
		a.mu.Unlock()
	})
}

// SetLogFile enables an append-only diagnostic log. It is intentionally
// configured by the native shell; it takes priority over the saved path.
func (a *App) SetLogFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	a.mu.Lock()
	a.logPath = path
	a.logOverride = true
	disabled := a.appearance.LogDisabled
	a.mu.Unlock()
	if disabled {
		return nil
	}
	return a.enableLogFile(path)
}

func openLogFile(path string) (*os.File, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, locales.Errorf("go.internal.app.app.text002", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, locales.Errorf("go.internal.app.app.text003", path, err)
	}
	return f, nil
}

func (a *App) enableLogFile(path string) error {
	f, err := openLogFile(path)
	if err != nil {
		return err
	}
	a.mu.Lock()
	if a.appearance.LogDisabled {
		a.mu.Unlock()
		_ = f.Close()
		return nil
	}
	old := a.logFile
	a.logFile = f
	a.logLocked(locales.Text("go.internal.app.app.text004"), runtime.GOOS, runtime.GOARCH, iceServerSummary(a.stun))
	a.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

// SetLogEnabled toggles writing the diagnostic trace to the configured file.
// The in-memory trace shown by the UI remains available in either mode.
func (a *App) SetLogEnabled(enabled bool) error {
	a.mu.Lock()
	oldDisabled := a.appearance.LogDisabled
	a.appearance.LogDisabled = !enabled
	if err := a.config.Save("appearance", a.appearance); err != nil {
		a.appearance.LogDisabled = oldDisabled
		a.mu.Unlock()
		return err
	}
	path := a.logPath
	if !enabled {
		old := a.logFile
		a.logFile = nil
		a.mu.Unlock()
		if old != nil {
			_ = old.Close()
		}
		return nil
	}
	a.mu.Unlock()
	if path == "" {
		return nil
	}
	return a.enableLogFile(path)
}
func (a *App) disconnectLocked() {
	a.cancelAILocked()
	a.voiceQueue = nil
	for _, l := range a.peers {
		go l.Peer.Close()
	}
	a.peers = map[int]*Link{}
	a.connected = nil
	a.connectionReasons = nil
}
func (a *App) pausedLocked() bool {
	if a.saved.State == nil {
		if savedFinished(a.saved) {
			return false
		}
		return a.clientPaused
	}
	if a.saved.State.Stage == "lobby" || a.saved.State.Stage == "finished" {
		return false
	}
	if a.saved.State.ManualPaused {
		return true
	}
	if a.ai.review != nil && a.ai.review.table == a.saved.State.ID {
		return true
	}
	for _, c := range a.connected {
		if !c {
			return true
		}
	}
	return false
}
func (a *App) statusLocked() Status {
	if a.syncPauseLocked() {
		a.broadcastLocked()
	}
	s := Status{Current: a.saved.Table != "", Connected: append([]bool{}, a.connected...), Paused: a.pausedLocked(), Error: a.lastError, Host: a.saved.State != nil, STUN: append([]string{}, a.stun...), Links: map[int]string{}}
	s.AIEnabled = a.aiEnabledLocked()
	s.LogPath, s.LogPathOverride = a.logPath, a.logOverride
	s.LogSize = a.logSizeLocked()
	s.AIReview = a.aiReviewSnapshotLocked()
	s.ConnectionSetup = !hasTURN(a.stun) || a.rooms.url == ""
	s.Appearance = a.appearance
	s.RoomMode = true
	s.RoomLocal = a.rooms.config.Current == localRoomID
	if s.RoomLocal {
		a.updateLocalRoomLocked()
		s.ConnectionSetup = false
	}
	s.RoomSelf = a.rooms.identity()
	s.Room = cloneRoom(a.rooms.room)
	s.RoomError = a.rooms.err
	s.RoomOnline = time.Since(a.rooms.seen) < time.Duration(config.Interface.RoomOnlineTimeoutMs)*time.Millisecond
	if s.RoomLocal {
		s.RoomOnline = true
	}
	s.Logs = append([]string(nil), a.logs...)
	s.FreeSeats = len(a.freeSeatsLocked())
	if a.saved.State != nil {
		v := a.viewWithAILocked(0)
		s.View = &v
	} else {
		s.View = a.saved.View
	}
	for p, l := range a.peers {
		s.Links[p] = l.Status
	}
	s.ConnectionReasons = a.connectionReasonsLocked()
	return s
}
func (a *App) Status() Status { a.mu.Lock(); defer a.mu.Unlock(); return a.statusLocked() }

// logLocked keeps a short, password-free connection trace for the UI.
func (a *App) logLocked(format string, args ...any) {
	line := fmt.Sprintf("%s  %s", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	a.logs = append(a.logs, line)
	if len(a.logs) > 240 {
		a.logs = a.logs[len(a.logs)-240:]
	}
	if a.logFile != nil {
		_, _ = fmt.Fprintln(a.logFile, line)
	}
}

func (a *App) DeleteSave(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == a.saved.Table {
		return locales.Errorf("go.internal.app.app.text005")
	}
	return a.store.Delete(id)
}

// Previously joined humans keep their seats even while disconnected.
func (a *App) freeSeatsLocked() []int {
	var seats []int
	if a.saved.State == nil || a.saved.State.Stage != "lobby" {
		return seats
	}
	for i, p := range a.saved.State.Players {
		if i < len(a.saved.PlayerIDs) && a.saved.PlayerIDs[i] != "" {
			continue // Room participants keep their seat while establishing a connection.
		}
		if i > 0 && !p.Bot && !a.connected[i] && !a.saved.Joined[i] {
			seats = append(seats, i)
		}
	}
	return seats
}

func (a *App) FillBots() (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.saved.State == nil || a.saved.State.Stage != "lobby" {
		return 0, locales.Errorf("go.internal.app.app.text006")
	}
	seats := a.freeSeatsLocked()
	if len(seats) == 0 {
		return 0, nil
	}
	sv := a.saved
	sv.State = sv.State.Clone()
	sv.Keys = append([]string{}, sv.Keys...)
	for _, i := range seats {
		sv.State.Players[i] = game.Player{Name: locales.Format("go.internal.app.app.text007", i), Bot: true, Ready: true}
		sv.Keys[i] = game.ID()
	}
	sv.State.Revision++
	if err := a.store.Save(sv.State.ID, sv); err != nil {
		return 0, err
	}
	a.saved = sv
	for _, i := range seats {
		if l := a.peers[i]; l != nil {
			delete(a.peers, i)
			go l.Peer.Close()
		}
		a.connected[i] = true
	}
	a.broadcastLocked()
	return len(seats), nil
}
func (a *App) List() ([]SaveInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ids, e := a.store.IDs()
	if e != nil {
		return nil, e
	}
	out := []SaveInfo{}
	for _, id := range ids {
		var s Saved
		if a.store.Load(id, &s) != nil {
			continue
		}
		// Guest saves retain their rejoin keys on disk, but are not resumable
		// games in the host's list. Finished games are also hidden.
		if s.State == nil || s.State.Stage == "finished" {
			continue
		}
		if (s.Room == localRoomID) != (a.rooms.config.Current == localRoomID) {
			continue
		}
		i := SaveInfo{ID: id, Host: true, Round: s.State.Round, Stage: s.State.Stage}
		for _, p := range s.State.Players {
			i.Names = append(i.Names, p.Name)
		}
		out = append(out, i)
	}
	return out, nil
}
func (a *App) Create(n, target int, name string, bots bool) error {
	s, e := game.New(n, target)
	if e != nil {
		return e
	}
	a.mu.Lock()
	if a.appearance.TableRules != nil {
		r := *a.appearance.TableRules
		r.PassPrices = append([]int{}, r.PassPrices...)
		s.Rules = &r
	}
	a.mu.Unlock()
	if name != "" {
		s.Players[0].Name = name
	}
	for i := 1; i < n; i++ {
		if bots {
			s.Players[i] = game.Player{Name: locales.Format("go.internal.app.app.text008", i), Bot: true, Ready: true}
		}
	}
	sv := Saved{State: s, Keys: make([]string, n), Joined: make([]bool, n), Table: s.ID}
	for i := range sv.Keys {
		sv.Keys[i] = game.ID()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if e = a.store.Save(s.ID, sv); e != nil {
		return e
	}
	a.disconnectLocked()
	a.saved = sv
	a.connected = make([]bool, n)
	for i, p := range s.Players {
		a.connected[i] = i == 0 || p.Bot
	}
	a.lastError = ""
	return nil
}
func (a *App) Resume(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	var s Saved
	if e := a.store.Load(id, &s); e != nil {
		return e
	}
	if s.State != nil {
		if s.State.Version != game.RulesVersion || len(s.Keys) != len(s.State.Players) || len(s.Joined) != len(s.Keys) {
			return locales.Errorf("go.internal.app.app.text009")
		}
	}
	a.disconnectLocked()
	a.saved = s
	a.lastError = ""
	if s.State != nil {
		if a.updateBotNamesLocked(s.State) {
			s.State.Revision++
			_ = a.store.Save(s.State.ID, s)
		}
		a.connected = make([]bool, len(s.State.Players))
		for i, p := range s.State.Players {
			a.connected[i] = i == 0 || p.Bot
		}
	} else {
		a.clientPaused = true
	}
	return nil
}
func (a *App) Leave() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.disconnectLocked()
	a.saved = Saved{}
	a.lastError = ""
}
func (a *App) Configure(stun []string) error {
	if len(stun) > 8 {
		return locales.Errorf("go.internal.app.app.text010")
	}
	for _, u := range stun {
		parts := strings.Split(u, "|")
		if len(parts) == 1 && !strings.HasPrefix(u, "stun:") {
			return locales.Errorf("go.internal.app.app.text011")
		}
		if len(parts) == 3 && !(strings.HasPrefix(parts[0], "turn:") || strings.HasPrefix(parts[0], "turns:")) {
			return locales.Errorf("go.internal.app.app.text012")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if e := a.config.Save("settings", stun); e != nil {
		return e
	}
	a.stun = append([]string{}, stun...)
	return nil
}
func (a *App) commandLocked(c game.Command) error {
	a.syncPauseLocked()
	pauseCommand := c.Action == "pause" || c.Action == "resume-play"
	if a.saved.State == nil {
		l := a.peers[0]
		if l == nil {
			return locales.Errorf("go.internal.app.app.text013")
		}
		if a.pausedLocked() && !pauseCommand {
			return locales.Errorf("go.internal.app.app.text014")
		}
		c.Seat = a.saved.Seat
		if a.saved.Pending != nil && a.saved.Pending.ID != c.ID {
			return locales.Errorf("go.internal.app.app.text015")
		}
		a.saved.Pending = &c
		if e := a.store.Save(a.saved.Table, a.saved); e != nil {
			return e
		}
		return l.Peer.Send(Packet{Type: "command", Command: &c})
	}
	s := a.saved.State
	if a.pausedLocked() && !pauseCommand {
		return locales.Errorf("go.internal.app.app.text016")
	}
	if c.Action == "start" {
		for _, v := range a.connected {
			if !v {
				return locales.Errorf("go.internal.app.app.text017")
			}
		}
	}
	next, e := game.Apply(s, c, nil)
	if e != nil {
		return e
	}
	if next == s {
		a.broadcastLocked()
		return nil
	}
	sv := a.saved
	sv.State = next
	if e = a.store.Save(next.ID, sv); e != nil {
		return locales.Errorf("go.internal.app.app.text018", e)
	}
	a.saved = sv
	if c.Action == "resume-play" {
		a.debugInspection = nil
	}
	a.syncPauseLocked()
	a.lastError = ""
	a.broadcastLocked()
	return nil
}
func (a *App) Command(c game.Command) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.saved.State != nil {
		c.Seat = 0
	}
	return a.commandLocked(c)
}
func (a *App) broadcastLocked() {
	if a.saved.State == nil {
		return
	}
	a.syncPauseLocked()
	for seat, l := range a.peers {
		if !l.Auth {
			continue
		}
		v := a.viewWithAILocked(seat)
		e := l.Peer.Send(Packet{Type: "view", View: &v, Connected: a.connected, Paused: a.pausedLocked(), ConnectionReasons: a.connectionReasonsLocked()})
		if e != nil {
			l.Status = "disconnected"
			a.rememberConnectionLossLocked(seat, l)
			a.connected[seat] = false
		}
	}
}
func (a *App) botLoop() {
	tick := time.NewTicker(time.Duration(config.Interface.BotTickIntervalMs) * time.Millisecond)
	defer tick.Stop()
	var trickKey string
	var trickSince time.Time
	for {
		select {
		case <-a.closed:
			return
		case <-tick.C:
			a.mu.Lock()
			if a.syncPauseLocked() {
				a.broadcastLocked()
			}
			s := a.saved.State
			if s != nil && s.Stage == "round" && s.FinishDue() && !a.pausedLocked() {
				if e := a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: s.Revision, Action: "expire"}); e != nil {
					a.lastError = e.Error()
				}
				s = a.saved.State
			}
			if s != nil && (s.Stage == "trick" || s.Stage == "round") && !a.pausedLocked() {
				key := fmt.Sprintf("%s/%d", s.ID, s.Revision)
				if key != trickKey {
					trickKey = key
					trickSince = time.Now()
				}
				if time.Since(trickSince) >= time.Duration(config.Interface.NextTrickDelayMs)*time.Millisecond {
					action := "collect"
					if s.Stage == "round" {
						action = "next"
					}
					if e := a.commandLocked(game.Command{ID: game.ID(), Seat: 0, Revision: s.Revision, Action: action}); e != nil {
						a.lastError = e.Error()
					}
				}
			} else {
				trickKey = ""
			}
			a.checkAIRequestLocked()
			if s != nil && !a.pausedLocked() && s.Stage != "lobby" && s.Stage != "round" && s.Stage != "finished" && s.Stage != "trick" {
				p := s.Actor()
				if p >= 0 && s.Players[p].Bot {
					a.botActionLocked(s, p)
				}
			}
			a.mu.Unlock()
		}
	}
}
func (a *App) receiveHost(seat int, l *Link, b []byte) {
	var p Packet
	if e := json.Unmarshal(b, &p); e != nil {
		a.mu.Lock()
		a.logLocked(locales.Text("go.internal.app.app.text019"), seat+1, e)
		a.mu.Unlock()
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.peers[seat] != l || a.saved.State == nil {
		return
	}
	if a.receiveExitLocked(seat, l, p) {
		return
	}
	if p.Type == "hello" {
		if p.Key != a.saved.Keys[seat] || p.Nonce != l.Nonce {
			a.logLocked(locales.Text("go.internal.app.app.text020"), seat+1)
			_ = l.Peer.Send(Packet{Type: "error", Error: locales.Text("go.internal.app.app.text021")})
			return
		}
		sv := a.saved
		sv.Joined = append([]bool{}, sv.Joined...)
		sv.Joined[seat] = true
		if name := strings.TrimSpace(p.Name); name != "" {
			if len([]rune(name)) > 24 {
				_ = l.Peer.Send(Packet{Type: "error", Error: locales.Text("go.internal.app.app.text022")})
				return
			}
			sv.State = sv.State.Clone()
			if sv.State.Players[seat].Name != name {
				sv.State.Players[seat].Name = name
				sv.State.Revision++
			}
		}
		if e := a.store.Save(sv.State.ID, sv); e != nil {
			_ = l.Peer.Send(Packet{Type: "error", Error: locales.Text("go.internal.app.app.text023")})
			return
		}
		a.saved = sv
		l.Auth = true
		l.LastSeen = time.Now()
		l.Status = "connected"
		delete(a.connectionReasons, seat)
		a.logLocked(locales.Text("go.internal.app.app.text024"), seat+1)
		a.connected[seat] = true
		a.broadcastLocked()
		return
	}
	if p.Type == "command" && l.Auth && a.connected[seat] && p.Command != nil {
		p.Command.Seat = seat
		if e := a.commandLocked(*p.Command); e != nil {
			a.logLocked(locales.Text("go.internal.app.app.text025"), seat+1, e)
			v := a.viewWithAILocked(seat)
			_ = l.Peer.Send(Packet{Type: "error", Ack: p.Command.ID, Error: e.Error(), View: &v, Connected: a.connected, Paused: a.pausedLocked(), ConnectionReasons: a.connectionReasonsLocked()})
		} else {
			_ = l.Peer.Send(Packet{Type: "ack", Ack: p.Command.ID})
		}
	}
	if p.Type == "voice" && p.Voice != nil && l.Auth && a.connected[seat] {
		p.Voice.From = seat
		_ = a.routeVoiceLocked(*p.Voice)
	}
	if l.Auth {
		a.receiveHeartbeatLocked(l, p)
		a.receiveProbeLocked(l, p)
	}
}
func (a *App) receiveClient(l *Link, b []byte) {
	var p Packet
	if e := json.Unmarshal(b, &p); e != nil {
		a.mu.Lock()
		a.logLocked(locales.Text("go.internal.app.app.text026"), e)
		a.mu.Unlock()
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.peers[0] != l {
		return
	}
	if a.receiveExitLocked(0, l, p) {
		return
	}
	a.receiveHeartbeatLocked(l, p)
	if p.Ack != "" && a.saved.Pending != nil && a.saved.Pending.ID == p.Ack {
		a.saved.Pending = nil
		_ = a.store.Save(a.saved.Table, a.saved)
	}
	if p.Type == "voice" && p.Voice != nil && a.validVoiceLocked(*p.Voice) && (p.Voice.To == -1 || p.Voice.To == a.saved.Seat) && p.Voice.From != a.saved.Seat {
		a.queueVoiceLocked(*p.Voice)
	}
	a.receiveProbeLocked(l, p)
	if p.Error != "" {
		a.lastError = p.Error
		a.logLocked(locales.Text("go.internal.app.app.text027"), p.Error)
	}
	if p.View != nil {
		if p.View.ID != a.saved.Table || p.View.Seat != a.saved.Seat {
			return
		}
		sv := a.saved
		sv.View = p.View
		if e := a.store.Save(sv.Table, sv); e != nil {
			a.lastError = locales.Text("go.internal.app.app.text028") + e.Error()
			return
		}
		a.saved = sv
		a.connected = p.Connected
		a.connectionReasons = p.ConnectionReasons
		a.clientPaused = p.Paused
		l.Auth = true
		l.LastSeen = time.Now()
		l.Status = "connected"
		a.logLocked("%s", locales.Text("go.internal.app.app.text029"))
	}
}
