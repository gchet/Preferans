package app

import (
	"preferans/config"
	"preferans/internal/rooms"
	"preferans/locales"
	"time"
)

func (a *App) applyAdminRemovals(requestRoom string, out rooms.Response) {
	if !out.Removed && len(out.RemovedRooms) == 0 && len(out.DeletedParties) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	removedRooms := map[string]bool{}
	deleted := map[string]map[string]bool{}
	for _, id := range out.RemovedRooms {
		removedRooms[id] = true
	}
	for _, p := range out.DeletedParties {
		if deleted[p.Room] == nil {
			deleted[p.Room] = map[string]bool{}
		}
		deleted[p.Room][p.ID] = true
	}
	ids, err := a.store.IDs()
	if err != nil {
		a.lastError = err.Error()
		return
	}
	for _, id := range ids {
		var saved Saved
		if a.store.Load(id, &saved) != nil || saved.Room == "" || saved.Room == localRoomID {
			continue
		}
		if removedRooms[saved.Room] || deleted[saved.Room][id] {
			if err := a.store.Delete(id); err != nil {
				a.lastError = err.Error()
			}
		}
	}
	current := a.rooms.config.Current
	leaveRoom := removedRooms[current] || out.Removed && requestRoom == current
	leaveParty := removedRooms[a.saved.Room] || deleted[a.saved.Room][a.saved.Table]
	if leaveRoom || leaveParty {
		a.disconnectLocked()
		a.saved = Saved{}
		a.rooms.offer = nil
		a.rooms.generation++
		a.rooms.config.Table = ""
		a.rooms.config.OptOut = ""
		if a.rooms.room != nil {
			a.rooms.room.Table = nil
		}
		a.rooms.err = locales.Text("go.admin.removed")
		if leaveRoom {
			a.rooms.config.Current = ""
			a.rooms.room = nil
		}
		if err := a.config.Save("room-session", a.rooms.config); err != nil {
			a.lastError = err.Error()
		}
	}
	if removedRooms[a.appearance.DefaultRoom] {
		a.appearance.DefaultRoom = ""
		if err := a.config.Save("appearance", a.appearance); err != nil {
			a.lastError = err.Error()
		}
	}
}

func (a *App) partyCatalog() ([]rooms.Party, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ids, err := a.store.IDs()
	if err != nil {
		return nil, err
	}
	parties := []rooms.Party{}
	self := a.rooms.identity()
	for _, id := range ids {
		var saved Saved
		if a.store.Load(id, &saved) != nil || saved.Room == "" || saved.Room == localRoomID {
			continue
		}
		v := savedResult(saved)
		if v == nil {
			continue
		}
		party := rooms.Party{ID: id, Room: saved.Room, Stage: v.Stage, Round: v.Round, Revision: v.Revision, StartedAt: v.StartedAt, FinishedAt: v.FinishedAt}
		for seat, p := range v.Players {
			playerID := ""
			if seat < len(saved.PlayerIDs) {
				playerID = saved.PlayerIDs[seat]
			}
			if !p.Bot && seat == saved.Seat && playerID == "" {
				playerID = self
			}
			party.Players = append(party.Players, rooms.PartyPlayer{ID: playerID, Name: p.Name, Bot: p.Bot})
			if seat == 0 {
				party.Host = playerID
			}
		}
		parties = append(parties, party)
		if len(parties) == 512 {
			break
		}
	}
	return parties, nil
}

func (a *App) AdminRooms() (rooms.Response, error) {
	parties, err := a.partyCatalog()
	if err != nil {
		return rooms.Response{}, err
	}
	return a.roomRequest(rooms.Request{Op: "list", Parties: parties})
}

func (a *App) partyCatalogLoop() {
	ticker := time.NewTicker(time.Duration(config.Interface.PartyCatalogIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-a.closed:
			return
		case <-ticker.C:
			a.mu.Lock()
			local := a.rooms.config.Current == localRoomID
			a.mu.Unlock()
			if local {
				continue
			}
			// Separate from reconnect polling; catalog latency never delays a handshake.
			_, _ = a.AdminRooms()
		}
	}
}
