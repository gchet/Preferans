package app

import (
	"preferans/internal/p2p"
	"preferans/locales"
	"strings"
)

// SetRoomService выбирает сервис комнат: пустая строка — собственный сервис этого устройства,
// иначе код сервиса другого игрока (PREF2...). Меняется только в пустой сессии: живой стол сохраняет свой сервис.
func (a *App) SetRoomService(value string) error {
	value = strings.TrimSpace(value)
	if value != "" {
		c, err := p2p.Decode(value)
		if err != nil {
			return err
		}
		if c.Kind != p2p.KindService {
			return locales.Errorf("go.connection.service_code_required")
		}
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
