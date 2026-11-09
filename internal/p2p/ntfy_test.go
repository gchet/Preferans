package p2p

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseNtfyServers(t *testing.T) {
	got, err := ParseNtfyServers(" https://ntfy.example.org/ ; http://192.168.1.5:8080;;https://ntfy.example.org ")
	if err != nil || len(got) != 2 || got[0] != "https://ntfy.example.org" || got[1] != "http://192.168.1.5:8080" {
		t.Fatalf("%v %v", got, err)
	}
	if got, err = ParseNtfyServers("  "); err != nil || len(got) != 0 {
		t.Fatal("пустая строка — сервер по умолчанию")
	}
	for _, bad := range []string{"ntfy.sh", "ftp://x.org", "https://", "a b", "https://a.org;https://b.org;https://c.org;https://d.org;https://e.org;https://f.org"} {
		if _, err = ParseNtfyServers(bad); err == nil {
			t.Fatalf("%q должен отвергаться", bad)
		}
	}
}

func TestLobbyWorksWhenOneNtfyServerIsDown(t *testing.T) {
	good, _ := lobbyStub(t)
	defer good.Close()
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close() // порт больше не слушает: соединение отклоняется
	old, oldLoop := NtfyURL, TestLoopback
	TestLoopback = true
	defer func() { NtfyURL, TestLoopback = old, oldLoop; SetNtfyServers(nil) }()
	SetNtfyServers([]string{deadURL, good.URL})
	n, err := NewNode(Config{LoopbackOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := n.Announce(ctx, 1000); err != nil {
		t.Fatalf("объявление должно уйти хотя бы на один сервер: %v", err)
	}
	list, err := ReadLobby(ctx, time.Minute)
	if err != nil || len(list) != 1 || list[0].ID != n.ID() {
		t.Fatalf("%+v %v", list, err)
	}
	// все серверы недоступны — ошибка со сводкой по серверам
	SetNtfyServers([]string{deadURL})
	if _, err = ReadLobby(ctx, time.Minute); err == nil {
		t.Fatal("без доступных серверов чтение должно вернуть ошибку")
	}
}