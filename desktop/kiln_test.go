package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/webapi"
)

// fakeWindow is the Wails app as the tests see it: no window opens.
type fakeWindow struct {
	mu      sync.Mutex
	batches []string
	opened  []string
	quit    bool
}

func (f *fakeWindow) Emit(name, data string) {
	if name != batchEvent {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, data)
}

func (f *fakeWindow) OpenURL(url string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, url)
	return nil
}

func (f *fakeWindow) Quit() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.quit = true
}

// sent is the batches so far.
func (f *fakeWindow) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.batches...)
}

func (f *fakeWindow) quitted() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.quit
}

// fakeDialogs answers the Save dialog with path ("" is a cancel).
type fakeDialogs struct {
	path      string
	name, dir string // what the dialog was asked
}

func (f *fakeDialogs) SaveFile(name, dir string) (string, error) {
	f.name, f.dir = name, dir
	return f.path, nil
}

func newKiln(t *testing.T) (*Kiln, *fakeWindow, *fakeDialogs) {
	t.Helper()
	w, d := &fakeWindow{}, &fakeDialogs{}
	k := &Kiln{win: w, dialogs: d, cfgDir: t.TempDir(), dataDir: t.TempDir()}
	t.Cleanup(func() { k.ServiceShutdown() })
	return k, w, d
}

// waitFor polls until ok, failing after 10s.
func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("gave up waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// starts is how many of batches start the page over.
func starts(t *testing.T, batches []string) int {
	t.Helper()
	n := 0
	for _, b := range batches {
		var evs []map[string]any
		if err := json.Unmarshal([]byte(b), &evs); err != nil {
			t.Fatalf("batch %q: %v", b, err)
		}
		for _, e := range evs {
			if e["type"] == "Start" {
				n++
			}
		}
	}
	return n
}

func TestStartTwiceStartsOneLoop(t *testing.T) {
	k, w, _ := newKiln(t)
	if err := k.Start(); err != nil {
		t.Fatal(err)
	}
	first := k.loop
	if err := k.Start(); err != nil {
		t.Fatal(err)
	}
	if k.loop != first {
		t.Error("a second Start made another loop")
	}
	waitFor(t, "the first batch", func() bool { return len(w.sent()) > 0 })
	k.Act(`{"Do":"here"}`)
	waitFor(t, "the here's turn", func() bool { return len(w.sent()) > 1 })
	if n := starts(t, w.sent()); n != 1 {
		t.Errorf("%d Start events; want one loop's", n)
	}
}

func TestActReachesTheLoop(t *testing.T) {
	k, w, _ := newKiln(t)
	k.Act(`{"Do":"here"}`) // before Start: nothing to reach, and no harm
	if err := k.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the first batch", func() bool { return len(w.sent()) > 0 })
	n := len(w.sent())
	k.Act(`{"Do":"here"}`)
	waitFor(t, "the here's turn", func() bool { return len(w.sent()) > n })
}

func TestQuitQuitsTheApp(t *testing.T) {
	k, w, _ := newKiln(t)
	if err := k.Start(); err != nil {
		t.Fatal(err)
	}
	k.Act(`{"Do":"quit"}`)
	waitFor(t, "the app to quit", w.quitted)
}

func TestShutdownStopsTheLoopWithoutQuitting(t *testing.T) {
	k, w, _ := newKiln(t)
	if err := k.Start(); err != nil {
		t.Fatal(err)
	}
	if err := k.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-k.done:
	default:
		t.Error("ServiceShutdown returned with the loop running")
	}
	if w.quitted() {
		t.Error("the app was asked to quit while it was already shutting down")
	}
	k.Act(`{"Do":"here"}`) // after: no harm
}

func TestCheckAndOverAreTheWebAPIs(t *testing.T) {
	k, _, _ := newKiln(t)
	if got, want := k.Check("port", "x"), webapi.CheckField("port", "x"); got != want || got == "" {
		t.Errorf("Check(port, x) = %q; want %q", got, want)
	}
	if got, want := k.Over("abcdef", 3, ""), webapi.OverRanges("abcdef", 3, ""); !reflect.DeepEqual(got, want) {
		t.Errorf("Over = %v; want %v", got, want)
	}
	if got := k.Over("ab", 3, ""); got == nil || len(got) != 0 {
		t.Errorf("Over with nothing over = %#v; want empty, not nil (the page gets [])", got)
	}
}

func TestSaveAsCancelledWritesNothing(t *testing.T) {
	k, _, d := newKiln(t)
	exports := t.TempDir()
	if err := os.WriteFile(filepath.Join(k.cfgDir, "config.toml"), fmt.Appendf(nil, "export_dir = %q\n", exports), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := k.Start(); err != nil {
		t.Fatal(err)
	}
	if err := k.SaveAs("scene.txt", []byte("Kit waves.")); err != nil {
		t.Errorf("a cancelled save: %v", err)
	}
	if d.name != "scene.txt" || d.dir != exports {
		t.Errorf("dialog asked for %q in %q; want scene.txt in %q", d.name, d.dir, exports)
	}
	if files, _ := os.ReadDir(exports); len(files) != 0 {
		t.Errorf("a cancelled save wrote %v", files)
	}
}

func TestSaveAsWritesWhereChosen(t *testing.T) {
	k, _, d := newKiln(t)
	d.path = filepath.Join(t.TempDir(), "scene.txt")
	if err := k.SaveAs("scene.txt", []byte("Kit waves.")); err != nil {
		t.Fatal(err)
	}
	if d.dir != "" {
		t.Errorf("before Start the dialog started in %q; want no export_dir", d.dir)
	}
	if b, err := os.ReadFile(d.path); err != nil || string(b) != "Kit waves." {
		t.Errorf("saved %q, %v", b, err)
	}
}

func TestOpenURLOnlyOpensWebLinks(t *testing.T) {
	k, w, _ := newKiln(t)
	err := k.OpenURL("file:///etc/passwd")
	if err == nil || err.Error() != str.DesktopNotALink("file:///etc/passwd") {
		t.Errorf("OpenURL(file:) = %v; want refused", err)
	}
	if err := k.OpenURL("javascript:alert(1)"); err == nil {
		t.Error("OpenURL(javascript:) wasn't refused")
	}
	if err := k.OpenURL("https://example.com/fm"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w.opened, []string{"https://example.com/fm"}) {
		t.Errorf("opened %v; want only the https link", w.opened)
	}
}
