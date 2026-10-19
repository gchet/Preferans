package network

import "os"

// Keep tests off real network interfaces so Windows Firewall does not prompt for network.test.exe.
// Set PREFERANS_TEST_STUN=1 to run the opt-in public STUN test over real interfaces.
func init() {
	turnCheckListenAddr = "127.0.0.1:0"
	if os.Getenv("PREFERANS_TEST_STUN") != "1" {
		SetLoopbackOnly(true)
	}
}
