package game

import "testing"

func TestTeethAreOwnedAndNeverInvalidateOtherCommands(t *testing.T) {
	s := testState(t, 3)
	aim := 1
	tooth := func(seat int, target, delta int) Command {
		return Command{ID: ID(), Seat: seat, Revision: s.Revision, Action: "teeth", Aim: &target, Delta: delta}
	}
	if _, e := Apply(s, tooth(0, aim, 1), nil); e == nil {
		t.Fatal("teeth must not be drawn in the lobby")
	}
	s = step(t, s, 0, "start", nil)
	revision := s.Revision

	next, e := Apply(s, tooth(0, 1, 2), nil)
	if e != nil {
		t.Fatal(e)
	}
	if next.Revision != revision || next.TeethMatrix()[0][1] != 2 || s.TeethMatrix()[0][1] != 0 {
		t.Fatalf("revision=%d teeth=%v", next.Revision, next.TeethMatrix())
	}
	s = next

	// A stale revision is fine: teeth are a table joke, not a game move.
	stale := tooth(2, 0, 1)
	stale.Revision = 0
	if s, e = Apply(s, stale, nil); e != nil {
		t.Fatal(e)
	}
	// Ordinary moves with the revision the player saw are still accepted afterwards.
	s = step(t, s, s.Actor(), "pass", nil)

	// Only the author changes his teeth: seat 1 cannot remove what seat 0 drew.
	if _, e = Apply(s, tooth(1, 0, -1), nil); e == nil {
		t.Fatal("removed someone else's tooth")
	}
	if s, e = Apply(s, tooth(0, 1, -1), nil); e != nil || s.TeethMatrix()[0][1] != 1 {
		t.Fatalf("remove: %v %v", e, s.TeethMatrix())
	}
	for _, bad := range []Command{tooth(0, 1, -2), tooth(0, 1, MaxTeeth), tooth(0, 0, 1), tooth(0, 3, 1), tooth(0, 1, 0), tooth(0, 1, MaxTeeth+1)} {
		if _, e = Apply(s, bad, nil); e == nil {
			t.Fatalf("accepted %+v delta=%d", bad.Aim, bad.Delta)
		}
	}
	full := tooth(0, 2, MaxTeeth)
	if s, e = Apply(s, full, nil); e != nil || s.TeethMatrix()[0][2] != MaxTeeth {
		t.Fatalf("full set: %v", e)
	}
	// Replaying a command ID changes nothing.
	if again, e := Apply(s, full, nil); e != nil || again != s {
		t.Fatal("replay must be ignored")
	}
	s.Players[2].Bot = true
	if _, e = Apply(s, tooth(2, 0, 1), nil); e == nil {
		t.Fatal("bots do not draw teeth")
	}
	v := s.View(1)
	if len(v.Teeth) != 3 || v.Teeth[0][1] != 1 || v.Teeth[0][2] != MaxTeeth {
		t.Fatalf("view teeth %v", v.Teeth)
	}
	old := testState(t, 4)
	if m := old.TeethMatrix(); len(m) != 4 || len(m[3]) != 4 {
		t.Fatal("old saves have a complete empty matrix")
	}
}
