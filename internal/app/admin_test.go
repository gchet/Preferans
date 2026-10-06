package app

import (
	"net/http/httptest"
	"preferans/internal/game"
	"preferans/internal/rooms"
	"testing"
)

func TestAdminPublishesSavedPartiesForAllRooms(t *testing.T) {
	directory, _ := rooms.New("")
	server := httptest.NewServer(directory)
	defer server.Close()
	a := New(t.TempDir())
	defer a.Close()
	a.mu.Lock()
	a.rooms.url = server.URL
	secret := a.rooms.config.Secret
	a.mu.Unlock()
	for _, name := range []string{"North", "South"} {
		out, err := directory.Handle(rooms.Request{Op: "create", Secret: secret, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		s, _ := game.New(3, 30)
		s.Stage = "finished"
		s.StartedAt = 100
		s.FinishedAt = 200
		if err = a.store.Save(s.ID, Saved{Table: s.ID, Room: out.Room.ID, State: s, PlayerIDs: []string{rooms.Identity(secret), "", ""}}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := a.AdminRooms()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rooms) != 2 {
		t.Fatal("missing rooms")
	}
	for _, room := range out.Rooms {
		if len(room.Parties) != 0 {
			t.Fatal("public client sees admin catalog")
		}
	}
	out, err = directory.HandleAdmin(rooms.Request{Op: "list", Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	for _, room := range out.Rooms {
		if len(room.Parties) != 1 || room.Parties[0].StartedAt != 100 || room.Parties[0].FinishedAt != 200 {
			t.Fatal("missing metadata", room)
		}
	}
}

func TestAdminRemovalDeletesLocalSaveAndDisconnectsCurrentTable(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	s, _ := game.New(3, 30)
	saved := Saved{Table: s.ID, Room: "room", State: s}
	if err := a.store.Save(s.ID, saved); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.saved = saved
	a.rooms.config.Current = "room"
	a.rooms.config.Table = s.ID
	a.mu.Unlock()
	a.applyAdminRemovals("", rooms.Response{DeletedParties: []rooms.PartyRef{{Room: "room", ID: s.ID}}})
	var restored Saved
	if a.store.Load(s.ID, &restored) == nil {
		t.Fatal("deleted save still exists")
	}
	a.mu.Lock()
	if a.saved.Table != "" || a.rooms.config.Table != "" || a.rooms.config.Current != "room" {
		t.Fatal("party removal changed wrong session state")
	}
	a.mu.Unlock()
	a.applyAdminRemovals("room", rooms.Response{Removed: true})
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rooms.config.Current != "" {
		t.Fatal("kicked player kept polling room")
	}
}
