package adminweb

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"preferans/internal/rooms"
	"strings"
	"testing"
)

func call(h http.Handler, path string, body any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	method := "POST"
	if body == nil {
		method = "GET"
	}
	r := httptest.NewRequest(method, "http://localhost/admin/api/"+path, bytes.NewReader(b))
	r.RemoteAddr = "127.0.0.1:1234"
	if cookie != nil {
		r.AddCookie(cookie)
	}
	r.Header.Set("X-Admin-CSRF", csrf)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func signin(t *testing.T, h http.Handler, password string) (*http.Cookie, string) {
	t.Helper()
	w := call(h, "login", map[string]string{"password": password}, nil, "")
	if w.Code != 200 {
		t.Fatal("login", w.Code, w.Body.String())
	}
	var s struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &s)
	return w.Result().Cookies()[0], s.CSRF
}
func TestPasswordAndAdminAuthorization(t *testing.T) {
	dir := t.TempDir()
	directory, _ := rooms.New(dir)
	room, err := directory.Handle(rooms.Request{Op: "create", Name: "Friends", Secret: strings.Repeat("x", 32)})
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(dir, directory)
	if err != nil {
		t.Fatal(err)
	}
	if call(h, "rooms", map[string]string{"op": "list"}, nil, "").Code != 401 {
		t.Fatal("anonymous admin read")
	}
	if call(h, "login", map[string]string{"password": "wrong"}, nil, "").Code != 401 {
		t.Fatal("wrong password accepted")
	}
	cookie, csrf := signin(t, h, "admin")
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	if call(h, "rooms", map[string]string{"op": "list"}, cookie, "").Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	if call(h, "rooms", map[string]string{"op": "list"}, cookie, csrf).Code != 200 {
		t.Fatal("admin directory unavailable")
	}
	if _, err = directory.Handle(rooms.Request{Op: "admin-delete-room", Room: room.Room.ID, Secret: strings.Repeat("x", 32)}); err == nil {
		t.Fatal("public deletion allowed")
	}
	if call(h, "password", map[string]string{"current": "wrong", "password": "new password"}, cookie, csrf).Code != 403 {
		t.Fatal("wrong current password accepted")
	}
	if call(h, "password", map[string]string{"current": "admin", "password": "short"}, cookie, csrf).Code != 400 {
		t.Fatal("short password accepted")
	}
	if call(h, "password", map[string]string{"current": "admin", "password": "new password"}, cookie, csrf).Code != 200 {
		t.Fatal("password change failed")
	}
	if call(h, "session", nil, cookie, "").Code != 401 {
		t.Fatal("old session survived password change")
	}
	h, err = New(dir, directory)
	if err != nil {
		t.Fatal(err)
	}
	if call(h, "login", map[string]string{"password": "admin"}, nil, "").Code != 401 {
		t.Fatal("initial password survived reset")
	}
	cookie, csrf = signin(t, h, "new password")
	data, _ := os.ReadFile(filepath.Join(dir, "admin-auth.json"))
	if bytes.Contains(data, []byte("new password")) {
		t.Fatal("password saved as plaintext")
	}
	if call(h, "rooms", map[string]string{"op": "admin-delete-room", "room": room.Room.ID}, cookie, csrf).Code != 200 {
		t.Fatal("authenticated deletion failed")
	}
	if call(h, "logout", map[string]string{}, cookie, csrf).Code != 200 || call(h, "session", nil, cookie, "").Code != 401 {
		t.Fatal("logout failed")
	}
}

func TestRemotePlainHTTPDoesNotAcceptPasswords(t *testing.T) {
	dir := t.TempDir()
	directory, _ := rooms.New(dir)
	h, _ := New(dir, directory)
	r := httptest.NewRequest("POST", "http://localhost/admin/api/login", strings.NewReader(`{"password":"admin"}`))
	r.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "https_required") {
		t.Fatal("remote plaintext login allowed")
	}
}

func TestOriginAndLoginThrottling(t *testing.T) {
	dir := t.TempDir()
	directory, _ := rooms.New(dir)
	h, _ := New(dir, directory)
	r := httptest.NewRequest("POST", "http://localhost/admin/api/login", strings.NewReader(`{"password":"admin"}`))
	r.Header.Set("Origin", "https://other.example")
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("foreign origin accepted")
	}
	for i := 0; i < 10; i++ {
		if call(h, "login", map[string]string{"password": "wrong"}, nil, "").Code != 401 {
			t.Fatal("unexpected throttle")
		}
	}
	if call(h, "login", map[string]string{"password": "admin"}, nil, "").Code != 429 {
		t.Fatal("login attempts not throttled")
	}
}
