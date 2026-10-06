//go:build windows

package main

import "preferans/locales"

import (
	"os"
	"os/exec"
	"path/filepath"
)

func openDevBrowser(url string) error {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
	}
	for _, chrome := range candidates {
		if chrome == "" {
			continue
		}
		if info, err := os.Stat(chrome); err == nil && !info.IsDir() {
			return exec.Command(chrome, url).Start()
		}
	}
	if chrome, err := exec.LookPath("chrome.exe"); err == nil {
		return exec.Command(chrome, url).Start()
	}
	return locales.Errorf("go.cmd.preferans.dev_windows.text001", url)
}
