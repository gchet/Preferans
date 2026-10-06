package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"preferans/internal/game"
	"preferans/internal/rooms"
	"testing"
	"time"
)

func waitRoom(t *testing.T, fn func() bool) {
	t.Helper()
	until := time.Now().Add(25 * time.Second)
	for time.Now().Before(until) {
		if fn() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("room condition timed out")
}

func TestFinishedRoomKeepsResultUntilExit(t *testing.T) {
	server, err := rooms.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	t.Setenv("PREFERANS_ROOMS_URL", httpServer.URL)
	host := New(t.TempDir())
	defer host.Close()
	r, err := host.RoomDirectory("create", "", "Итог партии")
	if err != nil {
		t.Fatal(err)
	}
	if err = host.EnterRoom(r.Room.ID); err != nil {
		t.Fatal(err)
	}
	if err = host.roomTable("", 3, 30, "Ведущий", true); err != nil {
		t.Fatal(err)
	}
	host.mu.Lock()
	host.saved.State.Stage = "finished"
	host.saved.State.FinishedAt = time.Now().Unix()
	err = host.store.Save(host.saved.Table, host.saved)
	host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	host.roomTick()
	st := host.Status()
	if !st.Current || st.View == nil || st.View.Stage != "finished" || st.Room.Table == nil {
		t.Fatal("finished table closed automatically")
	}
	if err = host.RoomLeaveTable(); err != nil {
		t.Fatal(err)
	}
	items, err := host.History()
	if err != nil || len(items) != 1 || host.Status().Current {
		t.Fatal("result not retained after exit")
	}
}

func TestHostCanCloseRoomTableWithoutLocalSave(t *testing.T) {
	server, err := rooms.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	t.Setenv("PREFERANS_ROOMS_URL", httpServer.URL)
	host := New(t.TempDir())
	defer host.Close()
	r, err := host.RoomDirectory("create", "", "Missing save")
	if err != nil {
		t.Fatal(err)
	}
	if err = host.EnterRoom(r.Room.ID); err != nil {
		t.Fatal(err)
	}
	if err = host.roomTable("", 3, 30, "Host", true); err != nil {
		t.Fatal(err)
	}
	host.mu.Lock()
	err = host.store.Delete(host.saved.Table)
	host.saved = Saved{}
	host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	host.roomTick()
	st := host.Status()
	if st.Current || st.Room.Table == nil {
		t.Fatal("missing-save scenario not reproduced")
	}
	if err = host.RoomLeaveTable(); err != nil {
		t.Fatal(err)
	}
	host.roomTick()
	st = host.Status()
	if st.Current || st.Room.Table != nil {
		t.Fatal("orphan table was not closed")
	}
	if err = host.roomTable("", 3, 30, "Host", true); err != nil {
		t.Fatal(err)
	}
}
func TestRoomFillBotsAfterCreationPreservesParticipant(t *testing.T) {
	server, err := rooms.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	t.Setenv("PREFERANS_ROOMS_URL", httpServer.URL)
	host, guest := New(t.TempDir()), New(t.TempDir())
	defer host.Close()
	defer guest.Close()
	r, err := host.RoomDirectory("create", "", "Боты после создания")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []*App{host, guest} {
		if err := a.EnterRoom(r.Room.ID); err != nil {
			t.Fatal(err)
		}
	}
	// Keep the guest's reservation, but do not let it connect before filling.
	guest.Close()
	host.roomTick()
	if err := host.roomTable("", 4, 20, "Ведущий", false); err != nil {
		t.Fatal(err)
	}
	if n := host.Status().FreeSeats; n != 2 {
		t.Fatalf("free seats = %d, want 2", n)
	}
	if n, err := host.FillBots(); err != nil || n != 2 {
		t.Fatalf("fill = %d, %v", n, err)
	}
	s := host.Status()
	if s.View.Players[1].Bot || !s.View.Players[2].Bot || !s.View.Players[3].Bot {
		t.Fatal("reserved participant replaced or empty seats not filled")
	}
	if s.FreeSeats != 0 {
		t.Fatal("filled seats still available")
	}
	var saved Saved
	if err := host.store.Load(s.View.ID, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.State.Players[2].Bot || !saved.State.Players[3].Bot {
		t.Fatal("bots not saved")
	}
}
func TestRoomAutomaticConnectionsAndReturn(t *testing.T) {
	server, e := rooms.New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	t.Setenv("PREFERANS_ROOMS_URL", httpServer.URL)
	dirs := []string{t.TempDir(), t.TempDir(), t.TempDir()}
	apps := []*App{New(dirs[0]), New(dirs[1]), New(dirs[2])}
	defer func() {
		for i, a := range apps {
			if t.Failed() {
				s := a.Status()
				t.Logf("client %d links=%v connected=%v roomError=%s logs=%v", i, s.Links, s.Connected, s.RoomError, s.Logs)
			}
			a.Close()
		}
	}()
	r, e := apps[0].RoomDirectory("create", "", "Вместе")
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range apps {
		if e = a.Configure([]string{}); e != nil {
			t.Fatal(e)
		}
		if e = a.EnterRoom(r.Room.ID); e != nil {
			t.Fatal(e)
		}
	}
	waitRoom(t, func() bool { return len(apps[0].Status().Room.Members) == 3 })
	if e = apps[0].roomTable("", 3, 20, "Ведущий", false); e != nil {
		t.Fatal(e)
	}
	waitRoom(t, func() bool { s := apps[0].Status(); return len(s.Connected) == 3 && s.Connected[1] && s.Connected[2] })
	original := apps[1].Status().View.Seat
	v := apps[1].Status().View
	c := game.Command{ID: game.ID(), Revision: v.Revision, Action: "ready"}
	if e = apps[1].Command(c); e != nil {
		t.Fatal(e)
	}
	waitRoom(t, func() bool {
		apps[1].mu.Lock()
		defer apps[1].mu.Unlock()
		return apps[1].saved.Pending == nil && apps[1].saved.View.Players[original].Ready
	})
	// Re-delivery of a command whose acknowledgement was lost is idempotent.
	c.Seat = original
	apps[1].mu.Lock()
	apps[1].saved.Pending = &c
	apps[1].mu.Unlock()
	apps[1].heartbeat()
	waitRoom(t, func() bool { apps[1].mu.Lock(); defer apps[1].mu.Unlock(); return apps[1].saved.Pending == nil })
	// Close and restart the guest, retaining identity and private seat key.
	recoveryStarted := time.Now()
	apps[1].Close()
	apps[1] = New(dirs[1])
	waitRoom(t, func() bool { return apps[1].Status().Room != nil })
	waitRoom(t, func() bool {
		s := apps[1].Status()
		return s.View != nil && s.View.Seat == original && !s.Paused && s.Links[0] == "connected"
	})
	t.Logf("Guest reconnected naturally in %.1f seconds", time.Since(recoveryStarted).Seconds())
	tableID := apps[0].Status().View.ID
	apps[0].Close()
	apps[0] = New(dirs[0])
	waitRoom(t, func() bool {
		s := apps[0].Status()
		return s.View != nil && s.View.ID == tableID && len(s.Connected) == 3 && s.Connected[1] && s.Connected[2]
	})
	if e = apps[0].RoomLeaveTable(); e != nil {
		t.Fatal(e)
	}
	waitRoom(t, func() bool { return !apps[1].Status().Current && !apps[2].Status().Current })
	if apps[1].Status().Room == nil {
		t.Fatal("room lost on table exit")
	}
	if e = apps[0].roomTable(tableID, 0, 0, "", false); e != nil {
		t.Fatal(e)
	}
	waitRoom(t, func() bool { s := apps[0].Status(); return len(s.Connected) == 3 && s.Connected[1] && s.Connected[2] })
	if apps[1].Status().View.Seat != original {
		t.Fatal("resumed seat changed")
	}
}

func TestDefaultRoomOnlyWithoutSelection(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		selected, useDefault bool
	}{
		{"selected lobby beats default", true, true},
		{"selected lobby without default", true, false},
		{"default without selection", false, true},
		{"no selection or default", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := rooms.New("")
			endpoint := httptest.NewServer(server)
			defer endpoint.Close()
			t.Setenv("PREFERANS_ROOMS_URL", endpoint.URL)
			dir := t.TempDir()
			a := New(dir)
			defer func() { a.Close() }()
			selected, err := a.RoomDirectory("create", "", "Выбранная")
			if err != nil {
				t.Fatal(err)
			}
			fallback, err := a.RoomDirectory("create", "", "По умолчанию")
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			if tc.useDefault {
				p := a.Status().Appearance
				p.DefaultRoom = fallback.Room.ID
				if err := a.SetAppearance(p); err != nil {
					t.Fatal(err)
				}
				want = fallback.Room.ID
			}
			if tc.selected {
				if err := a.EnterRoom(selected.Room.ID); err != nil {
					t.Fatal(err)
				}
				want = selected.Room.ID
			}
			// No table: the participant is on the Create Table screen.
			a.Close()
			a = New(dir)
			a.mu.Lock()
			got := a.rooms.config.Current
			a.mu.Unlock()
			if got != want {
				t.Fatalf("room after restart = %q, want %q", got, want)
			}
			if want != "" {
				waitRoom(t, func() bool { return a.Status().Room != nil })
				if s := a.Status(); s.Room.ID != want || s.Current {
					t.Fatal("room changed when entering the lobby, or a table was created")
				}
			}
		})
	}
}

func TestDeletedDefaultRoomCanBeChanged(t *testing.T) {
	server, _ := rooms.New("")
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	t.Setenv("PREFERANS_ROOMS_URL", httpServer.URL)
	dir := t.TempDir()
	a := New(dir)
	r, e := a.RoomDirectory("create", "", "Доступная")
	if e != nil {
		t.Fatal(e)
	}
	appearance := a.Status().Appearance
	appearance.DefaultRoom = "deleted-room"
	if e = a.SetAppearance(appearance); e != nil {
		t.Fatal(e)
	}
	a.Close()
	a = New(dir)
	defer a.Close()
	if e = a.EnterRoom(r.Room.ID); e != nil {
		t.Fatal("cannot switch away from removed default", e)
	}
	if a.Status().Room.ID != r.Room.ID {
		t.Fatal("wrong room")
	}
}

func TestRoomNamesAreTrimmedRequiredAndUnique(t *testing.T) {
	server, _ := rooms.New("")
	endpoint := httptest.NewServer(server)
	defer endpoint.Close()
	t.Setenv("PREFERANS_ROOMS_URL", endpoint.URL)
	a := New(t.TempDir())
	defer a.Close()
	if e := a.SetName("Гена"); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"", " \t\n\u00a0 "} {
		if _, e := a.RoomDirectory("create", "", name); e == nil {
			t.Fatal("accepted empty room name", name)
		}
	}
	r, e := a.RoomDirectory("create", "", "  Север\t")
	if e != nil {
		t.Fatal(e)
	}
	if r.Room.Name != "Север" {
		t.Fatal("room name not trimmed", r.Room.Name)
	}
	for _, name := range []string{"Север", "  Север  ", " север\t"} {
		if _, e = a.RoomDirectory("create", "", name); e == nil {
			t.Fatal("duplicate room accepted", name)
		}
	}
	other, e := a.RoomDirectory("create", "", "Юг")
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"", " \t ", " север "} {
		if _, e = a.RoomDirectory("rename", other.Room.ID, name); e == nil {
			t.Fatal("invalid rename accepted", name)
		}
	}
	renamed, e := a.RoomDirectory("rename", other.Room.ID, "  Восток  ")
	if e != nil {
		t.Fatal(e)
	}
	if renamed.Room.Name != "Восток" {
		t.Fatal("rename not trimmed")
	}
	if _, e = a.RoomDirectory("rename", r.Room.ID, "  Север  "); e != nil {
		t.Fatal("same room counted as duplicate", e)
	}
	list, e := a.RoomDirectory("list", "", "")
	if e != nil {
		t.Fatal(e)
	}
	if len(list.Rooms) != 2 {
		t.Fatal("invalid requests created extra rooms")
	}
}

func TestRoomRetryTimingProtectsPendingHandshakes(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		status string
		age    time.Duration
		want   bool
	}{
		{"failed", time.Second, false}, {"failed", 2 * time.Second, true},
		{"closed", 3 * time.Second, true}, {"disconnected", 3 * time.Second, true},
		{"waiting-answer", 3 * time.Second, false}, {"waiting-answer", 35 * time.Second, true},
		{"gathering", 40 * time.Second, false}, {"connecting", 40 * time.Second, false},
		{"authenticating", 40 * time.Second, false}, {"authenticating", 50 * time.Second, true},
	} {
		t.Run(tc.status+tc.age.String(), func(t *testing.T) {
			if got := roomRetryDue(&Link{Status: tc.status}, now.Add(-tc.age), now); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPublishedTableSurvivesLostHTTPReply(t *testing.T) {
	server, _ := rooms.New("")
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q rooms.Request
		_ = json.NewDecoder(r.Body).Decode(&q)
		result, e := server.Handle(q)
		if q.Op == "table" {
			w.WriteHeader(502)
			return
		}
		if e != nil {
			result.Error = e.Error()
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer endpoint.Close()
	t.Setenv("PREFERANS_ROOMS_URL", endpoint.URL)
	a := New(t.TempDir())
	defer a.Close()
	r, e := a.RoomDirectory("create", "", "Потерянный ответ")
	if e != nil {
		t.Fatal(e)
	}
	if e = a.EnterRoom(r.Room.ID); e != nil {
		t.Fatal(e)
	}
	if e = a.roomTable("", 3, 20, "Ведущий", true); e == nil {
		t.Fatal("expected simulated HTTP failure")
	}
	waitRoom(t, func() bool { s := a.Status(); return s.Current && s.Host && s.View != nil })
}
