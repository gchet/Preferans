package p2p

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
)

// clearBackoff сбрасывает «штраф» swarm за неудачные соединения с узлами: после потери сети (блокировка телефона,
// смена Wi-Fi/LTE) он растёт до минут и отказывает в новых попытках, хотя сеть уже вернулась.
func (n *Node) clearBackoff(ids ...peer.ID) {
	sw, ok := n.h.Network().(*swarm.Swarm)
	if !ok {
		return
	}
	for _, id := range ids {
		sw.Backoff().Clear(id)
	}
}

// NetworkChanged вызывается, когда сеть могла измениться или приложение вернулось из фона (экран разблокирован,
// смена Wi-Fi/LTE). Сбрасывает штрафы повторных попыток, проверяет живость соединений (ping) и закрывает мёртвые,
// чтобы новые попытки не упирались в зависшие сокеты. Возвращает число закрытых соединений; не чаще раза в 2 с.
func (n *Node) NetworkChanged() int {
	n.netMu.Lock()
	if time.Since(n.netAt) < 2*time.Second {
		n.netMu.Unlock()
		return 0
	}
	n.netAt = time.Now()
	n.netMu.Unlock()
	if n.cfg.LoopbackOnly {
		return 0
	}
	n.clearBackoff(n.h.Peerstore().Peers()...)
	peers := n.h.Network().Peers()
	var wg sync.WaitGroup
	var mu sync.Mutex
	dead := 0
	for _, id := range peers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(n.ctx, 3*time.Second)
			defer cancel()
			select {
			case r := <-ping.Ping(ctx, n.h, id):
				if r.Error == nil {
					return
				}
			case <-ctx.Done():
			}
			_ = n.h.Network().ClosePeer(id)
			mu.Lock()
			dead++
			mu.Unlock()
		}()
	}
	wg.Wait()
	n.clearBackoff(n.h.Peerstore().Peers()...)
	if n.kd != nil {
		go func() { _ = n.kd.Bootstrap(n.ctx) }()
	}
	n.logf("сеть изменилась или приложение вернулось: соединений проверено %d, закрыто мёртвых %d", len(peers), dead)
	return dead
}

// connectFresh соединяется с узлом, предварительно сбросив штрафы повторных попыток для него и его relay.
func (n *Node) forgetBackoffFor(inv Invite) {
	ids := []peer.ID{}
	if id, err := peer.Decode(inv.ID); err == nil {
		ids = append(ids, id)
	}
	_, relay := splitAddrs(inv.Addrs)
	ids = append(ids, relayPeers(relay)...)
	n.clearBackoff(ids...)
}

var _ = network.Connected

// virtualInterfaceIPs возвращает адреса интерфейсов виртуальных машин, контейнеров и мостов (Hyper-V, VirtualBox,
// VMware, Docker, WSL): другое устройство не может достичь их из своей сети, и они только раздувают приглашение.
// На Android перечисление интерфейсов недоступно — тогда список пуст.
func virtualInterfaceIPs() map[string]bool {
	out := map[string]bool{}
	ifs, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifs {
		if !isVirtualInterface(ifc.Name) {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				out[ipn.IP.String()] = true
			}
		}
	}
	return out
}

func isVirtualInterface(name string) bool {
	l := strings.ToLower(name)
	for _, p := range []string{"vethernet", "virtualbox", "vbox", "vmware", "vmnet", "hyper-v", "docker", "veth", "virbr", "br-", "wsl", "default switch"} {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}

// reachableDirect убирает из прямых адресов партнёра адреса локальной сети, до которых отсюда не дойти: они из другой подсети
// (телефон в LTE, партнёр в домашнем Wi-Fi). Попытка к ним ждёт таймаут (~5 с) при каждом восстановлении связи.
// Публичные адреса и IPv6 остаются. Если свой адрес неизвестен, список не меняется.
func reachableDirect(direct []string, outIP net.IP) []string {
	keep, _ := splitReachable(direct, outIP, nil)
	return keep
}

// splitReachable делит прямые адреса партнёра на достижимые и пропущенные (локальная сеть из другой подсети).
// mask — маска нашей сети: если известна, сравнивается точная подсеть; иначе (Android не отдаёт интерфейсы)
// грубо — по первым двум октетам.
func splitReachable(direct []string, outIP net.IP, mask net.IPMask) (keep, skipped []string) {
	out4 := outIP.To4()
	if out4 == nil {
		return direct, nil
	}
	for _, s := range direct {
		parts := strings.Split(s, "/")
		if len(parts) > 2 && parts[1] == "ip4" {
			if ip := net.ParseIP(parts[2]).To4(); ip != nil && (ip.IsPrivate() || ip[0] == 100 && ip[1]&0xc0 == 64) {
				same := ip[0] == out4[0] && ip[1] == out4[1]
				if len(mask) == net.IPv4len {
					same = ip.Mask(mask).Equal(out4.Mask(mask))
				}
				if !same {
					skipped = append(skipped, s)
					continue
				}
			}
		}
		keep = append(keep, s)
	}
	return keep, skipped
}

// interfaceMask возвращает маску сети, к которой принадлежит наш адрес ip (nil, если интерфейсы недоступны).
func interfaceMask(ip net.IP) net.IPMask {
	if ip.To4() == nil {
		return nil
	}
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifc := range ifs {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.Equal(ip) {
				if m := ipn.Mask; len(m) == net.IPv6len {
					return m[12:]
				} else if len(m) == net.IPv4len {
					return m
				}
			}
		}
	}
	return nil
}