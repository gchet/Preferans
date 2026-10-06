// Package adminweb serves the separate administrator site and its session API.
package adminweb

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"preferans/internal/rooms"
	"preferans/internal/storage"
	"strings"
	"sync"
	"time"
)

type credentials struct {
	Hash    string `json:"hash"`
	Initial bool   `json:"initial"`
}
type session struct {
	CSRF  string
	Until time.Time
}
type failures struct {
	Count int
	Until time.Time
}
type Handler struct {
	mu          sync.Mutex
	store       storage.Store
	credentials credentials
	sessions    map[[32]byte]session
	failures    map[string]failures
	directory   *rooms.Server
}

func New(dir string, directory *rooms.Server) (*Handler, error) {
	h := &Handler{store: storage.Store{Dir: dir}, directory: directory, sessions: map[[32]byte]session{}, failures: map[string]failures{}}
	if err := h.store.Load("admin-auth", &h.credentials); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		h.credentials = credentials{Hash: string(hash), Initial: true}
		if err = h.store.Save("admin-auth", h.credentials); err != nil {
			return nil, err
		}
	}
	if _, err := bcrypt.Cost([]byte(h.credentials.Hash)); err != nil {
		return nil, err
	}
	return h, nil
}
func token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func (h *Handler) setPassword(password string) error {
	if len(password) < 8 || len(password) > 72 {
		return errors.New("password_length")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	next := credentials{Hash: string(hash)}
	if err = h.store.Save("admin-auth", next); err != nil {
		return err
	}
	h.credentials = next
	h.sessions = map[[32]byte]session{}
	return nil
}

// ResetPassword is used only by the local command-line recovery option.
func (h *Handler) ResetPassword(password string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.setPassword(password)
}

func response(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, status int, message string) {
	response(w, status, map[string]string{"error": message})
}
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host == r.Host
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if !strings.HasPrefix(r.URL.Path, "/admin/api/") {
		h.assets(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if r.TLS == nil && !net.ParseIP(ip).IsLoopback() {
		failure(w, 403, "https_required")
		return
	}
	if !sameOrigin(r) {
		failure(w, 403, "forbidden")
		return
	}
	if r.Method != "POST" && !(r.Method == "GET" && r.URL.Path == "/admin/api/session") {
		failure(w, 405, "method")
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.URL.Path == "/admin/api/login" {
		h.login(w, r)
		return
	}
	cookie, err := r.Cookie("preferans_admin")
	if err != nil {
		failure(w, 401, "login_required")
		return
	}
	key := sha256.Sum256([]byte(cookie.Value))
	s, ok := h.sessions[key]
	if !ok || !time.Now().Before(s.Until) {
		delete(h.sessions, key)
		failure(w, 401, "login_required")
		return
	}
	if r.Method == "POST" && subtle.ConstantTimeCompare([]byte(s.CSRF), []byte(r.Header.Get("X-Admin-CSRF"))) != 1 {
		failure(w, 403, "forbidden")
		return
	}
	switch r.URL.Path {
	case "/admin/api/session":
		response(w, 200, map[string]any{"csrf": s.CSRF, "initial": h.credentials.Initial})
	case "/admin/api/logout":
		delete(h.sessions, key)
		http.SetCookie(w, &http.Cookie{Name: "preferans_admin", Path: "/admin", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil})
		response(w, 200, map[string]bool{"ok": true})
	case "/admin/api/password":
		var q struct {
			Current  string `json:"current"`
			Password string `json:"password"`
		}
		if decode(w, r, &q) != nil {
			failure(w, 400, "request")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(h.credentials.Hash), []byte(q.Current)) != nil {
			failure(w, 403, "wrong_password")
			return
		}
		if len(q.Password) < 8 || len(q.Password) > 72 {
			failure(w, 400, "password_length")
			return
		}
		if err := h.setPassword(q.Password); err != nil {
			failure(w, 500, "save_failed")
			return
		}
		response(w, 200, map[string]bool{"ok": true})
	case "/admin/api/rooms":
		var q rooms.Request
		if decode(w, r, &q) != nil {
			failure(w, 400, "request")
			return
		}
		if q.Op != "list" && q.Op != "admin-delete-room" && q.Op != "admin-delete-player" && q.Op != "admin-delete-party" {
			failure(w, 400, "request")
			return
		}
		q.Secret = strings.Repeat("a", 32)
		q.Parties = nil
		out, err := h.directory.HandleAdmin(q)
		if err != nil {
			failure(w, 400, err.Error())
			return
		}
		response(w, 200, out)
	default:
		failure(w, 404, "not_found")
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("extra JSON")
	}
	return nil
}
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	now := time.Now()
	for address, value := range h.failures {
		if now.After(value.Until) {
			delete(h.failures, address)
		}
	}
	if len(h.failures) >= 4096 {
		failure(w, 429, "try_later")
		return
	}
	f := h.failures[ip]
	if f.Count >= 10 && now.Before(f.Until) {
		failure(w, 429, "try_later")
		return
	}
	var q struct {
		Password string `json:"password"`
	}
	if decode(w, r, &q) != nil {
		failure(w, 400, "request")
		return
	}
	if len(q.Password) > 72 || bcrypt.CompareHashAndPassword([]byte(h.credentials.Hash), []byte(q.Password)) != nil {
		if now.After(f.Until) {
			f = failures{Until: now.Add(time.Minute)}
		}
		f.Count++
		h.failures[ip] = f
		failure(w, 401, "wrong_password")
		return
	}
	delete(h.failures, ip)
	for key, s := range h.sessions {
		if now.After(s.Until) {
			delete(h.sessions, key)
		}
	}
	if len(h.sessions) >= 256 {
		failure(w, 429, "try_later")
		return
	}
	t := token()
	s := session{CSRF: token(), Until: now.Add(8 * time.Hour)}
	h.sessions[sha256.Sum256([]byte(t))] = s
	http.SetCookie(w, &http.Cookie{Name: "preferans_admin", Value: t, Path: "/admin", MaxAge: 8 * 3600, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil})
	response(w, 200, map[string]any{"csrf": s.CSRF, "initial": h.credentials.Initial})
}
