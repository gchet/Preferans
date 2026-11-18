package app

import (
	"testing"
	"time"
)

// Пока лобби не определило ведущего, идёт «сбор игроков»; после определения и при недоступных живых игроках — тоже.
func TestLobbyGathering(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rooms.config.Current = "room1"
	if a.gatheringLocked() {
		t.Fatal("лобби выключено: сбора нет")
	}
	a.lobbyEnabled, a.lobbyGatherSince = true, time.Now()
	if !a.gatheringLocked() {
		t.Fatal("лобби ещё не определилось: сбор идёт")
	}
	a.lobbyGatherSince = time.Now().Add(-2 * lobbyGatherLimit)
	if a.gatheringLocked() {
		t.Fatal("по истечении предела ожидания сбор не показывается")
	}
	a.lobbyGatherSince = time.Now()
	a.lobbyReady = true
	if a.gatheringLocked() {
		t.Fatal("лобби определилось: сбора нет")
	}
	a.lobbyUnsettled = true
	if !a.gatheringLocked() {
		t.Fatal("часть игроков недоступна: сбор идёт")
	}
	a.rooms.config.Current = localRoomID
	if a.gatheringLocked() {
		t.Fatal("в локальной комнате сбора нет")
	}
}