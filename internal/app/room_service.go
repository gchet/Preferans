package app

import (
	"preferans/config"
	"preferans/locales"
	"strings"
)

// SetRoomService changes only an empty session. A live table retains its server.
func (a *App) SetRoomService(value string) error {
	value = strings.TrimSpace(value)
	if err := config.ValidateHTTPURL(value); err != nil {
		return err
	}
	a.roomControl.Lock()
	defer a.roomControl.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if value == a.rooms.url {
		return nil
	}
	if a.saved.Table != "" || (a.rooms.config.Current != "" && a.rooms.config.Current != localRoomID) {
		return locales.Errorf("go.connection.service_busy")
	}
	p := a.appearance
	p.RoomServiceURL = value
	if err := a.config.Save("appearance", p); err != nil {
		return err
	}
	a.appearance = p
	a.rooms.url = value
	a.rooms.generation++
	a.rooms.offer = nil
	a.rooms.err = ""
	return nil
}
