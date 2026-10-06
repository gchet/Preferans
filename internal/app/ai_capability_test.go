package app

import (
	"testing"

	"preferans/config"
	"preferans/internal/game"
	"preferans/internal/rooms"
)

func TestHostAICapabilityOverridesGuestBuild(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	no, yes := false, true
	for _, flag := range []*bool{nil, &yes, &no} {
		a.saved = Saved{Table: "table", View: &game.View{ID: "table", Seat: 1, HostAIEnabled: flag}}
		want := config.AIEnabled && (flag == nil || *flag)
		if got := a.Status().AIEnabled; got != want {
			t.Fatalf("guest capability: got %v, want %v", got, want)
		}
	}
	if _, err := a.RPC(Request{Action: "ai-settings"}); err == nil {
		t.Fatal("AI RPC accepted with no-AI host")
	}
	if err := a.store.Save("table", a.saved); err != nil {
		t.Fatal(err)
	}
	a.saved = Saved{}
	if err := a.Resume("table"); err != nil {
		t.Fatal(err)
	}
	if a.Status().AIEnabled {
		t.Fatal("saved guest forgot host capability")
	}
	a.Leave()
	if a.Status().AIEnabled != config.AIEnabled {
		t.Fatal("leaving did not restore local capability")
	}
	if err := a.Create(3, 30, "Host", true); err != nil {
		t.Fatal(err)
	}
	v := a.Status().View
	if v.HostAIEnabled == nil || *v.HostAIEnabled != config.AIEnabled {
		t.Fatal("host view omitted build capability")
	}
}

func TestWindowsRoomAIPriorityBeforeTable(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	no, yes := false, true
	a.rooms.room = &rooms.Room{Members: []rooms.Member{
		{ID: "tablet", Platform: "android", Online: true, AIEnabled: &yes},
		{ID: "pc", Platform: "windows", Online: true, AIEnabled: &no},
		{ID: "pc2", Platform: "windows", Online: true, AIEnabled: &yes},
	}}
	if a.Status().AIEnabled {
		t.Fatal("first Windows build flag ignored")
	}
	a.rooms.room.Members[1].Online = false
	if a.Status().AIEnabled != config.AIEnabled {
		t.Fatal("offline Windows retains priority")
	}
	a.rooms.room.Members[1].Online = true
	a.rooms.room.Table = &rooms.Table{ID: "table", Host: "tablet"}
	if a.Status().AIEnabled != config.AIEnabled {
		t.Fatal("Windows overrides table host")
	}
	a.saved = Saved{Table: "table", View: &game.View{ID: "table", HostAIEnabled: &no}}
	if a.Status().AIEnabled {
		t.Fatal("host view flag ignored")
	}
}

func TestHostAICapabilityTravelsWithTableView(t *testing.T) {
	host, client := New(t.TempDir()), New(t.TempDir())
	defer host.Close()
	defer client.Close()
	_ = host.Configure(nil)
	_ = client.Configure(nil)
	if err := host.Create(3, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	connect(t, host, client, 1)
	v := client.Status().View
	if v.HostAIEnabled == nil || *v.HostAIEnabled != config.AIEnabled {
		t.Fatal("guest did not receive host capability")
	}
}
