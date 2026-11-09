package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"preferans/internal/p2p"
)

func ntfyLobbyStub() *httptest.Server {
	var mu sync.Mutex
	var msgs []string
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(r.URL.Path, "/"+p2p.LobbyTopic) {
			http.NotFound(w, r)
			return
		}
		if r.Method == "POST" {
			b, _ := io.ReadAll(r.Body)
			msgs = append(msgs, string(b))
			return
		}
		for _, m := range msgs {
			b, _ := json.Marshal(map[string]string{"event": "message", "message": m})
			w.Write(append(b, '\n'))
		}
	}))
}

func lobbyState(a *App) (url, host string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rooms.url, a.lobbyHost
}

// Публичное лобби: игроки ничего не вводят, самая старая установка становится ведущей сервиса комнат,
// остальные следуют за ней; при исчезновении ведущего сервис переходит к следующему.
func TestLobbyElectsOldestAndFailsOver(t *testing.T) {
	ts := ntfyLobbyStub()
	defer ts.Close()
	oldURL, oldPeriod := p2p.NtfyURL, lobbyPeriod
	p2p.NtfyURL, lobbyPeriod = ts.URL, 250*time.Millisecond
	defer func() { p2p.NtfyURL, lobbyPeriod = oldURL, oldPeriod }()

	a, b := New(t.TempDir()), New(t.TempDir())
	defer b.Close()
	a.lobbySince, b.lobbySince = 100, 200
	a.EnableLobby()
	b.EnableLobby()

	eventually(t, func() bool {
		au, ah := lobbyState(a)
		bu, bh := lobbyState(b)
		return au == "" && ah == "" && bu != "" && bh != ""
	})
	// B создаёт комнату через сервис ведущего A, A видит её в своём списке
	r, err := b.RoomDirectory("create", "", "Лобби")
	if err != nil || r.Room == nil {
		t.Fatalf("%+v %v", r, err)
	}
	list, err := a.RoomDirectory("list", "", "")
	if err != nil || len(list.Rooms) != 1 {
		t.Fatalf("комната должна быть на сервисе ведущего: %+v %v", list, err)
	}
	// ведущий исчезает: B через несколько периодов становится ведущим сам
	a.Close()
	eventually(t, func() bool {
		bu, bh := lobbyState(b)
		return bu == "" && bh == ""
	})
}

// Каталог комнат реплицируется на устройства-последователи: когда ведущий исчезает, новый ведущий
// уже знает комнаты; когда прежний ведущий возвращается, он забирает комнаты, созданные без него.
func TestLobbyReplicatesRoomCatalogAcrossFailover(t *testing.T) {
	ts := ntfyLobbyStub()
	defer ts.Close()
	oldURL, oldPeriod := p2p.NtfyURL, lobbyPeriod
	p2p.NtfyURL, lobbyPeriod = ts.URL, 250*time.Millisecond
	defer func() { p2p.NtfyURL, lobbyPeriod = oldURL, oldPeriod }()

	a, b := New(t.TempDir()), New(t.TempDir())
	defer b.Close()
	a.lobbySince, b.lobbySince = 100, 200
	a.EnableLobby()
	b.EnableLobby()
	eventually(t, func() bool { bu, bh := lobbyState(b); return bu != "" && bh != "" })

	r, err := b.RoomDirectory("create", "", "Реплика")
	if err != nil || r.Room == nil {
		t.Fatalf("%+v %v", r, err)
	}
	replicated := func(x *App) bool {
		srv, err := x.roomService()
		if err != nil {
			return false
		}
		for _, sn := range srv.Snapshot() {
			if sn.ID == r.Room.ID && sn.Name == "Реплика" && !sn.Deleted {
				return true
			}
		}
		return false
	}
	eventually(t, func() bool { return replicated(b) })

	a.Close()
	eventually(t, func() bool { bu, bh := lobbyState(b); return bu == "" && bh == "" })
	list, err := b.RoomDirectory("list", "", "")
	if err != nil || len(list.Rooms) != 1 || list.Rooms[0].ID != r.Room.ID {
		t.Fatalf("новый ведущий должен знать комнату: %+v %v", list, err)
	}
}

// Игрок, зашедший в комнату на собственном сервисе до того, как увидел ведущего, не остаётся в расколе:
// каталоги обмениваются в обе стороны, и он переходит на сервис ведущего вместе со своей комнатой.
func TestLobbyMovesPlayerInRoomToElectedHost(t *testing.T) {
	ts := ntfyLobbyStub()
	defer ts.Close()
	oldURL, oldPeriod := p2p.NtfyURL, lobbyPeriod
	p2p.NtfyURL, lobbyPeriod = ts.URL, 250*time.Millisecond
	defer func() { p2p.NtfyURL, lobbyPeriod = oldURL, oldPeriod }()

	a, b := New(t.TempDir()), New(t.TempDir())
	defer a.Close()
	defer b.Close()
	a.lobbySince, b.lobbySince = 100, 200
	// у каждого уже есть комната на собственном сервисе, и оба игрока в своих комнатах
	ra, err := a.RoomDirectory("create", "", "Общая")
	if err != nil || ra.Room == nil {
		t.Fatalf("%+v %v", ra, err)
	}
	rb, err := b.RoomDirectory("create", "", "Своя")
	if err != nil || rb.Room == nil {
		t.Fatalf("%+v %v", rb, err)
	}
	if err = a.EnterRoom(ra.Room.ID); err != nil {
		t.Fatal(err)
	}
	if err = b.EnterRoom(rb.Room.ID); err != nil {
		t.Fatal(err)
	}
	a.EnableLobby()
	b.EnableLobby()
	eventually(t, func() bool { bu, bh := lobbyState(b); return bu != "" && bh != "" })

	// на сервисе ведущего A есть обе комнаты, и B числится участником своей комнаты
	eventually(t, func() bool {
		list, err := a.RoomDirectory("list", "", "")
		if err != nil || len(list.Rooms) != 2 {
			return false
		}
		for _, r := range list.Rooms {
			if r.ID == rb.Room.ID && len(r.Members) == 1 {
				return true
			}
		}
		return false
	})
	// и B видит комнату «Общая» из каталога ведущего
	list, err := b.RoomDirectory("list", "", "")
	if err != nil || len(list.Rooms) != 2 {
		t.Fatalf("%+v %v", list, err)
	}
}

// Если ntfy недоступен, после перезапуска устройство находит ведущего по адресу, запомненному при прошлой встрече.
func TestLobbyFindsRememberedHostWhenNtfyIsDown(t *testing.T) {
	ts := ntfyLobbyStub()
	defer ts.Close()
	oldURL, oldPeriod := p2p.NtfyURL, lobbyPeriod
	p2p.NtfyURL, lobbyPeriod = ts.URL, 250*time.Millisecond
	defer func() { p2p.NtfyURL, lobbyPeriod = oldURL, oldPeriod }()

	dirB := t.TempDir()
	a, b := New(t.TempDir()), New(dirB)
	defer a.Close()
	a.lobbySince, b.lobbySince = 100, 200
	a.EnableLobby()
	b.EnableLobby()
	eventually(t, func() bool { bu, bh := lobbyState(b); return bu != "" && bh != "" })
	eventually(t, func() bool { return b.loadSavedHost() != nil })
	b.Close()
	time.Sleep(2 * time.Second) // как при настоящем перезапуске: прежние соединения успевают закрыться

	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close() // «ntfy» больше не отвечает
	p2p.NtfyURL = deadURL

	b2 := New(dirB)
	defer b2.Close()
	b2.lobbySince = 200
	defer func() {
		if t.Failed() {
			for _, l := range b2.Status().Logs {
				t.Log(l)
			}
		}
	}()
	b2.EnableLobby()
	eventually(t, func() bool { bu, bh := lobbyState(b2); return bu != "" && bh != "" })
}