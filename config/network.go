package config

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
)

//go:embed network.json all:local
var networkFiles embed.FS

// Network contains public defaults and optional build-time site overrides.
// Passwords and API keys are never accepted as build configuration.
var Network struct {
	TurnServer         string `json:"turnServer"`
	TurnUser           string `json:"turnUser"`
	LegacySignalingURL string `json:"legacySignalingURL"`
	UpdateAddressHint  string `json:"updateAddressHint"`
}

func loadNetwork() {
	defaults, err := networkFiles.ReadFile("network.json")
	if err != nil {
		panic(err)
	}
	if err = json.Unmarshal(defaults, &Network); err != nil {
		panic(err)
	}
	if overrides, err := networkFiles.ReadFile("local/build.json"); err == nil {
		var site struct {
			RoomServiceURL     string `json:"roomServiceURL"`
			TurnServer         string `json:"turnServer"`
			TurnUser           string `json:"turnUser"`
			LegacySignalingURL string `json:"legacySignalingURL"`
			UpdateAddressHint  string `json:"updateAddressHint"`
		}
		if err = json.Unmarshal(overrides, &site); err != nil {
			panic(err)
		}
		if site.RoomServiceURL != "" {
			Interface.RoomServiceURL = site.RoomServiceURL
		}
		if site.TurnServer != "" {
			Network.TurnServer = site.TurnServer
		}
		if site.TurnUser != "" {
			Network.TurnUser = site.TurnUser
		}
		if site.LegacySignalingURL != "" {
			Network.LegacySignalingURL = site.LegacySignalingURL
		}
		if site.UpdateAddressHint != "" {
			Network.UpdateAddressHint = site.UpdateAddressHint
		}
	}
	for _, value := range []string{Interface.RoomServiceURL, Network.LegacySignalingURL, Network.UpdateAddressHint} {
		if err := ValidateHTTPURL(value); err != nil {
			panic(err)
		}
	}
}

func ValidateHTTPURL(value string) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("invalid HTTP service URL")
	}
	return nil
}
