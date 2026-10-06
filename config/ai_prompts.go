package config

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
)

//go:embed ai-prompts.json
var aiPromptsJSON []byte

var aiPrompts struct {
	Prompts    map[string]map[string]string `json:"prompts"`
	Agreements map[string]map[string]string `json:"agreements"`
}

// AIAgreement formats a resource-owned explanation of a selected table rule.
func AIAgreement(key, language string, values map[string]string) string {
	if language != "en" {
		language = "ru"
	}
	text := aiPrompts.Agreements[key][language]
	for name, value := range values {
		text = strings.ReplaceAll(text, "{"+name+"}", value)
	}
	return text
}

func init() {
	if err := json.Unmarshal(aiPromptsJSON, &aiPrompts); err != nil {
		panic(err)
	}
	for _, key := range []string{"common", "auction", "discard", "contract", "defence", "play", "repair", "discussion"} {
		for _, language := range []string{"ru", "en"} {
			if strings.TrimSpace(aiPrompts.Prompts[key][language]) == "" {
				panic("missing AI prompt: " + key + "/" + language)
			}
		}
	}
}

func AIDiscussionPrompt(language string) string {
	if language != "en" {
		language = "ru"
	}
	return aiPrompts.Prompts["discussion"][language]
}

func AIPrompt(stage, language string) string {
	if language != "en" {
		language = "ru"
	}
	switch stage {
	case "auction", "discard", "contract":
	case "defend", "option", "return", "dealer-choice", "dealer-whist", "mode", "catch":
		stage = "defence"
	default:
		stage = "play"
	}
	return aiPrompts.Prompts["common"][language] + "\n\n" + aiPrompts.Prompts[stage][language]
}

func AIRepairPrompt(language string, lastIndex int) string {
	if language != "en" {
		language = "ru"
	}
	return strings.ReplaceAll(aiPrompts.Prompts["repair"][language], "{lastIndex}", strconv.Itoa(lastIndex))
}
