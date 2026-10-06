package app

import "preferans/locales"

import (
	"encoding/json"

	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"preferans/internal/game"
	"preferans/internal/network"
)

type Request struct {
	URL        string       `json:"url"`
	AI         AISettings   `json:"ai"`
	ModelID    string       `json:"modelID"`
	Secret     string       `json:"secret"`
	ElementID  string       `json:"elementID,omitempty"`
	Voice      VoiceMessage `json:"voice"`
	After      uint64       `json:"after"`
	Action     string       `json:"action"`
	Players    int          `json:"players"`
	Target     int          `json:"target"`
	Name       string       `json:"name"`
	Bots       bool         `json:"bots"`
	Seat       int          `json:"seat"`
	Code       string       `json:"code"`
	ID         string       `json:"id"`
	STUN       []string     `json:"stun"`
	Command    game.Command `json:"command"`
	Appearance Appearance   `json:"appearance"`
	LogEnabled bool         `json:"logEnabled"`
}

func (a *App) RPC(r Request) (any, error) {
	if strings.HasPrefix(r.Action, "ai-") {
		a.mu.Lock()
		enabled := a.aiEnabledLocked()
		a.mu.Unlock()
		if !enabled {
			return nil, aiError("build-disabled")
		}
	}
	switch r.Action {
	case "ai-review-ask":
		return a.AskAIReview(r.ID, r.Name)
	case "ai-review-continue":
		return nil, a.ContinueAIReview(r.ID)
	case "debug-inspect":
		return nil, a.ToggleDebugInspection(r.Seat)
	case "debug-bot-options":
		return a.DebugBotOptions(r.Seat, r.Command.Revision, r.Command.Cards)
	case "debug-bot-continue":
		return nil, a.DebugBotContinue(r.Seat, r.Command.Revision)
	case "debug-bot-command":
		return nil, a.DebugBotCommand(r.Command)
	case "ai-transfer-create":
		return a.StartAITransfer()
	case "ai-transfer-import":
		return nil, a.ImportAITransfer(r.Name, r.Code)
	case "ai-settings":
		return a.AISettings(), nil
	case "ai-save":
		return nil, a.SaveAISettings(r.AI)
	case "ai-key-save":
		return a.SaveAIKey(r.ID, r.Name, r.Secret)
	case "ai-key-delete":
		return nil, a.DeleteAIKey(r.ID)
	case "ai-assign":
		return nil, a.AssignBot(r.Seat, r.ModelID)
	case "room-service":
		return nil, a.SetRoomService(r.URL)
	case "connection-setup":
		return nil, a.CheckAndSaveConnection(r.STUN, r.Name)
	case "voice-send":
		return nil, a.SendVoice(r.Voice)
	case "voice-poll":
		return a.PollVoice(r.ID, r.After)
	case "diagnostics":
		return a.Diagnostics(), nil
	case "status":
		return a.Status(), nil
	case "list":
		return a.List()
	case "history":
		return a.History()
	case "history-result":
		return a.HistoryResult(r.ID)
	case "delete-save":
		return nil, a.DeleteSave(r.ID)
	case "create":
		if a.inRoom() {
			return nil, a.roomTable("", r.Players, r.Target, r.Name, r.Bots)
		}
		return nil, a.Create(r.Players, r.Target, r.Name, r.Bots)
	case "fill-bots":
		return a.FillBots()
	case "resume":
		if a.inRoom() {
			return nil, a.roomTable(r.ID, 0, 0, "", false)
		}
		return nil, a.Resume(r.ID)
	case "leave":
		if a.inRoom() {
			return nil, a.RoomLeaveTable()
		}
		a.Leave()
		return nil, nil
	case "configure":
		return nil, a.Configure(r.STUN)
	case "appearance":
		return nil, a.SetAppearance(r.Appearance)
	case "logging":
		return nil, a.SetLogEnabled(r.LogEnabled)
	case "log-clear":
		return nil, a.ClearLogFile()
	case "profile":
		return nil, a.SetName(r.Name)
	case "invite":
		return a.Invite(r.Seat)
	case "answer":
		if err := a.Answer(r.Code); err != nil {
			return nil, err
		}
		code, err := network.Decode(r.Code)
		if err != nil {
			return nil, err
		}
		return map[string]int{"seat": code.Seat}, nil
	case "join":
		return a.Join(r.Code)
	case "command":
		return nil, a.Command(r.Command)
	case "rooms-list", "rooms-create", "rooms-rename", "rooms-delete":
		return a.RoomDirectory(strings.TrimPrefix(r.Action, "rooms-"), r.ID, r.Name)
	case "room-enter":
		return nil, a.EnterRoom(r.ID)
	case "room-local":
		return nil, a.EnterRoom(localRoomID)
	case "room-exit":
		return nil, a.ExitRoom()
	case "room-rejoin":
		a.mu.Lock()
		a.rooms.config.OptOut = ""
		err := a.config.Save("room-session", a.rooms.config)
		a.mu.Unlock()
		return nil, err
	}
	return nil, locales.Errorf("go.internal.app.http.text001")
}
func Serve(a *App, assets fs.FS) (string, func(), error) {
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return "", nil, e
	}
	token := game.ID()
	origin := "http://" + listener.Addr().String()
	mux := http.NewServeMux()
	files := http.FileServer(http.FS(assets))
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "POST" || r.Header.Get("X-Preferans-Token") != token || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) {
			http.Error(w, "Forbidden", 403)
			return
		}
		var q Request
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, networkLimit)).Decode(&q); e != nil {
			http.Error(w, "Bad request", 400)
			return
		}
		v, e := a.RPC(q)
		if e != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": e.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' http: https:; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			if data, err := fs.ReadFile(assets, "index.html"); err == nil {
				a.mu.Lock()
				language := a.appearance.Language
				a.mu.Unlock()
				if language != "ru" && language != "en" {
					language = "uk"
				}
				// Bootstrap before modules run; the HTTP port changes on every launch.
				html := strings.Replace(string(data), "<html", `<html data-language="`+language+`"`, 1)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(html))
				return
			}
		}
		files.ServeHTTP(w, r)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(listener) }()
	return origin + "/#" + token, func() { _ = srv.Close(); a.Close() }, nil
}

const networkLimit = 1024 * 1024
