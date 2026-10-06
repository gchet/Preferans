package storage

import "preferans/locales"

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type Store struct{ Dir string }

func (s Store) Delete(id string) error {
	if !safe.MatchString(id) {
		return fmt.Errorf("Invalid save identifier")
	}
	err := os.Remove(filepath.Join(s.Dir, id+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

var safe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type envelope struct {
	Hash string          `json:"sha256"`
	Data json.RawMessage `json:"data"`
}

func (s Store) Save(id string, value any) error {
	if !safe.MatchString(id) {
		return fmt.Errorf("Invalid save identifier")
	}
	if e := os.MkdirAll(s.Dir, 0700); e != nil {
		return e
	}
	b, e := json.Marshal(value)
	if e != nil {
		return e
	}
	h := sha256.Sum256(b)
	out, e := json.Marshal(envelope{hex.EncodeToString(h[:]), b})
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.Dir, ".save-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(out)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(tmp, filepath.Join(s.Dir, id+".json"))
}
func (s Store) Load(id string, v any) error {
	if !safe.MatchString(id) {
		return fmt.Errorf("Invalid save identifier")
	}
	b, e := os.ReadFile(filepath.Join(s.Dir, id+".json"))
	if e != nil {
		return e
	}
	var env envelope
	if e = json.Unmarshal(b, &env); e != nil {
		return e
	}
	h := sha256.Sum256(env.Data)
	if hex.EncodeToString(h[:]) != env.Hash {
		return locales.Errorf("go.internal.storage.store.text001")
	}
	return json.Unmarshal(env.Data, v)
}
func (s Store) IDs() ([]string, error) {
	files, e := os.ReadDir(s.Dir)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var ids []string
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			ids = append(ids, f.Name()[:len(f.Name())-5])
		}
	}
	return ids, nil
}
