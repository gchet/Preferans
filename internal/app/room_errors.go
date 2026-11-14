package app

import (
	"errors"
	"fmt"

	"preferans/locales"
)

// roomUnavailableError — сервис комнат недоступен (нет связи, таймаут и т.п.). Пользователь видит короткое
// сообщение, а подробности (многострочный список адресов и причин от libp2p) остаются только в журнале.
type roomUnavailableError struct{ cause error }

func (e *roomUnavailableError) Error() string { return locales.Text("go.internal.app.rooms.text001") }
func (e *roomUnavailableError) Unwrap() error { return e.cause }

// roomErrorDetail возвращает полный текст ошибки для журнала.
func roomErrorDetail(e error) string {
	var u *roomUnavailableError
	if errors.As(e, &u) && u.cause != nil {
		return fmt.Sprintf("%s: %v", e.Error(), u.cause)
	}
	return e.Error()
}