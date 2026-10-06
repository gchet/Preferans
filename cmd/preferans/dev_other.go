//go:build !windows

package main

import "preferans/locales"

func openDevBrowser(url string) error {
	return locales.Errorf("go.cmd.preferans.dev_other.text001", url)
}
