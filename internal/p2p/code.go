package p2p

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// Version кода приглашения. Старые коды (PREF1, WebRTC SDP) не поддерживаются.
const Version = 2

const (
	KindTable   = "table"
	KindService = "svc"
)

const codePrefix = "PREF2."

// Code — то, что игрок передаёт другому игроку (или хост комнаты — гостю): без SDP и без ответного кода.
type Code struct {
	V      int    `json:"v"`
	Kind   string `json:"k"`
	Table  string `json:"t,omitempty"`
	Seat   int    `json:"s,omitempty"`
	Nonce  string `json:"n,omitempty"`
	Key    string `json:"key,omitempty"`
	Invite Invite `json:"i"`
}

func Encode(c Code) (string, error) {
	c.V = Version
	if c.Kind == "" {
		c.Kind = KindTable
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return codePrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func Decode(s string) (Code, error) {
	var c Code
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, codePrefix) {
		return c, errors.New("код должен начинаться с PREF2.")
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, codePrefix))
	if err != nil {
		return c, errors.New("код повреждён")
	}
	if len(b) > 16*1024 {
		return c, errors.New("код слишком длинный")
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, errors.New("код повреждён")
	}
	if c.V != Version || c.Invite.ID == "" {
		return c, errors.New("неподдерживаемая версия кода")
	}
	if c.Kind != KindTable && c.Kind != KindService {
		return c, errors.New("неизвестный вид кода")
	}
	return c, nil
}

// ServiceCode — стабильный код сервиса комнат: только peer ID, свежие адреса берутся из публичного объявления.
func ServiceCode(id string) (string, error) {
	return Encode(Code{Kind: KindService, Invite: Invite{ID: id}})
}