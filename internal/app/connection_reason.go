package app

import (
	"preferans/config"
	"time"
)

// Reasons describe observed events, never guesses about a remote network.
func (a *App) setConnectionReasonLocked(seat int, reason string) {
	if a.connectionReasons == nil {
		a.connectionReasons = map[int]string{}
	}
	a.connectionReasons[seat] = reason
}

func (a *App) rememberConnectionLossLocked(seat int, l *Link) {
	if !l.LastSeen.IsZero() && a.connectionReasons[seat] != "exited" {
		a.setConnectionReasonLocked(seat, "lost")
	}
}

func (a *App) connectionReasonsLocked() map[int]string {
	out := map[int]string{}
	for seat, connected := range a.connected {
		if connected {
			continue
		}
		if a.saved.State == nil && seat != 0 {
			if len(a.connected) == 0 || !a.connected[0] {
				out[seat] = "host-unreachable"
				continue
			}
			out[seat] = a.connectionReasons[seat]
			if out[seat] == "" {
				out[seat] = "waiting"
			}
			continue
		}
		if reason := a.connectionReasons[seat]; reason != "" {
			out[seat] = reason
			continue
		}
		out[seat] = "waiting"
		if l := a.peers[seat]; l != nil {
			switch l.Status {
			case "gathering", "waiting-answer":
				out[seat] = "signaling"
			case "connecting":
				out[seat] = "connecting"
			case "authenticating":
				out[seat] = "authenticating"
			case "disconnected", "failed", "closed":
				out[seat] = "unreachable"
				if !l.LastSeen.IsZero() {
					out[seat] = "lost"
				}
			}
		}
	}
	return out
}

// A bounded acknowledgement gives a graceful shutdown notice time to arrive.
// If it cannot arrive, the remote side reports missing connectivity instead.
func (a *App) notifyExitLocked() []<-chan struct{} {
	var acks []<-chan struct{}
	for _, l := range a.peers {
		if !l.Auth {
			continue
		}
		l.ExitAck = make(chan struct{})
		if l.Peer.Send(Packet{Type: "app-exit"}) == nil {
			acks = append(acks, l.ExitAck)
		}
	}
	return acks
}

func waitExitAcks(acks []<-chan struct{}) {
	timer := time.NewTimer(time.Duration(config.Interface.ExitNoticeTimeoutMs) * time.Millisecond)
	defer timer.Stop()
	for _, ack := range acks {
		select {
		case <-ack:
		case <-timer.C:
			return
		}
	}
}

func (a *App) receiveExitLocked(seat int, l *Link, p Packet) bool {
	if !l.Auth {
		return false
	}
	if p.Type == "app-exit-ack" {
		if l.ExitAck != nil {
			close(l.ExitAck)
			l.ExitAck = nil
		}
		return true
	}
	if p.Type != "app-exit" {
		return false
	}
	_ = l.Peer.Send(Packet{Type: "app-exit-ack"})
	a.setConnectionReasonLocked(seat, "exited")
	l.Auth = false
	l.Status = "disconnected"
	if seat < len(a.connected) {
		a.connected[seat] = false
	}
	if a.saved.State != nil {
		a.broadcastLocked()
	} else {
		a.clientPaused = true
	}
	return true
}
