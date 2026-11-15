package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"preferans/config"
	"preferans/internal/selfupdate"
	"preferans/locales"
)

// Состояния обновления, которые видит интерфейс.
const (
	updateAvailable   = "available"
	updateDownloading = "downloading"
	updateInstalling  = "installing"
	updateFailed      = "error"
)

// UpdateInfo описывает найденную на GitHub новую версию и ход её установки.
type UpdateInfo struct {
	State      string `json:"state"`
	Version    string `json:"version"`
	Progress   int    `json:"progress"`
	Error      string `json:"error,omitempty"`
	CanInstall bool   `json:"canInstall"` // Windows: приложение обновляет себя; Android ставит APK через системный установщик
	APKURL     string `json:"apkUrl,omitempty"`
	APKSHA256  string `json:"apkSha256,omitempty"`
}

type updateState struct {
	info    UpdateInfo
	release *selfupdate.Release
}

// Переменные, а не константы: тесты (в том числе на Linux в CI) подменяют платформу и перезапуск.
var (
	updateSupported   = func() bool { return runtime.GOOS == "windows" || runtime.GOOS == "android" }
	selfInstallable   = func() bool { return runtime.GOOS == "windows" }
	restartExecutable = selfupdate.Restart
	executablePath    = os.Executable
	aiBuild           = func() bool { return config.AIEnabled }
)

func (a *App) autoUpdateOn() bool { return a.appearance.autoUpdateOn() }

// CheckUpdate проверяет на GitHub, есть ли версия новее current. Ошибки сети только пишутся в журнал.
// Ничего не делает, если автоматическое обновление выключено, платформа не поддерживается
// или это сборка с ИИ (её обновлять нельзя: выпуск на GitHub собран без ИИ).
func (a *App) CheckUpdate(current string) (*UpdateInfo, error) {
	a.mu.Lock()
	enabled := a.autoUpdateOn()
	a.mu.Unlock()
	if !enabled || aiBuild() || !updateSupported() {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rel, err := a.updater.Latest(ctx)
	if err != nil {
		a.mu.Lock()
		a.logLocked(locales.Text("go.update.check_failed"), err)
		a.mu.Unlock()
		return nil, nil
	}
	if !selfupdate.Newer(current, rel.Version) {
		return nil, nil
	}
	info := UpdateInfo{State: updateAvailable, Version: rel.Version, CanInstall: selfInstallable(), APKURL: rel.APKURL, APKSHA256: rel.APKSHA256}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.update.info.State == updateDownloading || a.update.info.State == updateInstalling {
		copyInfo := a.update.info
		return &copyInfo, nil
	}
	a.update = updateState{info: info, release: rel}
	a.logLocked(locales.Text("go.update.found"), current, rel.Version)
	return &info, nil
}

func (a *App) updateStatusLocked() *UpdateInfo {
	if a.update.info.State == "" {
		return nil
	}
	info := a.update.info
	return &info
}

// InstallUpdate скачивает найденную версию, заменяет exe и перезапускает программу (только Windows).
func (a *App) InstallUpdate() error {
	if !selfInstallable() {
		return locales.Errorf("go.update.not_supported")
	}
	a.mu.Lock()
	rel := a.update.release
	state := a.update.info.State
	if rel == nil || state == "" {
		a.mu.Unlock()
		return locales.Errorf("go.update.not_available")
	}
	if state == updateDownloading || state == updateInstalling {
		a.mu.Unlock()
		return locales.Errorf("go.update.busy")
	}
	a.update.info.State, a.update.info.Progress, a.update.info.Error = updateDownloading, 0, ""
	a.mu.Unlock()
	go a.runInstall(rel)
	return nil
}

func (a *App) setUpdateFailed(message string) {
	a.mu.Lock()
	a.update.info.State, a.update.info.Error = updateFailed, message
	a.logLocked(locales.Text("go.update.failed_log"), message)
	a.mu.Unlock()
}

func (a *App) runInstall(rel *selfupdate.Release) {
	exe, err := executablePath()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		a.setUpdateFailed(err.Error())
		return
	}
	fresh := exe + ".new"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	last := -1
	err = a.updater.Download(ctx, rel.ExeURL, fresh, rel.ExeSHA256, func(done, total int64) {
		if total <= 0 {
			return
		}
		if p := int(done * 100 / total); p != last {
			last = p
			a.mu.Lock()
			a.update.info.Progress = p
			a.mu.Unlock()
		}
	})
	if err != nil {
		_ = os.Remove(fresh)
		switch {
		case errors.Is(err, selfupdate.ErrHash):
			a.setUpdateFailed(locales.Text("go.update.hash"))
		case errors.Is(err, selfupdate.ErrHost):
			a.setUpdateFailed(locales.Text("go.update.host"))
		case errors.Is(err, selfupdate.ErrLarge):
			a.setUpdateFailed(locales.Text("go.update.large"))
		default:
			a.setUpdateFailed(locales.Format("go.update.download_failed", err))
		}
		return
	}
	a.mu.Lock()
	a.update.info.State, a.update.info.Progress = updateInstalling, 100
	a.mu.Unlock()
	if err := selfupdate.ReplaceExecutable(exe, fresh); err != nil {
		_ = os.Remove(fresh)
		a.setUpdateFailed(locales.Format("go.update.replace_failed", err, rel.PageURL))
		return
	}
	a.mu.Lock()
	a.logLocked(locales.Text("go.update.restarting"), rel.Version)
	a.mu.Unlock()
	if err := restartExecutable(exe, os.Args[1:]); err != nil {
		a.setUpdateFailed(locales.Format("go.update.restart_failed", err))
		return
	}
	a.quitAfterUpdate()
}

// quitAfterUpdate завершает старый процесс после запуска новой версии.
func (a *App) quitAfterUpdate() {
	if a.quit != nil {
		a.quit()
		return
	}
	a.Close()
	os.Exit(0)
}
