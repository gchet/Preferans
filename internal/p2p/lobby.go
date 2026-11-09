package p2p

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// LobbyTopic — общая тема публичного лобби на ntfy.sh. Все копии приложения читают её,
// а ведущий (самая старая установка среди доступных) периодически публикует там свой сервис комнат.
const LobbyTopic = "preferans-lobby-v1"

// Announcement — объявление сервиса комнат. Прямые адреса (с вашим публичным IP) не публикуются:
// только peer ID и адреса публичных relay.
type Announcement struct {
	V     int      `json:"v"`
	ID    string   `json:"id"`
	Addrs []string `json:"a"`
	Since int64    `json:"s"` // когда установлено приложение (чем раньше, тем выше приоритет)
	At    int64    `json:"t"` // когда опубликовано
}

// Since возвращает время первой установки узла (время создания файла ключа), unix-секунды.
func (n *Node) Since() int64 { return n.since }

// relayAddrs — до трёх адресов разных relay (в тестах без сети — любые адреса узла).
func (n *Node) relayAddrs() []string {
	var out []string
	seen := map[string]bool{}
	for _, a := range n.h.Addrs() {
		s := a.String()
		k := strings.Index(s, "/p2p-circuit")
		if k < 0 {
			if n.cfg.LoopbackOnly && len(out) < 3 {
				out = append(out, s)
			}
			continue
		}
		id := s[strings.LastIndex(s[:k], "/")+1 : k]
		if seen[id] || len(out) >= 3 {
			continue
		}
		seen[id] = true
		out = append(out, s)
	}
	return out
}

// Announce публикует объявление в лобби. Если адресов ещё нет (relay не зарезервирован), ничего не делает.
func (n *Node) Announce(ctx context.Context, since int64) error {
	addrs := n.relayAddrs()
	if len(addrs) == 0 {
		return fmt.Errorf("нет адресов relay для объявления")
	}
	b, _ := json.Marshal(Announcement{V: 1, ID: n.h.ID().String(), Addrs: addrs, Since: since, At: time.Now().Unix()})
	return ntfyPost(ctx, LobbyTopic, b)
}

// Invite превращает объявление в приглашение для запросов к сервису.
func (a Announcement) Invite() Invite { return Invite{ID: a.ID, Addrs: a.Addrs} }

// ReadLobby читает объявления за последние window, оставляет по самому свежему на каждый peer ID.
func ReadLobby(ctx context.Context, window time.Duration) ([]Announcement, error) {
	bodies, err := ntfyGet(ctx, fmt.Sprintf("%s/json?poll=1&since=%ds", LobbyTopic, int(window.Seconds())), 4<<20)
	if err != nil {
		return nil, err
	}
	latest := map[string]Announcement{}
	for _, body := range bodies {
		sc := bufio.NewScanner(bytes.NewReader(body))
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var e struct{ Event, Message string }
			if json.Unmarshal(bytes.TrimSpace(sc.Bytes()), &e) != nil || e.Event != "message" {
				continue
			}
			var an Announcement
			if json.Unmarshal([]byte(e.Message), &an) != nil || !validAnnouncement(an) {
				continue
			}
			if old, ok := latest[an.ID]; !ok || an.At > old.At {
				latest[an.ID] = an
			}
		}
	}
	out := make([]Announcement, 0, len(latest))
	for _, an := range latest {
		out = append(out, an)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func validAnnouncement(a Announcement) bool {
	if a.V != 1 || a.Since <= 0 || a.At <= 0 || len(a.Addrs) == 0 || len(a.Addrs) > 6 {
		return false
	}
	if _, err := peer.Decode(a.ID); err != nil {
		return false
	}
	for _, s := range a.Addrs {
		if len(s) > 220 || !strings.HasPrefix(s, "/") {
			return false
		}
		// публикуем и принимаем только relay-адреса (в тестах без сети разрешены любые)
		if !TestLoopbackEnabled() && !strings.Contains(s, "/p2p-circuit") {
			return false
		}
	}
	return true
}

// TestLoopbackEnabled — режим без публичной сети (тесты и PREFERANS_P2P_LOOPBACK=1).
func TestLoopbackEnabled() bool { return TestLoopback || loopbackEnv() }

// ChooseHost выбирает ведущего лобби среди живых объявлений: побеждает самая старая установка
// (при равенстве — меньший peer ID). Объявления старше maxAge считаются устаревшими.
func ChooseHost(anns []Announcement, now time.Time, maxAge time.Duration) (Announcement, bool) {
	var best Announcement
	found := false
	for _, a := range anns {
		// At хранится с точностью до секунды (усечена вниз), поэтому возраст завышен меньше чем на секунду: вычитаем её,
		// иначе при коротком maxAge (в тестах) свежее объявление считалось бы устаревшим.
		age := now.Sub(time.Unix(a.At, 0)) - time.Second
		if age > maxAge || age < -5*time.Minute {
			continue
		}
		if !found || a.Since < best.Since || (a.Since == best.Since && a.ID < best.ID) {
			best, found = a, true
		}
	}
	return best, found
}