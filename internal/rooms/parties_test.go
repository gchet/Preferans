package rooms

import "testing"

func TestPartyCatalogAcrossDevicesAndRestart(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	r := request(t, s, Request{Op: "create", Name: "North"}).Room
	party := Party{ID: "party-1", Room: r.ID, Host: Identity(secret(0)), Stage: "play", Revision: 2, Round: 1, Players: []PartyPlayer{
		{ID: Identity(secret(0)), Name: "Host"}, {ID: Identity(secret(1)), Name: "Guest"}, {Name: "Bot", Bot: true},
	}}
	out := request(t, s, Request{Op: "list", Parties: []Party{party}})
	if !out.AdminCatalog || len(out.Rooms[0].Parties) != 1 {
		t.Fatal("catalog not published")
	}
	party.Revision = 3
	party.Stage = "finished"
	party.FinishedAt = 123
	request(t, s, Request{Op: "list", Secret: secret(1), Parties: []Party{party}})
	party.Revision = 2
	party.Stage = "play"
	party.FinishedAt = 0
	request(t, s, Request{Op: "list", Parties: []Party{party}})
	reboot, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	out = request(t, reboot, Request{Op: "list", Secret: secret(2)})
	if p := out.Rooms[0].Parties; len(p) != 1 || p[0].Stage != "finished" || p[0].FinishedAt != 123 {
		t.Fatal("lost or regressed party", p)
	}
	// Normal connection polls must not carry the growing history catalog.
	joined := request(t, reboot, Request{Op: "join", Room: r.ID, Secret: secret(2)})
	if len(joined.Room.Parties) != 0 {
		t.Fatal("catalog sent with handshake poll")
	}
	party.ID = "unrelated"
	request(t, reboot, Request{Op: "list", Secret: secret(2), Parties: []Party{party}})
	if len(request(t, reboot, Request{Op: "list"}).Rooms[0].Parties) != 1 {
		t.Fatal("unrelated device published party")
	}
}
