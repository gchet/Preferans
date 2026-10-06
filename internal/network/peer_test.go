package network

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDirectManualExchange(t *testing.T) {
	a, e := New(nil)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := New(nil)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	received := make(chan string, 1)
	payload := strings.Repeat("hello", 50000)
	b.Message = func(data []byte) { received <- string(data) }
	a.Open = func() { _ = a.Send(payload) }
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	offer, e := a.Gather(ctx, true)
	if e != nil {
		t.Fatal(e)
	}
	code, e := Encode(Code{Version: Version, Table: strings.Repeat("a", 32), Seat: 1, Nonce: strings.Repeat("b", 32), Description: offer})
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := Decode(code)
	if e != nil {
		t.Fatal(e)
	}
	if e = b.PC.SetRemoteDescription(decoded.Description); e != nil {
		t.Fatal(e)
	}
	answer, e := b.Gather(ctx, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.PC.SetRemoteDescription(answer); e != nil {
		t.Fatal(e)
	}
	select {
	case msg := <-received:
		if msg != `"`+payload+`"` {
			t.Fatal("large message mismatch")
		}
	case <-ctx.Done():
		t.Fatal("data channel timeout")
	}
}
func TestRejectInvalidCodesAndTURN(t *testing.T) {
	for _, s := range []string{"12345", "PREF1.invalid", strings.Repeat("a", 200000)} {
		if _, e := Decode(s); e == nil {
			t.Fatal("invalid accepted")
		}
	}
	if _, e := New([]string{"turn:example.org"}); e == nil {
		t.Fatal("TURN accepted")
	}
}
