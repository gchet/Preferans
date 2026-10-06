// Package mobile exposes only gomobile-compatible primitives to the Android shell.
package mobile

import "preferans/locales"

import (
	"path/filepath"
	"preferans/internal/app"
	"preferans/netcheck"
	"preferans/web"
	"sync"
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

func Start(dataDir string) (string, error) {
	return start(dataDir, false)
}
func StartNetworkTest(dataDir string) (string, error) {
	return start(dataDir, true)
}
func start(dataDir string, diagnostic bool) (string, error) {
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
	assets := web.Files()
	if diagnostic {
		a.EnableDiagnostics()
		assets = netcheck.Files()
	}
	url, close, e := app.Serve(a, assets)
	if e != nil {
		a.Close()
		return "", e
	}
	stop = close
	address = url
	return url, nil
}
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if stop != nil {
		stop()
		stop = nil
		address = ""
	}
}
