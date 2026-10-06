// Package locales embeds the shared language resources used by all clients.
package locales

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed *.json
var files embed.FS

var catalogs = load()

func load() map[string]map[string]string {
	result := make(map[string]map[string]string)
	for _, language := range []string{"ru", "uk", "en"} {
		data, err := files.ReadFile(language + ".json")
		if err != nil {
			panic(err)
		}
		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			panic(err)
		}
		result[language] = messages
	}
	return result
}

// Lookup never changes a process-wide language: different clients may use
// different languages. Untranslated messages fall back to Russian.
func Lookup(language, key string) string {
	if value := catalogs[language][key]; value != "" {
		return value
	}
	if value := catalogs["ru"][key]; value != "" {
		return value
	}
	return key
}

// Shared game state and disk logs use stable Russian messages. Clients localize
// presentation per device so the host cannot change another player's language.
func Text(key string) string                { return Lookup("ru", key) }
func Format(key string, args ...any) string { return fmt.Sprintf(Text(key), args...) }
func Errorf(key string, args ...any) error  { return fmt.Errorf(Text(key), args...) }
