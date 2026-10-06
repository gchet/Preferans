// Package rooms implements the room directory and addressed rendezvous service.
// No game hands or audio are stored here.
package rooms

import "preferans/locales"

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"net/http"
	"preferans/internal/game"
	"preferans/internal/storage"
	"sort"
	"strings"
	"sync"
	"time"
)

func Identity(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

type Member struct {
	Platform  string `json:"platform,omitempty"`
	AIEnabled *bool  `json:"aiEnabled,omitempty"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slot      int    `json:"slot"`
	Online    bool   `json:"online"`
	Seen      int64  `json:"seen"`
}
type Table struct {
	ID      string   `json:"id"`
	Host    string   `json:"host"`
	Name    string   `json:"name"`
	Players []string `json:"players"`
	Bots    []bool   `json:"bots"`
	Keys    []string `json:"keys,omitempty"`
}
type Room struct {
	Deleted        bool            `json:"deleted,omitempty"`
	RemovedMembers map[string]bool `json:"removedMembers,omitempty"`
	DeletedParties map[string]bool `json:"deletedParties,omitempty"`
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Members        []Member        `json:"members"`
	Table          *Table          `json:"table,omitempty"`
	Parties        []Party         `json:"parties,omitempty"`
}
type Signal struct {
	Seq     uint64 `json:"seq"`
	From    string `json:"from"`
	To      string `json:"to"`
	Kind    string `json:"kind"`
	Table   string `json:"table"`
	Payload string `json:"payload"`
	At      int64  `json:"at"`
}
type Request struct {
	Platform  string  `json:"platform,omitempty"`
	AIEnabled *bool   `json:"aiEnabled,omitempty"`
	ElementID string  `json:"elementID,omitempty"`
	Parties   []Party `json:"parties,omitempty"`
	Op        string  `json:"op"`
	Secret    string  `json:"secret"`
	Room      string  `json:"room"`
	Name      string  `json:"name"`
	After     uint64  `json:"after"`
	Epoch     string  `json:"epoch"`
	Table     *Table  `json:"table,omitempty"`
	Signal    *Signal `json:"signal,omitempty"`
	Key       string  `json:"key,omitempty"`
	TableID   string  `json:"tableID,omitempty"`
}
type Response struct {
	AdminDelete    bool       `json:"adminDelete,omitempty"`
	Removed        bool       `json:"removed,omitempty"`
	RemovedRooms   []string   `json:"removedRooms,omitempty"`
	DeletedParties []PartyRef `json:"deletedParties,omitempty"`
	AdminCatalog   bool       `json:"adminCatalog,omitempty"`
	Rooms          []Room     `json:"rooms,omitempty"`
	Room           *Room      `json:"room,omitempty"`
	Signals        []Signal   `json:"signals"`
	Cursor         uint64     `json:"cursor"`
	Epoch          string     `json:"epoch"`
	Error          string     `json:"error,omitempty"`
}
type Server struct {
	mu      sync.Mutex
	Catalog map[string]*Room
	events  map[string][]Signal
	seq     uint64
	epoch   string
	store   storage.Store
	now     func() time.Time
}

func New(dir string) (*Server, error) {
	s := &Server{Catalog: map[string]*Room{}, events: map[string][]Signal{}, epoch: game.ID(), store: storage.Store{Dir: dir}, now: time.Now}
	if dir != "" {
		if ids, e := s.store.IDs(); e != nil {
			return nil, e
		} else {
			for _, id := range ids {
				if id == "rooms" {
					if e = s.store.Load("rooms", &s.Catalog); e != nil {
						return nil, e
					}
				}
			}
		}
	}
	if s.Catalog == nil {
		s.Catalog = map[string]*Room{}
	}
	// A restart never claims an old process is still online.
	for _, r := range s.Catalog {
		for i := range r.Members {
			r.Members[i].Seen = 0
			r.Members[i].Online = false
		}
	}
	return s, nil
}
func (s *Server) save() error {
	if s.store.Dir == "" {
		return nil
	}
	return s.store.Save("rooms", s.Catalog)
}
func (s *Server) refresh(r *Room) {
	for i := range r.Members {
		r.Members[i].Online = s.now().Unix()-r.Members[i].Seen < 12
	}
}
func copyRoom(r *Room) *Room {
	public := *r
	public.Parties = nil
	public.RemovedMembers = nil
	public.DeletedParties = nil
	b, _ := json.Marshal(public)
	var c Room
	_ = json.Unmarshal(b, &c)
	if c.Table != nil {
		c.Table.Keys = nil
	}
	return &c
}
func member(r *Room, id string) int {
	for i, m := range r.Members {
		if m.ID == id {
			return i
		}
	}
	return -1
}
func reserved(r *Room, id string) bool {
	if r.Table != nil {
		for _, p := range r.Table.Players {
			if p == id {
				return true
			}
		}
	}
	return false
}
func (s *Server) Handle(q Request) (Response, error) {
	return s.handle(q, false)
}

// HandleAdmin is called only after the web administrator session is verified.
func (s *Server) HandleAdmin(q Request) (Response, error) {
	return s.handle(q, true)
}

func (s *Server) handle(q Request, admin bool) (Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Response{Epoch: s.epoch, Signals: []Signal{}}
	if len(q.Secret) < 32 || len(q.Secret) > 128 {
		return out, locales.Errorf("go.internal.rooms.rooms.text001")
	}
	id := Identity(q.Secret)
	for _, r := range s.Catalog {
		s.refresh(r)
	}
	s.addRemovals(&out)
	if strings.HasPrefix(q.Op, "admin-delete-") {
		if !admin {
			return out, locales.Errorf("go.admin.login_required")
		}
		if err := s.adminDelete(q); err != nil {
			return out, err
		}
		out.AdminDelete = admin
		s.addRemovals(&out)
		return out, nil
	}
	name := strings.TrimSpace(q.Name)
	if err := s.registerParties(id, q.Parties); err != nil {
		return out, err
	}
	if q.Op == "list" {
		out.AdminCatalog = true
		out.AdminDelete = admin
		out.Rooms = []Room{}
		for _, r := range s.Catalog {
			if r.Deleted {
				continue
			}
			room := copyRoom(r)
			if admin {
				room.Parties = append([]Party(nil), r.Parties...)
			}
			for i := range room.Parties {
				room.Parties[i].Players = append([]PartyPlayer(nil), room.Parties[i].Players...)
			}
			out.Rooms = append(out.Rooms, *room)
		}
		sort.Slice(out.Rooms, func(i, j int) bool { return out.Rooms[i].Name < out.Rooms[j].Name })
		return out, nil
	}
	if q.Op == "create" {
		if len([]rune(name)) < 1 || len([]rune(name)) > 40 {
			return out, locales.Errorf("go.internal.rooms.rooms.text002")
		}
		count := 0
		for _, r := range s.Catalog {
			if !r.Deleted {
				count++
			}
		}
		if count >= 100 {
			return out, locales.Errorf("go.internal.rooms.rooms.text003")
		}
		for _, r := range s.Catalog {
			if !r.Deleted && strings.EqualFold(r.Name, name) {
				return out, locales.Errorf("go.internal.rooms.rooms.text004")
			}
		}
		r := &Room{ID: game.ID(), Name: name, Members: []Member{}}
		s.Catalog[r.ID] = r
		if e := s.save(); e != nil {
			delete(s.Catalog, r.ID)
			return out, e
		}
		out.Room = copyRoom(r)
		return out, nil
	}
	r := s.Catalog[q.Room]
	if q.Op == "poll" && (r == nil || r.Deleted || r.RemovedMembers[id]) {
		out.Removed = true
		// Older clients expect a room object even when the table was closed.
		out.Room = &Room{ID: q.Room, Members: []Member{}}
		return out, nil
	}
	if r == nil || r.Deleted {
		return out, locales.Errorf("go.internal.rooms.rooms.text005")
	}
	if q.Op == "rename" {
		if len([]rune(name)) < 1 || len([]rune(name)) > 40 {
			return out, locales.Errorf("go.internal.rooms.rooms.text006")
		}
		for _, other := range s.Catalog {
			if !other.Deleted && other.ID != r.ID && strings.EqualFold(other.Name, name) {
				return out, locales.Errorf("go.internal.rooms.rooms.text007")
			}
		}
		old := r.Name
		r.Name = name
		if e := s.save(); e != nil {
			r.Name = old
			return out, e
		}
		out.Room = copyRoom(r)
		return out, nil
	}
	if q.Op == "delete" {
		if r.Table != nil {
			return out, locales.Errorf("go.internal.rooms.rooms.text008")
		}
		for _, m := range r.Members {
			if m.Online {
				return out, locales.Errorf("go.internal.rooms.rooms.text009")
			}
		}
		delete(s.Catalog, r.ID)
		if e := s.save(); e != nil {
			s.Catalog[r.ID] = r
			return out, e
		}
		delete(s.events, r.ID)
		return out, nil
	}
	mi := member(r, id)
	if q.Op == "join" || q.Op == "poll" {
		if q.Op == "join" && r.RemovedMembers[id] {
			delete(r.RemovedMembers, id)
			if err := s.save(); err != nil {
				r.RemovedMembers[id] = true
				return out, err
			}
		}
		if len([]rune(name)) < 1 || len([]rune(name)) > 24 {
			return out, locales.Errorf("go.internal.rooms.rooms.text010")
		}
		if mi < 0 {
			for _, other := range s.Catalog {
				if other.ID != r.ID {
					if j := member(other, id); j >= 0 && other.Members[j].Online {
						return out, locales.Errorf("go.internal.rooms.rooms.text011")
					}
				}
			}
			kept := []Member{}
			for _, m := range r.Members {
				if m.Online || reserved(r, m.ID) {
					kept = append(kept, m)
				}
			}
			r.Members = kept
			used := [4]bool{}
			for _, m := range r.Members {
				if m.Slot >= 0 && m.Slot < 4 {
					used[m.Slot] = true
				}
			}
			slot := -1
			for i := range used {
				if !used[i] {
					slot = i
					break
				}
			}
			if slot < 0 {
				return out, locales.Errorf("go.internal.rooms.rooms.text012")
			}
			r.Members = append(r.Members, Member{ID: id, Slot: slot})
			mi = len(r.Members) - 1
		}
		r.Members[mi].Name = name
		r.Members[mi].Seen = s.now().Unix()
		r.Members[mi].Online = true
		r.Members[mi].Platform = q.Platform
		r.Members[mi].AIEnabled = q.AIEnabled
		// Bind legacy saved seats only with their existing private rejoin key.
		if t := r.Table; t != nil && q.TableID == t.ID && q.Key != "" {
			for i, k := range t.Keys {
				if i < len(t.Players) && t.Players[i] == "" && k == Identity(q.Key) {
					t.Players[i] = id
					_ = s.save()
				}
			}
		}
	} else if mi < 0 || !r.Members[mi].Online {
		return out, locales.Errorf("go.internal.rooms.rooms.text013")
	}
	switch q.Op {
	case "join", "poll":
	case "leave":
		if reserved(r, id) {
			r.Members[mi].Online = false
			r.Members[mi].Seen = 0
		} else {
			r.Members = append(r.Members[:mi], r.Members[mi+1:]...)
		}
	case "table":
		t := q.Table
		if t != nil && r.DeletedParties[t.ID] {
			return out, locales.Errorf("go.admin.party_deleted")
		}
		if t == nil || t.ID == "" || len(t.Players) < 3 || len(t.Players) > 4 || len(t.Bots) != len(t.Players) || len(t.Keys) != len(t.Players) || t.Players[0] != id || t.Bots[0] {
			return out, locales.Errorf("go.internal.rooms.rooms.text014")
		}
		if r.Table != nil {
			if r.Table.ID == t.ID && r.Table.Host == id {
				break
			}
			return out, locales.Errorf("go.internal.rooms.rooms.text015", r.Table.Name)
		}
		seen := map[string]bool{}
		for i, p := range t.Players {
			if p != "" {
				if seen[p] || (member(r, p) < 0 && t.Keys[i] == "") {
					return out, locales.Errorf("go.internal.rooms.rooms.text016")
				}
				seen[p] = true
			}
			if !t.Bots[i] && p == "" && t.Keys[i] == "" {
				return out, locales.Errorf("go.internal.rooms.rooms.text017")
			}
		}
		c := *t
		c.Players = append([]string{}, t.Players...)
		c.Bots = append([]bool{}, t.Bots...)
		c.Keys = append([]string{}, t.Keys...)
		c.Host = id
		c.Name = r.Members[mi].Name
		r.Table = &c
		if e := s.save(); e != nil {
			r.Table = nil
			return out, e
		}
	case "close":
		if r.Table != nil {
			if r.Table.Host != id {
				return out, locales.Errorf("go.internal.rooms.rooms.text018")
			}
			if q.TableID != r.Table.ID {
				return out, locales.Errorf("go.internal.rooms.rooms.text019")
			}
			old := r.Table
			r.Table = nil
			if e := s.save(); e != nil {
				r.Table = old
				return out, e
			}
			s.events[r.ID] = nil
		}
	case "signal":
		if q.Signal == nil {
			return out, locales.Errorf("go.internal.rooms.rooms.text020")
		}
		m := *q.Signal
		if len(m.Payload) > 512*1024 || (m.Kind != "offer" && m.Kind != "answer" && m.Kind != "voice") {
			return out, locales.Errorf("go.internal.rooms.rooms.text021")
		}
		if m.To != "" && member(r, m.To) < 0 {
			return out, locales.Errorf("go.internal.rooms.rooms.text022")
		}
		if m.Kind != "voice" {
			t := r.Table
			if t == nil || m.Table != t.ID {
				return out, locales.Errorf("go.internal.rooms.rooms.text023")
			}
			if m.Kind == "offer" && id != t.Host {
				return out, locales.Errorf("go.internal.rooms.rooms.text024")
			}
			if m.Kind == "answer" && (m.To != t.Host || !reserved(r, id)) {
				return out, locales.Errorf("go.internal.rooms.rooms.text025")
			}
		}
		m.From = id
		m.At = s.now().Unix()
		s.seq++
		m.Seq = s.seq
		s.events[r.ID] = append(s.events[r.ID], m)
		if len(s.events[r.ID]) > 512 {
			s.events[r.ID] = s.events[r.ID][len(s.events[r.ID])-512:]
		}
	default:
		return out, locales.Errorf("go.internal.rooms.rooms.text026")
	}
	if q.Op == "poll" || q.Op == "join" {
		for _, m := range s.events[r.ID] {
			if (q.Epoch != s.epoch || m.Seq > q.After) && m.From != id && (m.To == "" || m.To == id) && s.now().Unix()-m.At < 60 {
				out.Signals = append(out.Signals, m)
			}
		}
	}
	out.Room = copyRoom(r)
	out.Cursor = s.seq
	return out, nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	var q Request
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024)).Decode(&q); e != nil {
		http.Error(w, "bad request", 400)
		return
	}
	if strings.HasPrefix(q.Op, "admin-delete-") {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(Response{Error: locales.Text("go.admin.login_required")})
		return
	}
	out, e := s.Handle(q)
	if e != nil {
		out.Error = e.Error()
	}
	_ = json.NewEncoder(w).Encode(out)
}
