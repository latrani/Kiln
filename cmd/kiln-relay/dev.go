package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
)

// devRoutes serves the page and a preset config dir from one origin, the
// way production's nginx will, so the whole thing runs on a laptop.
func devRoutes(mux *http.ServeMux, web, preset string) {
	mux.Handle("/kiln/", http.StripPrefix("/kiln/", http.FileServer(http.Dir(web))))            //str:ok
	mux.HandleFunc("/kiln/preset/manifest.json", func(w http.ResponseWriter, r *http.Request) { //str:ok
		m, err := manifest(preset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json") //str:ok
		json.NewEncoder(w).Encode(m)
	})
	mux.Handle("/kiln/preset/", http.StripPrefix("/kiln/preset/", http.FileServer(http.Dir(preset)))) //str:ok
}

// manifest lists the files under dir as sorted slash paths relative to
// it: what the page copies into its in-memory config dir.
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
