package backup

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// zipOf builds a zip with the given names and contents.
func zipOf(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(body))
	}
	w.Close()
	return buf.Bytes()
}

func TestRoundTrip(t *testing.T) {
	src := t.TempDir()
	write(t, filepath.Join(src, "config", "config.toml"), "theme = 'x'")
	write(t, filepath.Join(src, "config", "worlds", "fm.toml"), "host = 'fm'")
	write(t, filepath.Join(src, "known_hosts"), "fm:5555 sha256")
	data, err := Zip(filepath.Join(src, "config"), filepath.Join(src, "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	write(t, filepath.Join(dst, "config", "worlds", "mine.toml"), "host = 'mine'") // added since: survives
	n, err := Unzip(data, filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts"))
	if err != nil || n != 3 {
		t.Fatalf("Unzip = %d, %v", n, err)
	}
	if read(t, filepath.Join(dst, "config", "worlds", "fm.toml")) != "host = 'fm'" ||
		read(t, filepath.Join(dst, "known_hosts")) != "fm:5555 sha256" ||
		read(t, filepath.Join(dst, "config", "worlds", "mine.toml")) != "host = 'mine'" {
		t.Error("files after restore don't match")
	}
}

func TestZipWithoutKnownHosts(t *testing.T) {
	src := t.TempDir()
	write(t, filepath.Join(src, "config", "config.toml"), "x")
	if _, err := Zip(filepath.Join(src, "config"), filepath.Join(src, "known_hosts")); err != nil {
		t.Fatal(err)
	}
}

func TestUnzipRejectsBeforeWriting(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"dotdot":   {"config/ok.toml": "x", "config/../../evil": "x"},
		"absolute": {"config/ok.toml": "x", "/etc/evil": "x"},
		"stray":    {"config/ok.toml": "x", "notes.txt": "x"},
	} {
		dst := t.TempDir()
		if _, err := Unzip(zipOf(t, files), filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts")); err == nil {
			t.Errorf("%s: no error", name)
		}
		if _, err := os.Stat(filepath.Join(dst, "config", "ok.toml")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: wrote ok.toml before rejecting", name)
		}
	}
}

func TestUnzipSkipsDirsAndRejectsEmptyAndJunk(t *testing.T) {
	dst := t.TempDir()
	n, err := Unzip(zipOf(t, map[string]string{"config/": "", "config/worlds/": "", "config/a.toml": "x"}), filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts"))
	if err != nil || n != 1 {
		t.Errorf("dirs: %d, %v", n, err)
	}
	if _, err := Unzip(zipOf(t, map[string]string{"config/": ""}), filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts")); err == nil {
		t.Error("empty backup: no error")
	}
	if _, err := Unzip([]byte("not a zip"), filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts")); err == nil {
		t.Error("junk: no error")
	}
}

func TestUnzipTooBig(t *testing.T) {
	dst := t.TempDir()
	big := string(bytes.Repeat([]byte("a"), MaxUnpacked+1))
	if _, err := Unzip(zipOf(t, map[string]string{"config/big": big}), filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts")); err == nil {
		t.Error("too big: no error")
	}
}

// Each file is under the cap, but together they pass it. The error must
// be the too-big one, and nothing may be written.
func TestUnzipTooBigTogetherWritesNothing(t *testing.T) {
	dst := t.TempDir()
	half := string(bytes.Repeat([]byte("a"), MaxUnpacked/2+1))
	data := zipOf(t, map[string]string{"config/a": half, "config/b": half})
	_, err := Unzip(data, filepath.Join(dst, "config"), filepath.Join(dst, "known_hosts"))
	if err == nil || err.Error() != str.BackupTooBig(MaxUnpacked>>20) {
		t.Errorf("err = %v, want the too-big error", err)
	}
	for _, f := range []string{"a", "b"} {
		if _, err := os.Stat(filepath.Join(dst, "config", f)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("wrote %s before rejecting", f)
		}
	}
}
