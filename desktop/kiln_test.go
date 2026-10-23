package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/webapi"
)

// fakeWindow is the Wails app as the tests see it: no window opens.
type fakeWindow struct {
	mu      sync.Mutex
	batches []string
	opened  []string
	quit    bool
	focused int
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

func (f *fakeWindow) Focus() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.focused++
}

func (f *fakeWindow) focuses() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.focused
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

// fakeDialogs answers the Save dialog with path ("" is a cancel), and
// the quit question with quit.
type fakeDialogs struct {
	path      string
	name, dir string // what the dialog was asked

	mu    sync.Mutex
	quit  bool
	hold  chan struct{} // when set, the quit question waits for it to close
	asked []int         // each quit question's count of connected characters
}

func (f *fakeDialogs) SaveFile(name, dir string) (string, error) {
	f.name, f.dir = name, dir
	return f.path, nil
}

func (f *fakeDialogs) ConfirmQuit(n int) bool {
	f.mu.Lock()
	f.asked = append(f.asked, n)
	hold, quit := f.hold, f.quit
	f.mu.Unlock()
	if hold != nil {
		<-hold
	}
	return quit
}

// asks is the counts each quit question was asked with, so far.
func (f *fakeDialogs) asks() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.asked...)
}

// note is one notification sent.
type note struct {
	id, title, body string
	data            map[string]any
}

// fakeNotes is the OS's notifications: it keeps what's sent, and answers
// whether they're allowed with allowed, and asked with ask, or err.
type fakeNotes struct {
	mu      sync.Mutex
	notes   []note
	allowed bool
	ask     bool
	err     error
}

func (f *fakeNotes) Send(id, title, body string, data map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.notes = append(f.notes, note{id, title, body, data})
	return nil
}

func (f *fakeNotes) Allowed() (bool, error) { return f.allowed, f.err }
func (f *fakeNotes) Ask() (bool, error)     { return f.ask, f.err }

func (f *fakeNotes) sent() []note {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]note(nil), f.notes...)
}

func newKiln(t *testing.T) (*Kiln, *fakeWindow, *fakeDialogs) {
	t.Helper()
	w, d := &fakeWindow{}, &fakeDialogs{}
	k := &Kiln{win: w, dialogs: d, notes: &fakeNotes{}, cfgDir: t.TempDir(), dataDir: t.TempDir()}
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

// The page never saves drafts as it's left, so quitting does, however
// it quits: the window closing, Cmd+Q (both ServiceShutdown) or /quit.
func TestQuittingSavesDrafts(t *testing.T) {
	for _, end := range []string{"shutdown", "quit"} {
		t.Run(end, func(t *testing.T) {
			k, w, _ := newKiln(t)
			if err := k.Start(); err != nil {
				t.Fatal(err)
			}
			waitFor(t, "the first batch", func() bool { return len(w.sent()) > 0 })
			k.Act(`{"Do":"input","Key":"","Text":"half a thought","N":1}`)
			if end == "shutdown" {
				k.ServiceShutdown()
			} else {
				k.Act(`{"Do":"quit"}`)
				waitFor(t, "the app to quit", w.quitted)
			}
			b, _ := os.ReadFile(filepath.Join(k.dataDir, "drafts.json"))
			if !strings.Contains(string(b), "half a thought") {
				t.Errorf("drafts file %q", b)
			}
		})
	}
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

// statusSaid is whether a batch so far put text up on the status line,
// as an error if isErr.
func statusSaid(t *testing.T, w *fakeWindow, isErr bool, text string) bool {
	t.Helper()
	for _, b := range w.sent() {
		var evs []map[string]any
		if err := json.Unmarshal([]byte(b), &evs); err != nil {
			t.Fatalf("batch %q: %v", b, err)
		}
		for _, e := range evs {
			if e["type"] == "Status" && e["Text"] == text && e["Err"] == isErr {
				return true
			}
		}
	}
	return false
}

func TestSaveAsSaysWhere(t *testing.T) {
	k, w, d := newKiln(t)
	if err := k.Start(); err != nil {
		t.Fatal(err)
	}
	d.path = filepath.Join(t.TempDir(), "scene.txt")
	if err := k.SaveAs("scene.txt", []byte("Kit waves.")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the status line to say where", func() bool { return statusSaid(t, w, false, str.BrowseSaved(d.path)) })
}

func TestSaveAsThatCantWriteSaysSo(t *testing.T) {
	k, w, d := newKiln(t)
	if err := k.Start(); err != nil {
		t.Fatal(err)
	}
	d.path = filepath.Join(t.TempDir(), "gone", "scene.txt") // no such folder
	err := k.SaveAs("scene.txt", []byte("Kit waves."))
	if err == nil {
		t.Fatal("SaveAs into a missing folder didn't fail")
	}
	waitFor(t, "the status line to say why", func() bool { return statusSaid(t, w, true, err.Error()) })
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

const fmWorld = `host = "muck.test"
port = 8888

[[characters]]
id = "kit"
name = "Kit"
`

// startWithKit is a started Kiln whose config has fm's Kit, with Kit
// open and connected when connected (the state set on the loop: there's
// no server).
func startWithKit(t *testing.T, connected bool) (*Kiln, *fakeWindow, *fakeDialogs) {
	t.Helper()
	k, w, d := newKiln(t)
	if err := config.EnsureDefaults(k.cfgDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(k.cfgDir, "worlds", "fm.toml"), []byte(fmWorld), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := k.Start(); err != nil {
		t.Fatal(err)
	}
	if connected {
		set := make(chan error, 1)
		k.loop.Call(func(a *app.App) {
			c, err := a.Open("fm/kit")
			if c == nil && err == nil {
				err = fmt.Errorf("fm/kit isn't configured")
			}
			if c != nil {
				c.State = session.Connected
			}
			set <- err
		})
		if err := <-set; err != nil {
			t.Fatal(err)
		}
	}
	return k, w, d
}

// kitState is Kit's state, read on the loop.
func kitState(t *testing.T, k *Kiln) session.State {
	t.Helper()
	got := make(chan session.State, 1)
	k.loop.Call(func(a *app.App) {
		if c := a.Char("fm/kit"); c != nil {
			got <- c.State
			return
		}
		got <- session.Disconnected
	})
	return <-got
}

func TestShouldQuitWithNothingConnectedDoesNotAsk(t *testing.T) {
	before, _, d0 := newKiln(t)
	k, _, d := startWithKit(t, false)
	if !before.shouldQuit() {
		t.Error("before Start: shouldQuit = false; want true")
	}
	if !k.shouldQuit() {
		t.Error("nothing connected: shouldQuit = false; want true")
	}
	if a := append(d0.asks(), d.asks()...); len(a) != 0 {
		t.Errorf("asked %v with nothing connected", a)
	}
}

func TestShouldQuitCancelledKeepsKitConnected(t *testing.T) {
	k, w, d := startWithKit(t, true)
	if k.shouldQuit() {
		t.Error("Kit connected, Cancel: shouldQuit = true; want false")
	}
	if a := d.asks(); !reflect.DeepEqual(a, []int{1}) {
		t.Errorf("asked %v; want once, with 1 connected", a)
	}
	if s := kitState(t, k); s != session.Connected {
		t.Errorf("after Cancel Kit is %v; want still connected", s)
	}
	if w.quitted() {
		t.Error("Cancel quit the app")
	}
}

func TestShouldQuitAnsweredQuitQuits(t *testing.T) {
	k, _, d := startWithKit(t, true)
	d.quit = true
	if !k.shouldQuit() {
		t.Error("Kit connected, Quit: shouldQuit = false; want true")
	}
	if a := d.asks(); !reflect.DeepEqual(a, []int{1}) {
		t.Errorf("asked %v; want once, with 1 connected", a)
	}
}

// Cmd+Q, the app menu's Quit, the close button and Alt+F4 all reach
// quitGate, and so does what follows a yes: the window closing as the
// app quits, and the app quitting as its last window closes. One
// question covers them all.
func TestQuittingAsksOnce(t *testing.T) {
	k, w, d := startWithKit(t, true)
	d.quit = true
	if k.quitGate() {
		t.Error("Kit connected: the first gate let the quit through before asking")
	}
	waitFor(t, "the app to quit", w.quitted)
	for _, then := range []string{"ShouldQuit", "WindowClosing"} {
		if !k.quitGate() {
			t.Errorf("%s after Quit was answered: held the quit", then)
		}
	}
	if a := d.asks(); len(a) != 1 {
		t.Errorf("asked %d times; want once", len(a))
	}
}

func TestCancelledQuitAsksAgainNextTime(t *testing.T) {
	k, w, d := startWithKit(t, true)
	for i := 1; i <= 2; i++ {
		if k.quitGate() {
			t.Fatalf("try %d: the gate let the quit through with Kit connected", i)
		}
		waitFor(t, "the question", func() bool { return len(d.asks()) == i })
		waitFor(t, "the answer", func() bool { return !k.isAsking() })
	}
	if w.quitted() {
		t.Error("Cancel quit the app")
	}
	if s := kitState(t, k); s != session.Connected {
		t.Errorf("after Cancel Kit is %v; want still connected", s)
	}
}

func TestQuitAsksOneQuestionAtATime(t *testing.T) {
	k, w, d := startWithKit(t, true)
	d.hold, d.quit = make(chan struct{}), true
	if k.quitGate() {
		t.Fatal("the gate let the quit through with Kit connected")
	}
	waitFor(t, "the question", func() bool { return len(d.asks()) == 1 })
	if k.quitGate() || k.quitGate() { // Cmd+Q, then the close button, while it's up
		t.Error("a gate let the quit through while the question was up")
	}
	close(d.hold)
	waitFor(t, "the app to quit", w.quitted)
	if a := d.asks(); len(a) != 1 {
		t.Errorf("asked %d times; want once", len(a))
	}
}

func TestQuitCommandDoesNotAsk(t *testing.T) {
	k, w, d := startWithKit(t, true)
	k.Act(`{"Do":"quit"}`) // /quit hangs up first
	waitFor(t, "the app to quit", w.quitted)
	if !k.quitGate() {
		t.Error("after /quit the gate held the quit")
	}
	if a := d.asks(); len(a) != 0 {
		t.Errorf("asked %v after /quit", a)
	}
}

// ShouldQuit runs on the main thread, which the loop may be waiting on
// (an emit's backpressure), so the gate doesn't wait long for it.
func TestQuitGateDoesNotWaitOnABusyLoop(t *testing.T) {
	k, w, d := startWithKit(t, false)
	release := make(chan struct{})
	k.loop.Call(func(*app.App) { <-release })
	gated := make(chan bool, 1)
	go func() { gated <- k.quitGate() }()
	select {
	case ok := <-gated:
		if ok {
			t.Error("the gate let the quit through without the loop's answer")
		}
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("the gate waited on the busy loop")
	}
	close(release)
	waitFor(t, "the app to quit", w.quitted)
	if a := d.asks(); len(a) != 0 {
		t.Errorf("asked %v with nothing connected", a)
	}
}

// notes is k's fake notifications.
func notes(k *Kiln) *fakeNotes { return k.notes.(*fakeNotes) }

// macOS updates a notification whose ID it has seen without showing it,
// so each one has its own.
func TestNotifySendsAFreshIDEachTime(t *testing.T) {
	k, _, _ := newKiln(t)
	for range 2 {
		if err := k.Notify("Kit", "Rook pages you.", "fm/kit"); err != nil {
			t.Fatal(err)
		}
	}
	sent := notes(k).sent()
	if len(sent) != 2 {
		t.Fatalf("sent %d notifications; want 2", len(sent))
	}
	if sent[0].id == sent[1].id || sent[0].id == "" {
		t.Errorf("IDs %q and %q; want two different ones", sent[0].id, sent[1].id)
	}
	for _, n := range sent {
		if n.title != "Kit" || n.body != "Rook pages you." {
			t.Errorf("sent %q: %q; want Kit: Rook pages you.", n.title, n.body)
		}
	}
}

func TestNotifyWithoutNotificationsIsNoError(t *testing.T) {
	k, _, _ := newKiln(t)
	notes(k).err = errNoNotifications
	if err := k.Notify("Kit", "Rook pages you.", "fm/kit"); err != nil {
		t.Errorf("Notify with no notifications to show: %v", err)
	}
}

func TestNotificationPermission(t *testing.T) {
	for _, c := range []struct {
		allowed, ask bool
		err          error
		perm, asked  string
	}{
		{allowed: true, ask: true, perm: "granted", asked: "granted"},
		// macOS says only whether they're allowed; asking says if they're blocked.
		{allowed: false, ask: false, perm: "default", asked: "denied"},
		{allowed: false, ask: true, perm: "default", asked: "granted"},
		{err: errNoNotifications, perm: "unsupported", asked: "unsupported"},
	} {
		k, _, _ := newKiln(t)
		n := notes(k)
		n.allowed, n.ask, n.err = c.allowed, c.ask, c.err
		if p, err := k.Permission(); p != c.perm || err != nil {
			t.Errorf("allowed %v, %v: Permission = %q, %v; want %q", c.allowed, c.err, p, err, c.perm)
		}
		if p, err := k.AskPermission(); p != c.asked || err != nil {
			t.Errorf("ask %v, %v: AskPermission = %q, %v; want %q", c.ask, c.err, p, err, c.asked)
		}
	}
}

func TestNotificationPermissionErrorIsTheOSs(t *testing.T) {
	k, _, _ := newKiln(t)
	boom := errors.New("timed out")
	notes(k).err = boom
	if _, err := k.Permission(); !errors.Is(err, boom) {
		t.Errorf("Permission's error = %v; want %v", err, boom)
	}
	if _, err := k.AskPermission(); !errors.Is(err, boom) {
		t.Errorf("AskPermission's error = %v; want %v", err, boom)
	}
}

// active is the active sidebar item, read on the loop.
func active(k *Kiln) string {
	got := make(chan string, 1)
	k.loop.Call(func(a *app.App) { got <- a.Active() })
	return <-got
}

// notifyKit sends a notification about Kit, showing fm's overview
// rather than Kit, and is what a click on it gives back.
func notifyKit(t *testing.T, k *Kiln) map[string]any {
	t.Helper()
	k.loop.Call(func(a *app.App) { a.Switch(app.WorldSel("fm")) })
	waitFor(t, "fm's overview", func() bool { return active(k) == app.WorldSel("fm") })
	if err := k.Notify("Kit", "Rook pages you.", "fm/kit"); err != nil {
		t.Fatal(err)
	}
	sent := notes(k).sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d notifications; want 1", len(sent))
	}
	return sent[0].data
}

func TestNotificationClickSwitchesToIt(t *testing.T) {
	k, w, _ := startWithKit(t, true)
	data := notifyKit(t, k)
	k.clicked(data)
	waitFor(t, "the switch to Kit", func() bool { return active(k) == "fm/kit" })
	if n := w.focuses(); n != 1 {
		t.Errorf("brought the window forward %d times; want once", n)
	}
}

func TestNotificationClickForAClosedCharacterDoesNothing(t *testing.T) {
	k, w, _ := startWithKit(t, true)
	data := notifyKit(t, k)
	k.Act(`{"Do":"close","Key":"fm/kit"}`)
	waitFor(t, "Kit to close", func() bool {
		gone := make(chan bool, 1)
		k.loop.Call(func(a *app.App) { gone <- a.Char("fm/kit") == nil })
		return <-gone
	})
	k.clicked(data)
	k.clicked(map[string]any{}) // not one of Kiln's
	if got := active(k); got == "fm/kit" {
		t.Errorf("active = %q after a click for closed Kit", got)
	}
	if n := w.focuses(); n != 0 {
		t.Errorf("brought the window forward %d times; want not at all", n)
	}
}

// A click can come as the app quits, its main thread waiting on the
// loop's end: it mustn't wait on the loop, or touch the window.
func TestNotificationClickBeforeStartOrAfterShutdownDoesNothing(t *testing.T) {
	before, w0, _ := newKiln(t)
	before.clicked(map[string]any{"key": "fm/kit"})
	k, w, _ := startWithKit(t, true)
	data := notifyKit(t, k)
	if err := k.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	clicked := make(chan struct{})
	go func() { k.clicked(data); close(clicked) }()
	select {
	case <-clicked:
	case <-time.After(10 * time.Second):
		t.Fatal("a click after shutdown waited on the ended loop")
	}
	if n := w0.focuses() + w.focuses(); n != 0 {
		t.Errorf("brought the window forward %d times; want not at all", n)
	}
}
