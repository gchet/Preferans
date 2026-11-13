package p2p

import (
	"net"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func TestSplitReachableUsesExactSubnetWhenMaskKnown(t *testing.T) {
	in := []string{"/ip4/192.168.88.66/udp/1/quic-v1", "/ip4/192.168.20.5/udp/1/quic-v1", "/ip4/93.184.216.34/udp/1/quic-v1"}
	pc := net.IPv4(192, 168, 20, 222)
	// без маски сравниваются два первых октета: чужая подсеть 192.168.88.x ещё «подходит»
	if keep, skipped := splitReachable(in, pc, nil); len(keep) != 3 || len(skipped) != 0 {
		t.Fatal(keep, skipped)
	}
	keep, skipped := splitReachable(in, pc, net.CIDRMask(24, 32))
	if len(keep) != 2 || keep[0] != in[1] || keep[1] != in[2] || len(skipped) != 1 || skipped[0] != in[0] {
		t.Fatal(keep, skipped)
	}
	// телефон в сети 10.1.10.x: оба адреса 192.168.x пропущены даже без маски
	keep, skipped = splitReachable(in, net.IPv4(10, 1, 10, 1), nil)
	if len(keep) != 1 || keep[0] != in[2] || len(skipped) != 2 {
		t.Fatal(keep, skipped)
	}
}

func TestSharesNAT(t *testing.T) {
	ours := map[string]bool{"93.184.216.34": true}
	cases := []struct {
		addr string
		want bool
	}{
		{"/ip4/93.184.216.34/udp/4001/quic-v1", true},
		{"/ip4/8.8.8.8/udp/4001/quic-v1", false},
		{"/ip4/192.168.88.66/udp/4001/quic-v1", false},
		{"/ip4/93.184.216.34/udp/4001/quic-v1/p2p/12D3KooWD3eckifWpRn9wQpMG9R9hX3sD158z7EqHWmweQAtGS2t/p2p-circuit", false},
	}
	for _, c := range cases {
		if got := sharesNAT(ours, []ma.Multiaddr{ma.StringCast(c.addr)}); got != c.want {
			t.Errorf("%s: %v", c.addr, got)
		}
	}
	if sharesNAT(nil, []ma.Multiaddr{ma.StringCast(cases[0].addr)}) {
		t.Error("без своих публичных адресов общий NAT не определяется")
	}
	if !inviteSharesNAT(ours, []string{"/ip4/192.168.88.66/udp/1/quic-v1", cases[0].addr}) {
		t.Error("приглашение с нашим публичным IP")
	}
}

func TestLanRetryBacksOff(t *testing.T) {
	if lanDelay(0) != 0 || lanDelay(1) != 30*time.Second || lanDelay(2) != time.Minute || lanDelay(3) != 2*time.Minute || lanDelay(30) != lanRetryMax {
		t.Fatal(lanDelay(1), lanDelay(2), lanDelay(3), lanDelay(30))
	}
	var tries lanTries
	id := peer.ID("partner")
	now := time.Now()
	if !tries.allow(id, now) {
		t.Fatal("первая попытка запрещена")
	}
	tries.done(id, false, now)
	if tries.allow(id, now.Add(10*time.Second)) || !tries.allow(id, now.Add(31*time.Second)) {
		t.Fatal("пауза после первой неудачи")
	}
	tries.done(id, false, now.Add(31*time.Second))
	if tries.allow(id, now.Add(31*time.Second+50*time.Second)) || !tries.allow(id, now.Add(31*time.Second+61*time.Second)) {
		t.Fatal("пауза после второй неудачи должна вырасти")
	}
	tries.done(id, true, now)
	if !tries.allow(id, now) || tries.failsOf(id) != 0 {
		t.Fatal("успех не сбросил счётчик")
	}
}

func TestSamePublicFilterDropsOwnPublicIP(t *testing.T) {
	notes := 0
	f := &samePublicFilter{ours: func() map[string]bool { return map[string]bool{"93.184.216.34": true} }, note: func(peer.ID) { notes++ }}
	in := []ma.Multiaddr{
		ma.StringCast("/ip4/93.184.216.34/udp/4001/quic-v1"),
		ma.StringCast("/ip4/8.8.8.8/udp/4001/quic-v1"),
		ma.StringCast("/ip4/192.168.1.2/udp/4001/quic-v1"),
	}
	id := peer.ID("partner")
	for i := 0; i < 3; i++ {
		out := f.FilterRemote(id, in)
		if len(out) != 2 || ipOf(out[0]) != "8.8.8.8" {
			t.Fatal(out)
		}
	}
	if notes != 1 {
		t.Fatal("сообщение в журнал должно быть одно, а не", notes)
	}
	if len(f.FilterLocal(id, in)) != 3 {
		t.Fatal("свои адреса не фильтруются")
	}
	none := &samePublicFilter{ours: func() map[string]bool { return nil }}
	if len(none.FilterRemote(id, in)) != 3 {
		t.Fatal("без своих публичных адресов ничего не убираем")
	}
}