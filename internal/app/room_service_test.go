package app

import "testing"

func TestRoomServicePersistenceAndValidation(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	target := "https://rooms.example.org/v2/rooms"
	if err := a.SetRoomService("  " + target + "  "); err != nil {
		t.Fatal(err)
	}
	if a.rooms.url != target {
		t.Fatal("URL not changed")
	}
	for _, bad := range []string{"file:///tmp/rooms", "https://user:secret@example.org/rooms", "not a URL"} {
		if err := a.SetRoomService(bad); err == nil {
			t.Fatal("invalid URL accepted")
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
		t.Fatal("server setting lost on restart")
	}
}

func TestRoomServiceCannotSwitchSelectedRoom(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	a.mu.Lock()
	a.rooms.config.Current = "existing-room"
	old := a.rooms.url
	a.mu.Unlock()
	if err := a.SetRoomService("https://other.example.org/rooms"); err == nil {
		t.Fatal("selected room allowed server switch")
	}
	if a.rooms.url != old {
		t.Fatal("failed switch changed URL")
	}
}
