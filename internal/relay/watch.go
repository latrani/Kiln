package relay

import (
	"io"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/latrani/Kiln/internal/str"
)

// WatchAllowlist loads path into s, then reloads it whenever it changes.
// A file that fails to load (half-saved, a typo) is logged and the old
// list kept. Close the result to stop watching.
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
	if err := fw.Add(filepath.Dir(path)); err != nil {
		fw.Close()
		return nil, err
	}
	go func() {
		settle := time.NewTimer(time.Hour)
		settle.Stop()
		for {
			select {
			case ev, ok := <-fw.Events:
				if !ok {
					return
				}
				if filepath.Clean(ev.Name) == filepath.Clean(path) {
					settle.Reset(200 * time.Millisecond)
				}
			case _, ok := <-fw.Errors:
				if !ok {
					return
				}
			case <-settle.C:
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
