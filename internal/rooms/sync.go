package rooms

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RoomSnap — долговечная часть каталога, которая реплицируется между устройствами игроков:
// название комнаты, надгробия удалений и история партий. Участники, стол и сигналы не реплицируются:
// они живые, и каждый игрок восстанавливает их, заходя в комнату на актуальном ведущем.
type RoomSnap struct {
	ID             string   `json:"id"`
	Name           string   `json:"name,omitempty"`
	Updated        int64    `json:"updated,omitempty"`
	Created        int64    `json:"created,omitempty"`
	Creator        string   `json:"creator,omitempty"`
	CreatorName    string   `json:"creatorName,omitempty"`
	Deleted        bool     `json:"deleted,omitempty"`
	DeletedParties []string `json:"deletedParties,omitempty"`
	Parties        []Party  `json:"parties,omitempty"`
}

const (
	maxSnapRooms   = 200
	maxRoomParties = 500
)

// snapshot вызывается с захваченным s.mu.
func (s *Server) snapshot() []RoomSnap {
	out := make([]RoomSnap, 0, len(s.Catalog))
	for _, r := range s.Catalog {
		snap := RoomSnap{ID: r.ID, Updated: r.Updated, Deleted: r.Deleted}
		if !r.Deleted {
			snap.Name = r.Name
			snap.Created, snap.Creator, snap.CreatorName = r.Created, r.Creator, r.CreatorName
			snap.Parties = append([]Party(nil), r.Parties...)
			for i := range snap.Parties {
				snap.Parties[i].Players = append([]PartyPlayer(nil), snap.Parties[i].Players...)
			}
		}
		for id := range r.DeletedParties {
			snap.DeletedParties = append(snap.DeletedParties, id)
		}
		sort.Strings(snap.DeletedParties)
		out = append(out, snap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Snapshot возвращает долговечную часть каталога для реплики.
func (s *Server) Snapshot() []RoomSnap {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

func validName(name string, max int) bool {
	n := len([]rune(strings.TrimSpace(name)))
	return n >= 1 && n <= max
}

func validSnapParty(p *Party) bool {
	if p.ID == "" || len(p.ID) > 128 || len(p.Players) < 3 || len(p.Players) > 4 || p.Round < 0 {
		return false
	}
	for _, pl := range p.Players {
		if len([]rune(pl.Name)) > 24 || len(pl.ID) > 128 {
			return false
		}
	}
	switch p.Stage {
	case "lobby", "auction", "discard", "declare", "defend", "return", "option", "play", "trick", "round", "finished", "catch", "mode":
		return true
	}
	return false
}

// Merge вливает снимок чужого каталога в этот. Слияние коммутативно и идемпотентно: надгробия
// комнат и партий побеждают, название берётся с более поздним Updated, партия — с большей Revision.
// Живое состояние (участники, стол) не меняется. Возвращает число изменённых комнат.
func (s *Server) Merge(in []RoomSnap) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mergeLocked(in)
}

// mergeLocked вызывается с захваченным s.mu.
func (s *Server) mergeLocked(in []RoomSnap) (int, error) {
	if len(in) > maxSnapRooms {
		return 0, fmt.Errorf("слишком много комнат в снимке: %d", len(in))
	}
	before, _ := json.Marshal(s.Catalog)
	changed := map[string]bool{}
	for _, sn := range in {
		if sn.ID == "" || len(sn.ID) > 128 {
			continue
		}
		r := s.Catalog[sn.ID]
		if sn.Deleted {
			switch {
			case r == nil:
				s.Catalog[sn.ID] = &Room{ID: sn.ID, Deleted: true, Updated: sn.Updated}
				changed[sn.ID] = true
			case !r.Deleted:
				r.Deleted, r.Members, r.Table, r.Parties = true, nil, nil, nil
				if sn.Updated > r.Updated {
					r.Updated = sn.Updated
				}
				changed[sn.ID] = true
				s.events[r.ID] = nil
			}
			continue
		}
		if r != nil && r.Deleted {
			continue
		}
		if r == nil {
			if !validName(sn.Name, 40) {
				continue
			}
			r = &Room{ID: sn.ID, Name: strings.TrimSpace(sn.Name), Updated: sn.Updated, Members: []Member{}}
			s.Catalog[sn.ID] = r
			changed[sn.ID] = true
		}
		if mergeCreator(r, sn) {
			changed[sn.ID] = true
		}
		if sn.Updated > r.Updated && validName(sn.Name, 40) && strings.TrimSpace(sn.Name) != r.Name {
			r.Name, r.Updated = strings.TrimSpace(sn.Name), sn.Updated
			changed[sn.ID] = true
		}
		for _, id := range sn.DeletedParties {
			if id == "" || len(id) > 128 || r.DeletedParties[id] {
				continue
			}
			if r.DeletedParties == nil {
				r.DeletedParties = map[string]bool{}
			}
			r.DeletedParties[id] = true
			for i, p := range r.Parties {
				if p.ID == id {
					r.Parties = append(r.Parties[:i], r.Parties[i+1:]...)
					break
				}
			}
			if r.Table != nil && r.Table.ID == id {
				r.Table = nil
			}
			changed[sn.ID] = true
		}
		for _, p := range sn.Parties {
			if !validSnapParty(&p) || r.DeletedParties[p.ID] {
				continue
			}
			p.Room = r.ID
			p.Players = append([]PartyPlayer(nil), p.Players...)
			index := -1
			for i, old := range r.Parties {
				if old.ID == p.ID {
					index = i
					break
				}
			}
			if index < 0 {
				if len(r.Parties) >= maxRoomParties {
					continue
				}
				r.Parties = append(r.Parties, p)
			} else {
				old := r.Parties[index]
				if !(p.Revision > old.Revision || p.Revision == old.Revision && old.Stage != "finished" && p.Stage == "finished") {
					continue
				}
				r.Parties[index] = p
			}
			changed[sn.ID] = true
		}
	}
	s.resolveNames(changed)
	if len(changed) == 0 {
		return 0, nil
	}
	if err := s.save(); err != nil {
		var restored map[string]*Room
		if json.Unmarshal(before, &restored) == nil {
			s.Catalog = restored
		}
		return 0, err
	}
	return len(changed), nil
}

// mergeCreator дописывает создателя комнаты из чужого снимка. Если создатели разошлись, побеждает
// записанный раньше (при равенстве — меньший идентификатор): так выбирают все реплики одинаково.
func mergeCreator(r *Room, sn RoomSnap) bool {
	if sn.Creator == "" || len(sn.Creator) > 128 {
		return false
	}
	if r.Creator != "" {
		if r.Creator == sn.Creator || r.Created == 0 && sn.Created == 0 {
			return false
		}
		older := sn.Created > 0 && (r.Created == 0 || sn.Created < r.Created || sn.Created == r.Created && sn.Creator < r.Creator)
		if !older {
			return false
		}
	}
	r.Creator, r.Created, r.CreatorName = sn.Creator, sn.Created, trimRunes(sn.CreatorName, 24)
	return true
}

// resolveNames: после слияния две живые комнаты могли получить одно название. Проигрывает комната с большим ID
// (так выбирают все реплики одинаково): ей добавляется номер.
func (s *Server) resolveNames(changed map[string]bool) {
	ids := make([]string, 0, len(s.Catalog))
	for id, r := range s.Catalog {
		if !r.Deleted {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	used := map[string]bool{}
	for _, id := range ids {
		r := s.Catalog[id]
		key := strings.ToLower(r.Name)
		if used[key] {
			base := []rune(r.Name)
			for n := 2; ; n++ {
				suffix := fmt.Sprintf(" %d", n)
				if len(base)+len([]rune(suffix)) > 40 {
					base = base[:40-len([]rune(suffix))]
				}
				cand := string(base) + suffix
				if !used[strings.ToLower(cand)] {
					r.Name = cand
					r.Updated = s.now().Unix()
					changed[id] = true
					key = strings.ToLower(cand)
					break
				}
			}
		}
		used[key] = true
	}
}