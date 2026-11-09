package app

import (
	"testing"

	"preferans/internal/p2p"
)

func TestSetAppearanceNtfyServers(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	defer p2p.SetNtfyServers(nil)
	p := a.Status().Appearance
	p.NtfyServers = "ntfy.sh"
	if err := a.SetAppearance(p); err == nil {
		t.Fatal("адрес без схемы должен отвергаться")
	}
	p.NtfyServers = " https://a.example/ ;https://b.example;; "
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
	if got := a.Status().Appearance.NtfyServers; got != "https://a.example; https://b.example" {
		t.Fatalf("%q", got)
	}
	p.NtfyServers = ""
	if err := a.SetAppearance(p); err != nil || a.Status().Appearance.NtfyServers != "" {
		t.Fatal("пустое значение возвращает сервер по умолчанию")
	}
	// сохранённое значение читается при запуске
	p.NtfyServers = "https://c.example"
	if err := a.SetAppearance(p); err != nil {
		t.Fatal(err)
	}
}