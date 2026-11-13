package rooms

import "testing"

func TestOnlyCreatorRenamesAndDeletesRoom(t *testing.T) {
	s, _ := New("")
	r := request(t, s, Request{Op: "create", Name: "Клуб", PlayerName: "Гена"}).Room
	if r.Creator != Identity(secret(0)) || r.CreatorName != "Гена" || r.Created == 0 {
		t.Fatalf("создатель не записан: %+v", r)
	}
	if _, e := s.Handle(Request{Op: "rename", Room: r.ID, Name: "Чужая", Secret: secret(1)}); e == nil {
		t.Fatal("чужой игрок переименовал комнату")
	}
	if _, e := s.Handle(Request{Op: "delete", Room: r.ID, Secret: secret(1)}); e == nil || s.Catalog[r.ID].Deleted {
		t.Fatal("чужой игрок удалил комнату")
	}
	request(t, s, Request{Op: "rename", Room: r.ID, Name: "Своя"})
	request(t, s, Request{Op: "delete", Room: r.ID})
	if !s.Catalog[r.ID].Deleted {
		t.Fatal("создатель не смог удалить комнату")
	}
}

func TestLegacyRoomWithoutCreatorIsOpenToEveryone(t *testing.T) {
	s, _ := New("")
	r := request(t, s, Request{Op: "create", Name: "Старая"}).Room
	s.Catalog[r.ID].Creator, s.Catalog[r.ID].CreatorName = "", ""
	request(t, s, Request{Op: "rename", Room: r.ID, Name: "Новая", Secret: secret(1)})
	request(t, s, Request{Op: "delete", Room: r.ID, Secret: secret(1)})
	if !s.Catalog[r.ID].Deleted {
		t.Fatal("комнату без создателя нельзя удалить")
	}
}

func TestPlayersSeeCreatorAndRecentHistory(t *testing.T) {
	s, _ := New("")
	r := request(t, s, Request{Op: "create", Name: "Север", PlayerName: "Гена"}).Room
	party := Party{ID: "p1", Room: r.ID, Host: Identity(secret(0)), Stage: "play", Revision: 1, Players: []PartyPlayer{
		{ID: Identity(secret(0)), Name: "Гена"}, {ID: Identity(secret(1)), Name: "Лена"}, {Name: "Бот", Bot: true},
	}}
	request(t, s, Request{Op: "list", Parties: []Party{party}})
	out, err := s.Handle(Request{Op: "list", Secret: secret(2), Name: "Игорь"})
	if err != nil || len(out.Rooms) != 1 {
		t.Fatal(err, out.Rooms)
	}
	got := out.Rooms[0]
	if got.CreatorName != "Гена" || got.Creator != Identity(secret(0)) || len(got.Parties) != 1 || got.Parties[0].Players[1].Name != "Лена" {
		t.Fatalf("игрок не видит создателя или историю: %+v", got)
	}
}

func TestRecentPartiesKeepsNewest(t *testing.T) {
	var parties []Party
	for i := 0; i < historyLimit+50; i++ {
		parties = append(parties, Party{ID: string(rune('a'+i%26)) + string(rune('A'+i/26)), UpdatedAt: int64(i)})
	}
	got := recentParties(parties, historyLimit)
	if len(got) != historyLimit || got[0].UpdatedAt != int64(historyLimit+49) || got[historyLimit-1].UpdatedAt != 50 {
		t.Fatalf("%d %d %d", len(got), got[0].UpdatedAt, got[len(got)-1].UpdatedAt)
	}
}

func TestCreatorReplicatesAndOlderRecordWins(t *testing.T) {
	a, _ := New("")
	b, _ := New("")
	r := request(t, a, Request{Op: "create", Name: "Клуб", PlayerName: "Гена"}).Room
	if _, err := b.Merge(snapOf(t, a)); err != nil {
		t.Fatal(err)
	}
	got := b.Catalog[r.ID]
	if got.Creator != Identity(secret(0)) || got.CreatorName != "Гена" || got.Created != r.Created {
		t.Fatalf("создатель не реплицирован: %+v", got)
	}
	// комната без создателя получает его из чужого снимка
	b.Catalog[r.ID].Creator, b.Catalog[r.ID].CreatorName, b.Catalog[r.ID].Created = "", "", 0
	if n, _ := b.Merge(snapOf(t, a)); n != 1 || b.Catalog[r.ID].Creator == "" {
		t.Fatal("создатель не дописан", n)
	}
	// при расхождении побеждает запись с меньшим временем создания, в любом порядке слияния
	x := &Room{ID: "r", Name: "Х", Creator: "x", Created: 200}
	if !mergeCreator(x, RoomSnap{Creator: "y", Created: 100, CreatorName: "Й"}) || x.Creator != "y" || x.CreatorName != "Й" {
		t.Fatal("старшая запись не победила", x)
	}
	if mergeCreator(x, RoomSnap{Creator: "x", Created: 200}) || x.Creator != "y" {
		t.Fatal("младшая запись затёрла старшую", x)
	}
}