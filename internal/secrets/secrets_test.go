package secrets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func roundTrip(t *testing.T, s Store) {
	t.Helper()
	if _, err := s.Get("fm", "kit"); err != ErrNotFound {
		t.Fatalf("Get before Set = %v, want ErrNotFound", err)
	}
	if err := s.Set("fm", "kit", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("fm", "rook", "swordfish"); err != nil {
		t.Fatal(err)
	}
	if pw, err := s.Get("fm", "kit"); err != nil || pw != "hunter2" {
		t.Errorf("Get = %q, %v", pw, err)
	}
	if _, err := s.Get("fm", "fox"); err != ErrNotFound {
		t.Errorf("other character = %v, want ErrNotFound", err)
	}
}

func TestKeychainRoundTrip(t *testing.T) {
	keyring.MockInit() // in-memory keychain; never touches the real one
	roundTrip(t, Open("keychain", t.TempDir()))
}

func TestFileRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kiln")
	roundTrip(t, Open("file", dir))
	fi, err := os.Stat(filepath.Join(dir, "passwords.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
	if di, _ := os.Stat(dir); di.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, want 0700", di.Mode().Perm())
	}
}

func TestNoneSavesNothing(t *testing.T) {
	s := Open("none", t.TempDir())
	if s.Set("fm", "kit", "hunter2") == nil {
		t.Error("Set succeeded")
	}
	if _, err := s.Get("fm", "kit"); err != ErrNotFound {
		t.Errorf("Get = %v, want ErrNotFound", err)
	}
}
