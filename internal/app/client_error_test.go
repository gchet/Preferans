package app

import (
	"strings"
	"testing"

	"preferans/internal/rooms"
)

func TestClientErrorsAreLoggedOncePerInterval(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	a.LogClientError("TypeError: x is undefined\n    at render (app.js:1:2)")
	a.LogClientError("TypeError: x is undefined\n    at render (app.js:1:2)")
	a.LogClientError("   ")
	a.LogClientError(strings.Repeat("я", 5000))
	a.mu.Lock()
	logs := append([]string(nil), a.logs...)
	a.mu.Unlock()
	same, long := 0, 0
	for _, l := range logs {
		if strings.Contains(l, "Ошибка интерфейса: TypeError: x is undefined") {
			same++
		}
		if strings.Contains(l, "Ошибка интерфейса: яяя") {
			long++
			if len([]rune(l)) > maxClientErrorRunes+80 {
				t.Fatal("длинное сообщение не обрезано", len([]rune(l)))
			}
		}
	}
	if same != 1 || long != 1 {
		t.Fatalf("записей: одинаковая %d, длинная %d, всего %d", same, long, len(logs))
	}
}

func TestRoomCopyNeverHasNilMembers(t *testing.T) {
	// members: null в состоянии роняет интерфейс (r.members.find) — копия комнаты всегда отдаёт пустой список.
	if c := cloneRoom(&rooms.Room{ID: "r", Name: "Комната"}); c == nil || c.Members == nil {
		t.Fatal("members не должен быть nil")
	}
}