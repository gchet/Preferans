package p2p

import (
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Перенос игрового канала с relay на прямое соединение.
//
// Канал игры открывается через relay, пока не пробилось прямое соединение (DCUtR занимает от секунд до минуты).
// Поток остаётся на relay до его обрыва по лимиту (около 70 с – 2 мин), то есть игра рвётся посреди хода.
// Когда появляется прямое соединение, поток один раз сбрасывается в удобный момент: игровой слой переподключается
// штатным путём (1–3 с), а новое подключение уже идёт по прямому соединению.

const migrateDelay = 2 * time.Second       // даём прямому соединению «устояться»
const migrateCooldown = 30 * time.Second   // не чаще раза в 30 с на партнёра: нет петли, если прямое соединение нестабильно

type convSet struct {
	mu       sync.Mutex
	convs    map[*Conn]struct{}
	migrated map[peer.ID]time.Time
}

func (n *Node) trackConn(c *Conn) {
	n.cs.mu.Lock()
	defer n.cs.mu.Unlock()
	if n.cs.convs == nil {
		n.cs.convs = map[*Conn]struct{}{}
	}
	n.cs.convs[c] = struct{}{}
}

func (n *Node) untrackConn(c *Conn) {
	n.cs.mu.Lock()
	delete(n.cs.convs, c)
	n.cs.mu.Unlock()
}

// maybeMigrate вызывается при открытии соединения: если оно прямое, переносит каналы игры с relay на него.
func (n *Node) maybeMigrate(c network.Conn) {
	if c.Stat().Limited || n.cfg.LoopbackOnly {
		return
	}
	id := c.RemotePeer()
	go func() {
		time.Sleep(migrateDelay)
		if !n.hasDirect(id) {
			return
		}
		n.migrateLimited(id, func(cc *Conn) bool { return cc.s.Conn().Stat().Limited })
	}()
}

// migrateLimited сбрасывает потоки игры с партнёром id, для которых limited возвращает true. Возвращает их число.
func (n *Node) migrateLimited(id peer.ID, limited func(*Conn) bool) int {
	n.cs.mu.Lock()
	if n.cs.migrated == nil {
		n.cs.migrated = map[peer.ID]time.Time{}
	}
	var victims []*Conn
	for c := range n.cs.convs {
		if c.s.Conn().RemotePeer() == id && !c.closed.Load() && limited(c) {
			victims = append(victims, c)
		}
	}
	if len(victims) == 0 || time.Since(n.cs.migrated[id]) < migrateCooldown {
		n.cs.mu.Unlock()
		return 0
	}
	n.cs.migrated[id] = time.Now()
	n.cs.mu.Unlock()
	n.logf("канал игры с %s переносится с relay на прямое соединение", short(id))
	for _, c := range victims {
		_ = c.s.Reset() // readLoop увидит ошибку и сообщит «disconnected»: игровой слой переподключится
	}
	return len(victims)
}