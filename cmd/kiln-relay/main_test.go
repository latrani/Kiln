package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/relay"
	"github.com/latrani/Kiln/internal/str"
)

// worlds is the allowlist parsed from relay.toml's [[world]] text.
func worlds(t *testing.T, toml string) []relay.World {
	t.Helper()
	a, err := relay.ParseConfig("relay.toml", []byte(toml))
	if err != nil {
		t.Fatal(err)
	}
	return a.Worlds
}

const fmWorld = "[[world]]\nid = \"fm\"\nhost = \"muck.example.org\"\nport = 8899\n\n[[world.characters]]\nid = \"kit\"\nname = \"Kit\"\n"
const bayWorld = "[[world]]\nid = \"bay\"\nhost = \"bay.example.org\"\nport = 7000\n\n[[world.characters]]\nid = \"rook\"\nname = \"Rook\"\n"

func TestManifestListsFilesAsSlashPaths(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o755)
	os.WriteFile(filepath.Join(dir, "config.toml"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), nil, 0o644)
	got, err := manifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"config.toml", "worlds/fm.toml"}; !slices.Equal(got, want) {
		t.Errorf("manifest = %q, want %q", got, want)
	}
}

func TestRunNeedsConfig(t *testing.T) {
	if err := run(nil, io.Discard); err == nil || err.Error() != str.RelayNeedConfig() {
		t.Errorf("err = %v", err)
	}
}

func TestAllowFlagIsGone(t *testing.T) {
	if err := run([]string{"-allow", "x"}, io.Discard); err == nil || !strings.Contains(err.Error(), "-allow") {
		t.Errorf("err = %v", err)
	}
}

// webMux is the routes run mounts for -web and -preset, without the
// relay, serving fm from the allowlist. The preset has a config file
// and a world of its own, which is ignored.
func webMux(t *testing.T, preset bool) *httptest.Server {
	t.Helper()
	web := t.TempDir()
	os.WriteFile(filepath.Join(web, "index.html"), []byte("page"), 0o644)
	dir := ""
	if preset {
		dir = t.TempDir()
		os.MkdirAll(filepath.Join(dir, "config", "worlds"), 0o755)
		os.WriteFile(filepath.Join(dir, "config", "config.toml"), []byte("theme"), 0o644)
		os.WriteFile(filepath.Join(dir, "config", "worlds", "old.toml"), []byte("host"), 0o644)
	}
	ws := worlds(t, fmWorld)
	mux := http.NewServeMux()
	webRoutes(mux, web, dir, func() []relay.World { return ws }, func(string) {})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestWebServesThePageAndPreset(t *testing.T) {
	srv := webMux(t, true)
	if code, body := get(t, srv.URL+"/kiln/index.html"); code != 200 || body != "page" {
		t.Errorf("page: %d %q", code, body)
	}
	if code, body := get(t, srv.URL+"/kiln/preset/manifest.json"); code != 200 || strings.TrimSpace(body) != `["config/config.toml","config/worlds/fm.toml"]` {
		t.Errorf("manifest: %d %q, want the preset's config and the allowlist's world", code, body)
	}
	if code, body := get(t, srv.URL+"/kiln/preset/config/config.toml"); code != 200 || body != "theme" {
		t.Errorf("preset file: %d %q", code, body)
	}
	if code, body := get(t, srv.URL+"/kiln/preset/config/worlds/fm.toml"); code != 200 || body != string(worlds(t, fmWorld)[0].File()) {
		t.Errorf("allowlist world: %d %q", code, body)
	}
	if code, _ := get(t, srv.URL+"/kiln/preset/config/worlds/old.toml"); code != 404 {
		t.Errorf("the preset's own world: %d, want 404", code)
	}
}

// The served worlds are the config's as it is now: a reloaded relay.toml
// reaches the page on its next load.
func TestServedWorldsFollowTheConfig(t *testing.T) {
	web := t.TempDir()
	ws := worlds(t, fmWorld)
	mux := http.NewServeMux()
	webRoutes(mux, web, "", func() []relay.World { return ws }, func(string) {})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	ws = worlds(t, bayWorld)
	if _, body := get(t, srv.URL+"/kiln/preset/manifest.json"); strings.TrimSpace(body) != `["config/worlds/bay.toml"]` {
		t.Errorf("manifest after a reload: %q", body)
	}
	if code, body := get(t, srv.URL+"/kiln/preset/config/worlds/bay.toml"); code != 200 || !strings.Contains(body, "Rook") {
		t.Errorf("bay: %d %q", code, body)
	}
	if code, _ := get(t, srv.URL+"/kiln/preset/config/worlds/fm.toml"); code != 404 {
		t.Errorf("fm after it left the allowlist: %d, want 404", code)
	}
}

// A preset's own worlds are ignored, and said so at start.
func TestPresetWorldsAreWarnedAbout(t *testing.T) {
	var said []string
	logf := func(l string) { said = append(said, l) }
	dir := t.TempDir()
	checkPresetWorlds(dir, logf)
	if len(said) != 0 {
		t.Errorf("no worlds folder: said %q", said)
	}
	os.MkdirAll(filepath.Join(dir, "config", "worlds"), 0o755)
	checkPresetWorlds(dir, logf)
	if want := []string{str.RelayPresetWorldsIgnored(dir)}; !slices.Equal(said, want) {
		t.Errorf("said %q, want %q", said, want)
	}
}

// Nothing outside the page and preset directories is served, however
// the path is written.
func TestWebStaysInItsDirectories(t *testing.T) {
	web := t.TempDir()
	os.WriteFile(filepath.Join(filepath.Dir(web), "secret"), []byte("secret"), 0o644)
	mux := http.NewServeMux()
	webRoutes(mux, web, "", func() []relay.World { return nil }, func(string) {})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	for _, p := range []string{"/kiln/../secret", "/kiln/%2e%2e/secret", "/kiln/preset/../../secret"} {
		if _, body := get(t, srv.URL+p); strings.Contains(body, "secret") {
			t.Errorf("%s served the file beside the page", p)
		}
	}
}

// Without -preset the page still gets the allowlist's worlds.
func TestWebWithoutPreset(t *testing.T) {
	srv := webMux(t, false)
	if code, body := get(t, srv.URL+"/kiln/preset/manifest.json"); code != 200 || strings.TrimSpace(body) != `["config/worlds/fm.toml"]` {
		t.Errorf("manifest: %d %q", code, body)
	}
}

// -dev is gone: -web and -preset say what to serve, -listen where.
func TestDevIsGone(t *testing.T) {
	if err := run([]string{"-config", "x.toml", "-dev"}, io.Discard); err == nil || !strings.Contains(err.Error(), "-dev") {
		t.Errorf("err = %v, want an unknown flag", err)
	}
}

// A -web or -preset that isn't a folder stops the relay at start, rather
// than serving a page that can't boot.
func TestBadDirsStopTheStart(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	for _, flag := range []string{"web", "preset"} {
		_, statErr := os.Stat(missing)
		err := run([]string{"-config", "x.toml", "-" + flag, missing}, io.Discard)
		if err == nil || err.Error() != str.RelayBadDir(flag, missing, statErr) {
			t.Errorf("-%s %s: err = %v", flag, missing, err)
		}
	}
}

// A preset that can't be listed is a server error that doesn't show
// where the server keeps its files.
func TestManifestErrorHidesThePath(t *testing.T) {
	web, preset := t.TempDir(), t.TempDir()
	mux := http.NewServeMux()
	webRoutes(mux, web, preset, func() []relay.World { return nil }, func(string) {})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	os.RemoveAll(preset)
	if code, body := get(t, srv.URL+"/kiln/preset/manifest.json"); code != 500 || strings.Contains(body, preset) {
		t.Errorf("%d %q", code, body)
	}
}

// The page and preset are checked again on each load (no-cache), so a
// new release reaches players as soon as the relay restarts.
func TestNoCache(t *testing.T) {
	srv := webMux(t, true)
	for _, p := range []string{"/kiln/index.html", "/kiln/preset/manifest.json", "/kiln/preset/config/worlds/fm.toml"} {
		res, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if got := res.Header.Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control %q", p, got)
		}
	}
}

// Dotfiles in a preset (a Mac's .DS_Store, a .git) are neither listed
// for the page nor served.
func TestPresetSkipsDotfiles(t *testing.T) {
	web, preset := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(preset, "config"), 0o755)
	os.MkdirAll(filepath.Join(preset, ".git"), 0o755)
	os.WriteFile(filepath.Join(preset, "config", "config.toml"), []byte("theme"), 0o644)
	os.WriteFile(filepath.Join(preset, "config", ".DS_Store"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(preset, ".git", "HEAD"), []byte("ref"), 0o644)
	mux := http.NewServeMux()
	webRoutes(mux, web, preset, func() []relay.World { return nil }, func(string) {})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	if _, body := get(t, srv.URL+"/kiln/preset/manifest.json"); strings.TrimSpace(body) != `["config/config.toml"]` {
		t.Errorf("manifest %s", body)
	}
	for _, p := range []string{"/kiln/preset/config/.DS_Store", "/kiln/preset/.git/HEAD"} {
		if code, _ := get(t, srv.URL+p); code != 404 {
			t.Errorf("%s: %d, want 404", p, code)
		}
	}
}
