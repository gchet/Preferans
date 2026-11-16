package app

import (
	"strings"

	"preferans/locales"
)

const (
	maxVoiceLogLines = 24
	maxVoiceLogRunes = 320
)

// LogVoice записывает в журнал события голоса из интерфейса (состояние WebRTC, типы кандидатов ICE, выбранная пара,
// поток аудио). Звук и адреса игроков в журнал не попадают: только типы кандидатов и счётчики.
// Сообщение может содержать несколько строк (интерфейс отправляет события пачками).
func (a *App) LogVoice(message string) {
	lines := strings.Split(message, "\n")
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > maxVoiceLogRunes {
			line = string(r[:maxVoiceLogRunes]) + "…"
		}
		if n++; n > maxVoiceLogLines {
			return
		}
		a.logLocked(locales.Text("go.log.voice"), line)
	}
}
