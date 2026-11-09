package app

import (
	"fmt"
	"time"

	"preferans/locales"
)

// SetLocalZone задаёт местный часовой пояс процесса по имени IANA (например, Europe/Kyiv). Нужен на Android:
// там у Go нет /etc/localtime, и без этого время в журнале шло бы по UTC. Если базы часовых поясов нет
// или имя неизвестно, берётся фиксированное смещение offsetSeconds.
func SetLocalZone(name string, offsetSeconds int) {
	if name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			time.Local = loc
			return
		}
	}
	time.Local = time.FixedZone("", offsetSeconds)
}

// zoneLabel описывает местный часовой пояс для первой строки журнала: «UTC+02:00 (EET)».
func zoneLabel(t time.Time) string {
	abbr, off := t.Zone()
	sign := '+'
	if off < 0 {
		sign, off = '-', -off
	}
	label := fmt.Sprintf("UTC%c%02d:%02d", sign, off/3600, off%3600/60)
	if name := t.Location().String(); name != "" && name != "Local" && name != abbr {
		return fmt.Sprintf("%s (%s, %s)", label, name, abbr)
	}
	if abbr != "" && abbr != label {
		return fmt.Sprintf("%s (%s)", label, abbr)
	}
	return label
}

// logZoneLocked пишет в журнал часовой пояс, в котором записаны времена. Вызывается при открытии файла журнала.
func (a *App) logZoneLocked() {
	a.logLocked(locales.Text("go.internal.app.app.log_zone"), zoneLabel(time.Now()))
}