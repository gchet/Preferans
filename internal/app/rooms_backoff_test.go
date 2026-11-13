package app

import (
	"testing"
	"time"
)

func TestRoomBackoffGrowsAndIsCapped(t *testing.T) {
	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second}
	for i, w := range want {
		if got := roomBackoff(i + 1); got != w {
			t.Errorf("после %d неудач: %s, ожидалось %s", i+1, got, w)
		}
	}
}