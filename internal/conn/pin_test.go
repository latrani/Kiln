package conn

import (
	"path/filepath"
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
