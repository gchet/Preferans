package p2p

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Серверы ntfy. По умолчанию один публичный (NtfyURL). Пользователь может перечислить несколько: объявления
// публикуются на всех, а читаются со всех параллельно, поэтому недоступность одного сервера ничего не ломает.
var (
	ntfyMu   sync.RWMutex
	ntfyList []string
)

const (
	maxNtfyServers = 5
	ntfyTimeout    = 8 * time.Second
)

// ParseNtfyServers разбирает список адресов серверов ntfy через «;» (пробелы и пустые элементы пропускаются).
// Пустая строка означает сервер по умолчанию и возвращает пустой список.
func ParseNtfyServers(s string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimRight(strings.TrimSpace(part), "/")
		if part == "" {
			continue
		}
		u, err := url.Parse(part)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(part) > 200 {
			return nil, fmt.Errorf("неверный адрес сервера ntfy: %q", part)
		}
		if !seen[part] {
			seen[part] = true
			out = append(out, part)
		}
	}
	if len(out) > maxNtfyServers {
		return nil, fmt.Errorf("слишком много серверов ntfy (не больше %d)", maxNtfyServers)
	}
	return out, nil
}

// SetNtfyServers задаёт список серверов ntfy; пустой список возвращает сервер по умолчанию.
func SetNtfyServers(list []string) {
	ntfyMu.Lock()
	ntfyList = append([]string(nil), list...)
	ntfyMu.Unlock()
}

func ntfyServers() []string {
	ntfyMu.RLock()
	defer ntfyMu.RUnlock()
	if len(ntfyList) == 0 {
		return []string{NtfyURL}
	}
	return append([]string(nil), ntfyList...)
}

func hostOf(server string) string {
	if u, err := url.Parse(server); err == nil && u.Host != "" {
		return u.Host
	}
	return server
}

// ntfyPost публикует сообщение в тему на всех серверах параллельно. Успех — если приняли хотя бы на одном.
func ntfyPost(ctx context.Context, topic string, body []byte) error {
	servers := ntfyServers()
	errs := make([]error, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequestWithContext(ctx, "POST", s+"/"+topic, bytes.NewReader(body))
			req.Header.Set("Content-Type", "text/plain")
			c := http.Client{Timeout: ntfyTimeout}
			resp, err := c.Do(req)
			if err != nil {
				errs[i] = fmt.Errorf("%s: %w", hostOf(s), err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				errs[i] = fmt.Errorf("%s: HTTP %d", hostOf(s), resp.StatusCode)
			}
		}()
	}
	wg.Wait()
	return firstSuccess(errs)
}

// ntfyGet читает путь (тема и параметры) со всех серверов параллельно и возвращает тела ответов тех, кто ответил.
func ntfyGet(ctx context.Context, path string, limit int64) ([][]byte, error) {
	servers := ntfyServers()
	errs := make([]error, len(servers))
	bodies := make([][]byte, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequestWithContext(ctx, "GET", s+"/"+path, nil)
			c := http.Client{Timeout: ntfyTimeout}
			resp, err := c.Do(req)
			if err != nil {
				errs[i] = fmt.Errorf("%s: %w", hostOf(s), err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				errs[i] = fmt.Errorf("%s: HTTP %d", hostOf(s), resp.StatusCode)
				return
			}
			b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
			if err != nil {
				errs[i] = fmt.Errorf("%s: %w", hostOf(s), err)
				return
			}
			bodies[i] = b
		}()
	}
	wg.Wait()
	var out [][]byte
	for i := range servers {
		if errs[i] == nil {
			out = append(out, bodies[i])
		}
	}
	if len(out) == 0 {
		return nil, firstSuccess(errs)
	}
	return out, nil
}

// firstSuccess возвращает nil, если хотя бы один запрос удался, иначе сводку ошибок всех серверов.
func firstSuccess(errs []error) error {
	var failed []string
	for _, e := range errs {
		if e == nil {
			return nil
		}
		failed = append(failed, e.Error())
	}
	if len(failed) == 0 {
		return errors.New("нет серверов ntfy")
	}
	return errors.New(strings.Join(failed, "; "))
}