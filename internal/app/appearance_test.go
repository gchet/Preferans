package app

import (
	"reflect"
	"testing"
)

func TestRetiredDeckMigrationPreservesPreferences(t *testing.T) {
	for _, deck := range []string{"modern", "russian", "european", "portraits", "jumbo"} {
		t.Run(deck, func(t *testing.T) {
			dir := t.TempDir()
			a := New(dir)
			p := Appearance{Name: "Гена", Deck: deck, Size: "extra", Back: "waves", LogDisabled: true}
			if err := a.config.Save("appearance", p); err != nil {
				t.Fatal(err)
			}
			a.Close()
			b := New(dir)
			defer b.Close()
			p.Deck = "atlas"
			if deck == "jumbo" {
				p.Deck = "english"
			}
			if !reflect.DeepEqual(b.Status().Appearance, p) {
				t.Fatal("migration lost local preferences")
			}
		})
	}
}

func TestAppearancePersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	p := Appearance{Name: "Тест", Deck: "woodcut", Size: "extra", LargeIndices: true, Back: "ornament", UpdateURL: "http://192.168.1.10:8787"}
	p.CardFrames = map[string]bool{"atlas": false, "english": true, "woodcut": false}
	if _, err := a.RPC(Request{Action: "appearance", Appearance: p}); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b := New(dir)
	defer b.Close()
	if !reflect.DeepEqual(b.Status().Appearance, p) {
		t.Fatal("appearance was not restored")
	}
	if err := b.SetAppearance(Appearance{Deck: "invalid", Size: "normal"}); err == nil {
		t.Fatal("invalid deck accepted")
	}
	if !reflect.DeepEqual(b.Status().Appearance, p) {
		t.Fatal("invalid request changed appearance")
	}
}
