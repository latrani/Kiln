package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
)

// webRoutes serves the built page at /kiln/ and a preset at
// /kiln/preset/, with a manifest of the preset's files made on each
// request, so it never drifts from the folder; with no preset the
// manifest is empty and the page starts with nothing preloaded. nginx
// proxies all of /kiln/ here. A preset is laid out like a backup:
// config/… and known_hosts.
func webRoutes(mux *http.ServeMux, web, preset string) {
	mux.Handle("/kiln/", http.StripPrefix("/kiln/", http.FileServer(http.Dir(web))))            //str:ok
	mux.HandleFunc("/kiln/preset/manifest.json", func(w http.ResponseWriter, r *http.Request) { //str:ok
		m := []string{}
		if preset != "" {
			var err error
			if m, err = manifest(preset); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json") //str:ok
		json.NewEncoder(w).Encode(m)
	})
	if preset != "" {
		mux.Handle("/kiln/preset/", http.StripPrefix("/kiln/preset/", http.FileServer(http.Dir(preset)))) //str:ok
	}
}

// manifest lists the files under dir as sorted slash paths relative to
// it: what the page copies into its in-memory files.
func manifest(dir string) ([]string, error) {
	out := []string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
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
