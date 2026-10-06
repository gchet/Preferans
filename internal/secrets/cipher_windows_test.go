package secrets

import (
	"strings"
	"testing"
)

func TestDPAPIRoundTrip(t *testing.T) {
	value := "fake-test-only-api-key"
	protected, err := Platform().Seal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(protected, value) {
		t.Fatal("plaintext not protected")
	}
	plain, err := Platform().Open(protected)
	if err != nil || plain != value {
		t.Fatalf("DPAPI round trip: %v", err)
	}
	if _, err = Platform().Open("not-a-valid-blob"); err == nil {
		t.Fatal("invalid blob accepted")
	}
}
