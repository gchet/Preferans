package app

import (
	"os"

	"preferans/locales"
)

// Size is also available when disk logging is temporarily disabled.
func (a *App) logSizeLocked() int64 {
	if a.logPath == "" {
		return 0
	}
	info, err := os.Stat(a.logPath)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		return -1
	}
	return info.Size()
}

// Truncate the active file under the same lock used by all log writers.
// Its append-only descriptor continues writing without restarting the app.
func (a *App) ClearLogFile() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.logPath == "" {
		return locales.Errorf("go.log.not_configured")
	}
	// Windows append-only handles cannot truncate; use a separate write handle.
	f, err := os.OpenFile(a.logPath, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if f != nil {
		if err := f.Close(); err != nil {
			return err
		}
	}
	a.logs = nil
	return nil
}
