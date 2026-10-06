package config

import (
	_ "embed"
	"encoding/json"
	"reflect"
)

//go:embed interface.json
var interfaceJSON []byte

// Timings are shared with WebUI and embedded in both native builds.
var Interface struct {
	LocalBotSearchTimeMs       int    `json:"localBotSearchTimeMs"`
	LocalBotSearchSamples      int    `json:"localBotSearchSamples"`
	LocalBotExactCards         int    `json:"localBotExactCards"`
	LocalBotExactCardsLimit    int    `json:"localBotExactCardsLimit"`
	RoomServiceURL             string `json:"roomServiceURL"`
	RoomRequestTimeoutMs       int    `json:"roomRequestTimeoutMs"`
	RoomPollIntervalMs         int    `json:"roomPollIntervalMs"`
	PartyCatalogIntervalMs     int    `json:"partyCatalogIntervalMs"`
	RoomOnlineTimeoutMs        int    `json:"roomOnlineTimeoutMs"`
	HeartbeatIntervalMs        int    `json:"heartbeatIntervalMs"`
	ConnectionSilenceTimeoutMs int    `json:"connectionSilenceTimeoutMs"`
	ExitNoticeTimeoutMs        int    `json:"exitNoticeTimeoutMs"`
	ReconnectRetryMs           int    `json:"reconnectRetryMs"`
	HandshakeRetryMs           int    `json:"handshakeRetryMs"`
	AnswerRetryMs              int    `json:"answerRetryMs"`
	BotTickIntervalMs          int    `json:"botTickIntervalMs"`
	NextTrickDelayMs           int    `json:"nextTrickDelayMs"`
}

func init() {
	if err := json.Unmarshal(interfaceJSON, &Interface); err != nil {
		panic(err)
	}
	fields := reflect.ValueOf(Interface)
	for i := 0; i < fields.NumField(); i++ {
		field := fields.Field(i)
		if field.Kind() == reflect.Int && (field.Int() <= 0 || field.Int() > 2147483647) {
			panic("invalid configuration parameter: " + fields.Type().Field(i).Name)
		}
	}
	loadNetwork()
}
