// Package secrets protects API credentials using the native platform's vault.
package secrets

type Cipher interface {
	Seal(string) (string, error)
	Open(string) (string, error)
}
