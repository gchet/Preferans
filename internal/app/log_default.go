package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"preferans/locales"
)

// logMaxBytes — предельный размер файла журнала: при превышении он переименовывается в «<имя>.1»
// (прежний «.1» заменяется) и запись продолжается в новом файле. Переменная нужна тестам.
var logMaxBytes int64 = 20 << 20

// UseDefaultLog задаёт файл журнала по умолчанию (в каталоге данных). Он используется, пока в настройках
// не указан свой путь и запись не отключена переключателем; путь из командной строки (-c) имеет приоритет.
func (a *App) UseDefaultLog(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	a.mu.Lock()
	a.defaultLogPath = path
	use := false
	if a.logPath == "" {
		a.logPath = path
		use = !a.appearance.LogDisabled
	}
	a.mu.Unlock()
	if !use {
		return nil
	}
	return a.enableLogFile(path)
}

// writeLogLineLocked дописывает строку в файл журнала и вовремя ротирует его. Вызывается с захваченным a.mu.
func (a *App) writeLogLineLocked(line string) {
	f := a.logFile
	if f == nil {
		return
	}
	if a.logCounted != f {
		a.logCounted, a.logBytes = f, 0
		if info, err := f.Stat(); err == nil {
			a.logBytes = info.Size()
		}
	}
	n, _ := fmt.Fprintln(f, line)
	a.logBytes += int64(n)
	if a.logBytes > logMaxBytes {
		a.rotateLogLocked()
	}
}

func (a *App) rotateLogLocked() {
	f, path := a.logFile, a.logPath
	if f == nil || path == "" {
		return
	}
	_ = f.Close()
	a.logFile, a.logCounted, a.logBytes = nil, nil, 0
	previous := path + ".1"
	_ = os.Remove(previous)
	if err := os.Rename(path, previous); err != nil {
		// Переименовать нельзя (файл занят другой программой): начинаем журнал заново, чтобы он не рос бесконечно.
		_ = os.Truncate(path, 0)
	}
	next, err := openLogFile(path)
	if err != nil {
		a.lastError = err.Error()
		return
	}
	a.logFile, a.logCounted = next, next
	n, _ := fmt.Fprintf(next, "%s  %s\n", time.Now().Format("2006-01-02 15:04:05.000"), locales.Format("go.log.rotated", filepath.Base(previous)))
	a.logBytes = int64(n)
}