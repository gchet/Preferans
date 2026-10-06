package app

import "testing"

// ICE readiness is not yet application readiness. A slow hello must not cause
// the host's automatic invitation loop to replace a working transport.
func TestInvitePreservesConnectionUntilHello(t *testing.T) {
	host, client := New(t.TempDir()), New(t.TempDir())
	defer host.Close()
	defer client.Close()
	_ = host.Configure(nil)
	_ = client.Configure(nil)
	if err := host.Create(3, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	offer, err := host.Invite(1)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := client.Join(offer)
	if err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	peer := client.peers[0].Peer
	originalOpen := peer.Open
	opened, release := make(chan struct{}), make(chan struct{})
	peer.Open = func() { close(opened); <-release; originalOpen() }
	client.mu.Unlock()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	if err := host.Answer(answer); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return host.Status().Links[1] == "authenticating" })
	if host.Status().Connected[1] {
		t.Fatal("participant ready before hello")
	}
	host.mu.Lock()
	link := host.peers[1]
	host.mu.Unlock()
	if _, err := host.Invite(1); err == nil {
		t.Fatal("replaced pending handshake")
	}
	host.mu.Lock()
	preserved := host.peers[1] == link
	host.mu.Unlock()
	if !preserved {
		t.Fatal("changed peer during handshake")
	}
	eventually(t, func() bool {
		select {
		case <-opened:
			return true
		default:
			return false
		}
	})
	close(release)
	released = true
	eventually(t, func() bool { return host.Status().Connected[1] && client.Status().View != nil })
}
