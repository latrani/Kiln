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

	"github.com/latrani/Kiln/internal/str"
)

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

func TestRunNeedsAllow(t *testing.T) {
	if err := run(nil, io.Discard); err == nil || err.Error() != str.RelayNeedAllow() {
		t.Errorf("err = %v", err)
	}
}

// webMux is the routes run mounts for -web and -preset, without the relay.
func webMux(t *testing.T, preset bool) *httptest.Server {
	t.Helper()
	web := t.TempDir()
	os.WriteFile(filepath.Join(web, "index.html"), []byte("page"), 0o644)
	dir := ""
	if preset {
		dir = t.TempDir()
		os.MkdirAll(filepath.Join(dir, "config", "worlds"), 0o755)
		os.WriteFile(filepath.Join(dir, "config", "worlds", "fm.toml"), []byte("host"), 0o644)
	}
	mux := http.NewServeMux()
	webRoutes(mux, web, dir)
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
	if code, body := get(t, srv.URL+"/kiln/preset/manifest.json"); code != 200 || strings.TrimSpace(body) != `["config/worlds/fm.toml"]` {
		t.Errorf("manifest: %d %q", code, body)
	}
	if code, body := get(t, srv.URL+"/kiln/preset/config/worlds/fm.toml"); code != 200 || body != "host" {
		t.Errorf("preset file: %d %q", code, body)
	}
}

// Nothing outside the page and preset directories is served, however
// the path is written.
func TestWebStaysInItsDirectories(t *testing.T) {
	web := t.TempDir()
	os.WriteFile(filepath.Join(filepath.Dir(web), "secret"), []byte("secret"), 0o644)
	mux := http.NewServeMux()
	webRoutes(mux, web, "")
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	for _, p := range []string{"/kiln/../secret", "/kiln/%2e%2e/secret", "/kiln/preset/../../secret"} {
		if _, body := get(t, srv.URL+p); strings.Contains(body, "secret") {
			t.Errorf("%s served the file beside the page", p)
		}
	}
}

// Without -preset the page still starts: nothing to preload.
func TestWebWithoutPreset(t *testing.T) {
	srv := webMux(t, false)
	if code, body := get(t, srv.URL+"/kiln/preset/manifest.json"); code != 200 || strings.TrimSpace(body) != "[]" {
		t.Errorf("manifest: %d %q", code, body)
	}
}

// -dev is gone: -web and -preset say what to serve, -listen where.
func TestDevIsGone(t *testing.T) {
	if err := run([]string{"-allow", "x.toml", "-dev"}, io.Discard); err == nil || !strings.Contains(err.Error(), "-dev") {
		t.Errorf("err = %v, want an unknown flag", err)
	}
}
