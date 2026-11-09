// Package mobile exposes only gomobile-compatible primitives to the Android shell.
package mobile

import "preferans/locales"

import (
	"path/filepath"
	"preferans/internal/app"
	"preferans/web"
	"sync"

	_ "time/tzdata" // база часовых поясов: на Android её нет в виде, доступном Go
)

var mu sync.Mutex
var stop func()
var address string

// SecretCipher is implemented by the Android shell with Android Keystore.
type SecretCipher interface {
	Seal(string) (string, error)
	Open(string) (string, error)
}

var secretCipher SecretCipher

func SetSecretCipher(cipher SecretCipher) { mu.Lock(); defer mu.Unlock(); secretCipher = cipher }

// SetTimeZone передаёт часовой пояс Android (TimeZone.getDefault): без него время в журнале шло бы по UTC.
// Вызывается до Start.
func SetTimeZone(name string, offsetSeconds int) { app.SetLocalZone(name, offsetSeconds) }

// NetworkChanged вызывается Android при возврате приложения из фона и при появлении сети: быстрое восстановление связи.
func NetworkChanged() {
	mu.Lock()
	a := current
	mu.Unlock()
	if a != nil {
		a.NetworkChanged()
	}
}

var current *app.App

func Start(dataDir string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if stop != nil {
		return address, nil
	}
	a := app.New(dataDir)
	a.SetSecretCipher(secretCipher)
	if err := a.SetLogFile(filepath.Join(dataDir, "preferans.log")); err != nil {
		a.Close()
		return "", locales.Errorf("go.mobile.mobile.text001", err)
	}
	a.EnableLobby() // публичное лобби: игроки ничего не вводят
	url, close, e := app.Serve(a, web.Files())
	if e != nil {
		a.Close()
		return "", e
	}
	stop = close
	address = url
	current = a
	return url, nil
}
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if stop != nil {
		stop()
		stop = nil
		current = nil
		address = ""
	}
}
