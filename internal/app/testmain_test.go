package app

import (
	"os"
	"testing"

	"preferans/internal/p2p"
)

// Peers created by tests stay on loopback so Windows Firewall does not prompt for app.test.exe.
func TestMain(m *testing.M) {
	p2p.TestLoopback = true
	os.Exit(m.Run())
}
