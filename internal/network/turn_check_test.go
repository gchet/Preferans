package network

import (
	"context"
	"github.com/pion/turn/v5"
	"net"
	"strings"
	"testing"
	"time"
)

func TestCheckTURNAllocationAndWrongPassword(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server, err := turn.NewServer(turn.ServerConfig{Realm: "test", AuthHandler: func(a *turn.RequestAttributes) (string, []byte, bool) {
		return a.Username, turn.GenerateAuthKey("player", "test", "correct-password"), a.Username == "player"
	}, PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: conn, RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{RelayAddress: net.ParseIP("127.0.0.1"), Address: "127.0.0.1"}}}})
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	defer server.Close()
	for _, password := range []string{"correct-password", "incorrect-password"} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		address, err := CheckTURN(ctx, "turn:"+conn.LocalAddr().String()+"?transport=udp|player|"+password)
		cancel()
		if password == "correct-password" {
			if err != nil || address == "" {
				t.Fatalf("allocation: %s, %v", address, err)
			}
		} else {
			if err == nil || !strings.Contains(err.Error(), "пароль") || strings.Contains(err.Error(), password) {
				t.Fatalf("wrong password: %v", err)
			}
		}
	}
}

func TestCheckTURNTimeout(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = CheckTURN(ctx, "turn:"+conn.LocalAddr().String()+"|player|password")
	if err == nil || time.Since(start) > time.Second || !strings.Contains(err.Error(), "вовремя") {
		t.Fatalf("timeout: %v", err)
	}
}
