package relay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/str"
)

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal(what)
}

func TestWatchAllowlistReloadsAndKeepsOldOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allow.toml")
	os.WriteFile(path, []byte(world(1000, true)), 0o644)
	logs := &logLines{}
	s := &Server{Log: logs.add}
	w, err := WatchAllowlist(s, path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, ok := s.allow.Load().Lookup("127.0.0.1", 1000); !ok {
		t.Fatal("initial list not loaded")
	}

	os.WriteFile(path, []byte(world(2000, true)), 0o644)
	eventually(t, "new list never loaded", func() bool { _, ok := s.allow.Load().Lookup("127.0.0.1", 2000); return ok })

	os.WriteFile(path, []byte("[[world]\nhost = "), 0o644) // half-saved
	eventually(t, "no reload error logged", func() bool {
		logs.mu.Lock()
		defer logs.mu.Unlock()
		for _, l := range logs.lines {
			if strings.HasPrefix(l, upTo(str.RelayAllowReloadFailed(errMark{}))) {
				return true
			}
		}
		return false
	})
	if _, ok := s.allow.Load().Lookup("127.0.0.1", 2000); !ok {
		t.Error("a broken file replaced the working list")
	}
}

func TestWatchAllowlistFailsOnBadStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allow.toml")
	os.WriteFile(path, []byte("nope = 1\n"), 0o644)
	if _, err := WatchAllowlist(&Server{}, path); err == nil {
		t.Error("started with a bad allowlist")
	}
}
