package rooms

import "reflect"

// Party contains public directory metadata, never hands or connection keys.
type PartyPlayer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Bot  bool   `json:"bot"`
}
type Party struct {
	ID         string        `json:"id"`
	Room       string        `json:"room"`
	Host       string        `json:"host"`
	Players    []PartyPlayer `json:"players"`
	Stage      string        `json:"stage"`
	Round      int           `json:"round"`
	Revision   uint64        `json:"revision"`
	StartedAt  int64         `json:"startedAt"`
	FinishedAt int64         `json:"finishedAt"`
	UpdatedAt  int64         `json:"updatedAt"`
}

// Called with the directory lock held. Duplicate device reports share one ID.
func (s *Server) registerParties(reporter string, parties []Party) error {
	if len(parties) > 512 {
		return nil
	}
	backup := map[string][]Party{}
	for _, party := range parties {
		r := s.Catalog[party.Room]
		if r == nil || r.Deleted || r.DeletedParties[party.ID] || party.ID == "" || len(party.ID) > 128 || len(party.Players) < 3 || len(party.Players) > 4 || party.Round < 0 {
			continue
		}
		valid, authorized := true, false
		for _, p := range party.Players {
			if len([]rune(p.Name)) > 24 || len(p.ID) > 128 {
				valid = false
			}
			if !p.Bot && p.ID == reporter {
				authorized = true
			}
		}
		if !valid || !authorized {
			continue
		}
		party.Players = append([]PartyPlayer(nil), party.Players...)
		switch party.Stage {
		case "lobby", "auction", "discard", "declare", "defend", "return", "option", "play", "trick", "round", "finished", "catch", "mode":
		default:
			continue
		}
		index := -1
		for i, p := range r.Parties {
			if p.ID == party.ID {
				index = i
				break
			}
		}
		if index >= 0 {
			old := r.Parties[index]
			if old.Revision > party.Revision || old.Stage == "finished" && party.Stage != "finished" && old.Revision >= party.Revision {
				continue
			}
			party.UpdatedAt = old.UpdatedAt
			if reflect.DeepEqual(old, party) {
				continue
			}
		} else if len(r.Parties) >= 500 {
			continue
		}
		if _, ok := backup[r.ID]; !ok {
			backup[r.ID] = append([]Party(nil), r.Parties...)
		}
		party.UpdatedAt = s.now().Unix()
		if index < 0 {
			r.Parties = append(r.Parties, party)
		} else {
			r.Parties[index] = party
		}
	}
	if len(backup) > 0 {
		if err := s.save(); err != nil {
			for id, parties := range backup {
				s.Catalog[id].Parties = parties
			}
			return err
		}
	}
	return nil
}
