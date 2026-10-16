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

func TestWatchConfigReloadsAndKeepsOldOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.toml")
	os.WriteFile(path, []byte(world(1000, "")), 0o644)
	logs := &logLines{}
	s := &Server{Log: logs.add}
	w, err := WatchConfig(s, path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, ok := s.cfg.Load().Lookup("127.0.0.1", 1000); !ok {
		t.Fatal("initial list not loaded")
	}

	os.WriteFile(path, []byte(world(2000, "")), 0o644)
	eventually(t, "new list never loaded", func() bool { _, ok := s.cfg.Load().Lookup("127.0.0.1", 2000); return ok })

	os.WriteFile(path, []byte("[[world]\nhost = "), 0o644) // half-saved
	eventually(t, "no reload error logged", func() bool {
		logs.mu.Lock()
		defer logs.mu.Unlock()
		for _, l := range logs.lines {
			if strings.HasPrefix(l, upTo(str.RelayConfigReloadFailed(errMark{}))) {
				return true
			}
		}
		return false
	})
	if _, ok := s.cfg.Load().Lookup("127.0.0.1", 2000); !ok {
		t.Error("a broken file replaced the working list")
	}
}

func TestWatchConfigFollowsSymlink(t *testing.T) {
	dir, store := t.TempDir(), t.TempDir()
	v1, v2 := filepath.Join(store, "v1.toml"), filepath.Join(store, "v2.toml")
	os.WriteFile(v1, []byte(world(1000, "")), 0o644)
	os.WriteFile(v2, []byte(world(3000, "")), 0o644)
	path := filepath.Join(dir, "relay.toml")
	if err := os.Symlink(v1, path); err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	w, err := WatchConfig(s, path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	has := func(port int) func() bool {
		return func() bool { _, ok := s.cfg.Load().Lookup("127.0.0.1", port); return ok }
	}

	os.WriteFile(v1, []byte(world(2000, "")), 0o644) // the target, edited where it is
	eventually(t, "an edit to the link's target never loaded", has(2000))

	tmp := filepath.Join(dir, "relay.tmp")
	os.Symlink(v2, tmp)
	os.Rename(tmp, path) // repointed, the way ln -sfn does it
	eventually(t, "the repointed link never loaded", has(3000))

	os.WriteFile(v2, []byte(world(4000, "")), 0o644)
	eventually(t, "an edit to the new target never loaded", has(4000))
}

func TestWatchConfigFailsOnBadStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.toml")
	os.WriteFile(path, []byte("nope = 1\n"), 0o644)
	if _, err := WatchConfig(&Server{}, path); err == nil {
		t.Error("started with a bad config")
	}
}

func TestWatchConfigNewPinForNewConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.toml")
	os.WriteFile(path, []byte(world(1000, "ca")), 0o644)
	s := &Server{}
	w, err := WatchConfig(s, path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	os.WriteFile(path, []byte(world(1000, testPin)), 0o644)
	eventually(t, "the new pin never loaded", func() bool {
		wl, _ := s.cfg.Load().Lookup("127.0.0.1", 1000)
		return wl.Pin == testPin && wl.TLSTrust == ""
	})
}
