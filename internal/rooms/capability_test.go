package rooms

import "testing"

func TestMemberBuildCapabilityOnJoinAndPoll(t *testing.T) {
	s, _ := New("")
	r := request(t, s, Request{Op: "create", Name: "Capabilities"}).Room
	no, yes := false, true
	r = request(t, s, Request{Op: "join", Room: r.ID, Platform: "windows", AIEnabled: &no}).Room
	if r.Members[0].Platform != "windows" || r.Members[0].AIEnabled == nil || *r.Members[0].AIEnabled {
		t.Fatal("capability not published", r)
	}
	r = request(t, s, Request{Op: "poll", Room: r.ID, Platform: "windows", AIEnabled: &yes}).Room
	if !*r.Members[0].AIEnabled {
		t.Fatal("capability not refreshed")
	}
	r = request(t, s, Request{Op: "poll", Room: r.ID}).Room
	if r.Members[0].Platform != "" || r.Members[0].AIEnabled != nil {
		t.Fatal("legacy client retained stale capability")
	}
}
