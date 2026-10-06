package rooms

import "testing"

func TestAdminDeletionsPersistAndCannotAutoReappear(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	r := request(t, s, Request{Op: "create", Name: "Admin"}).Room
	for i := 0; i < 2; i++ {
		request(t, s, Request{Op: "join", Room: r.ID, Secret: secret(i)})
	}
	p := Party{ID: "game", Room: r.ID, Host: Identity(secret(0)), Revision: 1, Stage: "play", Players: []PartyPlayer{{ID: Identity(secret(0)), Name: "Same"}, {ID: Identity(secret(1)), Name: "Same"}, {Bot: true, Name: "Bot"}}}
	request(t, s, Request{Op: "list", Parties: []Party{p}})
	request(t, s, Request{Op: "table", Room: r.ID, Table: &Table{ID: p.ID, Players: []string{Identity(secret(0)), Identity(secret(1)), ""}, Bots: []bool{false, false, true}, Keys: []string{"a", "b", ""}}})
	request(t, s, Request{Op: "admin-delete-player", Room: r.ID, ElementID: Identity(secret(1)), Secret: secret(9)})
	if !request(t, s, Request{Op: "poll", Room: r.ID, Secret: secret(1)}).Removed {
		t.Fatal("kicked player rejoined via poll")
	}
	listed := request(t, s, Request{Op: "list"})
	if len(listed.Rooms[0].Members) != 1 || listed.Rooms[0].Table == nil {
		t.Fatal("player deletion removed wrong elements")
	}
	request(t, s, Request{Op: "join", Room: r.ID, Secret: secret(1)})
	if request(t, s, Request{Op: "poll", Room: r.ID, Secret: secret(1)}).Removed {
		t.Fatal("explicit rejoin blocked")
	}
	request(t, s, Request{Op: "admin-delete-party", Room: r.ID, ElementID: p.ID, Secret: secret(9)})
	p.Revision = 99
	listed = request(t, s, Request{Op: "list", Parties: []Party{p}})
	if len(listed.Rooms[0].Parties) != 0 || listed.Rooms[0].Table != nil || len(listed.DeletedParties) != 1 {
		t.Fatal("deleted party resurrected")
	}
	if _, err := s.Handle(Request{Op: "table", Secret: secret(0), Room: r.ID, Table: &Table{ID: p.ID, Players: []string{Identity(secret(0)), Identity(secret(1)), ""}, Bots: []bool{false, false, true}, Keys: []string{"a", "b", ""}}}); err == nil {
		t.Fatal("deleted table restored from stale save")
	}
	s, _ = New(dir)
	listed = request(t, s, Request{Op: "list", Parties: []Party{p}})
	if len(listed.DeletedParties) != 1 || len(listed.Rooms[0].Parties) != 0 {
		t.Fatal("deletion lost on restart")
	}
	request(t, s, Request{Op: "admin-delete-room", Room: r.ID, Secret: secret(9)})
	if !request(t, s, Request{Op: "poll", Room: r.ID}).Removed {
		t.Fatal("deleted room retained player")
	}
	s, _ = New(dir)
	listed = request(t, s, Request{Op: "list", Parties: []Party{p}})
	if len(listed.Rooms) != 0 || len(listed.RemovedRooms) != 1 {
		t.Fatal("deleted room reappeared")
	}
	request(t, s, Request{Op: "create", Name: "Admin"})
}
