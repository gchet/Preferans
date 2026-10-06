package app

import (
	"path/filepath"
	"preferans/internal/game"
	"testing"
)

func TestJoiningUsesSavedPlayerName(t *testing.T) {
	host := New(t.TempDir())
	defer host.Close()
	dir := filepath.Join(t.TempDir(), "guest")
	client := New(dir)
	_ = host.Configure(nil)
	_ = client.Configure(nil)
	if err := host.Create(3, 30, "Ведущий", false); err != nil {
		t.Fatal(err)
	}
	if err := client.SetName("  Игорь  "); err != nil {
		t.Fatal(err)
	}
	connect(t, host, client, 1)
	if host.Status().View.Players[1].Name != "Игорь" || client.Status().View.Players[1].Name != "Игорь" {
		t.Fatal("joining lost name")
	}
	v := client.Status().View
	if err := client.Command(game.Command{ID: game.ID(), Seat: 1, Revision: v.Revision, Action: "ready", Name: "Игорь новый"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		return host.Status().View.Players[1].Name == "Игорь новый" && client.Status().View.Players[1].Name == "Игорь новый"
	})
	if err := client.SetName("Игорь новый"); err != nil {
		t.Fatal(err)
	}
	client.Close()
	eventually(t, func() bool { return !host.Status().Connected[1] })
	client = New(dir)
	defer client.Close()
	_ = client.Configure(nil)
	if client.Status().Appearance.Name != "Игорь новый" {
		t.Fatal("profile name lost on restart")
	}
	connect(t, host, client, 1)
	if client.Status().View.Players[1].Name != "Игорь новый" {
		t.Fatal("reconnection lost name")
	}
}
