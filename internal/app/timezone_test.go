package app

import (
	"regexp"
	"testing"
	"time"
)

func TestZoneLabel(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Skip("нет базы часовых поясов")
	}
	if got := zoneLabel(time.Date(2026, 11, 2, 0, 3, 0, 0, kyiv)); got != "UTC+02:00 (Europe/Kyiv, EET)" {
		t.Fatal(got)
	}
	if got := zoneLabel(time.Date(2026, 7, 2, 0, 3, 0, 0, time.FixedZone("", -(3*3600+30*60)))); got != "UTC-03:30" {
		t.Fatal(got)
	}
	if got := zoneLabel(time.Date(2026, 7, 2, 0, 3, 0, 0, time.UTC)); got != "UTC+00:00 (UTC)" {
		t.Fatal(got)
	}
}

func TestLogLineHasDateAndLocalZoneAnnounced(t *testing.T) {
	old := time.Local
	defer func() { time.Local = old }()
	SetLocalZone("", 2*3600)
	a := New(t.TempDir())
	defer a.Close()
	a.mu.Lock()
	a.logLocked("проба")
	a.logZoneLocked()
	a.mu.Unlock()
	logs := a.Status().Logs
	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}  `)
	for _, l := range logs {
		if !re.MatchString(l) {
			t.Fatalf("строка журнала без даты: %q", l)
		}
	}
	if last := logs[len(logs)-1]; !regexp.MustCompile(`UTC\+02:00`).MatchString(last) {
		t.Fatalf("нет часового пояса: %q", last)
	}
}