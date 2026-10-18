package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"

	"github.com/latrani/Kiln/internal/relay"
	"github.com/latrani/Kiln/internal/str"
)

// worldsDir is where the page's world files are, in the preset.
const worldsDir = "config/worlds/" //str:ok path

// webRoutes serves the built page at /kiln/ and a preset at
// /kiln/preset/: the allowlist's worlds (worlds, the config as it is
// now) as config/worlds/<id>.toml, and the files in a preset folder
// besides its own config/worlds, with a manifest of them all made on
// each request, so it never drifts from the folder or a reloaded
// relay.toml. nginx proxies all of /kiln/ here. A preset is laid out
// like a backup: config/… and known_hosts. Everything is no-cache:
// browsers check again on each load, so a new release reaches players
// once the relay restarts. logf hears why a manifest failed; the
// browser only that it did.
func webRoutes(mux *http.ServeMux, web, preset string, worlds func() []relay.World, logf func(string)) {
	mux.Handle("/kiln/", noCache(http.StripPrefix("/kiln/", http.FileServer(http.Dir(web)))))                        //str:ok
	mux.Handle("/kiln/preset/manifest.json", noCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { //str:ok
		m := []string{}
		if preset != "" {
			var err error
			if m, err = manifest(preset); err != nil {
				logf(str.RelayManifestFailed(err))
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}
		m = slices.DeleteFunc(m, func(f string) bool { return strings.HasPrefix(f, worldsDir) })
		for _, wl := range worlds() {
			m = append(m, worldsDir+wl.ID+".toml") //str:ok file name
		}
		sort.Strings(m)
		w.Header().Set("Content-Type", "application/json") //str:ok
		json.NewEncoder(w).Encode(m)
	})))
	mux.Handle("/kiln/preset/"+worldsDir, noCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { //str:ok
		name := strings.TrimPrefix(r.URL.Path, "/kiln/preset/"+worldsDir) //str:ok
		for _, wl := range worlds() {
			if name == wl.ID+".toml" { //str:ok file name
				w.Header().Set("Content-Type", "text/plain; charset=utf-8") //str:ok
				w.Write(wl.File())
				return
			}
		}
		http.NotFound(w, r)
	})))
	if preset != "" {
		mux.Handle("/kiln/preset/", noCache(noDots(http.StripPrefix("/kiln/preset/", http.FileServer(http.Dir(preset)))))) //str:ok
	}
}

// checkPresetWorlds says, through logf, that a preset's own worlds are
// ignored: the page's worlds are the allowlist's.
func checkPresetWorlds(preset string, logf func(string)) {
	if fi, err := os.Stat(filepath.Join(preset, filepath.FromSlash(worldsDir))); err == nil && fi.IsDir() {
		logf(str.RelayPresetWorldsIgnored(preset))
	}
}

// noCache has browsers check h's files again on every load (a cheap 304
// when they haven't changed).
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache") //str:ok
		h.ServeHTTP(w, r)
	})
}

// noDots answers 404 for any path with a part starting with a dot: a
// preset copied from a Mac carries .DS_Store, and a .git may sit there.
func noDots(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, part := range strings.Split(r.URL.Path, "/") {
			if strings.HasPrefix(part, ".") {
				http.NotFound(w, r)
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

// checkDir says why dir can't be served for -flag, or nil.
func checkDir(flag, dir string) error {
	fi, err := os.Stat(dir)
	if err == nil && !fi.IsDir() {
		err = &fs.PathError{Op: "stat", Path: dir, Err: syscall.ENOTDIR} //str:ok
	}
	if err != nil {
		return str.Wrap(str.RelayBadDir(flag, dir, err), err)
	}
	return nil
}

// manifest lists the files under dir as sorted slash paths relative to
// it, leaving out dotfiles: what the page copies into its in-memory
// files.
func manifest(dir string) ([]string, error) {
	out := []string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != dir && strings.HasPrefix(d.Name(), ".") { // .DS_Store, .git: not the page's (noDots)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}
