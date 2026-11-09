package app

import (
	"strings"
	"testing"

	"preferans/internal/p2p"
)

func TestAddrSummaryClassifiesLocalRelayAndPublic(t *testing.T) {
	inv := p2p.Invite{ID: "x", Addrs: []string{
		"/ip4/192.168.1.5/udp/4001/quic-v1", "/ip4/10.1.2.3/tcp/4001", "/ip4/172.20.0.2/tcp/4001", "/ip4/172.32.0.2/tcp/4001",
		"/ip4/203.0.113.7/udp/4001/quic-v1", "/ip4/198.51.100.2/tcp/4001/p2p/12D3KooWabc/p2p-circuit",
	}}
	if got := addrSummary(inv); got != "relay=1 lan=3 (192.168.1.5, 10.1.2.3, 172.20.0.2) public=2" {
		t.Fatalf("%s", got)
	}
}

func TestNetworkMessagesReachAppLog(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	a.p2pLog("пробитие NAT с abc: успешно")
	eventually(t, func() bool {
		for _, l := range a.Status().Logs {
			if strings.Contains(l, "пробитие NAT с abc") {
				return true
			}
		}
		return false
	})
}