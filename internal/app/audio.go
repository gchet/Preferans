package app

import (
	"time"

	"preferans/locales"
)

// Запасной путь голоса: если прямое WebRTC-соединение двух игроков не поднялось (STUN без TURN не пробивает часть
// NAT), кадры Opus идут по уже установленному игровому каналу libp2p: гость -> ведущий -> остальные гости.
// Кадры не сохраняются и не пишутся в журнал; очередь короткая, устаревшие кадры отбрасываются.
const (
	maxAudioFrames   = 12                     // кадров в пачке (по 20 мс)
	maxAudioFrame    = 400                    // байт в кадре Opus
	audioQueueLimit  = 150                    // пачек в очереди приёма
	audioQueueMaxAge = 1500 * time.Millisecond // старше этого кадры не нужны
	audioPollWait    = 250 * time.Millisecond
	audioLinkQueue   = 24 // пачек в очереди отправки одному месту: при переполнении новые отбрасываются
)

// AudioBatch — пачка кадров Opus одного игрока.
type AudioBatch struct {
	From   int      `json:"from"`
	Seq    uint64   `json:"seq"` // номер первого кадра в счётчике отправителя
	Ts     int64    `json:"ts"`  // время захвата первого кадра, мс (часы отправителя)
	Frames [][]byte `json:"frames"`
}
type AudioEvent struct {
	Seq   uint64     `json:"seq"`
	Batch AudioBatch `json:"batch"`
	at    time.Time
}
type AudioPoll struct {
	Events []AudioEvent `json:"events"`
	Last   uint64       `json:"last"`
}

func (a *App) validAudioLocked(b AudioBatch) bool {
	if a.saved.Table == "" || !a.voiceHumanLocked(b.From) || len(b.Frames) == 0 || len(b.Frames) > maxAudioFrames {
		return false
	}
	for _, f := range b.Frames {
		if len(f) == 0 || len(f) > maxAudioFrame {
			return false
		}
	}
	return true
}
func (a *App) queueAudioLocked(b AudioBatch) {
	a.audioSeq++
	a.audioQueue = append(a.audioQueue, AudioEvent{Seq: a.audioSeq, Batch: b, at: time.Now()})
	if len(a.audioQueue) > audioQueueLimit {
		a.audioQueue = append([]AudioEvent(nil), a.audioQueue[len(a.audioQueue)-audioQueueLimit:]...)
	}
}

// routeAudioLocked исполняется у ведущего: пачка уходит на его собственный динамик и всем остальным подключённым людям.
func (a *App) routeAudioLocked(b AudioBatch) {
	if b.From != 0 {
		a.queueAudioLocked(b)
	}
	for seat, link := range a.peers {
		if seat == b.From || !a.voiceHumanLocked(seat) || !link.Auth || seat >= len(a.connected) || !a.connected[seat] {
			continue
		}
		link.sendAudio(Packet{Type: "audio", Audio: &b})
	}
}

// SendAudio принимает пачку кадров от интерфейса: ведущий раздаёт её сам, гость отправляет ведущему.
func (a *App) SendAudio(b AudioBatch) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rooms.config.Current == localRoomID {
		return nil
	}
	if a.saved.State != nil {
		b.From = 0
	} else {
		b.From = a.saved.Seat
	}
	if !a.validAudioLocked(b) {
		return locales.Errorf("go.internal.app.voice.text004")
	}
	if a.saved.State != nil {
		a.routeAudioLocked(b)
		return nil
	}
	if l := a.peers[0]; l != nil && l.Status == "connected" {
		l.sendAudio(Packet{Type: "audio", Audio: &b})
		return nil
	}
	return locales.Errorf("go.internal.app.voice.text005")
}

// PollAudio возвращает пачки новее after; ждёт до audioPollWait, если новых нет (длинный опрос интерфейса).
func (a *App) PollAudio(after uint64) (AudioPoll, error) {
	deadline := time.Now().Add(audioPollWait)
	for {
		a.mu.Lock()
		if a.saved.Table == "" {
			a.mu.Unlock()
			return AudioPoll{}, locales.Errorf("go.internal.app.voice.text006")
		}
		if after > a.audioSeq {
			after = 0 // счётчик начат заново (перезапуск приложения)
		}
		now := time.Now()
		out := AudioPoll{Events: []AudioEvent{}, Last: a.audioSeq}
		for _, e := range a.audioQueue {
			if e.Seq > after && now.Sub(e.at) <= audioQueueMaxAge {
				out.Events = append(out.Events, e)
			}
		}
		a.mu.Unlock()
		if len(out.Events) > 0 || !time.Now().Before(deadline) {
			return out, nil
		}
		select {
		case <-a.closed:
			return out, nil
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// sendAudio ставит пачку в очередь отправки места; медленный канал не должен задерживать остальных и блокировать a.mu.
func (l *Link) sendAudio(p Packet) {
	l.pmu.Lock()
	defer l.pmu.Unlock()
	if l.audioQ == nil {
		l.audioQ = make(chan Packet, audioLinkQueue)
		go l.audioLoop(l.audioQ)
	}
	select {
	case l.audioQ <- p:
	default: // очередь полна: звук не копим
	}
}
func (l *Link) audioLoop(q chan Packet) {
	for p := range q {
		_ = l.send(p)
	}
}
func (l *Link) stopAudio() {
	l.pmu.Lock()
	defer l.pmu.Unlock()
	if l.audioQ != nil { // место может быть подключено заново тем же Link: очередь создаётся при следующей отправке
		close(l.audioQ)
		l.audioQ = nil
	}
}
