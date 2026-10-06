package app

import "preferans/locales"

import (
	"preferans/internal/game"
	"strings"
	"time"
)

type probeState struct {
	Pending                                        map[string]time.Time
	Sent, Received, Acknowledged, Timeouts, Errors int
	LastReply, LastReceived                        time.Time
	RTT                                            float64
	LastError                                      string
}
type ProbeReport struct {
	Seat                                           int `json:"seat"`
	State, Channel, Route, LocalType, RemoteType   string
	Sent, Received, Acknowledged, Timeouts, Errors int
	RTT                                            float64
	ReplyAge, ReceivedAge                          float64
	Healthy                                        bool
	LastError                                      string
}

// EnableDiagnostics uses the existing authenticated game links, not a second transport.
func (a *App) EnableDiagnostics() {
	a.mu.Lock()
	if a.diagnostic {
		a.mu.Unlock()
		return
	}
	a.diagnostic = true
	a.mu.Unlock()
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-a.closed:
				return
			case now := <-tick.C:
				a.mu.Lock()
				for seat, l := range a.peers {
					q := &l.Probe
					for id, sent := range q.Pending {
						if now.Sub(sent) > 5*time.Second {
							delete(q.Pending, id)
							q.Timeouts++
							a.logLocked(locales.Text("go.internal.app.diagnostics.text001"), seat+1)
						}
					}
					if l.Status != "connected" || (a.saved.State != nil && !l.Auth) {
						continue
					}
					if q.Pending == nil {
						q.Pending = map[string]time.Time{}
					}
					id := game.ID()
					// 32 KiB exercises the same fragmentation used for larger game states.
					if err := l.Peer.Send(Packet{Type: "probe", ProbeID: id, Payload: strings.Repeat("p", 32768)}); err != nil {
						q.Errors++
						q.LastError = err.Error()
						a.logLocked(locales.Text("go.internal.app.diagnostics.text002"), seat+1, err)
					} else {
						q.Pending[id] = now
						q.Sent++
					}
				}
				a.mu.Unlock()
			}
		}
	}()
}
func (a *App) receiveProbeLocked(l *Link, p Packet) {
	if !a.diagnostic {
		return
	}
	q := &l.Probe
	if p.Type == "probe" && len(p.ProbeID) == 32 && p.Payload == strings.Repeat("p", 32768) {
		q.Received++
		q.LastReceived = time.Now()
		a.logLocked("%s", locales.Text("go.internal.app.diagnostics.text003"))
		if err := l.Peer.Send(Packet{Type: "probe-ack", ProbeID: p.ProbeID}); err != nil {
			q.Errors++
			q.LastError = err.Error()
		}
	}
	if p.Type == "probe-ack" {
		if sent, ok := q.Pending[p.ProbeID]; ok {
			delete(q.Pending, p.ProbeID)
			q.Acknowledged++
			q.LastReply = time.Now()
			q.RTT = float64(time.Since(sent).Microseconds()) / 1000
			a.logLocked(locales.Text("go.internal.app.diagnostics.text004"), q.RTT)
		}
	}
}
func age(t time.Time) float64 {
	if t.IsZero() {
		return -1
	}
	return time.Since(t).Seconds()
}
func (a *App) Diagnostics() []ProbeReport {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []ProbeReport{}
	for seat, l := range a.peers {
		q := l.Probe
		channel, route, local, remote := l.Peer.DiagnosticRoute()
		reply, received := age(q.LastReply), age(q.LastReceived)
		out = append(out, ProbeReport{Seat: seat, State: l.Status, Channel: channel, Route: route, LocalType: local, RemoteType: remote,
			Sent: q.Sent, Received: q.Received, Acknowledged: q.Acknowledged, Timeouts: q.Timeouts, Errors: q.Errors, RTT: q.RTT,
			ReplyAge: reply, ReceivedAge: received, Healthy: l.Status == "connected" && reply >= 0 && reply < 5 && received >= 0 && received < 5, LastError: q.LastError})
	}
	return out
}
