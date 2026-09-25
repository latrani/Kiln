package conn

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestKnownHosts(t *testing.T) {
	k := KnownHosts{Path: filepath.Join(t.TempDir(), "sub", "known_hosts")}
	if _, ok, err := k.Lookup("a:1"); ok || err != nil {
		t.Fatalf("empty lookup = %v, %v", ok, err)
	}
	if err := k.Trust("a:1", "sha256:aa"); err != nil {
		t.Fatal(err)
	}
	k.Trust("b:2", "sha256:bb")
	k.Trust("a:1", "sha256:cc") // replace
	for host, want := range map[string]string{"a:1": "sha256:cc", "b:2": "sha256:bb"} {
		got, ok, err := k.Lookup(host)
		if !ok || err != nil || got != want {
			t.Errorf("Lookup(%s) = %q, %v, %v; want %q", host, got, ok, err, want)
		}
	}
}

func TestValidateFingerprint(t *testing.T) {
	good := "sha256:" + strings.Repeat("0a", 32)
	if err := ValidateFingerprint(good); err != nil {
		t.Errorf("%q: %v", good, err)
	}
	for _, bad := range []string{
		"",
		strings.Repeat("0a", 32),             // no prefix
		"sha1:" + strings.Repeat("0a", 32),   // wrong algorithm
		"sha256:" + strings.Repeat("0a", 31), // short
		"sha256:" + strings.Repeat("0a", 32) + "0", // long
		"sha256:" + strings.Repeat("0A", 32),       // uppercase
		"sha256:" + strings.Repeat("0g", 32),       // not hex
	} {
		if ValidateFingerprint(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
