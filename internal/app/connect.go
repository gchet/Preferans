package app

import "preferans/locales"

import (
	"context"

	"time"

	"github.com/pion/webrtc/v4"
	"preferans/internal/game"
	"preferans/internal/network"
)

func routeOrUnknown(route, channel string) string {
	if route != "" {
		return route
	}
	if channel != "" {
		return channel
	}
	return locales.Text("go.internal.app.connect.text001")
}
func localOrUnknown(value string) string {
	if value == "" {
		return locales.Text("go.internal.app.connect.text002")
	}
	return value
}

func (a *App) wireStatus(seat int, l *Link) {
	l.Peer.Status = func(status string) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.peers[seat] != l {
			return
		}
		l.Status = status
		a.logLocked(locales.Text("go.internal.app.connect.text003"), seat+1, status)
		if status == "connected" || status == "failed" {
			channel, route, local, remote := l.Peer.DiagnosticRoute()
			a.logLocked(locales.Text("go.internal.app.connect.text004"), seat+1, routeOrUnknown(route, channel), localOrUnknown(local), localOrUnknown(remote))
		}
		if status == "connected" && !l.Auth {
			l.Status = "authenticating"
			l.AuthDeadline = time.Now().Add(30 * time.Second)
			a.logLocked(locales.Text("go.internal.app.connect.text005"), seat+1)
			time.AfterFunc(30*time.Second, func() { a.expireHandshake(seat, l) })
		}
		if status == "disconnected" || status == "failed" || status == "closed" {
			a.rememberConnectionLossLocked(seat, l)
			l.Accepted = false
			if a.saved.State != nil {
				a.connected[seat] = false
				a.broadcastLocked()
			} else {
				a.clientPaused = true
				if len(a.connected) > 0 {
					a.connected[0] = false
				}
			}
			if status == "failed" {
				a.lastError = locales.Text("go.internal.app.connect.text006")
			}
		}
		if status == "connected" && a.saved.State != nil && l.Auth {
			a.connected[seat] = true
			a.broadcastLocked()
		}
	}
}

// ICE connected precedes SCTP/DataChannel and the authenticated hello exchange.
// Do not replace a connection in that interval; bound it with its own timeout.
func (a *App) expireHandshake(seat int, l *Link) {
	a.mu.Lock()
	if a.peers[seat] != l || l.Auth || l.Status != "authenticating" || time.Now().Before(l.AuthDeadline) {
		a.mu.Unlock()
		return
	}
	l.Status = "failed"
	l.Accepted = false
	a.logLocked(locales.Text("go.internal.app.connect.text007"), seat+1)
	a.mu.Unlock()
	l.Peer.Close()
}

func handshakeInProgress(l *Link) bool {
	return l != nil && (l.Status == "gathering" || l.Status == "connecting" || l.Status == "authenticating" || (l.Status == "connected" && !l.Auth))
}
func (a *App) Invite(seat int) (string, error) {
	a.mu.Lock()
	if a.saved.State == nil || seat < 1 || seat >= len(a.saved.State.Players) || a.saved.State.Players[seat].Bot {
		a.mu.Unlock()
		return "", locales.Errorf("go.internal.app.connect.text008")
	}
	if a.connected[seat] {
		a.mu.Unlock()
		return "", locales.Errorf("go.internal.app.connect.text009")
	}
	if handshakeInProgress(a.peers[seat]) {
		a.mu.Unlock()
		return "", locales.Errorf("go.internal.app.connect.text010")
	}
	stun := append([]string{}, a.stun...)
	a.logLocked(locales.Text("go.internal.app.connect.text011"), seat+1)
	table := a.saved.State.ID
	key := ""
	if !a.saved.Joined[seat] {
		key = a.saved.Keys[seat]
	}
	a.mu.Unlock()
	servers := network.SelectICEServers(stun)
	a.logICEServers(seat, servers)
	p, e := network.New(servers)
	if e != nil {
		return "", e
	}
	l := &Link{Peer: p, Nonce: game.ID(), Status: "gathering"}
	p.Message = func(b []byte) { a.receiveHost(seat, l, b) }
	p.Open = func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.peers[seat] == l {
			a.logLocked(locales.Text("go.internal.app.connect.text012"), seat+1)
		}
	}
	a.wireStatus(seat, l)
	a.mu.Lock()
	if a.saved.State == nil || a.saved.State.ID != table || a.saved.State.Players[seat].Bot || a.connected[seat] {
		a.mu.Unlock()
		p.Close()
		return "", locales.Errorf("go.internal.app.connect.text013")
	}
	if old := a.peers[seat]; old != nil {
		if handshakeInProgress(old) {
			a.mu.Unlock()
			p.Close()
			return "", locales.Errorf("go.internal.app.connect.text014")
		}
		go old.Peer.Close()
	}
	a.peers[seat] = l
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	desc, e := p.Gather(ctx, true)
	if e != nil {
		a.mu.Lock()
		a.logLocked(locales.Text("go.internal.app.connect.text015"), seat+1, e)
		a.mu.Unlock()
		p.Close()
		return "", e
	}
	code, e := network.Encode(network.Code{Version: network.Version, Table: table, Seat: seat, Nonce: l.Nonce, Key: key, Description: desc})
	a.mu.Lock()
	if a.peers[seat] == l {
		l.Status = "waiting-answer"
		a.logLocked(locales.Text("go.internal.app.connect.text016"), seat+1)
		a.logLocked(locales.Text("go.internal.app.connect.text017"), seat+1, iceCandidateSummary(desc.SDP))
	}
	a.mu.Unlock()
	return code, e
}
func (a *App) Answer(code string) error {
	c, e := network.Decode(code)
	if e != nil {
		return e
	}
	if c.Description.Type != webrtc.SDPTypeAnswer {
		return locales.Errorf("go.internal.app.connect.text018")
	}
	a.mu.Lock()
	if a.saved.State == nil || c.Table != a.saved.State.ID {
		a.mu.Unlock()
		return locales.Errorf("go.internal.app.connect.text019")
	}
	l := a.peers[c.Seat]
	if l == nil || l.Nonce != c.Nonce || l.Accepted {
		a.mu.Unlock()
		return locales.Errorf("go.internal.app.connect.text020")
	}
	l.Accepted = true
	l.Status = "connecting"
	a.logLocked(locales.Text("go.internal.app.connect.text021"), c.Seat+1)
	a.logLocked(locales.Text("go.internal.app.connect.text022"), c.Seat+1, iceCandidateSummary(c.Description.SDP))
	a.mu.Unlock()
	if e = l.Peer.PC.SetRemoteDescription(c.Description); e != nil {
		a.mu.Lock()
		l.Accepted = false
		a.mu.Unlock()
	}
	if e != nil {
		a.mu.Lock()
		a.logLocked(locales.Text("go.internal.app.connect.text023"), c.Seat+1, e)
		a.mu.Unlock()
	}
	return e
}
func (a *App) Join(code string) (string, error) {
	return a.join(code, nil)
}
func (a *App) join(code string, generation *uint64) (string, error) {
	c, e := network.Decode(code)
	if e != nil {
		return "", e
	}
	if c.Description.Type != webrtc.SDPTypeOffer {
		return "", locales.Errorf("go.internal.app.connect.text024")
	}
	a.mu.Lock()
	stun := append([]string{}, a.stun...)
	key := c.Key
	if a.saved.Table == c.Table && a.saved.Seat == c.Seat && a.saved.Key != "" {
		key = a.saved.Key
	} else {
		var old Saved
		if a.store.Load(c.Table, &old) == nil && old.Seat == c.Seat {
			key = old.Key
		}
	}
	a.mu.Unlock()
	a.mu.Lock()
	a.logLocked(locales.Text("go.internal.app.connect.text025"), c.Seat+1)
	a.logLocked(locales.Text("go.internal.app.connect.text026"), c.Seat+1, iceCandidateSummary(c.Description.SDP))
	a.mu.Unlock()
	if key == "" {
		return "", locales.Errorf("go.internal.app.connect.text027")
	}
	servers := network.SelectICEServers(stun)
	a.logICEServers(0, servers)
	p, e := network.New(servers)
	if e != nil {
		return "", e
	}
	l := &Link{Peer: p, Nonce: c.Nonce, Status: "gathering"}
	p.Message = func(b []byte) { a.receiveClient(l, b) }
	p.Open = func() {
		a.mu.Lock()
		if a.peers[0] != l {
			a.mu.Unlock()
			return
		}
		a.logLocked("%s", locales.Text("go.internal.app.connect.text028"))
		name := a.appearance.Name
		a.mu.Unlock()
		if err := p.Send(Packet{Type: "hello", Key: key, Nonce: c.Nonce, Name: name}); err != nil {
			a.mu.Lock()
			a.logLocked(locales.Text("go.internal.app.connect.text029"), err)
			a.mu.Unlock()
		}
	}
	a.wireStatus(0, l)
	a.mu.Lock()
	sv := Saved{Table: c.Table, Seat: c.Seat, Key: key}
	if generation != nil && a.rooms.generation != *generation {
		a.mu.Unlock()
		p.Close()
		return "", locales.Errorf("go.internal.app.connect.text030")
	}
	if a.saved.Table == c.Table {
		sv.View = a.saved.View
		sv.Pending = a.saved.Pending
		sv.Room = a.saved.Room
	}
	if e = a.store.Save(c.Table, sv); e != nil {
		a.mu.Unlock()
		p.Close()
		return "", e
	}
	a.disconnectLocked()
	a.saved = sv
	a.clientPaused = true
	a.lastError = ""
	a.peers[0] = l
	a.mu.Unlock()
	if e = p.PC.SetRemoteDescription(c.Description); e != nil {
		p.Close()
		return "", e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	desc, e := p.Gather(ctx, false)
	if e != nil {
		a.mu.Lock()
		a.logLocked(locales.Text("go.internal.app.connect.text031"), c.Seat+1, e)
		a.mu.Unlock()
		p.Close()
		return "", e
	}
	c.Description = desc
	c.Key = ""
	a.mu.Lock()
	a.logLocked(locales.Text("go.internal.app.connect.text032"), c.Seat+1)
	a.logLocked(locales.Text("go.internal.app.connect.text033"), c.Seat+1, iceCandidateSummary(desc.SDP))
	a.mu.Unlock()
	return network.Encode(c)
}
