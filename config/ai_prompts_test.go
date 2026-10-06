package config

import (
	"strings"
	"testing"
)

func TestAIPromptLanguagesAndStages(t *testing.T) {
	for key, translations := range aiPrompts.Agreements {
		for _, language := range []string{"ru", "en"} {
			if strings.TrimSpace(translations[language]) == "" {
				t.Fatal("missing agreement translation", key, language)
			}
		}
	}
	for _, language := range []string{"ru", "en"} {
		for _, stage := range []string{"auction", "discard", "contract", "defend", "dealer-choice", "dealer-whist", "catch", "mode", "play"} {
			prompt := AIPrompt(stage, language)
			if !strings.Contains(prompt, "legal_actions") || !strings.Contains(prompt, "why") {
				t.Fatal("incomplete prompt", language, stage)
			}
		}
		repair := AIRepairPrompt(language, 17)
		if !strings.Contains(repair, "17") || strings.Contains(repair, "{lastIndex}") {
			t.Fatal("repair placeholder not filled")
		}
		if AIDiscussionPrompt(language) == "" {
			t.Fatal("missing discussion prompt")
		}
	}
}
