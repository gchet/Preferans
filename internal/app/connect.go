package app

import "preferans/locales"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"preferans/internal/game"
	"preferans/internal/p2p"
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

var errNoPeer = errors.New("нет соединения")

// peer возвращает соединение места (nil, пока гость не подключился по приглашению).
func (l *Link) peer() *p2p.Conn {
	l.pmu.Lock()
	defer l.pmu.Unlock()
	return l.Peer
}
func (l *Link) setPeer(c *p2p.Conn) {
	l.pmu.Lock()
	l.Peer = c
	l.pmu.Unlock()
}
func (l *Link) send(v any) error {
	if c := l.peer(); c != nil {
		return c.Send(v)
	}
	return errNoPeer
}
func (l *Link) close() {
	if c := l.peer(); c != nil {
		c.Close()
	}
}
func (l *Link) route() (channel, route, local, remote string) {
	if c := l.peer(); c != nil {
		return c.DiagnosticRoute()
	}
	return "", "", "", ""
}

// statusHandler обрабатывает состояния соединения места: connected/disconnected/closed.
func (a *App) statusHandler(seat int, l *Link) func(string) {
	return func(status string) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.peers[seat] != l {
			return
		}
		l.Status = status
		a.logLocked(locales.Text("go.internal.app.connect.text003"), seat+1, status)
		if status == "connected" || status == "failed" {
			channel, route, local, remote := l.route()
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

// Связь установлена раньше, чем придёт подтверждение участника (hello).
// Не заменяем соединение в этом интервале; ограничиваем его своим таймаутом.
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
	l.close()
}

func handshakeInProgress(l *Link) bool {
	return l != nil && (l.Status == "gathering" || l.Status == "connecting" || l.Status == "authenticating" || (l.Status == "connected" && !l.Auth))
}

// addrSummary — сводка адресов приглашения для журнала. Публичные и relay-адреса не пишутся; адреса локальной сети
// (частные IP, не секрет) перечисляются без портов, чтобы было видно, нашёл ли узел свой адрес в сети.
func addrSummary(inv p2p.Invite) string {
	relay, lan, public := 0, 0, 0
	var lanIPs []string
	for _, s := range inv.Addrs {
		switch {
		case strings.Contains(s, "/p2p-circuit"):
			relay++
		case strings.HasPrefix(s, "/ip4/10.") || strings.HasPrefix(s, "/ip4/192.168.") || isPrivate172(s) || isCGNAT(s) || strings.HasPrefix(s, "/ip4/127.") || strings.HasPrefix(s, "/ip6/fd") || strings.HasPrefix(s, "/ip6/fc"):
			lan++
			if parts := strings.Split(s, "/"); len(parts) > 2 {
				ip, dup := parts[2], false
				for _, x := range lanIPs {
					dup = dup || x == ip
				}
				if !dup {
					lanIPs = append(lanIPs, ip)
				}
			}
		default:
			public++
		}
	}
	if len(lanIPs) > 0 {
		return fmt.Sprintf("relay=%d lan=%d (%s) public=%d", relay, lan, strings.Join(lanIPs, ", "), public)
	}
	return fmt.Sprintf("relay=%d lan=%d public=%d", relay, lan, public)
}

func isCGNAT(s string) bool {
	var second int
	if _, err := fmt.Sscanf(s, "/ip4/100.%d.", &second); err != nil {
		return false
	}
	return second >= 64 && second <= 127
}

func isPrivate172(s string) bool {
	if !strings.HasPrefix(s, "/ip4/172.") {
		return false
	}
	var second int
	_, _ = fmt.Sscanf(s, "/ip4/172.%d.", &second)
	return second >= 16 && second <= 31
}
// Invite готовит код приглашения для места seat (одно сообщение; ответный код не нужен).
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
	a.logLocked(locales.Text("go.internal.app.connect.text011"), seat+1)
	table := a.saved.State.ID
	key := ""
	if !a.saved.Joined[seat] {
		key = a.saved.Keys[seat]
	}
	a.mu.Unlock()
	node, e := a.p2pNode()
	if e != nil {
		return "", e
	}
	l := &Link{Nonce: game.ID(), Status: "gathering"}
	a.mu.Lock()
	if a.saved.State == nil || a.saved.State.ID != table || a.saved.State.Players[seat].Bot || a.connected[seat] {
		a.mu.Unlock()
		return "", locales.Errorf("go.internal.app.connect.text013")
	}
	if old := a.peers[seat]; old != nil {
		if handshakeInProgress(old) {
			a.mu.Unlock()
			return "", locales.Errorf("go.internal.app.connect.text014")
		}
		go old.close()
	}
	a.peers[seat] = l
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	inv, e := node.Invite(ctx, 25*time.Second)
	if e != nil {
		a.mu.Lock()
		a.logLocked(locales.Text("go.internal.app.connect.text015"), seat+1, e)
		if a.peers[seat] == l {
			delete(a.peers, seat)
		}
		a.mu.Unlock()
		return "", e
	}
	code, e := p2p.Encode(p2p.Code{Kind: p2p.KindTable, Table: table, Seat: seat, Nonce: l.Nonce, Key: key, Invite: inv})
	a.mu.Lock()
	if a.peers[seat] == l {
		l.Status = "waiting-guest"
		a.logLocked(locales.Text("go.internal.app.connect.text016"), seat+1)
		a.logLocked(locales.Text("go.internal.app.connect.text017"), seat+1, addrSummary(inv))
	}
	a.mu.Unlock()
	return code, e
}

// inbound — входящее соединение, ещё не привязанное к месту: первым должно прийти hello с Nonce приглашения.
type inbound struct {
	a    *App
	c    *p2p.Conn
	mu   sync.Mutex
	l    *Link
	seat int
	h    func(string)
}

func (a *App) acceptConn(c *p2p.Conn) {
	in := &inbound{a: a, c: c}
	c.Message = in.message
	c.Status = in.status
	c.Start()
}

func (in *inbound) status(s string) {
	in.mu.Lock()
	h := in.h
	in.mu.Unlock()
	if h != nil {
		h(s)
	}
}

func (in *inbound) message(b []byte) {
	in.mu.Lock()
	l, seat := in.l, in.seat
	in.mu.Unlock()
	if l != nil {
		in.a.receiveHost(seat, l, b)
		return
	}
	var p Packet
	if json.Unmarshal(b, &p) != nil || p.Type != "hello" || p.Nonce == "" {
		in.c.Close()
		return
	}
	a := in.a
	a.mu.Lock()
	var found *Link
	foundSeat := -1
	for s, link := range a.peers {
		if link.Nonce == p.Nonce && link.peer() == nil && a.saved.State != nil {
			found, foundSeat = link, s
		}
	}
	if found == nil {
		a.mu.Unlock()
		in.c.Close()
		return
	}
	found.setPeer(in.c)
	h := a.statusHandler(foundSeat, found)
	a.mu.Unlock()
	in.mu.Lock()
	in.l, in.seat, in.h = found, foundSeat, h
	in.mu.Unlock()
	h("connected")
	a.receiveHost(foundSeat, found, b)
}

// Join подключается к хосту по коду приглашения.
func (a *App) Join(code string) error {
	return a.join(code, nil)
}
func (a *App) join(code string, generation *uint64) error {
	c, e := p2p.Decode(code)
	if e != nil {
		return e
	}
	if c.Kind != p2p.KindTable {
		return locales.Errorf("go.internal.app.connect.text024")
	}
	a.mu.Lock()
	key := c.Key
	if a.saved.Table == c.Table && a.saved.Seat == c.Seat && a.saved.Key != "" {
		key = a.saved.Key
	} else {
		var old Saved
		if a.store.Load(c.Table, &old) == nil && old.Seat == c.Seat {
			key = old.Key
		}
	}
	a.logLocked(locales.Text("go.internal.app.connect.text025"), c.Seat+1)
	a.logLocked(locales.Text("go.internal.app.connect.text026"), c.Seat+1, addrSummary(c.Invite))
	a.mu.Unlock()
	if key == "" {
		return locales.Errorf("go.internal.app.connect.text027")
	}
	node, e := a.p2pNode()
	if e != nil {
		return e
	}
	l := &Link{Nonce: c.Nonce, Status: "gathering"}
	a.mu.Lock()
	sv := Saved{Table: c.Table, Seat: c.Seat, Key: key}
	if generation != nil && a.rooms.generation != *generation {
		a.mu.Unlock()
		return locales.Errorf("go.internal.app.connect.text030")
	}
	if a.saved.Table == c.Table {
		sv.View = a.saved.View
		sv.Pending = a.saved.Pending
		sv.Room = a.saved.Room
	}
	if e = a.store.Save(c.Table, sv); e != nil {
		a.mu.Unlock()
		return e
	}
	a.disconnectLocked()
	a.saved = sv
	a.clientPaused = true
	a.lastError = ""
	a.peers[0] = l
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	conn, e := node.Dial(ctx, c.Invite)
	if e != nil {
		a.mu.Lock()
		if a.peers[0] == l {
			l.Status = "failed"
		}
		a.logLocked(locales.Text("go.internal.app.connect.text031"), c.Seat+1, e)
		a.mu.Unlock()
		return e
	}
	conn.Message = func(b []byte) { a.receiveClient(l, b) }
	conn.Open = func() {
		a.mu.Lock()
		if a.peers[0] != l {
			a.mu.Unlock()
			return
		}
		a.logLocked("%s", locales.Text("go.internal.app.connect.text028"))
		name := a.appearance.Name
		a.mu.Unlock()
		if err := conn.Send(Packet{Type: "hello", Key: key, Nonce: c.Nonce, Name: name}); err != nil {
			a.mu.Lock()
			a.logLocked(locales.Text("go.internal.app.connect.text029"), err)
			a.mu.Unlock()
		}
	}
	conn.Status = a.statusHandler(0, l)
	l.setPeer(conn)
	a.mu.Lock()
	stale := a.peers[0] != l
	a.mu.Unlock()
	if stale {
		conn.Close()
		return locales.Errorf("go.internal.app.connect.text030")
	}
	conn.Start()
	return nil
}