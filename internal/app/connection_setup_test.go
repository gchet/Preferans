package app

import (
	"github.com/pion/turn/v5"
	"net"
	"strings"
	"testing"
)

func TestConnectionSetupChecksAndPersists(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server, err := turn.NewServer(turn.ServerConfig{Realm: "test", AuthHandler: func(a *turn.RequestAttributes) (string, []byte, bool) {
		return a.Username, turn.GenerateAuthKey("player", "test", "secret"), a.Username == "player"
	}, PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: conn, RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{RelayAddress: net.ParseIP("127.0.0.1"), Address: "127.0.0.1"}}}})
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	defer server.Close()
	dir := t.TempDir()
	a := New(dir)
	defer a.Close()
	if err := a.SetRoomService("https://rooms.example.org/v2/rooms"); err != nil {
		t.Fatal(err)
	}
	base := "turn:" + conn.LocalAddr().String() + "|player|"
	if !a.Status().ConnectionSetup {
		t.Fatal("first-run setup missing")
	}
	if err := a.CheckAndSaveConnection([]string{base + "wrong"}, "Друг"); err == nil {
		t.Fatal("bad password accepted")
	}
	if !a.Status().ConnectionSetup {
		t.Fatal("failed settings saved")
	}
	if err := a.CheckAndSaveConnection([]string{base + "secret"}, "Друг"); err != nil {
		t.Fatal(err)
	}
	if a.Status().ConnectionSetup {
		t.Fatal("setup still required")
	}
	for _, line := range a.Status().Logs {
		if strings.Contains(line, "secret") || strings.Contains(line, "wrong") {
			t.Fatal("credential in log")
		}
	}
	a.Close()
	a = New(dir)
	defer a.Close()
	if a.Status().ConnectionSetup || a.Status().Appearance.Name != "Друг" {
		t.Fatal("setup not persisted")
	}
}
