package app

import "testing"

// Подключение гостя по одному коду: хост привязывает входящее соединение к месту по Nonce из hello
// и не заменяет работающее соединение повторным приглашением.
func TestInvitePreservesConnectionUntilHello(t *testing.T) {
	host, client := New(t.TempDir()), New(t.TempDir())
	defer host.Close()
	defer client.Close()
	if err := host.Create(3, 30, "Host", false); err != nil {
		t.Fatal(err)
	}
	code, err := host.Invite(1)
	if err != nil {
		t.Fatal(err)
	}
	if st := host.Status().Links[1]; st != "waiting-guest" {
		t.Fatalf("статус после приглашения: %q", st)
	}
	if err := client.Join(code); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return host.Status().Connected[1] && client.Status().View != nil })
	if _, err := host.Invite(1); err == nil {
		t.Fatal("повторное приглашение для подключённого участника")
	}
}

func TestJoinRejectsForeignAndOldCodes(t *testing.T) {
	client := New(t.TempDir())
	defer client.Close()
	for _, code := range []string{"", "PREF1.abc", "PREF2.zzz", "garbage"} {
		if err := client.Join(code); err == nil {
			t.Fatalf("принят код %q", code)
		}
	}
}