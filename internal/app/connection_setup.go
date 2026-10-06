package app

import "preferans/locales"

import (
	"context"

	"preferans/internal/network"
	"strings"
	"time"
)

func hasTURN(servers []string) bool {
	for _, s := range servers {
		p := strings.Split(s, "|")
		if len(p) == 3 && (strings.HasPrefix(p[0], "turn:") || strings.HasPrefix(p[0], "turns:")) && p[1] != "" && p[2] != "" {
			return true
		}
	}
	return false
}

func (a *App) CheckAndSaveConnection(servers []string, name string) error {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 24 {
		return locales.Errorf("go.internal.app.connection_setup.text001")
	}
	if !hasTURN(servers) {
		return locales.Errorf("go.internal.app.connection_setup.text002")
	}
	if len(servers) > 8 {
		return locales.Errorf("go.internal.app.connection_setup.text003")
	}
	var entry string
	for _, s := range servers {
		if strings.HasPrefix(s, "turn:") || strings.HasPrefix(s, "turns:") {
			entry = s
			break
		}
	}
	a.mu.Lock()
	a.logLocked(locales.Text("go.internal.app.connection_setup.text004"), iceServerSummary([]string{entry}))
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	address, err := network.CheckTURN(ctx, entry)
	a.mu.Lock()
	if err != nil {
		a.logLocked(locales.Text("go.internal.app.connection_setup.text005"), err)
	} else {
		a.logLocked(locales.Text("go.internal.app.connection_setup.text006"), address)
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	if name != "" {
		if err = a.SetName(name); err != nil {
			return err
		}
	}
	return a.Configure(servers)
}
