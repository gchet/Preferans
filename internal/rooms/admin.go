package rooms

import (
	"encoding/json"
	"preferans/locales"
	"sort"
)

type PartyRef struct {
	Room string `json:"room"`
	ID   string `json:"id"`
}

func (s *Server) addRemovals(out *Response) {
	out.RemovedRooms = nil
	out.DeletedParties = nil
	for _, r := range s.Catalog {
		if r.Deleted {
			out.RemovedRooms = append(out.RemovedRooms, r.ID)
		}
		for id := range r.DeletedParties {
			out.DeletedParties = append(out.DeletedParties, PartyRef{Room: r.ID, ID: id})
		}
	}
	sort.Strings(out.RemovedRooms)
	sort.Slice(out.DeletedParties, func(i, j int) bool {
		return out.DeletedParties[i].Room+out.DeletedParties[i].ID < out.DeletedParties[j].Room+out.DeletedParties[j].ID
	})
}

// Called with mu held. Tombstones prevent deleted saves from being re-published.
func (s *Server) adminDelete(q Request) error {
	r := s.Catalog[q.Room]
	if r == nil || r.Deleted {
		return locales.Errorf("go.internal.rooms.rooms.text005")
	}
	b, _ := json.Marshal(r)
	var backup Room
	_ = json.Unmarshal(b, &backup)
	switch q.Op {
	case "admin-delete-room":
		r.Deleted = true
		r.Members = nil
		r.Table = nil
		r.Parties = nil
	case "admin-delete-player":
		index := member(r, q.ElementID)
		if index < 0 {
			return locales.Errorf("go.admin.player_missing")
		}
		if r.RemovedMembers == nil {
			r.RemovedMembers = map[string]bool{}
		}
		r.RemovedMembers[q.ElementID] = true
		r.Members = append(r.Members[:index], r.Members[index+1:]...)
		if r.Table != nil && r.Table.Host == q.ElementID {
			r.Table = nil
		}
	case "admin-delete-party":
		found := false
		for i, p := range r.Parties {
			if p.ID == q.ElementID {
				r.Parties = append(r.Parties[:i], r.Parties[i+1:]...)
				found = true
				break
			}
		}
		if r.Table != nil && r.Table.ID == q.ElementID {
			r.Table = nil
			found = true
		}
		if !found {
			return locales.Errorf("go.admin.party_missing")
		}
		if r.DeletedParties == nil {
			r.DeletedParties = map[string]bool{}
		}
		r.DeletedParties[q.ElementID] = true
	default:
		return locales.Errorf("go.internal.rooms.rooms.text026")
	}
	if err := s.save(); err != nil {
		s.Catalog[r.ID] = &backup
		return err
	}
	s.events[r.ID] = nil
	return nil
}
