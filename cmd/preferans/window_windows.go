//go:build windows && cgo

package main

import "preferans/locales"

import (
	webview "github.com/webview/webview_go"
	"os"
)

func window(url string) error {
	// Set before creating WebView2: changing it afterwards still flashes white.
	_ = os.Setenv("WEBVIEW2_DEFAULT_BACKGROUND_COLOR", "FF141A23")
	stopBackgroundHook := prepareWindowBackground()
	w := webview.New(false)
	stopBackgroundHook()
	if w == nil {
		return locales.Errorf("go.cmd.preferans.window_windows.text001")
	}
	defer w.Destroy()
	if err := w.Bind("preferansExit", func() {
		w.Dispatch(func() { w.Terminate() })
	}); err != nil {
		return err
	}
	titleKey := "go.cmd.preferans.window_windows.text002"
	if appMode == "netcheck" {
		titleKey = "go.cmd.preferans.window_windows.text003"
	}
	w.SetTitle(locales.Lookup(appLanguage, titleKey))
	if err := w.Bind("preferansLanguage", func(language string) {
		w.Dispatch(func() { w.SetTitle(locales.Lookup(language, titleKey)) })
	}); err != nil {
		return err
	}
	w.SetSize(1100, 820, webview.HintNone)
	w.SetSize(360, 540, webview.HintMin)
	w.Navigate(url)
	w.Run()
	return nil
}
