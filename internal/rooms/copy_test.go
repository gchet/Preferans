package rooms

import "testing"

func TestCopyRoomNeverHasNilMembers(t *testing.T) {
	if c := copyRoom(&Room{ID: "r", Name: "Комната"}); c.Members == nil {
		t.Fatal("members не должен быть nil")
	}
}