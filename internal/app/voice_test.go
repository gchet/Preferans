package app

import (
	"encoding/json"
	"preferans/internal/game"
	"testing"
)

func TestVoiceRoutesOnlyAuthenticatedHumans(t *testing.T) {
	host, one, two := New(t.TempDir()), New(t.TempDir()), New(t.TempDir())
	defer host.Close()
	defer one.Close()
	defer two.Close()
	for _, a := range []*App{host, one, two} {
		if err := a.Configure(nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := host.Create(4, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	connect(t, host, one, 1)
	connect(t, host, two, 2)
	if _, err := host.FillBots(); err != nil {
		t.Fatal(err)
	}
	table := host.Status().View.ID
	m := VoiceMessage{Table: table, From: 3, To: -1, Session: game.ID(), Kind: "hello", Mic: true}
	if err := one.SendVoice(m); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { events, _ := two.PollVoice(table, 0); return len(events) > 0 })
	for _, a := range []*App{host, two} {
		events, err := a.PollVoice(table, 0)
		if err != nil || len(events) != 1 {
			t.Fatalf("broadcast: %v %v", events, err)
		}
		if events[0].Message.From != 1 || !events[0].Message.Mic {
			t.Fatal("sender was not authenticated")
		}
		none, _ := a.PollVoice(table, events[0].Seq)
		if len(none) != 0 {
			t.Fatal("acknowledged event repeated")
		}
	}
	m.Kind = "offer"
	m.To = 2
	m.Target = game.ID()
	m.Call = game.ID()
	m.Payload = `{"type":"offer","sdp":"test"}`
	if err := one.SendVoice(m); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { e, _ := two.PollVoice(table, 0); return len(e) == 2 })
	if e, _ := host.PollVoice(table, 0); len(e) != 1 {
		t.Fatal("targeted signal leaked to host UI")
	}
	m.To = 3
	if err := host.SendVoice(m); err == nil {
		t.Fatal("bot accepted voice signalling")
	}
	m.To = 2
	m.Table = game.ID()
	if err := host.SendVoice(m); err == nil {
		t.Fatal("foreign table accepted")
	}
	// An unverified link cannot inject voice, even with a valid table ID.
	host.mu.Lock()
	host.peers[1].Auth = false
	link := host.peers[1]
	before := len(host.voiceQueue)
	host.mu.Unlock()
	m.Table = table
	m.To = -1
	m.Kind = "hello"
	m.Payload = ""
	b, _ := json.Marshal(Packet{Type: "voice", Voice: &m})
	host.receiveHost(1, link, b)
	host.mu.Lock()
	after := len(host.voiceQueue)
	host.mu.Unlock()
	if before != after {
		t.Fatal("unauthenticated signal accepted")
	}
	host.Leave()
	if _, err := host.PollVoice(table, 0); err == nil {
		t.Fatal("voice remained active after leaving")
	}
}

func TestVoiceQueueBounded(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	for i := 0; i < 300; i++ {
		a.queueVoiceLocked(VoiceMessage{Kind: "hello"})
	}
	if len(a.voiceQueue) != 128 || a.voiceQueue[0].Seq != 173 {
		t.Fatal("voice queue is not bounded")
	}
}
