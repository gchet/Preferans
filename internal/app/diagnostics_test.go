package app

import (
	"preferans/internal/game"
	"testing"
	"time"
)

func TestDiagnosticsBidirectionalAndBot(t *testing.T) {
	host, client := New(t.TempDir()), New(t.TempDir())
	defer host.Close()
	defer client.Close()
	for _, a := range []*App{host, client} {
		if err := a.Configure(nil); err != nil {
			t.Fatal(err)
		}
		a.EnableDiagnostics()
	}
	if err := host.Create(3, 30, "Ведущий", false); err != nil {
		t.Fatal(err)
	}
	connect(t, host, client, 1)
	eventually(t, func() bool {
		h, c := host.Diagnostics(), client.Diagnostics()
		return len(h) == 1 && len(c) == 1 && h[0].Healthy && c[0].Healthy
	})
	h0, c0 := host.Diagnostics()[0].Acknowledged, client.Diagnostics()[0].Acknowledged
	if n, err := host.FillBots(); err != nil || n != 1 {
		t.Fatalf("bot: %d %v", n, err)
	}
	eventually(t, func() bool {
		return client.Status().View.Players[2].Bot && host.Diagnostics()[0].Acknowledged > h0 && client.Diagnostics()[0].Acknowledged > c0
	})
	v := client.Status().View
	if err := client.Command(game.Command{ID: game.ID(), Revision: v.Revision, Action: "ready"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return host.Status().View.Players[1].Ready })
	host.mu.Lock()
	link := host.peers[1]
	link.Probe.LastReply = time.Now().Add(-6 * time.Second)
	host.mu.Unlock()
	if host.Diagnostics()[0].Healthy {
		t.Fatal("stale reply shown healthy")
	}
	// Disconnect clears diagnostic links as well as cached connectivity.
	client.Leave()
	if len(client.Diagnostics()) != 0 {
		t.Fatal("stale link after leave")
	}
}
