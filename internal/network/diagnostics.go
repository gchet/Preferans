package network

import "fmt"

func (p *Peer) DiagnosticRoute() (channel, route, localType, remoteType string) {
	p.mu.Lock()
	channel = "not-created"
	if p.dc != nil {
		channel = p.dc.ReadyState().String()
	}
	p.mu.Unlock()
	if s := p.PC.SCTP(); s != nil && s.Transport() != nil && s.Transport().ICETransport() != nil {
		pair, err := s.Transport().ICETransport().GetSelectedCandidatePair()
		if err == nil && pair != nil {
			localType = pair.Local.Typ.String()
			remoteType = pair.Remote.Typ.String()
			route = fmt.Sprintf("%s:%d ↔ %s:%d (%s)", pair.Local.Address, pair.Local.Port, pair.Remote.Address, pair.Remote.Port, pair.Local.Protocol)
		}
	}
	return
}
