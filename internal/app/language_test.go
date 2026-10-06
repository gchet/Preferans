package app

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLanguagePersistsAndBootstrapsBeforeUI(t *testing.T) {
	for _, language := range []string{"", "ru", "uk", "en"} {
		t.Run(language, func(t *testing.T) {
			dir := t.TempDir()
			a := New(dir)
			p := a.Status().Appearance
			p.Language = language
			if err := a.SetAppearance(p); err != nil {
				t.Fatal(err)
			}
			a.Close()
			b := New(dir)
			defer b.Close()
			if b.Status().Appearance.Language != language {
				t.Fatal("language lost after restart")
			}
			assets := fstest.MapFS{"index.html": {Data: []byte(`<html lang="ru"><body></body></html>`)}}
			url, stop, err := Serve(b, assets)
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			response, err := http.Get(strings.Split(url, "#")[0])
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			data, _ := io.ReadAll(response.Body)
			want := language
			if want == "" {
				want = "uk"
			}
			if !strings.Contains(string(data), `data-language="`+want+`"`) {
				t.Fatalf("missing initial language in %s", data)
			}
			p.Language = "invalid"
			if err := b.SetAppearance(p); err == nil {
				t.Fatal("invalid language accepted")
			}
			if b.Status().Appearance.Language != language {
				t.Fatal("invalid update changed language")
			}
		})
	}
}
