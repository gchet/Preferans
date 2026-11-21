package app

import (
	"strings"
	"testing"
	"time"

	"preferans/internal/rooms"
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
// Пока игрок не выбрал «Не ждать», сбор игроков блокирует создание стола; после — нет, и это сбрасывается при смене комнаты.
func TestSkipRoomWait(t *testing.T) {
	a := New(t.TempDir())
	defer time.Sleep(400 * time.Millisecond) // фоновый опрос комнаты после SkipRoomWait успевает завершиться до удаления каталога
	defer a.Close()
	a.mu.Lock()
	a.rooms.config.Current = "room1"
	a.lobbyEnabled, a.lobbyGatherSince, a.lobbyUnsettled, a.lobbyUnsettledSince = true, time.Now().Add(-90*time.Second), true, time.Now().Add(-30*time.Second)
	kind, sec := a.roomWaitLocked(a.gatheringLocked(), true)
	a.mu.Unlock()
	if kind != "gathering" || sec < 29 || sec > 33 {
		t.Fatalf("ожидание сбора: %q %d с", kind, sec)
	}
	if _, err := a.roomRequest(rooms.Request{Op: "create", Room: "room1", Name: "x"}); err == nil {
		t.Fatal("пока идёт сбор, создание должно отклоняться")
	}
	if err := a.SkipRoomWait(); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	if a.gatheringLocked() {
		t.Fatal("после «Не ждать» сбора нет")
	}
	a.rooms.seen = time.Now().Add(-45 * time.Second)
	kind, sec = a.roomWaitLocked(false, false)
	a.mu.Unlock()
	if kind != "offline" || sec < 44 || sec > 48 {
		t.Fatalf("нет связи с сервисом: %q %d с", kind, sec)
	}
	a.setLobbyUnsettled(false)
	a.setLobbyUnsettled(true)
	a.mu.Lock()
	skipped := a.lobbySkipWait
	a.mu.Unlock()
	if skipped {
		t.Fatal("когда все игроки были доступны, пропуск ожидания сбрасывается")
	}
}

func TestSetNetworkKindLogsOnlyChanges(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	a.SetNetworkKind("wifi", true)
	a.SetNetworkKind("wifi", true)
	a.SetNetworkKind("cellular", false)
	a.SetNetworkKind("none", false)
	deadline := time.Now().Add(2 * time.Second)
	var joined string
	for time.Now().Before(deadline) {
		a.mu.Lock()
		joined = strings.Join(a.logs, "\n")
		a.mu.Unlock()
		if strings.Contains(joined, "нет сети") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Count(joined, "тип подключения устройства") != 3 {
		t.Fatalf("ожидались 3 записи о смене сети:\n%s", joined)
	}
	for _, want := range []string{"Wi-Fi", "мобильная сеть (доступ в интернет пока не подтверждён) (было: Wi-Fi)", "нет сети (было: мобильная сеть)"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("нет %q:\n%s", want, joined)
		}
	}
}