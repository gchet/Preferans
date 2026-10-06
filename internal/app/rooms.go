package app

import "preferans/locales"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"preferans/config"
	"preferans/internal/game"
	"preferans/internal/network"
	"preferans/internal/rooms"
	"runtime"
	"sort"
	"time"
)

type roomConfig struct {
	Secret        string      `json:"secret"`
	Current       string      `json:"current"`
	Table         string      `json:"table"`
	OptOut        string      `json:"optOut"`
	PendingCloses []roomClose `json:"pendingCloses,omitempty"`
}
type roomState struct {
	offer      *rooms.Signal
	config     roomConfig
	room       *rooms.Room
	url        string
	err        string
	seen       time.Time
	cursor     uint64
	epoch      string
	attempts   map[string]time.Time
	working    map[string]bool
	generation uint64
	busy       bool
	closing    bool
	nextClose  time.Time
}

func (r *roomState) identity() string { return rooms.Identity(r.config.Secret) }
func cloneRoom(r *rooms.Room) *rooms.Room {
	if r == nil {
		return nil
	}
	b, _ := json.Marshal(r)
	var c rooms.Room
	_ = json.Unmarshal(b, &c)
	return &c
}
func (a *App) inRoom() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.rooms.config.Current != "" }
func (a *App) initRooms() {
	a.rooms = roomState{url: config.Interface.RoomServiceURL, attempts: map[string]time.Time{}, working: map[string]bool{}}
	if a.appearance.RoomServiceURL != "" {
		a.rooms.url = a.appearance.RoomServiceURL
	}
	if u := os.Getenv("PREFERANS_ROOMS_URL"); u != "" {
		a.rooms.url = u
	}
	_ = a.config.Load("room-session", &a.rooms.config)
	if a.appearance.LocalOnly && a.rooms.config.Table == "" {
		a.rooms.config.Current = localRoomID
	}
	if len(a.rooms.config.Secret) < 32 {
		a.rooms.config.Secret = game.ID() + game.ID()
		if e := a.config.Save("room-session", a.rooms.config); e != nil {
			a.rooms.err = e.Error()
		}
	}
	// A selected room remains selected even when no table has been created.
	// The default is only a fallback for a session without any room selection.
	if a.rooms.config.Current == "" {
		a.rooms.config.Current = a.appearance.DefaultRoom
	}
	if a.rooms.config.Current != "" && a.rooms.config.Table != "" {
		if e := a.Resume(a.rooms.config.Table); e != nil {
			a.rooms.err = e.Error()
		}
	}
	if a.rooms.config.Current == localRoomID {
		a.updateLocalRoomLocked()
	}
	go a.roomLoop()
	go a.partyCatalogLoop()
	go a.heartbeatLoop()
}
func (a *App) roomRequest(q rooms.Request) (rooms.Response, error) {
	if q.Op == "join" || q.Op == "poll" {
		enabled := config.AIEnabled
		q.Platform, q.AIEnabled = runtime.GOOS, &enabled
	}
	a.mu.Lock()
	q.Secret = a.rooms.config.Secret
	u := a.rooms.url
	if q.Name == "" && (q.Op == "join" || q.Op == "poll") {
		q.Name = a.appearance.Name
	}
	a.mu.Unlock()
	b, e := json.Marshal(q)
	if e != nil {
		return rooms.Response{}, e
	}
	client := http.Client{Timeout: time.Duration(config.Interface.RoomRequestTimeoutMs) * time.Millisecond}
	resp, e := client.Post(u, "application/json", bytes.NewReader(b))
	if e != nil {
		return rooms.Response{}, locales.Errorf("go.internal.app.rooms.text001", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return rooms.Response{}, locales.Errorf("go.internal.app.rooms.text002", resp.StatusCode)
	}
	var out rooms.Response
	if e = json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 4*1024*1024)).Decode(&out); e != nil {
		return out, e
	}
	if out.Error != "" {
		return out, fmt.Errorf("%s", out.Error)
	}
	a.applyAdminRemovals(q.Room, out)
	return out, nil
}
func (a *App) RoomDirectory(op, id, name string) (rooms.Response, error) {
	return a.roomRequest(rooms.Request{Op: op, Room: id, Name: name})
}
func (a *App) EnterRoom(id string) error {
	a.roomControl.Lock()
	defer a.roomControl.Unlock()
	a.mu.Lock()
	old := a.rooms.config.Current
	current := a.saved.Table
	localOnly := a.appearance.LocalOnly
	a.mu.Unlock()
	if current != "" {
		return locales.Errorf("go.internal.app.rooms.text003")
	}
	if id != localRoomID && localOnly {
		return locales.Errorf("go.local.only")
	}
	if id == localRoomID {
		a.mu.Lock()
		defer a.mu.Unlock()
		previous := a.rooms.config
		a.rooms.config.Current, a.rooms.config.Table, a.rooms.config.OptOut = localRoomID, "", ""
		if err := a.config.Save("room-session", a.rooms.config); err != nil {
			a.rooms.config = previous
			return err
		}
		a.rooms.generation++
		a.rooms.offer = nil
		a.rooms.err = ""
		a.voiceQueue = nil
		a.updateLocalRoomLocked()
		return nil
	}
	if old != "" && old != id && old != localRoomID {
		// A deleted/full default room was never joined and cannot require a leave.
		_, _ = a.roomRequest(rooms.Request{Op: "leave", Room: old})
	}
	out, e := a.roomRequest(rooms.Request{Op: "join", Room: id})
	if e != nil {
		return e
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rooms.generation++
	a.rooms.offer = nil
	a.rooms.config.Current = id
	a.rooms.config.OptOut = ""
	a.rooms.room = out.Room
	a.rooms.cursor = out.Cursor
	a.rooms.epoch = out.Epoch
	a.rooms.err = ""
	a.rooms.seen = time.Now()
	a.voiceQueue = nil
	a.rooms.attempts = map[string]time.Time{}
	a.rooms.working = map[string]bool{}
	return a.config.Save("room-session", a.rooms.config)
}
func (a *App) ExitRoom() error {
	a.roomControl.Lock()
	defer a.roomControl.Unlock()
	a.mu.Lock()
	id := a.rooms.config.Current
	if a.saved.Table != "" {
		a.mu.Unlock()
		return locales.Errorf("go.internal.app.rooms.text004")
	}
	a.mu.Unlock()
	if id != "" && id != localRoomID {
		_, _ = a.roomRequest(rooms.Request{Op: "leave", Room: id})
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rooms.generation++
	a.rooms.config.Current = ""
	a.rooms.config.Table = ""
	a.rooms.room = nil
	a.rooms.err = ""
	a.voiceQueue = nil
	return a.config.Save("room-session", a.rooms.config)
}
func (a *App) roomTable(resume string, n, target int, name string, bots bool) error {
	a.roomControl.Lock()
	defer a.roomControl.Unlock()
	a.mu.Lock()
	local := a.rooms.config.Current == localRoomID
	a.mu.Unlock()
	if local {
		return a.localTable(resume, n, target, name)
	}
	a.mu.Lock()
	r := cloneRoom(a.rooms.room)
	old := a.saved
	if r == nil {
		a.mu.Unlock()
		return locales.Errorf("go.internal.app.rooms.text005")
	}
	if old.Table != "" {
		a.mu.Unlock()
		return locales.Errorf("go.internal.app.rooms.text006")
	}
	a.rooms.busy = true
	self := a.rooms.identity()
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.rooms.busy = false; a.mu.Unlock() }()
	var e error
	if resume != "" {
		var saved Saved
		if a.store.Load(resume, &saved) == nil && saved.Room == localRoomID {
			return locales.Errorf("go.local.wrong_room")
		}
		e = a.Resume(resume)
	} else {
		e = a.Create(n, target, name, false)
	}
	if e != nil {
		return e
	}
	a.mu.Lock()
	sv := a.saved
	rollback := func() { a.disconnectLocked(); a.saved = old }
	if sv.State == nil {
		rollback()
		a.mu.Unlock()
		return locales.Errorf("go.internal.app.rooms.text007")
	}
	sv.Room = r.ID
	if resume == "" {
		members := append([]rooms.Member{}, r.Members...)
		sort.Slice(members, func(i, j int) bool { return members[i].Slot < members[j].Slot })
		sv.PlayerIDs = make([]string, n)
		sv.PlayerIDs[0] = self
		seat := 1
		for _, m := range members {
			if m.ID == self || !m.Online {
				continue
			}
			if seat >= n {
				rollback()
				a.mu.Unlock()
				return locales.Errorf("go.internal.app.rooms.text008")
			}
			sv.PlayerIDs[seat] = m.ID
			sv.State.Players[seat].Name = m.Name
			seat++
		}
		for ; seat < n; seat++ {
			if !bots {
				continue // The host can fill these empty seats from the table lobby.
			}
			sv.State.Players[seat] = game.Player{Name: locales.Format("go.internal.app.rooms.text009", seat), Bot: true, Ready: true}
			a.connected[seat] = true
		}
	} else if len(sv.PlayerIDs) != len(sv.State.Players) {
		sv.PlayerIDs = make([]string, len(sv.State.Players))
		sv.PlayerIDs[0] = self
	}
	if sv.PlayerIDs[0] != self {
		rollback()
		a.mu.Unlock()
		return locales.Errorf("go.internal.app.rooms.text010")
	}
	t := &rooms.Table{ID: sv.Table, Players: append([]string{}, sv.PlayerIDs...), Bots: make([]bool, len(sv.Keys)), Keys: make([]string, len(sv.Keys))}
	for i, k := range sv.Keys {
		t.Keys[i] = rooms.Identity(k)
		t.Bots[i] = sv.State.Players[i].Bot
	}
	a.saved = sv
	if e = a.store.Save(sv.Table, sv); e != nil {
		rollback()
		a.mu.Unlock()
		return e
	}
	a.mu.Unlock()
	out, e := a.roomRequest(rooms.Request{Op: "table", Room: r.ID, Table: t})
	a.mu.Lock()
	defer a.mu.Unlock()
	if e != nil {
		rollback()
		// The server may have accepted publication before the HTTP reply was lost.
		// Keep the save so the next room poll can recover that exact table.
		return e
	}
	a.rooms.room = out.Room
	a.rooms.config.Table = sv.Table
	a.rooms.config.OptOut = ""
	a.rooms.attempts = map[string]time.Time{}
	return a.config.Save("room-session", a.rooms.config)
}
func (a *App) RoomLeaveTable() error {
	a.roomControl.Lock()
	defer a.roomControl.Unlock()
	a.mu.Lock()
	r := cloneRoom(a.rooms.room)
	table := a.saved.Table
	host := a.saved.State != nil
	finished := savedFinished(a.saved)
	closingRoom := a.saved.Room
	// Room ownership survives loss of the local game save.
	if r != nil && r.Table != nil && r.Table.Host == a.rooms.identity() {
		table = r.Table.ID
		host = true
	}
	if r != nil {
		closingRoom = r.ID
	}
	a.rooms.busy = true
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.rooms.busy = false; a.mu.Unlock() }()
	if r != nil && host && r.ID != localRoomID && !finished {
		if _, e := a.roomRequest(rooms.Request{Op: "close", Room: r.ID, TableID: table}); e != nil {
			return e
		}
	}
	a.mu.Lock()
	previous := a.rooms.config
	if finished && host && closingRoom != "" && closingRoom != localRoomID {
		a.rooms.config.PendingCloses = append(append([]roomClose(nil), a.rooms.config.PendingCloses...), roomClose{Room: closingRoom, Table: table})
	}
	a.rooms.config.Table = ""
	a.rooms.config.OptOut = table
	if a.appearance.LocalOnly {
		a.rooms.config.Current = localRoomID
	}
	if err := a.config.Save("room-session", a.rooms.config); err != nil {
		a.rooms.config = previous
		a.mu.Unlock()
		return err
	}
	a.disconnectLocked()
	a.saved = Saved{}
	a.lastError = ""
	if host && a.rooms.room != nil && a.rooms.room.Table != nil && a.rooms.room.Table.ID == table {
		a.rooms.room.Table = nil
	}
	a.rooms.generation++
	if a.rooms.config.Current == localRoomID {
		a.updateLocalRoomLocked()
	}
	a.mu.Unlock()
	if finished {
		go a.retryRoomCloses()
	}
	return nil
}
func (a *App) roomSignal(room, kind, to, table, payload string) error {
	_, e := a.roomRequest(rooms.Request{Op: "signal", Room: room, Signal: &rooms.Signal{Kind: kind, To: to, Table: table, Payload: payload}})
	return e
}
func (a *App) roomLoop() {
	timer := time.NewTicker(time.Duration(config.Interface.RoomPollIntervalMs) * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-a.closed:
			return
		case <-timer.C:
			go a.retryRoomCloses()
			a.roomTick()
		}
	}
}
func (a *App) roomTick() {
	a.mu.Lock()
	id := a.rooms.config.Current
	generation := a.rooms.generation
	q := rooms.Request{Op: "poll", Room: id, After: a.rooms.cursor, Epoch: a.rooms.epoch}
	if a.rooms.room != nil && a.rooms.room.Table != nil {
		q.TableID = a.rooms.room.Table.ID
		var s Saved
		if a.store.Load(q.TableID, &s) == nil {
			q.Key = s.Key
		}
	}
	a.mu.Unlock()
	if id == "" || id == localRoomID {
		return
	}
	out, e := a.roomRequest(q)
	a.mu.Lock()
	if generation != a.rooms.generation || id != a.rooms.config.Current {
		a.mu.Unlock()
		return
	}
	if e != nil {
		if a.rooms.err != e.Error() {
			a.logLocked(locales.Text("go.internal.app.rooms.text011"), e)
		}
		a.rooms.err = e.Error()
		a.mu.Unlock()
		return
	}
	a.rooms.err = ""
	a.rooms.seen = time.Now()
	a.rooms.room = out.Room
	// A close queued locally must not recreate its old table in the UI.
	if out.Room != nil && out.Room.Table != nil {
		for _, closing := range a.rooms.config.PendingCloses {
			if closing.Room == id && closing.Table == out.Room.Table.ID {
				out.Room = cloneRoom(out.Room)
				out.Room.Table = nil
				a.rooms.room = out.Room
				break
			}
		}
	}
	a.rooms.cursor = out.Cursor
	a.rooms.epoch = out.Epoch
	if a.rooms.busy {
		a.mu.Unlock()
		return
	}
	r := cloneRoom(out.Room)
	self := a.rooms.identity()
	if r.Table == nil && a.saved.Table != "" && a.saved.Room == r.ID {
		a.disconnectLocked()
		// Keep the final result visible even if the host has already left.
		if !savedFinished(a.saved) {
			a.saved = Saved{}
		}
		a.rooms.config.Table = ""
		_ = a.config.Save("room-session", a.rooms.config)
	}
	if r.Table != nil && r.Table.ID != a.rooms.config.OptOut {
		t := r.Table
		if t.Host == self && a.saved.Table == "" {
			var sv Saved
			if a.store.Load(t.ID, &sv) == nil && sv.State != nil {
				a.saved = sv
				a.connected = make([]bool, len(sv.State.Players))
				for i, p := range sv.State.Players {
					a.connected[i] = i == 0 || p.Bot
				}
			}
		}
		if a.saved.State != nil && a.saved.Table == t.ID {
			a.saved.PlayerIDs = append([]string{}, t.Players...)
		}
		if a.saved.Table == t.ID && a.rooms.config.Table != t.ID {
			a.rooms.config.Table = t.ID
			_ = a.config.Save("room-session", a.rooms.config)
		}
	}
	for _, m := range out.Signals {
		if m.Kind == "voice" {
			a.roomVoiceReceiveLocked(r, m)
		} else if m.Kind == "offer" && r.Table != nil && m.Table == r.Table.ID && m.From == r.Table.Host {
			// Keep the newest offer, including one arriving during ICE gathering.
			copy := m
			a.rooms.offer = &copy
		}
	}
	offer := a.rooms.offer
	a.mu.Unlock()
	if r.Table == nil {
		return
	}
	for _, m := range out.Signals {
		if m.Kind == "answer" && m.Table == r.Table.ID {
			a.handleRoomSignal(r, m, generation)
		}
	}
	if offer != nil && offer.Table == r.Table.ID {
		a.handleRoomSignal(r, *offer, generation)
	}
	if r.Table.Host == self {
		for seat, p := range r.Table.Players {
			if seat > 0 && p != "" && !r.Table.Bots[seat] {
				a.startRoomInvite(r, seat, p, generation)
			}
		}
	}
}

var connectionSilenceTimeout = time.Duration(config.Interface.ConnectionSilenceTimeoutMs) * time.Millisecond

// Signaling HTTP can wait for several seconds during a network outage.
// Keep the game channel watchdog independent of those requests.
func (a *App) heartbeatLoop() {
	tick := time.NewTicker(time.Duration(config.Interface.HeartbeatIntervalMs) * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-a.closed:
			return
		case <-tick.C:
			a.heartbeat()
		}
	}
}

func (a *App) heartbeat() {
	a.mu.Lock()
	defer a.mu.Unlock()
	changed := false
	for seat, l := range a.peers {
		if !l.Auth {
			continue
		}
		if !l.LastSeen.IsZero() && time.Since(l.LastSeen) > connectionSilenceTimeout {
			if l.Status != "disconnected" {
				a.logLocked(locales.Text("go.internal.app.rooms.text012"), seat+1)
			}
			l.Status = "disconnected"
			a.rememberConnectionLossLocked(seat, l)
			l.Auth = false
			go l.Peer.Close()
			if seat < len(a.connected) {
				a.connected[seat] = false
			}
			if a.saved.State == nil {
				a.clientPaused = true
			}
			changed = true
		} else {
			_ = l.Peer.Send(Packet{Type: "heartbeat"})
			if a.saved.State == nil && a.saved.Pending != nil && (!a.clientPaused || a.saved.Pending.Action == "pause" || a.saved.Pending.Action == "resume-play") {
				_ = l.Peer.Send(Packet{Type: "command", Command: a.saved.Pending})
			}
		}
	}
	if changed {
		a.broadcastLocked()
	}
}
func (a *App) receiveHeartbeatLocked(l *Link, p Packet) {
	if !l.Auth {
		return
	}
	l.LastSeen = time.Now()
	if p.Type == "heartbeat" {
		_ = l.Peer.Send(Packet{Type: "heartbeat-ack"})
	}
}
func (a *App) startRoomInvite(r *rooms.Room, seat int, player string, generation uint64) {
	online := false
	for _, m := range r.Members {
		if m.ID == player {
			online = m.Online
		}
	}
	if !online {
		return
	}
	a.mu.Lock()
	if a.rooms.generation != generation || a.rooms.busy || a.saved.State == nil || a.saved.Table != r.Table.ID || seat >= len(a.connected) || a.connected[seat] || a.rooms.working[player] {
		a.mu.Unlock()
		return
	}
	if !roomRetryDue(a.peers[seat], a.rooms.attempts[player], time.Now()) {
		a.mu.Unlock()
		return
	}
	if l := a.peers[seat]; l != nil {
		a.logLocked(locales.Text("go.internal.app.rooms.text013"), seat+1, time.Since(a.rooms.attempts[player]).Seconds(), l.Status, l.Accepted, l.Auth)
		delete(a.peers, seat)
		go l.Peer.Close()
	}
	a.rooms.working[player] = true
	a.rooms.attempts[player] = time.Now()
	a.mu.Unlock()
	go func() {
		code, e := a.Invite(seat)
		if e == nil {
			a.mu.Lock()
			valid := a.rooms.generation == generation && a.saved.Table == r.Table.ID
			a.mu.Unlock()
			if valid {
				e = a.roomSignal(r.ID, "offer", player, r.Table.ID, code)
			}
		}
		a.mu.Lock()
		a.rooms.working[player] = false
		if e != nil {
			a.logLocked(locales.Text("go.internal.app.rooms.text014"), seat+1, e)
		}
		a.mu.Unlock()
	}()
}

func roomRetryDue(link *Link, started, now time.Time) bool {
	delay := time.Duration(config.Interface.ReconnectRetryMs) * time.Millisecond
	if link != nil {
		if handshakeInProgress(link) {
			delay = time.Duration(config.Interface.HandshakeRetryMs) * time.Millisecond
		} else if link.Status == "waiting-answer" {
			delay = time.Duration(config.Interface.AnswerRetryMs) * time.Millisecond
		}
	}
	return now.Sub(started) >= delay
}
func (a *App) handleRoomSignal(r *rooms.Room, m rooms.Signal, generation uint64) {
	a.mu.Lock()
	self := a.rooms.identity()
	if a.rooms.generation != generation || a.rooms.busy || a.rooms.config.OptOut == m.Table {
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()
	if m.Kind == "answer" && r.Table.Host == self {
		c, e := network.Decode(m.Payload)
		if e != nil || c.Seat < 1 || c.Seat >= len(r.Table.Players) || r.Table.Players[c.Seat] != m.From {
			return
		}
		if e = a.Answer(m.Payload); e != nil {
			a.mu.Lock()
			a.logLocked(locales.Text("go.internal.app.rooms.text015"), e)
			a.mu.Unlock()
		}
		return
	}
	if m.Kind != "offer" || m.From != r.Table.Host {
		return
	}
	c, e := network.Decode(m.Payload)
	if e != nil || c.Seat < 1 || c.Seat >= len(r.Table.Players) || r.Table.Players[c.Seat] != self {
		return
	}
	a.mu.Lock()
	if a.rooms.working["join"] {
		a.mu.Unlock()
		return
	}
	if l := a.peers[0]; l != nil && l.Auth && l.Status == "connected" && a.saved.Table == c.Table && l.Nonce == c.Nonce {
		a.rooms.offer = nil
		a.mu.Unlock()
		return
	}
	a.rooms.working["join"] = true
	a.rooms.offer = nil
	a.mu.Unlock()
	go func() {
		answer, e := a.join(m.Payload, &generation)
		if e == nil {
			a.mu.Lock()
			valid := a.rooms.generation == generation
			if valid {
				a.saved.Room = r.ID
				a.rooms.config.Table = c.Table
				_ = a.store.Save(c.Table, a.saved)
				_ = a.config.Save("room-session", a.rooms.config)
			}
			a.mu.Unlock()
			if valid {
				e = a.roomSignal(r.ID, "answer", r.Table.Host, c.Table, answer)
			}
		}
		a.mu.Lock()
		a.rooms.working["join"] = false
		if e != nil {
			a.logLocked(locales.Text("go.internal.app.rooms.text016"), e)
		}
		a.mu.Unlock()
	}()
}
func (a *App) roomVoiceReceiveLocked(r *rooms.Room, m rooms.Signal) {
	var v VoiceMessage
	if json.Unmarshal([]byte(m.Payload), &v) != nil {
		return
	}
	from := -1
	for _, p := range r.Members {
		if p.ID == m.From {
			from = p.Slot
		}
	}
	if from < 0 || v.Table != r.ID {
		return
	}
	v.From = from
	a.queueVoiceLocked(v)
}
