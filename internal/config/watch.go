package config

import (
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Debounce is how long the watcher waits for writes to settle before
// reporting a change; editors often save in several steps.
const Debounce = 200 * time.Millisecond

// Watcher reports changes under a config directory.
type Watcher struct {
	w       *fsnotify.Watcher
	changes chan struct{}
}

// Watch starts watching dir, dir/worlds and dir/packs. Call EnsureDefaults
// first so all three exist.
func Watch(dir string) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for _, d := range []string{dir, filepath.Join(dir, "worlds"), filepath.Join(dir, "packs")} {
		if err := fw.Add(d); err != nil {
			fw.Close()
			return nil, err
		}
	}
	w := &Watcher{w: fw, changes: make(chan struct{}, 1)}
	go w.loop()
	return w, nil
}

// Changes receives one value per settled burst of .toml changes. It is
// closed when the watcher is closed.
func (w *Watcher) Changes() <-chan struct{} { return w.changes }

// Close stops watching.
func (w *Watcher) Close() error { return w.w.Close() }

func (w *Watcher) loop() {
	defer close(w.changes)
	timer := time.NewTimer(Debounce)
	timer.Stop()
	for {
		select {
		case ev, ok := <-w.w.Events:
			if !ok {
				return
			}
			if filepath.Ext(ev.Name) == ".toml" {
				timer.Reset(Debounce)
			}
		case _, ok := <-w.w.Errors:
			if !ok {
				return
			}
		case <-timer.C:
			select {
			case w.changes <- struct{}{}:
			default: // a change is already pending
			}
		}
	}
}
