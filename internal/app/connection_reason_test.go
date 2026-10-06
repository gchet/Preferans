package app

import (
	"testing"
	"time"

	"preferans/internal/game"
)

func TestConnectionReasons(t *testing.T) {
	a := &App{saved: Saved{State: &game.State{}}, connected: []bool{true, false}, peers: map[int]*Link{}}
	if got := a.connectionReasonsLocked()[1]; got != "waiting" {
		t.Fatal(got)
	}
	a.peers[1] = &Link{Status: "failed"}
	if got := a.connectionReasonsLocked()[1]; got != "unreachable" {
		t.Fatal(got)
	}
	a.peers[1].LastSeen = time.Now()
	if got := a.connectionReasonsLocked()[1]; got != "lost" {
		t.Fatal(got)
	}
	a.setConnectionReasonLocked(1, "exited")
	a.peers[1] = &Link{Status: "gathering"}
	if got := a.connectionReasonsLocked()[1]; got != "exited" {
		t.Fatal(got)
	}
	a.connected[1] = true
	a.saved.State.ManualPaused = true
	if got := a.connectionReasonsLocked(); len(got) != 0 {
		t.Fatal("pause must not imply connectivity loss", got)
	}
	a.saved.State = nil
	a.connected = []bool{false, true, false}
	a.setConnectionReasonLocked(2, "exited")
	if got := a.connectionReasonsLocked()[2]; got != "host-unreachable" {
		t.Fatal("remote reason must not be trusted without the host", got)
	}
}

func TestGracefulExitAndReconnect(t *testing.T) {
	clientDir := t.TempDir()
	host, client := New(t.TempDir()), New(clientDir)
	defer host.Close()
	defer client.Close()
	_ = host.Configure(nil)
	_ = client.Configure(nil)
	if err := host.Create(3, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	connect(t, host, client, 1)
	observer := New(t.TempDir())
	defer observer.Close()
	_ = observer.Configure(nil)
	connect(t, host, observer, 2)
	client.Close()
	eventually(t, func() bool { return host.Status().ConnectionReasons[1] == "exited" })
	eventually(t, func() bool { return observer.Status().ConnectionReasons[1] == "exited" })
	if host.Status().Connected[1] {
		t.Fatal("exited client remains connected")
	}
	other := New(clientDir)
	defer other.Close()
	_ = other.Configure(nil)
	connect(t, host, other, 1)
	if got := host.Status().ConnectionReasons[1]; got != "" {
		t.Fatal("reconnection did not clear reason", got)
	}
	host.Close()
	eventually(t, func() bool { return other.Status().ConnectionReasons[0] == "exited" })
}

func TestAbortedTransportReportsLoss(t *testing.T) {
	host, client := New(t.TempDir()), New(t.TempDir())
	defer host.Close()
	defer client.Close()
	_ = host.Configure(nil)
	_ = client.Configure(nil)
	if err := host.Create(3, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	connect(t, host, client, 1)
	host.mu.Lock()
	peer := host.peers[1].Peer
	host.mu.Unlock()
	peer.Close()
	eventually(t, func() bool { return host.Status().ConnectionReasons[1] == "lost" })
	if host.Status().Connected[1] {
		t.Fatal("closed transport remains connected")
	}
}
