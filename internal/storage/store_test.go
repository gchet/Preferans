package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicReplaceAndCorruption(t *testing.T) {
	s := Store{t.TempDir()}
	for i := 0; i < 10; i++ {
		if e := s.Save("table", map[string]int{"revision": i}); e != nil {
			t.Fatal(e)
		}
		var v map[string]int
		if e := s.Load("table", &v); e != nil || v["revision"] != i {
			t.Fatal(e, v)
		}
	}
	if e := s.Save("../escape", 1); e == nil {
		t.Fatal("path traversal")
	}
	_ = os.WriteFile(filepath.Join(s.Dir, "table.json"), []byte(`{"sha256":"bad","data":{}}`), 0600)
	var v any
	if e := s.Load("table", &v); e == nil {
		t.Fatal("corruption accepted")
	}
}
