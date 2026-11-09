package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"preferans/config"
	"preferans/internal/p2p"
	"preferans/internal/rooms"
	"preferans/locales"
)

// p2pNode создаёт узел libp2p при первом обращении (в тестах и без сети узел не поднимается).
func (a *App) p2pNode() (*p2p.Node, error) {
	a.nodeOnce.Do(func() {
		if err := os.MkdirAll(a.config.Dir, 0o700); err != nil {
			a.nodeErr = err
			return
		}
		n, err := p2p.NewNode(p2p.Config{KeyFile: filepath.Join(a.config.Dir, "p2p-identity"), Logf: a.p2pLog})
		if err != nil {
			a.nodeErr = err
			return
		}
		n.OnAccept(a.acceptConn)
		n.HandleRequest(a.serveRooms)
		n.StartAdvert()
		a.node = n
	})
	return a.node, a.nodeErr
}

// roomService — собственный сервис комнат этого устройства: тот же код, что был в cmd/signaling.
func (a *App) roomService() (*rooms.Server, error) {
	a.roomSvcOnce.Do(func() {
		a.roomSvc, a.roomSvcErr = rooms.New(filepath.Join(a.config.Dir, "room-service"))
	})
	return a.roomSvc, a.roomSvcErr
}

// serveRooms отвечает другим игрокам, которые используют это устройство как сервис комнат.
func (a *App) serveRooms(from string, req []byte) []byte {
	var q rooms.Request
	if json.Unmarshal(req, &q) != nil {
		b, _ := json.Marshal(rooms.Response{Error: "bad request"})
		return b
	}
	if len(q.Op) >= 13 && q.Op[:13] == "admin-delete-" {
		b, _ := json.Marshal(rooms.Response{Error: "forbidden"})
		return b
	}
	srv, err := a.roomService()
	var out rooms.Response
	if err != nil {
		out = rooms.Response{Error: err.Error()}
	} else if out, err = srv.Handle(q); err != nil {
		out = rooms.Response{Error: err.Error()}
	}
	b, _ := json.Marshal(out)
	return b
}

// ServiceCode возвращает постоянный код сервиса комнат этого устройства: его можно передать другим игрокам.
func (a *App) ServiceCode() (string, error) {
	n, err := a.p2pNode()
	if err != nil {
		return "", err
	}
	return p2p.ServiceCode(n.ID())
}

// roomCall отправляет запрос выбранному сервису комнат: собственному (url пуст) или чужому по коду.
func (a *App) roomCall(url string, q rooms.Request) (rooms.Response, error) {
	if url == "" {
		srv, err := a.roomService()
		if err != nil {
			return rooms.Response{}, err
		}
		return srv.Handle(q)
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return a.roomHTTP(url, q)
	}
	code, err := p2p.Decode(url)
	if err != nil {
		return rooms.Response{}, err
	}
	node, err := a.p2pNode()
	if err != nil {
		return rooms.Response{}, err
	}
	body, err := json.Marshal(q)
	if err != nil {
		return rooms.Response{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	raw, err := node.Request(ctx, code.Invite, body)
	if err != nil {
		return rooms.Response{}, err
	}
	var out rooms.Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return rooms.Response{}, err
	}
	if out.Error != "" {
		return out, errString(out.Error)
	}
	return out, nil
}

type errString string

func (e errString) Error() string { return string(e) }
// roomHTTP — служебный путь для тестов и разработки: сервис комнат по HTTP (задаётся только переменной
// окружения PREFERANS_ROOMS_URL; в интерфейсе и сохранённых настройках не поддерживается).
func (a *App) roomHTTP(url string, q rooms.Request) (rooms.Response, error) {
	b, err := json.Marshal(q)
	if err != nil {
		return rooms.Response{}, err
	}
	client := http.Client{Timeout: time.Duration(config.Interface.RoomRequestTimeoutMs) * time.Millisecond}
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return rooms.Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return rooms.Response{}, locales.Errorf("go.internal.app.rooms.text002", resp.StatusCode)
	}
	var out rooms.Response
	if err = json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 4*1024*1024)).Decode(&out); err != nil {
		return out, err
	}
	if out.Error != "" {
		return out, errString(out.Error)
	}
	return out, nil
}
// p2pLog принимает сообщения сети из libp2p без блокировки и пишет их в журнал приложения.
func (a *App) p2pLog(msg string) {
	a.p2pLogOnce.Do(func() {
		a.p2pLogs = make(chan string, 128)
		go func() {
			for {
				select {
				case <-a.closed:
					return
				case m := <-a.p2pLogs:
					a.mu.Lock()
					a.logLocked(locales.Text("go.internal.app.p2p.text001"), m)
					a.mu.Unlock()
				}
			}
		}()
	})
	select {
	case a.p2pLogs <- msg:
	default:
	}
}