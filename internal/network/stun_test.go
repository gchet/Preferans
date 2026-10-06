package network

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pion/stun/v4"
)

func TestSTUNSelection(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 1500)
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, addr, e := conn.ReadFromUDP(buf)
		if e != nil {
			return
		}
		req := &stun.Message{Raw: buf[:n]}
		if req.Decode() != nil {
			return
		}
		response := stun.MustBuild(stun.NewTransactionIDSetter(req.TransactionID), stun.BindingSuccess, &stun.XORMappedAddress{IP: addr.IP, Port: addr.Port})
		_, _ = conn.WriteToUDP(response.Raw, addr)
	}()
	url := "stun:" + conn.LocalAddr().String()
	selected := SelectSTUN([]string{"stun:127.0.0.1:1", url})
	if len(selected) != 1 || selected[0] != url {
		t.Fatal("STUN fallback failed", selected)
	}
	<-done
}

func TestPublicSTUNGathering(t *testing.T) {
	if os.Getenv("PREFERANS_TEST_STUN") != "1" {
		t.Skip("Set PREFERANS_TEST_STUN=1 to contact public STUN")
	}
	urls := SelectSTUN([]string{"stun:stun.l.google.com:19302", "stun:stun.cloudflare.com:3478"})
	if len(urls) == 0 {
		t.Fatal("No public STUN response")
	}
	p, err := New(urls)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	sdp, err := p.Gather(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sdp.SDP, " typ srflx") {
		t.Fatal("STUN did not produce a server-reflexive candidate")
	}
	t.Log("Public STUN response and ICE server-reflexive candidate verified")
}
