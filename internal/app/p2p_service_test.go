package app

import (
	"context"
	"testing"
	"time"

	"preferans/internal/p2p"
)

// Один игрок держит сервис комнат у себя, остальные подключаются к нему по коду сервиса
// (запросы идут по libp2p), а за столом соединяются напрямую по приглашениям.
func TestRoomsOverPeerToPeerService(t *testing.T) {
	apps := []*App{New(t.TempDir()), New(t.TempDir()), New(t.TempDir())}
	defer func() {
		for i, a := range apps {
			if t.Failed() {
				s := a.Status()
				t.Logf("client %d links=%v connected=%v roomError=%s logs=%v", i, s.Links, s.Connected, s.RoomError, s.Logs)
			}
			a.Close()
		}
	}()
	node, err := apps[0].p2pNode()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	inv, err := node.Invite(ctx, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	code, err := p2p.Encode(p2p.Code{Kind: p2p.KindService, Invite: inv})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range apps[1:] {
		if err = a.SetRoomService(code); err != nil {
			t.Fatal(err)
		}
	}
	r, err := apps[1].RoomDirectory("create", "", "Вместе")
	if err != nil {
		t.Fatal(err)
	}
	list, err := apps[0].RoomDirectory("list", "", "")
	if err != nil || len(list.Rooms) != 1 {
		t.Fatalf("комната должна быть видна на хозяине сервиса: %+v %v", list, err)
	}
	for _, a := range apps {
		if err = a.EnterRoom(r.Room.ID); err != nil {
			t.Fatal(err)
		}
	}
	waitRoom(t, func() bool { return len(apps[1].Status().Room.Members) == 3 })
	if err = apps[1].roomTable("", 3, 20, "Ведущий", false); err != nil {
		t.Fatal(err)
	}
	waitRoom(t, func() bool { s := apps[1].Status(); return len(s.Connected) == 3 && s.Connected[1] && s.Connected[2] })
}