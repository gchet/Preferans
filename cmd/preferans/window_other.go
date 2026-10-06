//go:build !windows || !cgo

package main

import "preferans/locales"

func window(url string) error {
	return locales.Errorf("go.cmd.preferans.window_other.text001", url)
}
