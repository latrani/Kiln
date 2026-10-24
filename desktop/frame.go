package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// frameFile is where the window's place is kept, in the data directory.
const frameFile = "desktop-window.json" //str:ok file name

// frame is the window's place: its bounds when neither maximised nor
// full screen, and whether it was maximised.
type frame struct {
	Bounds    application.Rect
	Maximised bool
}

// framed is the window as placing it needs it (wailsWindow's win).
type framed interface {
	Bounds() application.Rect
	SetBounds(application.Rect)
	IsMaximised() bool
	IsMinimised() bool
	IsFullscreen() bool
	Maximise() application.Window
	Show() application.Window
}

// placer keeps the window where it was left: place puts it back and
// shows it, and moved, after any move or resize, saves where it is.
type placer struct {
	win     framed
	path    string
	screens func() []application.Rect // the work areas, without the menu bar or Dock
	wait    time.Duration             // how long moving must stop before a save

	once   sync.Once
	mu     sync.Mutex
	placed bool        // moves before place are the window opening, not the person
	last   frame       // what's saved
	save   *time.Timer // the save waiting on wait
}

func newPlacer(win framed, dataDir string, screens func() []application.Rect) *placer {
	return &placer{win: win, path: filepath.Join(dataDir, frameFile), screens: screens, wait: 500 * time.Millisecond}
}

// place puts the window where it was saved, if that's still on a
// screen, and shows it; once, however often the page loads.
func (p *placer) place() {
	p.once.Do(func() {
		defer p.win.Show()
		f, ok := loadFrame(p.path)
		if ok && onScreen(f.Bounds, p.screens()) {
			p.win.SetBounds(f.Bounds)
			if f.Maximised {
				p.win.Maximise()
			}
		} else {
			f = frame{}
		}
		p.mu.Lock()
		p.placed, p.last = true, f
		p.mu.Unlock()
	})
}

// moved saves where the window is once it's stopped moving. Minimised
// or full screen it keeps what's saved; maximised, it keeps the bounds
// to come back to.
func (p *placer) moved() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.placed {
		return
	}
	if p.save != nil {
		p.save.Stop()
	}
	p.save = time.AfterFunc(p.wait, p.snapshot)
}

func (p *placer) snapshot() {
	if p.win.IsMinimised() || p.win.IsFullscreen() {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	f := p.last
	f.Maximised = p.win.IsMaximised()
	if !f.Maximised {
		f.Bounds = p.win.Bounds()
	}
	if f == p.last {
		return
	}
	if saveFrame(p.path, f) == nil {
		p.last = f
	}
}

// onScreen is whether r's top edge, where the title bar is, lies well
// enough inside one of areas to grab: a screen that's gone, or moved,
// can leave a saved window out of reach.
func onScreen(r application.Rect, areas []application.Rect) bool {
	const grab = 40 // px of the title bar across, and down
	if r.Width < grab || r.Height < grab {
		return false
	}
	for _, a := range areas {
		left, right := max(r.X, a.X), min(r.X+r.Width, a.X+a.Width)
		if right-left >= grab && r.Y >= a.Y && r.Y+grab <= a.Y+a.Height {
			return true
		}
	}
	return false
}

func loadFrame(path string) (frame, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return frame{}, false
	}
	var f frame
	if json.Unmarshal(b, &f) != nil {
		return frame{}, false
	}
	return f, true
}

// saveFrame writes f whole or not at all.
func saveFrame(path string, f frame) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".window-*") //str:ok
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
