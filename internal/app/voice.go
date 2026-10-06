package app

import "preferans/locales"

import (
	"encoding/json"
)

// VoiceMessage carries negotiation only. Audio flows over encrypted browser
// WebRTC connections and is never stored in game saves or diagnostic logs.
type VoiceMessage struct {
	Table   string `json:"table"`
	From    int    `json:"from"`
	To      int    `json:"to"`
	Session string `json:"session"`
	Target  string `json:"target,omitempty"`
	Call    string `json:"call,omitempty"`
	Kind    string `json:"kind"`
	Payload string `json:"payload,omitempty"`
	Mic     bool   `json:"mic"`
}
type VoiceEvent struct {
	Seq     uint64       `json:"seq"`
	Message VoiceMessage `json:"message"`
}

func (a *App) voiceRoomLocked() (string, int) {
	if r := a.rooms.room; r != nil {
		for _, m := range r.Members {
			if m.ID == a.rooms.identity() {
				return r.ID, m.Slot
			}
		}
	}
	if a.saved.State != nil {
		return a.saved.State.ID, 0
	}
	return a.saved.Table, a.saved.Seat
}
func (a *App) voiceHumanLocked(seat int) bool {
	if seat < 0 {
		return false
	}
	if s := a.saved.State; s != nil {
		return seat < len(s.Players) && !s.Players[seat].Bot
	}
	if v := a.saved.View; v != nil {
		return seat < len(v.Players) && !v.Players[seat].Bot
	}
	return false
}
func (a *App) validVoiceLocked(m VoiceMessage) bool {
	id, _ := a.voiceRoomLocked()
	if id == "" || m.Table != id || !a.voiceHumanLocked(m.From) || len(m.Session) < 8 || len(m.Session) > 64 || len(m.Target) > 64 || len(m.Call) > 64 || len(m.Payload) > 65536 {
		return false
	}
	switch m.Kind {
	case "hello", "bye":
		return m.To == -1 && m.Payload == ""
	case "offer", "answer", "ice":
		return m.To != m.From && a.voiceHumanLocked(m.To) && len(m.Target) >= 8 && len(m.Call) >= 8
	}
	return false
}
func (a *App) queueVoiceLocked(m VoiceMessage) {
	a.voiceSeq++
	a.voiceQueue = append(a.voiceQueue, VoiceEvent{a.voiceSeq, m})
	if len(a.voiceQueue) > 128 {
		a.voiceQueue = append([]VoiceEvent(nil), a.voiceQueue[len(a.voiceQueue)-128:]...)
	}
}
func (a *App) routeVoiceLocked(m VoiceMessage) error {
	if !a.validVoiceLocked(m) {
		return locales.Errorf("go.internal.app.voice.text001")
	}
	if m.From != 0 && (m.To == 0 || m.To == -1) {
		a.queueVoiceLocked(m)
	}
	for seat, link := range a.peers {
		if seat == m.From || (m.To != -1 && m.To != seat) || !a.voiceHumanLocked(seat) || !link.Auth || seat >= len(a.connected) || !a.connected[seat] {
			continue
		}
		if err := link.Peer.Send(Packet{Type: "voice", Voice: &m}); err != nil {
			return err
		}
	}
	return nil
}
func (a *App) SendVoice(m VoiceMessage) error {
	a.mu.Lock()
	if a.rooms.config.Current == localRoomID {
		a.mu.Unlock()
		return nil
	}
	if r := a.rooms.room; r != nil {
		id, slot := a.voiceRoomLocked()
		m.Table = id
		m.From = slot
		to := ""
		if m.To >= 0 {
			for _, p := range r.Members {
				if p.Slot == m.To {
					to = p.ID
				}
			}
			if to == "" {
				a.mu.Unlock()
				return locales.Errorf("go.internal.app.voice.text002")
			}
		}
		if len(m.Payload) > 65536 || len(m.Session) > 64 {
			a.mu.Unlock()
			return locales.Errorf("go.internal.app.voice.text003")
		}
		a.mu.Unlock()
		b, e := json.Marshal(m)
		if e != nil {
			return e
		}
		return a.roomSignal(id, "voice", to, id, string(b))
	}
	defer a.mu.Unlock()
	_, seat := a.voiceRoomLocked()
	m.From = seat // Never trust the sender identity supplied by the UI.
	if !a.validVoiceLocked(m) {
		return locales.Errorf("go.internal.app.voice.text004")
	}
	if a.saved.State != nil {
		return a.routeVoiceLocked(m)
	}
	if l := a.peers[0]; l != nil && l.Status == "connected" {
		return l.Peer.Send(Packet{Type: "voice", Voice: &m})
	}
	return locales.Errorf("go.internal.app.voice.text005")
}
func (a *App) PollVoice(table string, after uint64) ([]VoiceEvent, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	id, _ := a.voiceRoomLocked()
	if table == "" || table != id {
		return nil, locales.Errorf("go.internal.app.voice.text006")
	}
	events := []VoiceEvent{}
	for _, event := range a.voiceQueue {
		if event.Seq > after {
			events = append(events, event)
		}
	}
	return events, nil
}
