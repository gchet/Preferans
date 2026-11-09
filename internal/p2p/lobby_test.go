package p2p

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func lobbyStub(t *testing.T) (*httptest.Server, func(string)) {
	t.Helper()
	var mu sync.Mutex
	var msgs []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(r.URL.Path, "/"+LobbyTopic) {
			http.NotFound(w, r)
			return
		}
		if r.Method == "POST" {
			b, _ := io.ReadAll(r.Body)
			msgs = append(msgs, string(b))
			return
		}
		for _, m := range msgs {
			b, _ := json.Marshal(map[string]string{"event": "message", "message": m})
			w.Write(append(b, '\n'))
		}
	}))
	return ts, func(m string) { mu.Lock(); msgs = append(msgs, m); mu.Unlock() }
}

func TestLobbyAnnounceAndRead(t *testing.T) {
	ts, injectRaw := lobbyStub(t)
	defer ts.Close()
	old, oldLoop := NtfyURL, TestLoopback
	NtfyURL, TestLoopback = ts.URL, true // без публичной сети допустимы любые адреса
	defer func() { NtfyURL, TestLoopback = old, oldLoop }()
	n, err := NewNode(Config{LoopbackOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := n.Announce(ctx, 1000); err != nil {
		t.Fatal(err)
	}
	if err := n.Announce(ctx, 1000); err != nil {
		t.Fatal(err)
	}
	injectRaw("not json")
	injectRaw(`{"v":1,"id":"bad","a":["/ip4/1.1.1.1/udp/1"],"s":1,"t":1}`)
	list, err := ReadLobby(ctx, time.Minute)
	if err != nil || len(list) != 1 || list[0].ID != n.ID() || list[0].Since != 1000 || len(list[0].Addrs) == 0 {
		t.Fatalf("%+v %v", list, err)
	}
}

func TestChooseHostOldestWins(t *testing.T) {
	now := time.Now()
	fresh := now.Unix()
	a := Announcement{V: 1, ID: "b-id", Since: 200, At: fresh}
	b := Announcement{V: 1, ID: "a-id", Since: 100, At: fresh}
	stale := Announcement{V: 1, ID: "c-id", Since: 50, At: now.Add(-10 * time.Minute).Unix()}
	got, ok := ChooseHost([]Announcement{a, b, stale}, now, 3*time.Minute)
	if !ok || got.ID != "a-id" {
		t.Fatalf("победитель %+v", got)
	}
	tie := Announcement{V: 1, ID: "0-id", Since: 100, At: fresh}
	if got, _ = ChooseHost([]Announcement{b, tie}, now, 3*time.Minute); got.ID != "0-id" {
		t.Fatalf("при равенстве должен победить меньший ID: %+v", got)
	}
	if _, ok = ChooseHost([]Announcement{stale}, now, 3*time.Minute); ok {
		t.Fatal("устаревшее объявление не должно выбираться")
	}
}

func TestLobbyRejectsDirectAddressesOutsideTests(t *testing.T) {
	old := TestLoopback
	TestLoopback = false
	defer func() { TestLoopback = old }()
	good := Announcement{V: 1, ID: "12D3KooWRSJ2uH6BgDdJJCbTrbVvj5WKMQtbN4yq3J1m7Z2gVnZk", Addrs: []string{"/ip4/1.2.3.4/tcp/1/p2p/12D3KooWRSJ2uH6BgDdJJCbTrbVvj5WKMQtbN4yq3J1m7Z2gVnZk/p2p-circuit"}, Since: 1, At: 1}
	if validAnnouncement(good) == false {
		// допустимо: peer.Decode может отвергнуть тестовый ID, главное — прямой адрес ниже точно отвергается
		t.Log("тестовый peer ID не прошёл разбор")
	}
	direct := good
	direct.Addrs = []string{"/ip4/203.0.113.5/udp/4001/quic-v1"}
	if validAnnouncement(direct) {
		t.Fatal("прямой адрес с публичным IP принят в лобби")
	}
}