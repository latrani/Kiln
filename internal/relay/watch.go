package relay

import (
	"io"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/latrani/Kiln/internal/str"
)

// relinkEvery is how often the watcher checks whether the allowlist's
// link points somewhere new, for systems that don't say.
const relinkEvery = time.Second

// WatchAllowlist loads path into s, then reloads it whenever it changes.
// A file that fails to load (half-saved, a typo) is logged and the old
// list kept. If path is a symlink, a change to its target counts too,
// and so does repointing it.
// Close the result to stop watching.
func WatchAllowlist(s *Server, path string) (io.Closer, error) {
	a, err := LoadAllowlist(path)
	if err != nil {
		return nil, err
	}
	s.SetAllowlist(a)
	s.logf(str.RelayAllowLoaded(len(a.Worlds), path))
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	// Watch the directory: editors often save by replacing the file.
	path = filepath.Clean(path)
	if err := fw.Add(filepath.Dir(path)); err != nil {
		fw.Close()
		return nil, err
	}
	// target is what path points to, watched the same way. It's looked
	// up again after each reload, since the link can be repointed.
	target := ""
	follow := func() {
		t, err := filepath.EvalSymlinks(path)
		if err != nil || t == path || t == target {
			return
		}
		if target != "" && filepath.Dir(target) != filepath.Dir(path) {
			fw.Remove(filepath.Dir(target))
		}
		target = t
		if filepath.Dir(t) != filepath.Dir(path) {
			if err := fw.Add(filepath.Dir(t)); err != nil {
				s.logf(str.RelayWatchFailed(err))
			}
		}
	}
	follow()
	go func() {
		settle := time.NewTimer(time.Hour)
		settle.Stop()
		// A link repointed in place (ln -sfn) sends no event on macOS
		// (kqueue), so the link is also looked at now and then.
		relink := time.NewTicker(relinkEvery)
		defer relink.Stop()
		for {
			select {
			case <-relink.C:
				if t, err := filepath.EvalSymlinks(path); err == nil && t != path && t != target {
					settle.Reset(200 * time.Millisecond)
				}
			case ev, ok := <-fw.Events:
				if !ok {
					return
				}
				if n := filepath.Clean(ev.Name); n == path || n == target {
					settle.Reset(200 * time.Millisecond)
				}
			case err, ok := <-fw.Errors:
				if !ok {
					return
				}
				s.logf(str.RelayWatchFailed(err))
			case <-settle.C:
				follow()
				a, err := LoadAllowlist(path)
				if err != nil {
					s.logf(str.RelayAllowReloadFailed(err))
					continue
				}
				s.SetAllowlist(a)
				s.logf(str.RelayAllowLoaded(len(a.Worlds), path))
			}
		}
	}()
	return fw, nil
}
