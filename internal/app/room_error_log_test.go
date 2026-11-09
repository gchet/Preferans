package app

import (
	"errors"
	"strings"
	"testing"
)

func TestRoomErrorLogIsThrottled(t *testing.T) {
	a := New(t.TempDir())
	defer a.Close()
	count := func() int {
		n := 0
		for _, l := range a.Status().Logs {
			if strings.Contains(l, "комната:") {
				n++
			}
		}
		return n
	}
	a.mu.Lock()
	for i := 0; i < 50; i++ {
		// адреса и порты в тексте меняются, ошибка по смыслу та же
		a.logRoomErrorLocked(errors.New("failed to dial: all dials failed\n  * [/ip4/10.0.0." + string(rune('0'+i%10)) + "/udp/" + string(rune('1'+i%9)) + "000] timeout"))
	}
	a.mu.Unlock()
	if got := count(); got != 1 {
		t.Fatalf("повторы должны сворачиваться в одну строку, записано %d", got)
	}
	a.mu.Lock()
	a.logRoomErrorLocked(errors.New("другая ошибка"))
	a.mu.Unlock()
	if got := count(); got != 2 {
		t.Fatalf("новая ошибка пишется сразу: %d", got)
	}
}