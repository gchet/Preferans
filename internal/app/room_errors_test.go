package app

import (
	"errors"
	"strings"
	"testing"
)

func TestRoomUnavailableErrorIsShortAndKeepsDetailForLog(t *testing.T) {
	cause := errors.New("failed to dial 12D3KooWAAAA: all dials failed\n  * [/ip4/1.2.3.4/udp/1/quic-v1] timeout: no recent network activity")
	e := &roomUnavailableError{cause: cause}
	if strings.ContainsAny(e.Error(), "\n[") || strings.Contains(e.Error(), "dial") || e.Error() != "Нет связи с сервисом комнат" {
		t.Fatalf("для интерфейса нужно короткое сообщение: %q", e.Error())
	}
	if !errors.Is(e, cause) {
		t.Fatal("причина должна быть доступна через Unwrap")
	}
	if d := roomErrorDetail(e); !strings.Contains(d, "all dials failed") || !strings.HasPrefix(d, "Нет связи с сервисом комнат: ") {
		t.Fatalf("в журнал нужен полный текст: %q", d)
	}
	if d := roomErrorDetail(errors.New("Комната занята")); d != "Комната занята" {
		t.Fatal(d)
	}
}

func TestRoomLogsKeepDialDetailsButStatusDoesNot(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	e := &roomUnavailableError{cause: errors.New("failed to dial 12D3KooWAAAA: all dials failed\n  * [/ip4/1.2.3.4/udp/1/quic-v1] timeout")}
	a.mu.Lock()
	a.logRoomErrorLocked(e)
	a.rooms.err = e.Error()
	logs := strings.Join(a.logs, "\n")
	a.mu.Unlock()
	if !strings.Contains(logs, "all dials failed") {
		t.Fatal("подробности пропали из журнала")
	}
	if status := a.Status(); strings.Contains(status.RoomError, "dial") || strings.Contains(status.RoomError, "\n") {
		t.Fatalf("в состояние для интерфейса попала техническая ошибка: %q", status.RoomError)
	}
}