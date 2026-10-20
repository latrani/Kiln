package native

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/secrets"
)

// newCore is a core on fresh config and data directories.
func newCore(t *testing.T) (*Core, string, string) {
	t.Helper()
	cfgDir, dataDir := t.TempDir(), t.TempDir()
	if err := config.EnsureDefaults(cfgDir); err != nil {
		t.Fatal(err)
	}
	c, err := New(Options{ConfigDir: cfgDir, DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, cfgDir, dataDir
}

func TestPasswordsRoundTrip(t *testing.T) {
	c, _, _ := newCore(t)
	d := c.Deps
	if err := d.SavePassword("file", "fm", "kit", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if pw, err := d.Password("file", "fm", "kit"); err != nil || pw != "hunter2" {
		t.Fatalf("Password = %q, %v; want the saved one", pw, err)
	}
	if err := d.DeletePassword("file", "fm", "kit"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Password("file", "fm", "kit"); !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("Password after DeletePassword: %v; want ErrNotFound", err)
	}
}

func TestFilesLiveInTheDataDir(t *testing.T) {
	c, cfgDir, dataDir := newCore(t)
	if got, want := c.Deps.KnownHosts.Path, filepath.Join(dataDir, "known_hosts"); got != want {
		t.Errorf("KnownHosts.Path = %q; want %q", got, want)
	}
	if got, want := c.Deps.StateFile, filepath.Join(dataDir, "state.json"); got != want {
		t.Errorf("StateFile = %q; want %q", got, want)
	}
	if got, want := c.Deps.LogRoot, filepath.Join(dataDir, "logs"); got != want {
		t.Errorf("LogRoot = %q; want %q", got, want)
	}
	if c.Deps.ConfigDir != cfgDir || c.Deps.Load == nil || c.Deps.Dial == nil {
		t.Errorf("ConfigDir %q, Load set %v, Dial set %v", c.Deps.ConfigDir, c.Deps.Load != nil, c.Deps.Dial != nil)
	}
}

func TestOneLogWriterPerLayout(t *testing.T) {
	c, _, dataDir := newCore(t)
	l := logstore.Layout{Root: filepath.Join(dataDir, "logs"), World: "fm", Char: "kit", CharName: "Kit"}
	a, b := c.Deps.NewLog(l), c.Deps.NewLog(l)
	if a != b {
		t.Error("the same layout got two writers")
	}
	l.Dir = "elsewhere"
	if c.Deps.NewLog(l) == a {
		t.Error("a changed log_dir kept the old writer")
	}
}

func TestChangesUntilClose(t *testing.T) {
	c, cfgDir, _ := newCore(t)
	if err := os.WriteFile(filepath.Join(cfgDir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Changes:
	case <-time.After(10 * time.Second):
		t.Fatal("writing a world file sent nothing on Changes")
	}
	c.Close()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case _, ok := <-c.Changes:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("Close left Changes open")
		}
	}
}
