package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// fakeFramed is the window as placing it sees it.
type fakeFramed struct {
	mu                       sync.Mutex
	bounds                   application.Rect
	maximised, min, full     bool
	shown, sets, maximisings int
}

func (f *fakeFramed) Bounds() application.Rect {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bounds
}

func (f *fakeFramed) SetBounds(r application.Rect) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bounds = r
	f.sets++
}

func (f *fakeFramed) IsMaximised() bool  { f.mu.Lock(); defer f.mu.Unlock(); return f.maximised }
func (f *fakeFramed) IsMinimised() bool  { f.mu.Lock(); defer f.mu.Unlock(); return f.min }
func (f *fakeFramed) IsFullscreen() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.full }

func (f *fakeFramed) Maximise() application.Window {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.maximised = true
	f.maximisings++
	return nil
}

func (f *fakeFramed) Show() application.Window {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shown++
	return nil
}

func (f *fakeFramed) set(do func(f *fakeFramed)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	do(f)
}

// seen is what's been done to a fakeFramed.
type seen struct {
	bounds                   application.Rect
	shown, sets, maximisings int
}

func (f *fakeFramed) get() seen {
	f.mu.Lock()
	defer f.mu.Unlock()
	return seen{bounds: f.bounds, shown: f.shown, sets: f.sets, maximisings: f.maximisings}
}

// One screen with a 25px menu bar, and one to its right.
var screens = []application.Rect{{X: 0, Y: 25, Width: 1512, Height: 920}, {X: 1512, Y: 0, Width: 1920, Height: 1080}}

func newTestPlacer(t *testing.T) (*placer, *fakeFramed) {
	t.Helper()
	w := &fakeFramed{bounds: application.Rect{X: 216, Y: 80, Width: 1080, Height: 838}}
	p := newPlacer(w, t.TempDir(), func() []application.Rect { return screens })
	p.wait = 0
	return p, w
}

func TestOnScreenNeedsATitleBarToGrab(t *testing.T) {
	for _, c := range []struct {
		name string
		r    application.Rect
		want bool
	}{
		{"inside the first", application.Rect{X: 100, Y: 100, Width: 800, Height: 600}, true},
		{"on the second", application.Rect{X: 2000, Y: 50, Width: 800, Height: 600}, true},
		{"across both", application.Rect{X: 1200, Y: 100, Width: 800, Height: 600}, true},
		{"right of both", application.Rect{X: 3500, Y: 100, Width: 800, Height: 600}, false},
		{"a sliver at the edge", application.Rect{X: 3412, Y: 100, Width: 800, Height: 600}, false},
		{"title bar under the menu bar", application.Rect{X: 100, Y: 10, Width: 800, Height: 600}, false},
		{"below the bottom", application.Rect{X: 100, Y: 930, Width: 800, Height: 600}, false},
		{"no size", application.Rect{X: 100, Y: 100}, false},
	} {
		if got := onScreen(c.r, screens); got != c.want {
			t.Errorf("%s: onScreen = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPlacePutsTheWindowBackAndShowsIt(t *testing.T) {
	p, w := newTestPlacer(t)
	saved := frame{Bounds: application.Rect{X: 2000, Y: 50, Width: 900, Height: 700}}
	if err := saveFrame(p.path, saved); err != nil {
		t.Fatal(err)
	}
	p.place()
	p.place() // a reloaded page
	got := w.get()
	if got.bounds != saved.Bounds || got.sets != 1 || got.shown != 1 || got.maximisings != 0 {
		t.Errorf("placed %+v", got)
	}
}

func TestPlaceMaximisesOverTheBoundsToComeBackTo(t *testing.T) {
	p, w := newTestPlacer(t)
	saved := frame{Bounds: application.Rect{X: 100, Y: 100, Width: 900, Height: 700}, Maximised: true}
	if err := saveFrame(p.path, saved); err != nil {
		t.Fatal(err)
	}
	p.place()
	if got := w.get(); got.bounds != saved.Bounds || got.maximisings != 1 || got.shown != 1 {
		t.Errorf("placed %+v", got)
	}
}

func TestPlaceLeavesItWhereItOpensIfNotOnScreen(t *testing.T) {
	for name, write := range map[string]func(path string) error{
		"none saved": func(string) error { return nil },
		"off screen": func(path string) error {
			return saveFrame(path, frame{Bounds: application.Rect{X: 5000, Y: 100, Width: 900, Height: 700}})
		},
		"not JSON": func(path string) error { return os.WriteFile(path, []byte("{"), 0o600) },
	} {
		p, w := newTestPlacer(t)
		if err := write(p.path); err != nil {
			t.Fatal(err)
		}
		opened := w.get().bounds
		p.place()
		if got := w.get(); got.bounds != opened || got.sets != 0 || got.shown != 1 {
			t.Errorf("%s: placed %+v", name, got)
		}
	}
}

func TestMovedSavesWhereItIs(t *testing.T) {
	p, w := newTestPlacer(t)
	p.moved() // the window opening, before place
	if p.save != nil {
		t.Fatal("a move before place is saved")
	}
	p.place()
	moved := application.Rect{X: 300, Y: 200, Width: 1000, Height: 760}
	w.set(func(f *fakeFramed) { f.bounds = moved })
	p.moved()
	waitFor(t, "the save", func() bool { f, _ := loadFrame(p.path); return f.Bounds == moved })
}

func TestMovedKeepsTheBoundsUnderMaximised(t *testing.T) {
	p, w := newTestPlacer(t)
	p.place()
	normal := application.Rect{X: 300, Y: 200, Width: 1000, Height: 760}
	w.set(func(f *fakeFramed) { f.bounds = normal })
	p.moved()
	waitFor(t, "the save", func() bool { f, _ := loadFrame(p.path); return f.Bounds == normal })
	w.set(func(f *fakeFramed) {
		f.maximised, f.bounds = true, application.Rect{X: 0, Y: 25, Width: 1512, Height: 920}
	})
	p.moved()
	waitFor(t, "the maximised save", func() bool { f, _ := loadFrame(p.path); return f.Maximised })
	if f, _ := loadFrame(p.path); f.Bounds != normal {
		t.Errorf("saved %+v; want the bounds before maximising", f.Bounds)
	}
}

func TestMovedSkipsMinimisedAndFullScreen(t *testing.T) {
	for name, state := range map[string]func(f *fakeFramed){
		"minimised":   func(f *fakeFramed) { f.min = true },
		"full screen": func(f *fakeFramed) { f.full = true },
	} {
		p, w := newTestPlacer(t)
		p.place()
		w.set(func(f *fakeFramed) { state(f); f.bounds = application.Rect{X: 1, Y: 2, Width: 3, Height: 4} })
		p.snapshot()
		if _, err := os.Stat(p.path); !os.IsNotExist(err) {
			t.Errorf("%s: saved a frame (%v)", name, err)
		}
	}
}

func TestSaveFrameMakesTheDataDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", frameFile)
	want := frame{Bounds: application.Rect{X: 1, Y: 2, Width: 3, Height: 4}, Maximised: true}
	if err := saveFrame(path, want); err != nil {
		t.Fatal(err)
	}
	if got, ok := loadFrame(path); !ok || got != want {
		t.Errorf("loaded %+v, %v", got, ok)
	}
}
