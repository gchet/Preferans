package app

import (
	"os"
	"testing"

	"preferans/internal/network"
)

// Peers created by tests stay on loopback so Windows Firewall does not prompt for app.test.exe.
func TestMain(m *testing.M) {
	network.SetLoopbackOnly(true)
	os.Exit(m.Run())
}
