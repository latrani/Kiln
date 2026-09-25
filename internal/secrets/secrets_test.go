package secrets

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestRoundTrip(t *testing.T) {
	keyring.MockInit() // in-memory keychain; never touches the real one
	if _, err := Get("fm", "kit"); err != keyring.ErrNotFound {
		t.Fatalf("Get before Set = %v, want ErrNotFound", err)
	}
	if err := Set("fm", "kit", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if pw, err := Get("fm", "kit"); err != nil || pw != "hunter2" {
		t.Errorf("Get = %q, %v", pw, err)
	}
	if _, err := Get("fm", "rook"); err != keyring.ErrNotFound {
		t.Errorf("other character = %v, want ErrNotFound", err)
	}
}
