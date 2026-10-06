package rooms

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func secret(i int) string { return fmt.Sprintf("test-device-secret-%032d", i) }
func request(t *testing.T, s *Server, q Request) Response {
	t.Helper()
	if q.Secret == "" {
		q.Secret = secret(0)
	}
	if q.Name == "" {
		q.Name = "Игрок"
	}
	var r Response
	var e error
	if q.Op == "list" || len(q.Op) >= 13 && q.Op[:13] == "admin-delete-" {
		r, e = s.HandleAdmin(q)
	} else {
		r, e = s.Handle(q)
	}
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestRoomsCapacityIsolationAndRename(t *testing.T) {
	s, _ := New("")
	r := request(t, s, Request{Op: "create", Name: "Друзья"}).Room
	other := request(t, s, Request{Op: "create", Name: "Другая"}).Room
	for i := 0; i < 4; i++ {
		request(t, s, Request{Op: "join", Room: r.ID, Secret: secret(i)})
	}
	if _, e := s.Handle(Request{Op: "join", Room: r.ID, Secret: secret(4), Name: "Пятый"}); e == nil {
		t.Fatal("fifth joined")
	}
	request(t, s, Request{Op: "join", Room: other.ID, Secret: secret(4)})
	request(t, s, Request{Op: "signal", Room: r.ID, Signal: &Signal{Kind: "voice", To: Identity(secret(1)), Payload: "hello"}})
	if len(request(t, s, Request{Op: "poll", Room: other.ID, Secret: secret(4)}).Signals) != 0 {
		t.Fatal("cross-room leak")
	}
	if len(request(t, s, Request{Op: "poll", Room: r.ID, Secret: secret(2)}).Signals) != 0 {
		t.Fatal("wrong recipient")
	}
	if len(request(t, s, Request{Op: "poll", Room: r.ID, Secret: secret(1)}).Signals) != 1 {
		t.Fatal("missing message")
	}
	renamed := request(t, s, Request{Op: "rename", Room: r.ID, Name: "Переименована"}).Room
	if renamed.ID != r.ID {
		t.Fatal("identity changed")
	}
	if _, e := s.Handle(Request{Op: "delete", Room: r.ID, Secret: secret(0)}); e == nil {
		t.Fatal("deleted occupied room")
	}
}
func TestSingleTableRaceAndPersistentRecovery(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	r := request(t, s, Request{Op: "create", Name: "Стол"}).Room
	for i := 0; i < 3; i++ {
		request(t, s, Request{Op: "join", Room: r.ID, Secret: secret(i)})
	}
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for host := 0; host < 2; host++ {
		wg.Add(1)
		go func(h int) {
			defer wg.Done()
			ids := []string{Identity(secret(h)), Identity(secret(1 - h)), Identity(secret(2))}
			_, e := s.Handle(Request{Op: "table", Room: r.ID, Secret: secret(h), Table: &Table{ID: fmt.Sprint(h), Players: ids, Bots: []bool{false, false, false}, Keys: []string{"a", "b", "c"}}})
			if e == nil {
				results <- h
			}
		}(host)
	}
	wg.Wait()
	close(results)
	winners := []int{}
	for h := range results {
		winners = append(winners, h)
	}
	if len(winners) != 1 {
		t.Fatal("table race", winners)
	}
	before := request(t, s, Request{Op: "poll", Room: r.ID}).Room
	if len(before.Table.Keys) != 0 {
		t.Fatal("private key verifiers leaked")
	}
	reboot, e := New(dir)
	if e != nil {
		t.Fatal(e)
	}
	after := request(t, reboot, Request{Op: "poll", Room: r.ID}).Room
	if after.Table.ID != before.Table.ID {
		t.Fatal("table lost")
	}
	request(t, reboot, Request{Op: "poll", Room: r.ID, Secret: secret(winners[0])})
	request(t, reboot, Request{Op: "close", Room: r.ID, TableID: after.Table.ID, Secret: secret(winners[0])})
	for i := 0; i < 3; i++ {
		_, _ = reboot.Handle(Request{Op: "leave", Room: r.ID, Secret: secret(i)})
	}
	reboot.now = func() time.Time { return time.Now().Add(time.Minute) }
	request(t, reboot, Request{Op: "delete", Room: r.ID})
}
func TestLegacySeatRequiresExistingKey(t *testing.T) {
	s, _ := New("")
	r := request(t, s, Request{Op: "create", Name: "Сохранение"}).Room
	request(t, s, Request{Op: "join", Room: r.ID})
	request(t, s, Request{Op: "join", Room: r.ID, Secret: secret(1)})
	request(t, s, Request{Op: "table", Room: r.ID, Table: &Table{ID: "saved", Players: []string{Identity(secret(0)), "", ""}, Bots: []bool{false, false, true}, Keys: []string{"host", Identity("original-key"), "bot"}}})
	wrong := request(t, s, Request{Op: "poll", Room: r.ID, Secret: secret(1), TableID: "saved", Key: "wrong"})
	if wrong.Room.Table.Players[1] != "" {
		t.Fatal("seat stolen")
	}
	right := request(t, s, Request{Op: "poll", Room: r.ID, Secret: secret(1), TableID: "saved", Key: "original-key"})
	if right.Room.Table.Players[1] != Identity(secret(1)) {
		t.Fatal("original guest not restored")
	}
}
