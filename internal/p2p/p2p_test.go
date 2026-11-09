package p2p

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ma "github.com/multiformats/go-multiaddr"
)

func TestLoopbackRoundTrip(t *testing.T) {
	host, err := NewNode(Config{LoopbackOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	guest, err := NewNode(Config{LoopbackOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()

	got := make(chan []byte, 4)
	var hostConn *Conn
	var mu sync.Mutex
	accepted := make(chan struct{})
	host.OnAccept(func(c *Conn) {
		c.Message = func(b []byte) { got <- b }
		mu.Lock()
		hostConn = c
		mu.Unlock()
		c.Start()
		close(accepted)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	inv, err := host.Invite(ctx, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Addrs) == 0 {
		t.Fatal("пустое приглашение")
	}
	gc, err := guest.Dial(ctx, inv)
	if err != nil {
		t.Fatal(err)
	}
	reply := make(chan []byte, 4)
	statuses := make(chan string, 8)
	gc.Message = func(b []byte) { reply <- b }
	gc.Status = func(s string) { statuses <- s }
	gc.Start()
	if s := <-statuses; s != "connected" {
		t.Fatalf("статус %q", s)
	}
	// libp2p открывает поток лениво: первая запись «будит» обработчик на стороне хоста
	if err := gc.Send(map[string]string{"type": "hello"}); err != nil {
		t.Fatal(err)
	}
	select {
	case b := <-got:
		if !strings.Contains(string(b), `"hello"`) {
			t.Fatalf("получено %s", b)
		}
	case <-ctx.Done():
		t.Fatal("хост не получил hello")
	}
	<-accepted
	mu.Lock()
	hc := hostConn
	mu.Unlock()
	if err := hc.Send(map[string]string{"type": "state"}); err != nil {
		t.Fatal(err)
	}
	select {
	case b := <-reply:
		if !strings.Contains(string(b), `"state"`) {
			t.Fatalf("получено %s", b)
		}
	case <-ctx.Done():
		t.Fatal("гость не получил ответ")
	}

	// большое сообщение (кадры больше 16 КБ у DataChannel раньше требовали фрагментации)
	big := strings.Repeat("x", 3<<20)
	if err := gc.Send(map[string]string{"blob": big}); err != nil {
		t.Fatal(err)
	}
	select {
	case b := <-got:
		if len(b) < len(big) {
			t.Fatalf("получено %d байт", len(b))
		}
	case <-ctx.Done():
		t.Fatal("большое сообщение не дошло")
	}

	if ch, route, _, _ := gc.DiagnosticRoute(); ch != "libp2p" || route == "" {
		t.Fatalf("диагностика: %q %q", ch, route)
	}
	hc.Close()
	select {
	case s := <-statuses:
		if s != "disconnected" {
			t.Fatalf("после закрытия хоста статус %q", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("нет статуса после закрытия")
	}
}
func TestCodeRoundTrip(t *testing.T) {
	c := Code{Kind: KindTable, Table: "tbl", Seat: 2, Nonce: "n1", Key: "k1", Invite: Invite{ID: "12D3KooWabc", Addrs: []string{"/ip4/1.2.3.4/udp/1/quic-v1"}}}
	s, err := Encode(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s, "PREF2.") {
		t.Fatal(s)
	}
	got, err := Decode(" " + s + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if got.Table != "tbl" || got.Seat != 2 || got.Nonce != "n1" || got.Key != "k1" || got.Invite.ID != "12D3KooWabc" || len(got.Invite.Addrs) != 1 {
		t.Fatalf("%+v", got)
	}
	for _, bad := range []string{"", "PREF1.abc", "PREF2.!!!", "PREF2." + "e30"} {
		if _, err := Decode(bad); err == nil {
			t.Fatalf("принят %q", bad)
		}
	}
	svc, _ := ServiceCode("12D3KooWabc")
	if d, err := Decode(svc); err != nil || d.Kind != KindService || len(d.Invite.Addrs) != 0 {
		t.Fatalf("service %+v %v", d, err)
	}
}

func TestRequestResponse(t *testing.T) {
	srv, err := NewNode(Config{LoopbackOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	cli, err := NewNode(Config{LoopbackOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	srv.HandleRequest(func(from string, req []byte) []byte { return append([]byte("echo:"), req...) })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	inv, _ := srv.Invite(ctx, time.Second)
	out, err := cli.Request(ctx, inv, []byte("hello"))
	if err != nil || string(out) != "echo:hello" {
		t.Fatalf("%q %v", out, err)
	}
	big := make([]byte, 1<<20)
	if out, err = cli.Request(ctx, inv, big); err != nil || len(out) != len(big)+5 {
		t.Fatalf("big: %d %v", len(out), err)
	}
}

func TestKeyFileKeepsIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	a, err := NewNode(Config{LoopbackOnly: true, KeyFile: path})
	if err != nil {
		t.Fatal(err)
	}
	id := a.ID()
	a.Close()
	b, err := NewNode(Config{LoopbackOnly: true, KeyFile: path})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if b.ID() != id {
		t.Fatal("peer ID изменился между запусками")
	}
}

func TestAdvertResolve(t *testing.T) {
	var mu sync.Mutex
	var last string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == "POST" {
			b, _ := io.ReadAll(r.Body)
			last = string(b)
			return
		}
		if last != "" {
			b, _ := json.Marshal(map[string]string{"event": "message", "message": last})
			w.Write(append(b, '\n'))
		}
	}))
	defer ts.Close()
	old := NtfyURL
	NtfyURL = ts.URL
	defer func() { NtfyURL = old }()
	n, err := NewNode(Config{LoopbackOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := n.Resolve(ctx, n.ID()); err == nil {
		t.Fatal("объявления ещё нет, ожидалась ошибка")
	}
	inv, _ := n.Invite(ctx, time.Second)
	b, _ := json.Marshal(inv)
	if err := n.publish(ctx, string(b)); err != nil {
		t.Fatal(err)
	}
	got, err := n.Resolve(ctx, n.ID())
	if err != nil || got.ID != n.ID() || len(got.Addrs) == 0 {
		t.Fatalf("%+v %v", got, err)
	}
}
func TestSplitAddrsAndRelayPeers(t *testing.T) {
	relayID := "12D3KooWRSJ2uH6BgDdJJCbTrbVvj5WKMQtbN4yq3J1m7Z2gVnZk"
	addrs := []string{
		"/ip4/192.168.1.5/udp/4001/quic-v1",
		"/ip4/203.0.113.7/tcp/4001/p2p/" + relayID + "/p2p-circuit",
		"/ip4/10.0.0.2/tcp/4001",
	}
	direct, relay := splitAddrs(addrs)
	if len(direct) != 2 || len(relay) != 1 {
		t.Fatalf("direct=%v relay=%v", direct, relay)
	}
	if ids := relayPeers(relay); len(ids) != 1 || ids[0].String() != relayID {
		t.Fatalf("%v", ids)
	}
}

// Прямые адреса пробуются первыми: если они недоступны, а relay в приглашении нет, ошибка понятная и без зависания.
func TestDialDirectFailureWithoutRelay(t *testing.T) {
	a, err := NewNode(Config{LoopbackOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, _ := NewNode(Config{LoopbackOnly: true})
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	inv := Invite{ID: b.ID(), Addrs: []string{"/ip4/127.0.0.1/udp/1/quic-v1"}}
	start := time.Now()
	if _, err := a.Dial(ctx, inv); err == nil || !strings.Contains(err.Error(), "relay") {
		t.Fatalf("ожидалась ошибка про relay: %v", err)
	}
	if time.Since(start) > directDialTimeout+3*time.Second {
		t.Fatalf("слишком долго: %s", time.Since(start))
	}
}

// Соединения с участником защищены от обрезки, защита снимается при закрытии.
func TestConnectionProtectionReleased(t *testing.T) {
	host, _ := NewNode(Config{LoopbackOnly: true})
	defer host.Close()
	guest, _ := NewNode(Config{LoopbackOnly: true})
	defer guest.Close()
	accepted := make(chan *Conn, 1)
	host.OnAccept(func(c *Conn) { c.Start(); accepted <- c })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	inv, _ := host.Invite(ctx, time.Second)
	gc, err := guest.Dial(ctx, inv)
	if err != nil {
		t.Fatal(err)
	}
	gc.Start()
	if err := gc.Send(map[string]string{"x": "y"}); err != nil {
		t.Fatal(err)
	}
	hc := <-accepted
	if !guest.h.ConnManager().IsProtected(host.h.ID(), "") {
		t.Fatal("соединение с хостом должно быть защищено")
	}
	gc.Close()
	hc.Close()
	if guest.h.ConnManager().IsProtected(host.h.ID(), "") {
		t.Fatal("защита должна сниматься при закрытии")
	}
}

func TestLanAddrsSubstitutesUnspecifiedIPv4Only(t *testing.T) {
	var listen []ma.Multiaddr
	for _, s := range []string{"/ip4/0.0.0.0/tcp/34501", "/ip4/0.0.0.0/udp/40000/quic-v1", "/ip6/::/tcp/1", "/ip4/10.0.0.9/tcp/2"} {
		listen = append(listen, ma.StringCast(s))
	}
	got := lanAddrs(listen, net.IPv4(192, 168, 1, 136))
	want := []string{"/ip4/192.168.1.136/tcp/34501", "/ip4/192.168.1.136/udp/40000/quic-v1"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("%v", got)
	}
}

func TestLanRankPrefersDefaultRouteAddressAndSkipsLinkLocal(t *testing.T) {
	out := net.IPv4(192, 168, 1, 74)
	virtual := map[string]bool{"192.168.200.1": true}
	rank := func(s string) int { return lanRank(ma.StringCast(s), out, virtual) }
	if rank("/ip4/192.168.1.74/tcp/1") != 0 || rank("/ip4/10.1.2.3/tcp/1") != 1 || rank("/ip6/fd00::a701:1823/tcp/1") != 2 || rank("/ip4/100.64.1.35/tcp/1") != 3 {
		t.Fatal("порядок: адрес маршрута, частный IPv4, ULA IPv6, CGNAT")
	}
	if rank("/ip4/192.168.200.1/tcp/1") != -1 || rank("/ip6/fe80::1/tcp/1") != -1 || rank("/ip4/127.0.0.1/tcp/1") != -1 || rank("/ip4/169.254.3.4/tcp/1") != -1 {
		t.Fatal("link-local и loopback не годятся для приглашения")
	}
}

func TestVirtualInterfaceNames(t *testing.T) {
	for _, n := range []string{"vEthernet (Default Switch)", "VirtualBox Host-Only Network", "docker0", "veth12ab", "br-1a2b", "virbr0", "vmnet8", "vEthernet (WSL)"} {
		if !isVirtualInterface(n) {
			t.Fatalf("%s — виртуальный адаптер", n)
		}
	}
	for _, n := range []string{"Wi-Fi", "Ethernet", "wlan0", "eth0", "Беспроводная сеть", "Tailscale"} {
		if isVirtualInterface(n) {
			t.Fatalf("%s — не виртуальный", n)
		}
	}
}

func TestReachableDirectSkipsOtherSubnetLAN(t *testing.T) {
	in := []string{"/ip4/192.168.1.74/tcp/1", "/ip4/172.21.224.1/tcp/1", "/ip4/100.64.1.35/tcp/1", "/ip4/203.0.113.7/udp/2/quic-v1", "/ip6/2001:db8::1/tcp/3"}
	got := reachableDirect(in, net.IPv4(192, 168, 1, 136))
	if len(got) != 3 || got[0] != in[0] || got[1] != in[3] || got[2] != in[4] {
		t.Fatalf("%v", got)
	}
	lte := reachableDirect(in, net.IPv4(10, 77, 5, 9))
	if len(lte) != 2 || lte[0] != in[3] {
		t.Fatalf("в LTE остаются только публичные адреса: %v", lte)
	}
	if len(reachableDirect(in, nil)) != len(in) {
		t.Fatal("без своего адреса список не меняется")
	}
}