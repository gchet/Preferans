package network

import (
	"github.com/pion/stun/v4"
	"net"
	"strings"
	"time"
)

// SelectSTUN probes in user order. ICE then discovers candidates using its own
// sockets; the probe's mapped address is deliberately never advertised.
func SelectSTUN(urls []string) []string {
	for _, u := range urls {
		addr := strings.TrimPrefix(u, "stun:")
		if _, _, e := net.SplitHostPort(addr); e != nil {
			addr = net.JoinHostPort(strings.Trim(addr, "[]"), "3478")
		}
		conn, e := net.DialTimeout("udp", addr, 1500*time.Millisecond)
		if e != nil {
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))
		req := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
		_, e = conn.Write(req.Raw)
		if e == nil {
			b := make([]byte, 1500)
			var n int
			n, e = conn.Read(b)
			if e == nil {
				msg := &stun.Message{Raw: b[:n]}
				e = msg.Decode()
				if e == nil && msg.Type == stun.BindingSuccess && msg.TransactionID == req.TransactionID {
					_ = conn.Close()
					return []string{u}
				}
			}
		}
		_ = conn.Close()
	}
	return nil
}

// SelectICEServers prefers TURN and keeps responsive STUN servers as fallback.
func SelectICEServers(urls []string) []string {
	var turn, stun []string
	for _, u := range urls {
		if strings.HasPrefix(u, "turn:") || strings.HasPrefix(u, "turns:") {
			turn = append(turn, u)
		} else if strings.HasPrefix(u, "stun:") {
			stun = append(stun, u)
		}
	}
	selected := append([]string{}, turn...)
	probed := SelectSTUN(stun)
	if len(probed) == 0 {
		probed = stun
	}
	selected = append(selected, probed...)
	return selected
}
