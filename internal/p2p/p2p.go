// Package p2p — транспорт игры поверх libp2p без собственного сервера: публичные bootstrap/relay
// узлы IPFS, прямое соединение через DCUtR (hole punching) и обычный поток с тем же кадрированием,
// что и у WebRTC DataChannel в internal/network. Peer ID удостоверяет участника криптографически.
package p2p

import (
	"bufio"
	"context"
	"crypto/rand"
	"os"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

const (
	Protocol   = "/preferans/1.0.0"
	MaxMessage = 8 * 1024 * 1024
)

// TestLoopback включает режим LoopbackOnly для всех узлов процесса (тесты не трогают сетевые интерфейсы).
var TestLoopback bool

// Config. LoopbackOnly нужен тестам: только 127.0.0.1, без bootstrap, DHT и relay.
type Config struct {
	LoopbackOnly bool
	// KeyFile - файл постоянного ключа узла: peer ID не меняется между запусками (нужен для кода сервиса).
	KeyFile string
	// Logf получает диагностические сообщения сети (пробитие NAT, закрытие соединений). Не должен блокироваться.
	Logf func(string)
}

// Node — долгоживущий узел приложения.
type Node struct {
	h      host.Host
	kd     *dht.IpfsDHT
	cancel context.CancelFunc
	ctx    context.Context
	cfg    Config
	advert bool
	cs     convSet
	netMu  sync.Mutex
	netAt  time.Time
	since  int64
	tagSeq atomic.Int64
	watchMu sync.Mutex
	watch   map[peer.ID]string

	mu     sync.Mutex
	accept func(*Conn)
}

func NewNode(cfg Config) (*Node, error) {
	if TestLoopback || loopbackEnv() { // переменная — для UI-тестов с headless-приложением
		cfg.LoopbackOnly = true
	}
	ctx, cancel := context.WithCancel(context.Background())
	n := &Node{cfg: cfg, cancel: cancel, ctx: ctx, since: time.Now().Unix()}
	var opts []libp2p.Option
	if cfg.LoopbackOnly {
		opts = []libp2p.Option{libp2p.ListenAddrStrings("/ip4/127.0.0.1/udp/0/quic-v1"), libp2p.DisableRelay()}
	} else {
		peerSource := func(ctx context.Context, num int) <-chan peer.AddrInfo {
			ch := make(chan peer.AddrInfo, num)
			go func() {
				defer close(ch)
				for ctx.Err() == nil {
					if n.kd != nil && n.h != nil {
						sent := 0
						for _, id := range n.kd.RoutingTable().ListPeers() {
							select {
							case ch <- peer.AddrInfo{ID: id, Addrs: n.h.Peerstore().Addrs(id)}:
								sent++
							case <-ctx.Done():
								return
							}
							if sent >= num {
								return
							}
						}
						if sent > 0 {
							return
						}
					}
					select {
					case <-time.After(2 * time.Second):
					case <-ctx.Done():
						return
					}
				}
			}()
			return ch
		}
		opts = []libp2p.Option{
			libp2p.ListenAddrStrings("/ip4/0.0.0.0/udp/0/quic-v1", "/ip4/0.0.0.0/tcp/0", "/ip6/::/udp/0/quic-v1", "/ip6/::/tcp/0"),
			libp2p.NATPortMap(),
			// Детектор «чёрных дыр» после потери сети (блокировка телефона) надолго запрещал все QUIC-попытки.
			libp2p.UDPBlackHoleSuccessCounter(nil),
			libp2p.IPv6BlackHoleSuccessCounter(nil),
			libp2p.EnableRelay(),
			libp2p.EnableHolePunching(holepunch.WithTracer(hpTracer{n})),
			libp2p.EnableAutoNATv2(),
			libp2p.EnableAutoRelayWithPeerSource(peerSource, autorelay.WithMinCandidates(2), autorelay.WithNumRelays(2), autorelay.WithBootDelay(0)),
		}
	}
	if cfg.KeyFile != "" {
		priv, err := loadKey(cfg.KeyFile)
		if err != nil {
			cancel()
			return nil, err
		}
		opts = append(opts, libp2p.Identity(priv))
		if st, err := os.Stat(cfg.KeyFile); err == nil {
			n.since = st.ModTime().Unix()
		}
	}
	h, err := libp2p.New(opts...)
	if err != nil {
		cancel()
		return nil, err
	}
	n.h = h
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, c network.Conn) {
			n.logConn("открыто", c)
			n.maybeMigrate(c)
		},
		DisconnectedF: func(_ network.Network, c network.Conn) { n.logConn("закрыто", c) },
	})
	h.SetStreamHandler(Protocol, n.onStream)
	if !cfg.LoopbackOnly {
		kd, err := dht.New(h, dht.Mode(dht.ModeClient), dht.BootstrapPeers(dht.GetDefaultBootstrapPeerAddrInfos()...))
		if err != nil {
			h.Close()
			cancel()
			return nil, err
		}
		n.kd = kd
		go func() {
			for _, ai := range dht.GetDefaultBootstrapPeerAddrInfos() {
				go func() {
					c, cc := context.WithTimeout(ctx, 20*time.Second)
					defer cc()
					_ = h.Connect(c, ai)
				}()
			}
			_ = kd.Bootstrap(ctx)
		}()
	}
	return n, nil
}

func (n *Node) ID() string { return n.h.ID().String() }

func (n *Node) Close() {
	n.cancel()
	if n.kd != nil {
		_ = n.kd.Close()
	}
	_ = n.h.Close()
}

// OnAccept задаёт обработчик входящих соединений. Conn передаётся не запущенным:
// обработчик выставляет Message/Status/Open и вызывает Start.
func (n *Node) OnAccept(f func(*Conn)) {
	n.mu.Lock()
	n.accept = f
	n.mu.Unlock()
}

func (n *Node) onStream(s network.Stream) {
	n.mu.Lock()
	f := n.accept
	n.mu.Unlock()
	if f == nil {
		_ = s.Reset()
		return
	}
	f(newConn(n, s))
}

// Invite — всё, что нужно второй стороне для соединения (без SDP и без ответного кода).
type Invite struct {
	ID    string   `json:"id"`
	Addrs []string `json:"a"`
}

// Invite ждёт резервирование на публичном relay (до wait) и возвращает компактное приглашение:
// не более 3 разных relay и 2 публичных прямых адресов.
func (n *Node) Invite(ctx context.Context, wait time.Duration) (Invite, error) {
	inv := Invite{ID: n.h.ID().String()}
	deadline := time.Now().Add(wait)
	for {
		inv.Addrs = n.pick()
		if n.cfg.LoopbackOnly || len(inv.Addrs) > 0 && hasRelay(inv.Addrs) {
			return inv, nil
		}
		if time.Now().After(deadline) {
			if len(inv.Addrs) > 0 {
				return inv, nil // хотя бы прямые адреса (например IPv6)
			}
			return inv, errors.New("не удалось получить адрес для приглашения (нет доступа к публичным relay)")
		}
		select {
		case <-ctx.Done():
			return inv, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func hasRelay(a []string) bool {
	for _, s := range a {
		if strings.Contains(s, "/p2p-circuit") {
			return true
		}
	}
	return false
}

func (n *Node) pick() []string {
	var out []string
	seen := map[string]bool{}
	relays, direct := 0, 0
	type lanCand struct {
		s    string
		rank int
	}
	var lans []lanCand
	outIP := outboundIP()
	virtual := virtualInterfaceIPs()
	addLan := func(a ma.Multiaddr) {
		s := a.String()
		if seen[s] {
			return
		}
		if r := lanRank(a, outIP, virtual); r >= 0 {
			seen[s] = true
			lans = append(lans, lanCand{s, r})
		}
	}
	for _, a := range n.h.Addrs() {
		s := a.String()
		if k := strings.Index(s, "/p2p-circuit"); k >= 0 {
			id := s[strings.LastIndex(s[:k], "/")+1 : k]
			if seen[id] || relays >= 3 {
				continue
			}
			seen[id] = true
			relays++
			out = append(out, s)
			continue
		}
		switch {
		case n.cfg.LoopbackOnly:
			out = append(out, s)
		case manet.IsPublicAddr(a):
			if direct < 2 {
				direct++
				out = append(out, s)
			}
		case manet.IsPrivateAddr(a) && !manet.IsIPLoopback(a):
			addLan(a)
		}
	}
	if !n.cfg.LoopbackOnly && outIP != nil {
		// Адрес маршрута по умолчанию добавляем всегда: на Android libp2p не видит интерфейсов вовсе,
		// а на ПК с виртуальными адаптерами (Hyper-V, VirtualBox, VPN) нужный адрес легко теряется среди лишних.
		for _, s := range lanAddrs(n.h.Network().ListenAddresses(), outIP) {
			if m, err := ma.NewMultiaddr(s); err == nil {
				addLan(m)
			}
		}
	}
	sort.SliceStable(lans, func(i, j int) bool { return lans[i].rank < lans[j].rank })
	for i, c := range lans {
		if i >= 6 {
			break
		}
		out = append(out, c.s)
	}
	return out
}

// lanRank оценивает адрес локальной сети: меньше — лучше; -1 — не годится для приглашения.
// Лучший — адрес на интерфейсе маршрута по умолчанию, затем частные IPv4, затем ULA IPv6, затем CGNAT (VPN вроде Tailscale).
func lanRank(a ma.Multiaddr, outIP net.IP, virtual map[string]bool) int {
	first, _ := ma.SplitFirst(a)
	if first == nil || (first.Protocol().Code != ma.P_IP4 && first.Protocol().Code != ma.P_IP6) {
		return -1
	}
	ip := net.IP(first.RawValue())
	switch {
	case ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified():
		return -1
	case outIP != nil && ip.Equal(outIP):
		return 0
	case virtual[ip.String()]:
		return -1 // виртуальные адаптеры (Hyper-V, VirtualBox, Docker): снаружи недоступны
	case ip.To4() != nil && ip.IsPrivate():
		return 1
	case ip.To4() == nil && ip.IsPrivate():
		return 2
	default:
		return 3
	}
}
// directDialTimeout — сколько ждём прямое соединение (локальная сеть, публичный адрес), прежде чем идти через relay.
const directDialTimeout = 6 * time.Second

func (n *Node) logf(format string, args ...any) {
	if n.cfg.Logf != nil {
		n.cfg.Logf(fmt.Sprintf(format, args...))
	}
}

func (n *Node) hasDirect(id peer.ID) bool {
	for _, c := range n.h.Network().ConnsToPeer(id) {
		if !c.Stat().Limited {
			return true
		}
	}
	return false
}

// protect защищает соединения с участниками игры и их relay от обрезки диспетчером соединений.
func (n *Node) protect(ids ...peer.ID) func() {
	tag := fmt.Sprintf("preferans-%d", n.tagSeq.Add(1))
	n.watchMu.Lock()
	if n.watch == nil {
		n.watch = map[peer.ID]string{}
	}
	for i, id := range ids {
		n.h.ConnManager().Protect(id, tag)
		role := "relay"
		if i == 0 {
			role = "участник"
		}
		n.watch[id] = role
	}
	n.watchMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			for _, id := range ids {
				n.h.ConnManager().Unprotect(id, tag)
			}
		})
	}
}

func splitAddrs(addrs []string) (direct, relay []string) {
	for _, a := range addrs {
		if strings.Contains(a, "/p2p-circuit") {
			relay = append(relay, a)
		} else {
			direct = append(direct, a)
		}
	}
	return
}

func relayPeers(relay []string) []peer.ID {
	var out []peer.ID
	for _, a := range relay {
		k := strings.Index(a, "/p2p-circuit")
		if id, err := peer.Decode(a[strings.LastIndex(a[:k], "/")+1 : k]); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func (n *Node) connectAddrs(ctx context.Context, id peer.ID, addrs []string, limitedOK bool) error {
	ai := peer.AddrInfo{ID: id}
	for _, s := range addrs {
		if m, err := ma.NewMultiaddr(s); err == nil {
			ai.Addrs = append(ai.Addrs, m)
		}
	}
	if len(ai.Addrs) == 0 {
		return errors.New("в приглашении нет подходящих адресов")
	}
	if limitedOK {
		ctx = network.WithAllowLimitedConn(ctx, "preferans")
	}
	return n.h.Connect(ctx, ai)
}

// Dial соединяется по приглашению. Сначала пробует прямые адреса (локальная сеть, публичный адрес),
// и только если не вышло — через relay. Возвращает Conn, не запущенный (см. Start).
func (n *Node) Dial(ctx context.Context, inv Invite) (*Conn, error) {
	id, err := peer.Decode(inv.ID)
	if err != nil {
		return nil, fmt.Errorf("неверный peer id: %w", err)
	}
	n.forgetBackoffFor(inv)
	direct, relay := splitAddrs(inv.Addrs)
	if all := len(direct); all > 0 && !n.cfg.LoopbackOnly {
		if direct = reachableDirect(direct, outboundIP()); len(direct) < all {
			n.logf("адреса локальной сети партнёра %s из другой подсети пропущены: %d из %d", short(id), all-len(direct), all)
		}
	}
	release := n.protect(append([]peer.ID{id}, relayPeers(relay)...)...)
	ok := false
	defer func() {
		if !ok {
			release()
		}
	}()
	if len(direct) > 0 && !n.hasDirect(id) {
		dctx, cancel := context.WithTimeout(ctx, directDialTimeout)
		t0 := time.Now()
		if err := n.connectAddrs(network.WithForceDirectDial(dctx, "preferans-direct"), id, direct, false); err != nil {
			n.logf("прямое подключение к %s не удалось за %s: %v", short(id), time.Since(t0).Round(time.Millisecond), err)
		} else if n.hasDirect(id) {
			n.logf("прямое подключение к %s установлено за %s", short(id), time.Since(t0).Round(time.Millisecond))
		} else {
			n.logf("прямое подключение к %s: соединение есть, но только через relay", short(id))
		}
		cancel()
	}
	if !n.hasDirect(id) {
		if len(relay) == 0 {
			return nil, errors.New("прямое подключение не удалось, а адресов relay в приглашении нет")
		}
		if err := n.connectAddrs(ctx, id, relay, true); err != nil {
			return nil, fmt.Errorf("соединение через relay: %w", err)
		}
		n.logf("подключение к %s через relay (прямое пока не получилось)", short(id))
	}
	s, err := n.h.NewStream(network.WithAllowLimitedConn(ctx, "preferans"), id, Protocol)
	if err != nil {
		return nil, fmt.Errorf("поток: %w", err)
	}
	ok = true
	c := newConn(n, s)
	inner := c.release
	c.release = func() {
		inner()
		release()
	}
	return c, nil
}

func short(id peer.ID) string {
	s := id.String()
	if len(s) > 8 {
		return s[len(s)-8:]
	}
	return s
}
// Conn — замена network.Peer: те же колбэки и тот же Send(any).
type Conn struct {
	n       *Node
	s       network.Stream
	Message func([]byte)
	Status  func(string)
	Open    func()

	sendMu  sync.Mutex
	started atomic.Bool
	closed  atomic.Bool
	release func() // снимает защиту соединений от обрезки
}

func newConn(n *Node, s network.Stream) *Conn {
	c := &Conn{n: n, s: s}
	c.release = n.protect(s.Conn().RemotePeer())
	n.trackConn(c)
	return c
}

func (c *Conn) releaseProtection() {
	if c.release != nil {
		c.release()
	}
}

func (c *Conn) status(v string) {
	if c.Status != nil {
		c.Status(v)
	}
}

// Start запускает чтение: вызывает Status("connected"), Open и затем Message на каждый кадр.
func (c *Conn) Start() {
	if !c.started.CompareAndSwap(false, true) {
		return
	}
	c.status("connected")
	if c.Open != nil {
		c.Open()
	}
	go c.readLoop()
}

func (c *Conn) readLoop() {
	r := bufio.NewReaderSize(c.s, 64*1024)
	for {
		n, err := binary.ReadUvarint(r)
		if err == nil && (n == 0 || n > MaxMessage) {
			err = errors.New("недопустимая длина сообщения")
		}
		var b []byte
		if err == nil {
			b = make([]byte, n)
			_, err = io.ReadFull(r, b)
		}
		if err != nil {
			if c.closed.Load() {
				return
			}
			c.releaseProtection()
			c.n.untrackConn(c)
			c.status("disconnected")
			_ = c.s.Reset()
			return
		}
		if c.Message != nil {
			c.Message(b)
		}
	}
}

func (c *Conn) Send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > MaxMessage {
		return errors.New("сообщение слишком большое")
	}
	var hdr [binary.MaxVarintLen64]byte
	k := binary.PutUvarint(hdr[:], uint64(len(b)))
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	_ = c.s.SetWriteDeadline(time.Now().Add(20 * time.Second))
	if _, err = c.s.Write(append(hdr[:k:k], b...)); err != nil {
		return err
	}
	return nil
}

func (c *Conn) Close() {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	_ = c.s.Close()
	c.releaseProtection()
	c.n.untrackConn(c)
	c.status("closed")
}

// DiagnosticRoute описывает маршрут для журнала приложения (аналог Peer.DiagnosticRoute).
func (c *Conn) DiagnosticRoute() (channel, route, local, remote string) {
	cn := c.s.Conn()
	kind := "прямое"
	if strings.Contains(cn.RemoteMultiaddr().String(), "/p2p-circuit") {
		kind = "через relay"
	}
	return "libp2p", kind, cn.LocalMultiaddr().String(), cn.RemoteMultiaddr().String()
}

// RemoteID — криптографически подтверждённый peer ID собеседника.
func (c *Conn) RemoteID() string { return c.s.Conn().RemotePeer().String() }
func (n *Node) connect(ctx context.Context, inv Invite) error {
	id, err := peer.Decode(inv.ID)
	if err != nil {
		return fmt.Errorf("неверный peer id: %w", err)
	}
	ai := peer.AddrInfo{ID: id}
	for _, s := range inv.Addrs {
		m, err := ma.NewMultiaddr(s)
		if err != nil {
			continue
		}
		ai.Addrs = append(ai.Addrs, m)
	}
	if len(ai.Addrs) == 0 {
		return errors.New("в приглашении нет адресов")
	}
	n.forgetBackoffFor(inv)
	if err := n.h.Connect(network.WithAllowLimitedConn(ctx, "preferans"), ai); err != nil {
		return fmt.Errorf("соединение: %w", err)
	}
	return nil
}

func loadKey(path string) (crypto.PrivKey, error) {
	if b, err := os.ReadFile(path); err == nil {
		return crypto.UnmarshalPrivateKey(b)
	}
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	b, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	return priv, nil
}
func loopbackEnv() bool { return os.Getenv("PREFERANS_P2P_LOOPBACK") == "1" }

// logConn пишет в журнал события соединений только с участниками игры и их relay.
func (n *Node) logConn(what string, c network.Conn) {
	n.watchMu.Lock()
	role, ok := n.watch[c.RemotePeer()]
	n.watchMu.Unlock()
	if !ok {
		return
	}
	kind := "прямое"
	if c.Stat().Limited {
		kind = "через relay, с ограничениями"
	}
	age := ""
	if what == "закрыто" {
		age = fmt.Sprintf(", прожило %s", time.Since(c.Stat().Opened).Round(time.Second))
	}
	n.logf("соединение с %s (%s) %s: %s%s", short(c.RemotePeer()), role, what, kind, age)
}

// hpTracer переводит события пробития NAT (DCUtR) в журнал.
type hpTracer struct{ n *Node }

func (t hpTracer) Trace(e *holepunch.Event) {
	switch ev := e.Evt.(type) {
	case *holepunch.StartHolePunchEvt:
		t.n.logf("пробитие NAT с %s: начато, адресов партнёра %d", short(e.Remote), len(ev.RemoteAddrs))
	case *holepunch.EndHolePunchEvt:
		if ev.Success {
			t.n.logf("пробитие NAT с %s: успешно за %s", short(e.Remote), ev.EllapsedTime.Round(time.Millisecond))
		} else {
			t.n.logf("пробитие NAT с %s: не удалось за %s: %s", short(e.Remote), ev.EllapsedTime.Round(time.Millisecond), ev.Error)
		}
	case *holepunch.DirectDialEvt:
		if ev.Success {
			t.n.logf("прямое соединение с %s по адресам партнёра: успешно за %s", short(e.Remote), ev.EllapsedTime.Round(time.Millisecond))
		} else {
			t.n.logf("прямое соединение с %s по адресам партнёра: не удалось: %s", short(e.Remote), ev.Error)
		}
	case *holepunch.ProtocolErrorEvt:
		t.n.logf("пробитие NAT с %s: ошибка протокола: %s", short(e.Remote), ev.Error)
	}
}

var (
	outboundMu   sync.Mutex
	outboundAt   time.Time
	outboundAddr net.IP
)

// outboundIP возвращает локальный IPv4-адрес маршрута по умолчанию (адрес в локальной сети). Пакет не отправляется:
// connect у UDP-сокета только выбирает маршрут. Результат кэшируется на 10 секунд.
func outboundIP() net.IP {
	outboundMu.Lock()
	defer outboundMu.Unlock()
	if time.Since(outboundAt) < 10*time.Second {
		return outboundAddr
	}
	outboundAt, outboundAddr = time.Now(), nil
	c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 9})
	if err != nil {
		return nil
	}
	defer c.Close()
	if u, ok := c.LocalAddr().(*net.UDPAddr); ok && u.IP.To4() != nil && !u.IP.IsUnspecified() && !u.IP.IsLoopback() {
		outboundAddr = u.IP.To4()
	}
	return outboundAddr
}

// lanAddrs подставляет ip вместо 0.0.0.0 в адресах, на которых слушает узел (TCP и QUIC).
func lanAddrs(listen []ma.Multiaddr, ip net.IP) []string {
	var out []string
	for _, a := range listen {
		first, rest := ma.SplitFirst(a)
		if first == nil || first.Protocol().Code != ma.P_IP4 || !net.IP(first.RawValue()).IsUnspecified() {
			continue
		}
		host, err := ma.NewComponent("ip4", ip.String())
		if err != nil {
			continue
		}
		out = append(out, host.Encapsulate(rest).String())
	}
	return out
}