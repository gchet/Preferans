package rooms

import (
	"strings"
	"testing"
)

func snapOf(t *testing.T, s *Server) []RoomSnap {
	t.Helper()
	r := request(t, s, Request{Op: "snapshot", Secret: secret(0)})
	return r.Snapshot
}

func partyFor(id string, rev uint64, stage string) Party {
	return Party{ID: id, Stage: stage, Revision: rev, Players: []PartyPlayer{{ID: "a", Name: "А"}, {ID: "b", Name: "Б"}, {ID: "c", Name: "В"}}}
}

func liveNames(s *Server) []string {
	var out []string
	for _, r := range s.Catalog {
		if !r.Deleted {
			out = append(out, r.Name)
		}
	}
	return out
}

func TestSnapshotMergeCopiesRoomsAndPartiesButNotLiveState(t *testing.T) {
	a, _ := New("")
	b, _ := New("")
	room := request(t, a, Request{Op: "create", Name: "Клуб"}).Room
	request(t, a, Request{Op: "join", Room: room.ID})
	a.Catalog[room.ID].Parties = []Party{partyFor("p1", 3, "play")}
	n, err := b.Merge(snapOf(t, a))
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	got := b.Catalog[room.ID]
	if got == nil || got.Name != "Клуб" || len(got.Parties) != 1 || got.Parties[0].Room != room.ID {
		t.Fatalf("%+v", got)
	}
	if len(got.Members) != 0 || got.Table != nil {
		t.Fatal("живое состояние не должно реплицироваться")
	}
	if n, _ = b.Merge(snapOf(t, a)); n != 0 {
		t.Fatalf("повторное слияние ничего не меняет, а изменило %d", n)
	}
}

func TestMergeTombstonesWinAndSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	a, _ := New("")
	b, _ := New(dir)
	room := request(t, a, Request{Op: "create", Name: "Старая"}).Room
	if _, err := b.Merge(snapOf(t, a)); err != nil {
		t.Fatal(err)
	}
	// удаление у ведущего A превращается в надгробие и доходит до B
	request(t, a, Request{Op: "delete", Room: room.ID})
	if !a.Catalog[room.ID].Deleted {
		t.Fatal("удаление должно оставлять надгробие")
	}
	if _, err := b.Merge(snapOf(t, a)); err != nil {
		t.Fatal(err)
	}
	if !b.Catalog[room.ID].Deleted {
		t.Fatal("надгробие должно победить")
	}
	// старый снимок с живой комнатой комнату не воскрешает
	old, _ := New("")
	old.Catalog[room.ID] = &Room{ID: room.ID, Name: "Старая", Members: []Member{}}
	if _, err := b.Merge(snapOf(t, old)); err != nil {
		t.Fatal(err)
	}
	reload, err := New(dir)
	if err != nil || !reload.Catalog[room.ID].Deleted {
		t.Fatal("надгробие должно сохраняться на диске")
	}
}

func TestMergePartiesByRevisionAndDeletedParties(t *testing.T) {
	a, _ := New("")
	b, _ := New("")
	room := request(t, a, Request{Op: "create", Name: "Партии"}).Room
	a.Catalog[room.ID].Parties = []Party{partyFor("p1", 2, "play"), partyFor("p2", 1, "play")}
	if _, err := b.Merge(snapOf(t, a)); err != nil {
		t.Fatal(err)
	}
	a.Catalog[room.ID].Parties[0] = partyFor("p1", 5, "finished")
	b.Catalog[room.ID].Parties[1] = partyFor("p2", 9, "play")
	if _, err := b.Merge(snapOf(t, a)); err != nil {
		t.Fatal(err)
	}
	ps := b.Catalog[room.ID].Parties
	if ps[0].Revision != 5 || ps[1].Revision != 9 {
		t.Fatalf("побеждает большая Revision: %+v", ps)
	}
	a.Catalog[room.ID].DeletedParties = map[string]bool{"p2": true}
	if _, err := b.Merge(snapOf(t, a)); err != nil {
		t.Fatal(err)
	}
	if len(b.Catalog[room.ID].Parties) != 1 || !b.Catalog[room.ID].DeletedParties["p2"] {
		t.Fatalf("%+v", b.Catalog[room.ID])
	}
	if _, err := b.Merge([]RoomSnap{{ID: room.ID, Name: "Партии", Parties: []Party{partyFor("p2", 99, "play")}}}); err != nil {
		t.Fatal(err)
	}
	if len(b.Catalog[room.ID].Parties) != 1 {
		t.Fatal("удалённую партию нельзя вернуть")
	}
}

func TestMergeRenameLatestWinsAndNameConflictIsResolved(t *testing.T) {
	a, _ := New("")
	b, _ := New("")
	ra := request(t, a, Request{Op: "create", Name: "Общая"}).Room
	b.Merge(snapOf(t, a))
	a.Catalog[ra.ID].Name, a.Catalog[ra.ID].Updated = "Новая", b.Catalog[ra.ID].Updated+10
	b.Merge(snapOf(t, a))
	if b.Catalog[ra.ID].Name != "Новая" {
		t.Fatal("более позднее название должно победить")
	}
	// две разные комнаты с одним названием, созданные на разных устройствах
	c, _ := New("")
	d, _ := New("")
	request(t, c, Request{Op: "create", Name: "Дубль"})
	request(t, d, Request{Op: "create", Name: "дубль"})
	if _, err := c.Merge(snapOf(t, d)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Merge(snapOf(t, c)); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*Server{c, d} {
		names := liveNames(s)
		if len(names) != 2 || strings.EqualFold(names[0], names[1]) {
			t.Fatalf("названия должны различаться: %v", names)
		}
	}
	if x, y := liveNames(c), liveNames(d); len(x) != len(y) {
		t.Fatalf("%v %v", x, y)
	}
}

func TestMergeRejectsGarbage(t *testing.T) {
	s, _ := New("")
	bad := []RoomSnap{{ID: ""}, {ID: "x", Name: ""}, {ID: "y", Name: strings.Repeat("я", 41)},
		{ID: "z", Name: "Ок", Parties: []Party{{ID: "q", Stage: "bogus", Players: []PartyPlayer{{}, {}, {}}}, partyFor("", 1, "play")}}}
	if _, err := s.Merge(bad); err != nil {
		t.Fatal(err)
	}
	if r := s.Catalog["z"]; r == nil || len(r.Parties) != 0 || s.Catalog["x"] != nil || s.Catalog["y"] != nil {
		t.Fatalf("%+v", s.Catalog)
	}
	if _, err := s.Merge(make([]RoomSnap, maxSnapRooms+1)); err == nil {
		t.Fatal("слишком большой снимок должен отвергаться")
	}
}

func TestSyncOpMergesBothWays(t *testing.T) {
	host, _ := New("")
	other, _ := New("")
	rh := request(t, host, Request{Op: "create", Name: "У ведущего"}).Room
	ro := request(t, other, Request{Op: "create", Name: "У гостя"}).Room
	out, err := host.Handle(Request{Op: "sync", Snapshot: other.Snapshot()})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Snapshot) != 2 || host.Catalog[ro.ID] == nil {
		t.Fatalf("ведущий должен получить комнату гостя: %+v", out.Snapshot)
	}
	if _, err = other.Merge(out.Snapshot); err != nil || other.Catalog[rh.ID] == nil {
		t.Fatal("гость должен получить комнату ведущего")
	}
}