// Package selfupdate проверяет новую версию на GitHub, скачивает файлы выпуска с проверкой SHA-256 и заменяет
// запущенную программу Windows. Установку на Android выполняет системный установщик (см. MainActivity).
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAPI        = "https://api.github.com"
	DefaultRepository = "gchet/Preferans"

	maxMetadata = 1 << 20   // ответ API и SHA256SUMS.txt
	maxDownload = 256 << 20 // файл выпуска
)

var (
	tagRe    = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)
	sumRe    = regexp.MustCompile(`^([0-9a-fA-F]{64})\s+\*?(.+?)\s*$`)
	ErrBad   = errors.New("selfupdate: release is incomplete or invalid")
	ErrHash  = errors.New("selfupdate: checksum mismatch")
	ErrHost  = errors.New("selfupdate: download address is not allowed")
	ErrLarge = errors.New("selfupdate: file is too large")
)

// Release — последний опубликованный выпуск с файлами для Windows и Android.
type Release struct {
	Tag       string
	Version   string // без «v»
	PageURL   string
	ExeURL    string
	ExeSHA256 string
	APKURL    string
	APKSHA256 string
}

// Client обращается к GitHub API. Поля API и Repository нужны тестам; AllowHost заменяет проверку адресов загрузки.
type Client struct {
	API        string
	Repository string
	HTTP       *http.Client
	AllowHost  func(*url.URL) bool
}

func New() *Client {
	c := &Client{API: DefaultAPI, Repository: DefaultRepository}
	c.HTTP = &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if !c.allowed(req.URL) {
			return ErrHost
		}
		return nil
	}}
	return c
}

func (c *Client) allowed(u *url.URL) bool {
	if c.AllowHost != nil {
		return c.AllowHost(u)
	}
	return AllowedDownloadHost(u)
}

// AllowedDownloadHost: только HTTPS и узлы GitHub, с которых отдаются файлы выпусков.
func AllowedDownloadHost(u *url.URL) bool {
	if u == nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "github.com" || host == "api.github.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

type apiRelease struct {
	Tag        string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (c *Client) get(ctx context.Context, rawURL, accept string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !c.allowed(u) {
		return nil, ErrHost
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Preferans-updater")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrLarge
	}
	return data, nil
}

// Latest возвращает последний опубликованный (не черновик, не пре-релиз) выпуск.
func (c *Client) Latest(ctx context.Context) (*Release, error) {
	body, err := c.get(ctx, strings.TrimRight(c.API, "/")+"/repos/"+c.Repository+"/releases/latest", "application/vnd.github+json", maxMetadata)
	if err != nil {
		return nil, err
	}
	var raw apiRelease
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBad, err)
	}
	if raw.Draft || raw.Prerelease || !tagRe.MatchString(raw.Tag) {
		return nil, ErrBad
	}
	r := &Release{Tag: raw.Tag, Version: strings.TrimPrefix(raw.Tag, "v"), PageURL: raw.HTMLURL}
	var sumsURL string
	for _, a := range raw.Assets {
		switch a.Name {
		case "Preferans.exe":
			r.ExeURL = a.URL
		case "Preferans.apk":
			r.APKURL = a.URL
		case "SHA256SUMS.txt":
			sumsURL = a.URL
		}
	}
	if r.ExeURL == "" || r.APKURL == "" || sumsURL == "" {
		return nil, ErrBad
	}
	sums, err := c.get(ctx, sumsURL, "", maxMetadata)
	if err != nil {
		return nil, err
	}
	hashes := map[string]string{}
	for _, line := range strings.Split(string(sums), "\n") {
		if m := sumRe.FindStringSubmatch(strings.TrimRight(line, "\r")); m != nil {
			hashes[m[2]] = strings.ToLower(m[1])
		}
	}
	r.ExeSHA256, r.APKSHA256 = hashes["Preferans.exe"], hashes["Preferans.apk"]
	if r.ExeSHA256 == "" || r.APKSHA256 == "" {
		return nil, ErrBad
	}
	return r, nil
}

// Newer: candidate строго новее current. Если current не похож на «x.y.z» (например, «dev»), обновление не предлагается.
func Newer(current, candidate string) bool {
	a, ok1 := parseVersion(current)
	b, ok2 := parseVersion(candidate)
	if !ok1 || !ok2 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return b[i] > a[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	m := tagRe.FindStringSubmatch("v" + strings.TrimPrefix(strings.TrimSpace(v), "v"))
	if m == nil {
		return out, false
	}
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Download скачивает файл в dest (через dest+".part"), проверяет SHA-256 и размер. progress вызывается по ходу загрузки.
func (c *Client) Download(ctx context.Context, rawURL, dest, wantSHA256 string, progress func(done, total int64)) error {
	u, err := url.Parse(rawURL)
	if err != nil || !c.allowed(u) {
		return ErrHost
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Preferans-updater")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxDownload {
		return ErrLarge
	}
	part := dest + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(part)
		}
	}()
	h := sha256.New()
	buf := make([]byte, 64<<10)
	var done int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			done += int64(n)
			if done > maxDownload {
				return ErrLarge
			}
			h.Write(buf[:n])
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			if progress != nil {
				progress(done, resp.ContentLength)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, wantSHA256) {
		return ErrHash
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(part, dest); err != nil {
		return err
	}
	ok = true
	return nil
}

// ReplaceExecutable подменяет current скачанным файлом: прежний файл остаётся рядом как «.old»
// (запущенный exe в Windows можно переименовать, но нельзя перезаписать). При ошибке прежний файл возвращается.
func ReplaceExecutable(current, downloaded string) error {
	old := current + ".old"
	_ = os.Remove(old)
	if err := os.Rename(current, old); err != nil {
		return err
	}
	if err := os.Rename(downloaded, current); err != nil {
		_ = os.Rename(old, current)
		return err
	}
	return nil
}

// CleanupOld удаляет «.old» от прошлого обновления (после перезапуска файл уже не занят).
func CleanupOld(current string) { _ = os.Remove(current + ".old") }
