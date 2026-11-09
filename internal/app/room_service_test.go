package app

import (
	"testing"

	"preferans/internal/p2p"
)

func serviceCodeFor(t *testing.T, id string) string {
	t.Helper()
	code, err := p2p.ServiceCode(id)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func TestRoomServicePersistenceAndValidation(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	target := serviceCodeFor(t, "12D3KooWExampleServicePeerIdentifier")
	if err := a.SetRoomService("  " + target + "  "); err != nil {
		t.Fatal(err)
	}
	if a.rooms.url != target {
		t.Fatal("код сервиса не изменился")
	}
	table, _ := p2p.Encode(p2p.Code{Kind: p2p.KindTable, Table: "t", Seat: 1, Nonce: "n", Invite: p2p.Invite{ID: "x"}})
	for _, bad := range []string{"file:///tmp/rooms", "https://rooms.example.org/v2/rooms", "not a code", "PREF1.abc", table} {
		if err := a.SetRoomService(bad); err == nil {
			t.Fatalf("принято %q", bad)
		}
	}
	p := a.Status().Appearance
	p.RoomServiceURL = ""
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b := New(dir)
	defer b.Close()
	if b.rooms.url != target || b.Status().Appearance.RoomServiceURL != target {
		t.Fatal("сервис комнат потерян после перезапуска")
	}
	if err := b.SetRoomService(""); err != nil || b.rooms.url != "" {
		t.Fatal("возврат к собственному сервису не удался")
	}
}

func TestRoomServiceCannotSwitchSelectedRoom(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	a.mu.Lock()
	a.rooms.config.Current = "existing-room"
	old := a.rooms.url
	a.mu.Unlock()
	if err := a.SetRoomService(serviceCodeFor(t, "12D3KooWAnotherServicePeerIdentifier")); err == nil {
		t.Fatal("выбранная комната допустила смену сервиса")
	}
	if a.rooms.url != old {
		t.Fatal("неудачная смена изменила сервис")
	}
}

// Собственный сервис комнат работает без сети: комнату можно создать и увидеть в списке.
func TestOwnRoomServiceWorksOffline(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	if a.rooms.url != "" {
		t.Fatalf("по умолчанию должен использоваться собственный сервис, а не %q", a.rooms.url)
	}
	r, err := a.RoomDirectory("create", "", "Дом")
	if err != nil || r.Room == nil {
		t.Fatalf("%+v %v", r, err)
	}
	list, err := a.RoomDirectory("list", "", "")
	if err != nil || len(list.Rooms) == 0 {
		t.Fatalf("%+v %v", list, err)
	}
}
// Настройки прежних версий с адресом HTTP-сервера не должны оставлять приложение на старом сервере.
func TestLegacyRoomServerURLIsDropped(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	p := a.Status().Appearance
	p.RoomServiceURL = "https://signaling.example.org/v2/rooms"
	if err := a.config.Save("appearance", p); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b := New(dir)
	defer b.Close()
	if b.rooms.url != "" || b.Status().Appearance.RoomServiceURL != "" {
		t.Fatalf("прежний адрес сервера сохранился: %q", b.rooms.url)
	}
}