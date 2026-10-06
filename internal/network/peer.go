package network

import "preferans/locales"

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"

	"io"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

const Version = 1
const MaxMessage = 8 * 1024 * 1024

type Code struct {
	Version     int                       `json:"v"`
	Table       string                    `json:"table"`
	Seat        int                       `json:"seat"`
	Nonce       string                    `json:"nonce"`
	Key         string                    `json:"key,omitempty"`
	Description webrtc.SessionDescription `json:"description"`
}

func Encode(c Code) (string, error) {
	b, e := json.Marshal(c)
	if e != nil {
		return "", e
	}
	var buf bytes.Buffer
	z := zlib.NewWriter(&buf)
	if _, e = z.Write(b); e != nil {
		return "", e
	}
	if e = z.Close(); e != nil {
		return "", e
	}
	return "PREF1." + base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}
func Decode(s string) (Code, error) {
	var c Code
	s = strings.Join(strings.Fields(s), "")
	if len(s) > 128*1024 || !strings.HasPrefix(s, "PREF1.") {
		return c, locales.Errorf("go.internal.network.peer.text001")
	}
	b, e := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, "PREF1."))
	if e != nil {
		return c, e
	}
	z, e := zlib.NewReader(bytes.NewReader(b))
	if e != nil {
		return c, e
	}
	defer z.Close()
	b, e = io.ReadAll(io.LimitReader(z, MaxMessage+1))
	if e != nil || len(b) > MaxMessage {
		return c, locales.Errorf("go.internal.network.peer.text002")
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, e
	}
	if c.Version != Version || len(c.Table) != 32 || c.Seat < 1 || c.Seat > 3 || len(c.Nonce) != 32 {
		return c, locales.Errorf("go.internal.network.peer.text003")
	}
	if c.Description.Type != webrtc.SDPTypeOffer && c.Description.Type != webrtc.SDPTypeAnswer {
		return c, locales.Errorf("go.internal.network.peer.text004")
	}
	return c, nil
}

type Peer struct {
	PC        *webrtc.PeerConnection
	mu        sync.Mutex
	sendMu    sync.Mutex
	rxMu      sync.Mutex
	rx        []byte
	receiving bool
	dc        *webrtc.DataChannel
	closed    bool
	Message   func([]byte)
	Status    func(string)
	Open      func()
}

func New(serversConfig []string) (*Peer, error) {
	servers := []webrtc.ICEServer{}
	for _, entry := range serversConfig {
		parts := strings.Split(entry, "|")
		u := parts[0]
		if strings.HasPrefix(u, "turn:") || strings.HasPrefix(u, "turns:") {
			if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
				return nil, locales.Errorf("go.internal.network.peer.text005")
			}
			servers = append(servers, webrtc.ICEServer{URLs: []string{u}, Username: parts[1], Credential: parts[2]})
		} else if strings.HasPrefix(u, "stun:") {
			servers = append(servers, webrtc.ICEServer{URLs: []string{u}})
		} else {
			return nil, locales.Errorf("go.internal.network.peer.text006")
		}
	}
	se := webrtc.SettingEngine{}
	se.SetICETimeouts(15*time.Second, 45*time.Second, 3*time.Second)
	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	pc, e := api.NewPeerConnection(webrtc.Configuration{ICEServers: servers})
	if e != nil {
		return nil, e
	}
	p := &Peer{PC: pc}
	pc.OnDataChannel(p.attach)
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if p.Status != nil {
			p.Status(s.String())
		}
	})
	return p, nil
}
func (p *Peer) attach(dc *webrtc.DataChannel) {
	if dc.Label() != "preferans-v1" {
		_ = dc.Close()
		return
	}
	p.mu.Lock()
	if p.dc != nil && p.dc != dc {
		p.mu.Unlock()
		_ = dc.Close()
		return
	}
	p.dc = dc
	p.mu.Unlock()
	dc.OnOpen(func() {
		if p.Open != nil {
			p.Open()
		}
	})
	dc.OnMessage(func(m webrtc.DataChannelMessage) { p.receiveFrame(m.Data) })
}
func (p *Peer) receiveFrame(b []byte) {
	p.rxMu.Lock()
	if len(b) < 2 || len(b) > 16384 || b[0] > 3 {
		p.rxMu.Unlock()
		go p.Close()
		return
	}
	if b[0]&1 != 0 {
		if p.receiving {
			p.rxMu.Unlock()
			go p.Close()
			return
		}
		p.rx = nil
		p.receiving = true
	}
	if !p.receiving || len(p.rx)+len(b)-1 > MaxMessage {
		p.rxMu.Unlock()
		go p.Close()
		return
	}
	p.rx = append(p.rx, b[1:]...)
	if b[0]&2 == 0 {
		p.rxMu.Unlock()
		return
	}
	msg := p.rx
	p.rx = nil
	p.receiving = false
	p.rxMu.Unlock()
	if p.Message != nil {
		p.Message(msg)
	}
}
func (p *Peer) Gather(ctx context.Context, offer bool) (webrtc.SessionDescription, error) {
	var desc webrtc.SessionDescription
	var e error
	if offer {
		dc, err := p.PC.CreateDataChannel("preferans-v1", nil)
		if err != nil {
			return desc, err
		}
		p.attach(dc)
		desc, e = p.PC.CreateOffer(nil)
	} else {
		desc, e = p.PC.CreateAnswer(nil)
	}
	if e != nil {
		return desc, e
	}
	done := webrtc.GatheringCompletePromise(p.PC)
	if e = p.PC.SetLocalDescription(desc); e != nil {
		return desc, e
	}
	select {
	case <-done:
	case <-ctx.Done():
		return desc, locales.Errorf("go.internal.network.peer.text007", ctx.Err())
	}
	return *p.PC.LocalDescription(), nil
}
func (p *Peer) Send(v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if len(b) > MaxMessage {
		return locales.Errorf("go.internal.network.peer.text008")
	}
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	p.mu.Lock()
	dc := p.dc
	p.mu.Unlock()
	if dc == nil || dc.ReadyState() != webrtc.DataChannelStateOpen {
		return locales.Errorf("go.internal.network.peer.text009")
	}
	for i := 0; i < len(b); i += 16383 {
		end := i + 16383
		if end > len(b) {
			end = len(b)
		}
		frame := make([]byte, 1+end-i)
		if i == 0 {
			frame[0] |= 1
		}
		if end == len(b) {
			frame[0] |= 2
		}
		copy(frame[1:], b[i:end])
		if e := dc.Send(frame); e != nil {
			return e
		}
	}
	return nil
}
func (p *Peer) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()
	_ = p.PC.Close()
}
