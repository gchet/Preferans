package app

import (
	"preferans/config"
	"preferans/internal/rooms"
	"preferans/locales"
	"time"
)

const localRoomID = "local-bots"

type roomClose struct {
	Room  string `json:"room"`
	Table string `json:"table"`
}

// Local rooms never enter the network directory, even when connectivity returns.
func (a *App) updateLocalRoomLocked() {
	name := a.appearance.Name
	if name == "" {
		name = locales.Text("go.internal.app.appearance.text001")
	}
	r := &rooms.Room{ID: localRoomID, Name: locales.Text("web.local.room"), Members: []rooms.Member{{ID: a.rooms.identity(), Name: name, Slot: 0, Online: true}}}
	if a.saved.Room == localRoomID && a.saved.State != nil {
		s := a.saved.State
		r.Table = &rooms.Table{ID: a.saved.Table, Host: a.rooms.identity(), Name: s.Players[0].Name}
	}
	a.rooms.room = r
}

// Called under roomControl; Create/Resume acquire mu themselves.
func (a *App) localTable(resume string, n, target int, name string) error {
	a.mu.Lock()
	if a.saved.Table != "" {
		a.mu.Unlock()
		return locales.Errorf("go.internal.app.rooms.text006")
	}
	a.mu.Unlock()
	if resume != "" {
		var saved Saved
		if err := a.store.Load(resume, &saved); err != nil {
			return err
		}
		if saved.Room != localRoomID || saved.State == nil {
			return locales.Errorf("go.local.wrong_room")
		}
		for i, p := range saved.State.Players {
			if i > 0 && !p.Bot {
				return locales.Errorf("go.local.wrong_room")
			}
		}
		if err := a.Resume(resume); err != nil {
			return err
		}
	} else if err := a.Create(n, target, name, true); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.saved.Room = localRoomID
	a.saved.PlayerIDs = make([]string, len(a.saved.State.Players))
	a.saved.PlayerIDs[0] = a.rooms.identity()
	if err := a.store.Save(a.saved.Table, a.saved); err != nil {
		a.disconnectLocked()
		a.saved = Saved{}
		return err
	}
	previous := a.rooms.config
	a.rooms.config.Table, a.rooms.config.OptOut = a.saved.Table, ""
	if err := a.config.Save("room-session", a.rooms.config); err != nil {
		a.rooms.config = previous
		a.disconnectLocked()
		a.saved = Saved{}
		return err
	}
	a.updateLocalRoomLocked()
	return nil
}

// Completed network tables are closed asynchronously from a durable queue.
// Check the exact table first: retries must never close a replacement table.
func (a *App) retryRoomCloses() {
	a.mu.Lock()
	select {
	case <-a.closed:
		a.mu.Unlock()
		return
	default:
	}
	if a.rooms.closing || len(a.rooms.config.PendingCloses) == 0 || time.Now().Before(a.rooms.nextClose) {
		a.mu.Unlock()
		return
	}
	a.rooms.closing = true
	closing := a.rooms.config.PendingCloses[0]
	self := a.rooms.identity()
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.rooms.closing = false
		a.rooms.nextClose = time.Now().Add(time.Duration(config.Interface.RoomRequestTimeoutMs) * time.Millisecond)
		a.mu.Unlock()
	}()
	out, err := a.roomRequest(rooms.Request{Op: "poll", Room: closing.Room})
	if err == nil && out.Room != nil && out.Room.Table != nil && out.Room.Table.ID == closing.Table && out.Room.Table.Host == self {
		_, err = a.roomRequest(rooms.Request{Op: "close", Room: closing.Room, TableID: closing.Table})
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	select {
	case <-a.closed:
		return
	default:
	}
	if err != nil {
		a.logLocked("%s", locales.Format("go.local.close_retry", err))
		return
	}
	previous := a.rooms.config
	remaining := []roomClose{}
	for _, q := range a.rooms.config.PendingCloses {
		if q != closing {
			remaining = append(remaining, q)
		}
	}
	a.rooms.config.PendingCloses = remaining
	if err = a.config.Save("room-session", a.rooms.config); err != nil {
		a.rooms.config = previous
		a.logLocked("%s", err)
		return
	}
	a.logLocked("%s", locales.Text("go.local.closed"))
}

// Only switches an empty session. A live game retains its room until exit.
func (a *App) localPreferenceLocked(enabled bool) error {
	if a.saved.Table != "" {
		return nil
	}
	previous := a.rooms.config
	if enabled {
		a.rooms.config.Current = localRoomID
	} else if a.rooms.config.Current == localRoomID {
		a.rooms.config.Current = ""
	} else {
		return nil
	}
	a.rooms.config.Table, a.rooms.config.OptOut = "", ""
	if err := a.config.Save("room-session", a.rooms.config); err != nil {
		a.rooms.config = previous
		return err
	}
	a.rooms.generation++
	a.rooms.offer = nil
	a.rooms.err = ""
	a.voiceQueue = nil
	if enabled {
		a.updateLocalRoomLocked()
	} else {
		a.rooms.room = nil
	}
	return nil
}
