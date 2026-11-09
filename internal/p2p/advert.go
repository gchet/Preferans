package p2p

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// NtfyURL — публичный сервис, через который узел объявляет свои свежие адреса.
// Тема выводится из peer ID: зная код сервиса, любой игрок находит текущие адреса.
var NtfyURL = "https://ntfy.sh"

const advertPeriod = 90 * time.Second

func topicFor(id string) string {
	h := sha256.Sum256([]byte("preferans-v2/" + id))
	return "preferans-" + hex.EncodeToString(h[:10])
}

// StartAdvert периодически публикует актуальное приглашение узла. Остановка: Close узла.
func (n *Node) StartAdvert() {
	if n.cfg.LoopbackOnly {
		return
	}
	n.mu.Lock()
	if n.advert {
		n.mu.Unlock()
		return
	}
	n.advert = true
	n.mu.Unlock()
	go func() {
		ctx := n.ctx
		var last string
		var lastAt time.Time
		for ctx.Err() == nil {
			ic, cancel := context.WithTimeout(ctx, 60*time.Second)
			inv, err := n.Invite(ic, 45*time.Second)
			cancel()
			if err == nil {
				b, _ := json.Marshal(inv)
				// публикуем при изменении адресов или не реже, чем раз в 10 минут
				if string(b) != last || time.Since(lastAt) > 10*time.Minute {
					if n.publish(ctx, string(b)) == nil {
						last, lastAt = string(b), time.Now()
					}
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(advertPeriod):
			}
		}
	}()
}

func (n *Node) publish(ctx context.Context, msg string) error {
	return ntfyPost(ctx, topicFor(n.h.ID().String()), []byte(msg))
}

// Resolve возвращает самое свежее объявленное приглашение узла.
func (n *Node) Resolve(ctx context.Context, id string) (Invite, error) {
	var inv Invite
	bodies, err := ntfyGet(ctx, topicFor(id)+"/json?poll=1&since=latest", 1<<20)
	if err != nil {
		return inv, fmt.Errorf("не удалось найти адреса сервиса: %w", err)
	}
	var msg string
	for _, body := range bodies {
		sc := bufio.NewScanner(bytes.NewReader(body))
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var e struct{ Event, Message string }
			if json.Unmarshal(bytes.TrimSpace(sc.Bytes()), &e) == nil && e.Event == "message" {
				msg = e.Message
			}
		}
		if msg != "" {
			break
		}
	}
	if msg == "" {
		return inv, errors.New("сервис комнат не объявлял адреса (хозяин не в сети?)")
	}
	if err := json.Unmarshal([]byte(msg), &inv); err != nil || inv.ID != id || len(inv.Addrs) == 0 {
		return inv, errors.New("объявление сервиса повреждено")
	}
	return inv, nil
}