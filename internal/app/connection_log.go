package app

import "preferans/locales"

import (
	"fmt"
	"strings"
)

// Never log SDP credentials, room secrets or the credential suffix of an ICE URL.
func iceServerSummary(servers []string) string {
	var safe []string
	turn := 0
	for _, entry := range servers {
		u := strings.SplitN(entry, "|", 2)[0]
		if strings.HasPrefix(u, "turn:") || strings.HasPrefix(u, "turns:") {
			turn++
		}
		safe = append(safe, u)
	}
	return locales.Format("go.internal.app.connection_log.text001", turn, strings.Join(safe, ", "))
}

func iceCandidateSummary(sdp string) string {
	counts := map[string]int{}
	var details []string
	for _, line := range strings.Split(sdp, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "a=candidate:") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 8 || f[6] != "typ" {
			continue
		}
		counts[f[7]]++
		if len(details) < 32 {
			details = append(details, fmt.Sprintf("%s %s %s:%s", f[7], f[2], f[4], f[5]))
		}
	}
	return fmt.Sprintf("host=%d srflx=%d relay=%d prflx=%d [%s]", counts["host"], counts["srflx"], counts["relay"], counts["prflx"], strings.Join(details, "; "))
}

func (a *App) logICEServers(seat int, servers []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logLocked(locales.Text("go.internal.app.connection_log.text002"), seat+1, iceServerSummary(servers))
}
