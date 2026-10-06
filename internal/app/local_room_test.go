package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"preferans/internal/game"
	"preferans/internal/rooms"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalRoomWithoutNetworkAndRestart(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); http.Error(w, "offline", 503) }))
	defer server.Close()
	t.Setenv("PREFERANS_ROOMS_URL", server.URL)
	for _, n := range []int{3, 4} {
		dir := t.TempDir()
		a := New(dir)
		if err := a.SetName("Локальный"); err != nil {
			t.Fatal(err)
		}
		if err := a.EnterRoom(localRoomID); err != nil {
			t.Fatal(err)
		}
		if err := a.roomTable("", n, 10, "Локальный", false); err != nil {
			t.Fatal(err)
		}
		st := a.Status()
		if !st.RoomLocal || !st.RoomOnline || st.ConnectionSetup || st.AIEnabled || len(st.Room.Members) != 1 {
			t.Fatalf("local status: %+v", st)
		}
		for i, p := range st.View.Players {
			if i > 0 && !p.Bot {
				t.Fatal("local room accepted a human seat")
			}
		}
		id := st.View.ID
		a.roomTick()
		catalog, err := a.partyCatalog()
		if err != nil || len(catalog) != 0 {
			t.Fatal("local game leaked into server catalog")
		}
		if err = a.RoomLeaveTable(); err != nil {
			t.Fatal(err)
		}
		if a.Status().Current {
			t.Fatal("cannot leave unfinished local game")
		}
		if err = a.roomTable(id, 0, 0, "", false); err != nil {
			t.Fatal(err)
		}
		a.Close()
		a = New(dir)
		if !a.Status().RoomLocal || a.Status().View == nil || a.Status().View.ID != id {
			t.Fatal("local session lost on restart")
		}
		// Play one complete deal using only engine-generated legal decisions.
		a.mu.Lock()
		s := a.saved.State.Clone()
		var applyErr error
		for moves := 0; moves < 200 && s.Stage != "round"; moves++ {
			var c game.Command
			switch s.Stage {
			case "lobby":
				if !s.Players[0].Ready {
					c = game.Command{ID: game.ID(), Revision: s.Revision, Seat: 0, Action: "ready"}
				} else {
					c = game.Command{ID: game.ID(), Revision: s.Revision, Seat: 0, Action: "start"}
				}
			case "trick":
				c = game.Command{ID: game.ID(), Revision: s.Revision, Seat: 0, Action: "collect"}
			default:
				c = game.BotCommand(s.View(s.Actor()))
			}
			s, applyErr = game.Apply(s, c, nil)
			if applyErr != nil {
				break
			}
		}
		if applyErr != nil || s.Stage != "round" {
			a.mu.Unlock()
			t.Fatalf("local deal failed: %v", applyErr)
		}
		s.Stage = "finished"
		s.FinishedAt = time.Now().Unix()
		a.saved.State = s
		err = a.store.Save(id, a.saved)
		a.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if err = a.RoomLeaveTable(); err != nil {
			t.Fatal(err)
		}
		history, err := a.History()
		if err != nil || len(history) != 1 || history[0].ID != id {
			t.Fatal("local result missing from history")
		}
		result, err := a.HistoryResult(id)
		if err != nil || result.Stage != "finished" {
			t.Fatal("local result cannot be viewed")
		}
		a.Close()
	}
	if requests.Load() != 0 {
		t.Fatalf("local mode contacted the server %d times", requests.Load())
	}
}

func TestLocalOnlyPreferenceSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	p := a.Status().Appearance
	p.LocalOnly = true
	p.DefaultRoom = "network-default"
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	if !a.Status().RoomLocal {
		t.Fatal("preference did not enter local room")
	}
	if err := a.EnterRoom("network"); err == nil {
		t.Fatal("local-only mode entered network room")
	}
	a.Close()
	a = New(dir)
	defer a.Close()
	if !a.Status().RoomLocal || a.Status().ConnectionSetup {
		t.Fatal("local preference did not bypass network setup")
	}
	p = a.Status().Appearance
	p.LocalOnly = false
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	if a.Status().RoomLocal || a.Status().Room != nil || a.Status().Appearance.DefaultRoom != "network-default" {
		t.Fatal("cannot return to room picker or default room was lost")
	}
}

func TestFinishedExitQueuesCloseAndNeverClosesReplacement(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		store, err := rooms.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		var offline atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if offline.Load() {
				http.Error(w, "offline", 503)
				return
			}
			store.ServeHTTP(w, r)
		}))
		t.Setenv("PREFERANS_ROOMS_URL", server.URL)
		dir := t.TempDir()
		a := New(dir)
		room, err := a.RoomDirectory("create", "", "Exit test")
		if err != nil {
			t.Fatal(err)
		}
		if err = a.EnterRoom(room.Room.ID); err != nil {
			t.Fatal(err)
		}
		if err = a.roomTable("", 3, 10, "Host", true); err != nil {
			t.Fatal(err)
		}
		a.mu.Lock()
		id := a.saved.Table
		a.saved.State.Stage = "finished"
		a.saved.State.FinishedAt = time.Now().Unix()
		err = a.store.Save(id, a.saved)
		a.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		offline.Store(true)
		started := time.Now()
		if err = a.RoomLeaveTable(); err != nil {
			t.Fatal(err)
		}
		if time.Since(started) > time.Second || a.Status().Current || a.Status().Room.Table != nil {
			t.Fatal("completed exit waited for server")
		}
		history, err := a.History()
		if err != nil || len(history) != 1 {
			t.Fatal("finished result lost after offline exit")
		}
		waitRoom(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.rooms.closing && !a.rooms.nextClose.IsZero() })
		a.Close()
		a = New(dir)
		a.mu.Lock()
		if len(a.rooms.config.PendingCloses) != 1 || a.rooms.config.Table != "" {
			a.mu.Unlock()
			t.Fatal("close queue lost on restart")
		}
		a.mu.Unlock()
		offline.Store(false)
		a.roomTick()
		if replacement {
			if _, err = a.roomRequest(rooms.Request{Op: "close", Room: room.Room.ID, TableID: id}); err != nil {
				t.Fatal(err)
			}
			if err = a.roomTable("", 3, 10, "Host", true); err != nil {
				t.Fatal(err)
			}
		}
		a.retryRoomCloses()
		a.mu.Lock()
		count := len(a.rooms.config.PendingCloses)
		a.mu.Unlock()
		if count != 0 {
			t.Fatal("server close was not retried")
		}
		out, err := a.roomRequest(rooms.Request{Op: "poll", Room: room.Room.ID})
		if err != nil || (out.Room.Table != nil) != replacement {
			t.Fatal("retry closed replacement table or failed to close old table")
		}
		a.Close()
		server.Close()
	}
}

func TestCompletedExitDoesNotWaitForSlowServer(t *testing.T) {
	block := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block; json.NewEncoder(w).Encode(rooms.Response{}) }))
	a := New(t.TempDir())
	a.mu.Lock()
	a.rooms.url = server.URL
	a.rooms.config.Current = "remote"
	a.rooms.room = &rooms.Room{ID: "remote"}
	a.mu.Unlock()
	if err := a.Create(3, 10, "Host", true); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.saved.Room = "remote"
	a.saved.State.Stage = "finished"
	a.mu.Unlock()
	if err := a.RoomLeaveTable(); err != nil {
		t.Fatal(err)
	}
	if a.Status().Current {
		t.Fatal("UI remains on the completed table")
	}
	close(block)
	a.Close()
	server.Close()
}
