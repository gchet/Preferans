package app

import (
	"encoding/json"
	"testing"
	"time"
)

func audioBatch(n int, fill byte) AudioBatch {
	b := AudioBatch{Seq: 1, Ts: 1000}
	for i := 0; i < n; i++ {
		f := make([]byte, 60)
		for j := range f {
			f[j] = fill
		}
		b.Frames = append(b.Frames, f)
	}
	return b
}

// Кадры гостя доходят до ведущего и другого гостя; отправитель их не получает, источник подставляет ведущий.
func TestAudioRelaysThroughHost(t *testing.T) {
	host, one, two := New(t.TempDir()), New(t.TempDir()), New(t.TempDir())
	defer host.Close()
	defer one.Close()
	defer two.Close()
	if err := host.Create(4, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	connect(t, host, one, 1)
	connect(t, host, two, 2)
	if _, err := host.FillBots(); err != nil {
		t.Fatal(err)
	}
	b := audioBatch(3, 7)
	b.From = 2 // чужое место: ведущий подставит настоящее
	if err := one.SendAudio(b); err != nil {
		t.Fatal(err)
	}
	for name, a := range map[string]*App{"host": host, "two": two} {
		a := a
		var got AudioPoll
		eventually(t, func() bool { got, _ = a.PollAudio(0); return len(got.Events) > 0 })
		if len(got.Events) != 1 || got.Events[0].Batch.From != 1 || len(got.Events[0].Batch.Frames) != 3 || got.Events[0].Batch.Frames[0][0] != 7 {
			t.Fatalf("%s: %+v", name, got)
		}
	}
	if got, _ := one.PollAudio(0); len(got.Events) != 0 {
		t.Fatal("отправитель получил собственный звук")
	}
	// ведущий говорит всем гостям
	if err := host.SendAudio(audioBatch(2, 9)); err != nil {
		t.Fatal(err)
	}
	for name, a := range map[string]*App{"one": one, "two": two} {
		a := a
		var got AudioPoll
		eventually(t, func() bool { got, _ = a.PollAudio(0); return len(got.Events) > 0 })
		if got.Events[len(got.Events)-1].Batch.From != 0 {
			t.Fatalf("%s: источник должен быть ведущим: %+v", name, got)
		}
	}
	if got, _ := host.PollAudio(0); len(got.Events) != 1 {
		t.Fatal("ведущий получил собственный звук")
	}
}

func TestAudioRejectsGarbageAndBoundsQueue(t *testing.T) {
	host, one := New(t.TempDir()), New(t.TempDir())
	defer host.Close()
	defer one.Close()
	if err := host.Create(3, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	connect(t, host, one, 1)
	if err := host.SendAudio(AudioBatch{}); err == nil {
		t.Fatal("пустая пачка принята")
	}
	if err := host.SendAudio(audioBatch(maxAudioFrames+1, 1)); err == nil {
		t.Fatal("слишком длинная пачка принята")
	}
	big := audioBatch(1, 1)
	big.Frames[0] = make([]byte, maxAudioFrame+1)
	if err := host.SendAudio(big); err == nil {
		t.Fatal("слишком большой кадр принят")
	}
	// неавторизованное место не может подать звук
	host.mu.Lock()
	host.peers[1].Auth = false
	link := host.peers[1]
	host.mu.Unlock()
	b := audioBatch(1, 5)
	b.From = 1
	data, _ := json.Marshal(Packet{Type: "audio", Audio: &b})
	host.receiveHost(1, link, data)
	if got, _ := host.PollAudio(0); len(got.Events) != 0 {
		t.Fatal("звук от непроверенного канала принят")
	}
	// очередь ограничена, а старые кадры не отдаются
	host.mu.Lock()
	for i := 0; i < audioQueueLimit+40; i++ {
		host.queueAudioLocked(AudioBatch{From: 1, Frames: [][]byte{{1}}})
	}
	n := len(host.audioQueue)
	host.audioQueue[0].at = time.Now().Add(-time.Minute)
	host.mu.Unlock()
	if n != audioQueueLimit {
		t.Fatalf("очередь не ограничена: %d", n)
	}
	if got, _ := host.PollAudio(0); len(got.Events) != audioQueueLimit-1 {
		t.Fatalf("устаревший кадр отдан: %d", len(got.Events))
	}
	// после перезапуска счётчик начинается заново: опрос с большим after не зависает
	if got, _ := host.PollAudio(1 << 40); len(got.Events) == 0 {
		t.Fatal("сброшенный счётчик должен отдавать очередь")
	}
	host.Leave()
	if _, err := host.PollAudio(0); err == nil {
		t.Fatal("звук остался активным после выхода")
	}
}
