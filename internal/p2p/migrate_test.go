package p2p

import (
	"context"
	"testing"
	"time"
)

func TestMigrateLimitedResetsOnlyMatchingStreamsAndHonorsCooldown(t *testing.T) {
	host, err := NewNode(Config{LoopbackOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	guest, err := NewNode(Config{LoopbackOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	accepted := make(chan *Conn, 1)
	host.OnAccept(func(c *Conn) { c.Start(); accepted <- c })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	inv, err := host.Invite(ctx, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	gc, err := guest.Dial(ctx, inv)
	if err != nil {
		t.Fatal(err)
	}
	statuses := make(chan string, 8)
	gc.Status = func(s string) { statuses <- s }
	gc.Start()
	<-statuses // connected
	if err := gc.Send(map[string]string{"type": "hello"}); err != nil {
		t.Fatal(err)
	}
	hc := <-accepted
	hostStatus := make(chan string, 8)
	hc.Status = func(s string) { hostStatus <- s }

	hostID := host.h.ID()
	if n := guest.migrateLimited(hostID, func(*Conn) bool { return false }); n != 0 {
		t.Fatalf("прямые потоки не переносятся: %d", n)
	}
	if n := guest.migrateLimited(hostID, func(*Conn) bool { return true }); n != 1 {
		t.Fatalf("поток на relay должен быть сброшен: %d", n)
	}
	select {
	case s := <-statuses:
		if s != "disconnected" {
			t.Fatalf("статус %q", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("гость не получил disconnected")
	}
	select {
	case s := <-hostStatus:
		if s != "disconnected" {
			t.Fatalf("статус хоста %q", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("хост не получил disconnected")
	}
	// сброшенный поток больше не отслеживается, а новый — в период ожидания — не сбрасывается повторно
	gc2, err := guest.Dial(ctx, inv)
	if err != nil {
		t.Fatal(err)
	}
	gc2.Start()
	if n := guest.migrateLimited(hostID, func(*Conn) bool { return true }); n != 0 {
		t.Fatalf("повторный перенос в период ожидания: %d", n)
	}
}