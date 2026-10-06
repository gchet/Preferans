//go:build !windows

package secrets

// Android supplies its Keystore-backed implementation before starting the app.
func Platform() Cipher { return nil }
