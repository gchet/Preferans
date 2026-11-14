package app

import (
	"strings"
	"time"

	"preferans/locales"
)

const maxClientErrorRunes = 1500

// LogClientError записывает ошибку интерфейса (JS) в журнал: на экране она видна считанные секунды, а причину
// приходится искать потом. Одинаковые сообщения подряд записываются не чаще раза в 30 секунд.
func (a *App) LogClientError(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	if r := []rune(message); len(r) > maxClientErrorRunes {
		message = string(r[:maxClientErrorRunes]) + "…"
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if message == a.lastClientError && time.Since(a.lastClientErrorAt) < 30*time.Second {
		return
	}
	a.lastClientError, a.lastClientErrorAt = message, time.Now()
	a.logLocked(locales.Text("go.log.client_error"), message)
}