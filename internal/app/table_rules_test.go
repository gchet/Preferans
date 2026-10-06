package app

import (
	"preferans/internal/game"
	"reflect"
	"testing"
)

func TestTableRulesPersistAndReachGuest(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	r := game.DefaultRules()
	r.PassExit = 8
	r.EndCondition = "time"
	r.Minutes = 90
	appearance := a.Status().Appearance
	appearance.TableRules = &r
	if e := a.SetAppearance(appearance); e != nil {
		t.Fatal(e)
	}
	a.Close()
	a = New(dir)
	defer a.Close()
	if e := a.Configure(nil); e != nil {
		t.Fatal(e)
	}
	if e := a.Create(3, 30, "Host", false); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(a.Status().View.Rules, r) {
		t.Fatal("new table ignored saved agreements")
	}
	guest := New(t.TempDir())
	defer guest.Close()
	if e := guest.Configure(nil); e != nil {
		t.Fatal(e)
	}
	connect(t, a, guest, 1)
	if !reflect.DeepEqual(guest.Status().View.Rules, r) {
		t.Fatal("guest rules differ")
	}
	r.Stalingrad = false
	if e := a.Command(game.Command{ID: game.ID(), Revision: a.Status().View.Revision, Action: "rules", Rules: &r, Target: 40}); e != nil {
		t.Fatal(e)
	}
	eventually(t, func() bool { return guest.Status().View.Target == 40 && !guest.Status().View.Rules.Stalingrad })
	id := a.Status().View.ID
	a.Leave()
	if e := a.Resume(id); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(a.Status().View.Rules, r) {
		t.Fatal("resume lost agreements")
	}
}
