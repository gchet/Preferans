package p2p

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// Общий NAT. Два устройства за одним роутером (даже в разных подсетях, например Wi-Fi 192.168.88.x и 10.1.10.x)
// видны снаружи с одним публичным IP. Прямое соединение «через публичный адрес» между ними невозможно
// (роутер не возвращает пакеты с внутреннего интерфейса на внутренний), а адреса локальной сети партнёра из другой
// подсети обычно достижимы через маршрутизацию роутера. Поэтому при общем публичном IP их всё же пробуем,
// а пробитие NAT на собственный публичный адрес не затеваем.

// ipOf возвращает IP из первого компонента адреса (ip4/ip6) или пустую строку.
func ipOf(a ma.Multiaddr) string {
	first, _ := ma.SplitFirst(a)
	if first == nil {
		return ""
	}
	switch first.Protocol().Code {
	case ma.P_IP4, ma.P_IP6:
		return net.IP(first.RawValue()).String()
	}
	return ""
}

// publicIPs — публичные IP этого узла в том виде, как его видят другие (наблюдаемые адреса).
func (n *Node) publicIPs() map[string]bool {
	out := map[string]bool{}
	for _, a := range n.h.Addrs() {
		if strings.Contains(a.String(), "/p2p-circuit") || !manet.IsPublicAddr(a) {
			continue
		}
		if ip := ipOf(a); ip != "" {
			out[ip] = true
		}
	}
	return out
}

// sharesNAT: среди адресов партнёра есть публичный (не relay) адрес с тем же IP, что у нас.
func sharesNAT(ours map[string]bool, theirs []ma.Multiaddr) bool {
	if len(ours) == 0 {
		return false
	}
	for _, a := range theirs {
		if strings.Contains(a.String(), "/p2p-circuit") || !manet.IsPublicAddr(a) {
			continue
		}
		if ours[ipOf(a)] {
			return true
		}
	}
	return false
}

func inviteSharesNAT(ours map[string]bool, addrs []string) bool {
	var parsed []ma.Multiaddr
	for _, s := range addrs {
		if m, err := ma.NewMultiaddr(s); err == nil {
			parsed = append(parsed, m)
		}
	}
	return sharesNAT(ours, parsed)
}

// samePublicFilter убирает из адресов пробития NAT адреса партнёра с нашим публичным IP: такая попытка заведомо
// безуспешна и повторялась бы на каждое переподключение через relay.
type samePublicFilter struct {
	ours  func() map[string]bool
	note  func(id peer.ID)
	mu    sync.Mutex
	noted map[peer.ID]time.Time
}

func (f *samePublicFilter) FilterLocal(_ peer.ID, addrs []ma.Multiaddr) []ma.Multiaddr { return addrs }

func (f *samePublicFilter) FilterRemote(id peer.ID, addrs []ma.Multiaddr) []ma.Multiaddr {
	ours := f.ours()
	if len(ours) == 0 {
		return addrs
	}
	out := make([]ma.Multiaddr, 0, len(addrs))
	dropped := false
	for _, a := range addrs {
		if ours[ipOf(a)] && manet.IsPublicAddr(a) {
			dropped = true
			continue
		}
		out = append(out, a)
	}
	if dropped && f.note != nil {
		f.mu.Lock()
		if f.noted == nil {
			f.noted = map[peer.ID]time.Time{}
		}
		first := time.Since(f.noted[id]) > 5*time.Minute
		if first {
			f.noted[id] = time.Now()
		}
		f.mu.Unlock()
		if first {
			f.note(id)
		}
	}
	return out
}

// Повторы прямой попытки по локальной сети не чаще, чем растёт пауза: 30 с, 1 мин, 2 мин … до 10 минут.
const (
	lanRetryMin = 30 * time.Second
	lanRetryMax = 10 * time.Minute
)

func lanDelay(fails int) time.Duration {
	if fails < 1 {
		return 0
	}
	d := lanRetryMin
	for i := 1; i < fails && d < lanRetryMax; i++ {
		d *= 2
	}
	if d > lanRetryMax {
		d = lanRetryMax
	}
	return d
}

type lanTry struct {
	fails int
	next  time.Time
}

type lanTries struct {
	mu sync.Mutex
	m  map[peer.ID]lanTry
}

func (t *lanTries) allow(id peer.ID, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return !now.Before(t.m[id].next)
}

func (t *lanTries) failsOf(id peer.ID) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.m[id].fails
}

func (t *lanTries) done(id peer.ID, ok bool, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if ok {
		delete(t.m, id)
		return
	}
	if t.m == nil {
		t.m = map[peer.ID]lanTry{}
	}
	e := t.m[id]
	e.fails++
	e.next = now.Add(lanDelay(e.fails))
	t.m[id] = e
}

// tryRoutedLAN: игра уже идёт через relay, а адреса партнёра в локальной сети отброшены как «чужая подсеть».
// Если у нас с партнёром общий публичный IP (один роутер), пробуем эти адреса в фоне; успех даст прямое соединение,
// и канал игры перенесётся на него (maybeMigrate). Неудача откладывает следующую попытку (lanDelay).
func (n *Node) tryRoutedLAN(id peer.ID, lan []string) {
	deadline := time.Now().Add(3 * time.Second) // identify приходит чуть позже соединения
	for {
		if n.hasDirect(id) {
			return
		}
		if sharesNAT(n.publicIPs(), n.h.Peerstore().Addrs(id)) {
			break
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-n.ctx.Done():
			return
		case <-time.After(300 * time.Millisecond):
		}
	}
	if !n.lan.allow(id, time.Now()) {
		return
	}
	ctx, cancel := context.WithTimeout(n.ctx, 4*time.Second)
	defer cancel()
	t0 := time.Now()
	err := n.connectAddrs(network.WithForceDirectDial(ctx, "preferans-lan"), id, lan, false)
	ok := err == nil && n.hasDirect(id)
	n.lan.done(id, ok, time.Now())
	if ok {
		n.logf("прямое подключение к %s по локальной сети (общий роутер) установлено за %s", short(id), time.Since(t0).Round(time.Millisecond))
		return
	}
	n.logf("партнёр %s за тем же роутером, но его локальные адреса недоступны (%v); следующая попытка не раньше чем через %s", short(id), err, lanDelay(n.lan.failsOf(id)))
}
