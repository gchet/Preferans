package locales

import (
	"errors"
	"testing"
)

func TestFallbackAndErrorWrapping(t *testing.T) {
	key := "go.internal.game.types.text005"
	for _, language := range []string{"ru", "uk", "en", "unknown"} {
		want := catalogs[language][key]
		if want == "" {
			want = catalogs["ru"][key]
		}
		if got := Lookup(language, key); got != want {
			t.Fatalf("fallback %s: %q", language, got)
		}
	}
	if Lookup("en", "missing.key") != "missing.key" {
		t.Fatal("missing key must remain diagnosable")
	}
	catalogs["ru"]["test.wrap"] = "Ошибка: %w"
	defer delete(catalogs["ru"], "test.wrap")
	original := errors.New("underlying")
	if !errors.Is(Errorf("test.wrap", original), original) {
		t.Fatal("lost wrapped error")
	}
}

func TestAllLanguagesComplete(t *testing.T) {
	for _, language := range []string{"uk", "en"} {
		for key := range catalogs["ru"] {
			if catalogs[language][key] == "" {
				t.Errorf("%s: missing %s", language, key)
			}
		}
	}
}
