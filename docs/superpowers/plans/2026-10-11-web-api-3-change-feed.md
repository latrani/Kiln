# Web API, step 3: the change feed — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A new package, `internal/feed`, that after each turn of a front end's loop compares what a page shows with what it last sent and returns events for what changed: one place, so new core code can't forget to announce a change.

**Architecture:**
- `feed.New(a *app.App) *Feed`. `Turn(effs []app.Effect) []Event` runs after every message the loop handles, with the effects that message produced. `All() []Event` forgets what was sent and returns everything (a page starting, or catching up). `Since(key string, seq int) []app.Line` is a character's scrollback after line `seq`.
- **Small things** are built whole each turn and compared with `reflect.DeepEqual` to what was last sent, by an identity string (`"order"`, `"char fm/kit"`, `"log fm/kit"`, …). Changed ones go out whole. A character that's gone sends `Closed`.
- **Lines** (scrollback and log mode) are tracked per key by owner (`*app.Char`, `*app.Log`), rules generation, and the lowest and highest number sent. A new owner, or lines that went away (`Preload`), sends `Reset`; a new rules generation sends `Replace` (all lines, same numbers, new tags); otherwise new lines at either end go out as `Prepend` and `Append`. Only the two ends are scanned.
- **Effects** go out after the state changes, in order; `app.Run` is the loop's own and is left out.
- The TUI doesn't use the feed. Step 4's wasm loop calls it and turns events into JSON; nothing here is about JSON.

**Tech Stack:** Go; new `internal/feed`; `internal/app` gains `Char.RulesGen()`.

**Spec:** `docs/superpowers/specs/2026-10-11-web-api-design.md`, "The change feed", and step 2's PR (#138) notes: reopening and `Preload` reset; an unshown session line's `Seq` means nothing; log lines the filter hides are sent too.

## Global Constraints

- Nothing visible changes in the TUI; golden screens are NOT updated. No strings change (the feed has no user-facing text; event fields carry the core's own).
- `internal/feed` depends on `internal/app` and its data packages, never on `internal/ui`, Bubble Tea or lipgloss. A test pins it.
- Run `go vet ./... && go test ./...` and the web module's tests (`cd web && go test ./... && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...`) before each commit. No `-race`.
- End each commit message with the session's attribution lines.

## The events

| Event | Identity | When |
|---|---|---|
| `Worlds{Worlds []World}` | `worlds` | the config's worlds and characters change |
| `Order{Keys []string}` | `order` | the open characters or their sidebar order change |
| `Char{…}` | `char <key>` | anything about one open character changes |
| `Closed{Key}` | — | an open character is gone |
| `Active{Key}` | `active` | the active item changes (a key, `app.WorldSel(world)`, or "") |
| `Status{Text, Err, Gen}` | `status` | the status line changes |
| `Confirm{On}` | `confirm` | an over-limit line starts or stops waiting for Enter again |
| `SaveQuestion{World, Char, Asking}` | `save` | the save-password question comes up or goes |
| `Lines{Key, Op, Lines}` | per key | scrollback changes |
| `Log{…}` | `log <key>` | log mode's state changes (`Open` false once it closes) |
| `LogLines{Key, Op, Lines}` | per key | log mode's lines change |
| `Filter{Key, Entries}` | `filter <key>` | the filter panel's list changes (while log mode is open) |
| `Effect{app.Effect}` | — | the turn's effects, except `app.Run` |

Order within a turn: `Worlds`, `Order`, each `Char`, each `Closed`, `Active`, `Status`, `Confirm`, `SaveQuestion`, each `Lines`, each `Log`, `LogLines` and `Filter`, then the effects.

## Review Focus

1. **Reopen and Preload reset.** Closing and reopening a character (or log mode) in one turn must send `Reset`, not `Append`, even though the numbers start over. `TestReopenResets` pins it.
2. **One change, one event; no change, nothing.** A line for an inactive character sends exactly its `Char` (unread) and `Lines` (append). A second `Turn` with nothing new sends nothing. `TestOneLineOneChange`, `TestNothingNewNothingSent`.
3. **`Since` matches what was sent.** Every line sent as `Reset`/`Append`/`Prepend`, after number N, equals `Since(key, N)`. `TestSinceMatchesWhatWasSent`.
4. **Calling into the core from the feed has side effects.** `Log.Entries()` folds newly seen parents (`FoldSeen`), as the TUI's panel does on drawing; the feed calls it every turn while log mode is open. The reviewer should check nothing else the feed calls changes core state.
5. **Cost per turn.** Scrollback is diffed at the ends only. Log mode's `Hidden` and `Matches` are worked out over all loaded log lines each turn while log mode is open: fine at today's sizes (a few thousand lines), noted for later.

---

### Task 1: the feed, small things and effects

**Files:**
- Modify: `internal/app/app.go` (`Char.RulesGen`)
- Create: `internal/feed/feed.go`, `internal/feed/events.go`, `internal/feed/feed_test.go`

**Interfaces:**
- Produces (in `app`): `func (c *Char) RulesGen() int`.
- Produces (in `feed`): every type in "The events"; `type Op int` with `Reset`, `Replace`, `Append`, `Prepend`; `type World struct{ ID string; Characters []Character }`; `type Character struct{ ID, Name string }`; `type FilterEntry struct{ app.PanelEntry; Hidden, Only, Folded, FilteredUnder bool }`; `New`, `(*Feed).Turn`, `(*Feed).All`, `(*Feed).Since`.

- [ ] **Step 1: Tests** — `internal/feed/feed_test.go`:

```go
package feed

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
)

const fmWorld = `host = "muck.test"
port = 8888
tls = true
use = ["fuzzball"]

[[characters]]
id = "kit"
name = "Kit"

[[characters]]
id = "rook"
name = "Rook"
`

var day = time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)

// newApp is an App over fm (Kit, Rook) with fake connections, and its
// config dir.
func newApp(t *testing.T) (*app.App, string) {
	t.Helper()
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	writeWorld(t, dir, fmWorld)
	a := app.New(app.Deps{
		ConfigDir: dir,
		Load:      config.Load,
		LogRoot:   filepath.Join(dir, "logs"),
		Dial: func(context.Context, config.Character) (session.LineConn, error) {
			return kilntest.NewConn(), nil
		},
		NewLog:   func(logstore.Layout) session.Appender { return kilntest.NopLog{} },
		Password: func(string, string, string) (string, error) { return "", errors.New("none") },
		Now:      func() time.Time { return day },
	})
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	return a, dir
}

func writeWorld(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T, a *app.App, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if c, err := a.Open(k); c == nil || err != nil {
			t.Fatalf("Open(%s) = %v, %v", k, c, err)
		}
	}
}

// attach gives k a session, so session messages for it are taken.
func attach(a *app.App, k string) *session.Session {
	c := a.Char(k)
	c.Sess = session.New(session.Options{Char: c.Ch, Log: kilntest.NopLog{}})
	return c.Sess
}

func line(a *app.App, k string, s *session.Session, text string) {
	a.Handle(app.SessionMsg{Key: k, Sess: s, OK: true, Ev: session.Event{Kind: session.EventLine, Entry: logstore.Entry{Time: day, Dir: logstore.In, Text: text}}})
}

// kinds names evs' types, in order.
func kinds(evs []Event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, strings.TrimPrefix(reflect.TypeOf(e).String(), "feed."))
	}
	return out
}

func find[T Event](evs []Event) (T, bool) {
	for _, e := range evs {
		if v, ok := e.(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

func TestFirstTurnSendsEverything(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	f := New(a)
	evs := f.Turn(nil)
	want := []string{"Worlds", "Order", "Char", "Active", "Status", "Confirm", "SaveQuestion", "Lines"}
	if got := kinds(evs); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds %v, want %v", got, want)
	}
	if w, _ := find[Worlds](evs); len(w.Worlds) != 1 || w.Worlds[0].ID != "fm" || len(w.Worlds[0].Characters) != 2 {
		t.Errorf("worlds %+v", w)
	}
	if o, _ := find[Order](evs); !reflect.DeepEqual(o.Keys, []string{"fm/kit"}) {
		t.Errorf("order %+v", o)
	}
	if c, _ := find[Char](evs); c.Key != "fm/kit" || c.Name != "Kit" || c.World != "fm" {
		t.Errorf("char %+v", c)
	}
	if act, _ := find[Active](evs); act.Key != "fm/kit" {
		t.Errorf("active %+v", act)
	}
}

func TestNothingNewNothingSent(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	f := New(a)
	f.Turn(nil)
	if evs := f.Turn(nil); len(evs) != 0 {
		t.Errorf("a turn with nothing new sent %v", kinds(evs))
	}
}

func TestClosedAndStatus(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit", "fm/rook")
	f := New(a)
	f.Turn(nil)
	a.Close("fm/rook")
	a.SetStatus(true, "oops") //str:ok: a test status
	evs := f.Turn(nil)
	if got := kinds(evs); !reflect.DeepEqual(got, []string{"Order", "Closed", "Status"}) {
		t.Fatalf("kinds %v", got)
	}
	if c, _ := find[Closed](evs); c.Key != "fm/rook" {
		t.Errorf("closed %+v", c)
	}
	if s, _ := find[Status](evs); s.Text != "oops" || !s.Err {
		t.Errorf("status %+v", s)
	}
}

func TestEffectsComeLastWithoutRun(t *testing.T) {
	a, _ := newApp(t)
	f := New(a)
	f.Turn(nil)
	effs := []app.Effect{app.Run{Func: func() any { return nil }}, app.Copy{Text: "x"}, app.Quit{}}
	evs := f.Turn(effs)
	if got := kinds(evs); !reflect.DeepEqual(got, []string{"Effect", "Effect"}) {
		t.Fatalf("kinds %v", got)
	}
	if e := evs[0].(Effect); e.Effect != (app.Copy{Text: "x"}) {
		t.Errorf("first effect %+v", e)
	}
}

func TestAllSendsEverythingAgain(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	f := New(a)
	first := f.Turn(nil)
	if again := f.All(); !reflect.DeepEqual(kinds(again), kinds(first)) {
		t.Errorf("All sent %v, the first turn %v", kinds(again), kinds(first))
	}
	if evs := f.Turn(nil); len(evs) != 0 {
		t.Errorf("a turn after All sent %v", kinds(evs))
	}
}

// The feed is plain Go: no terminal, no UI package.
func TestFeedDependsOnNoFrontEnd(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasSuffix(dep, "/internal/ui") || strings.Contains(dep, "bubbletea") || strings.Contains(dep, "lipgloss") {
			t.Errorf("feed depends on %s", dep)
		}
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/feed`
Expected: build failure (`New`, `Event`, … undefined).

- [ ] **Step 3: The events** — `internal/feed/events.go`:

```go
package feed

import (
	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/session"
)

// Event is one change for a page to apply.
type Event interface{ event() }

// Worlds is the config's worlds and their characters, for the picker.
type Worlds struct{ Worlds []World }

// World is one world in Worlds.
type World struct {
	ID         string
	Characters []Character
}

// Character is one character in a World.
type Character struct{ ID, Name string }

// Order is the open characters' keys, in sidebar order.
type Order struct{ Keys []string }

// Char is one open character: what the sidebar shows of it, and its
// input as the core holds it.
type Char struct {
	Key, World, ID, Name string
	State                session.State
	Unread               int
	Attention            bool
	CertChanged          bool   // its server's certificate changed; /trust to accept
	Orphan               bool   // removed from the config; closes when it disconnects
	Text                 string // the input's text, as the core holds it
	Prompt               string // the server's unterminated prompt, sanitized; "" for none
	NeedPW               bool   // the password prompt is up
	More                 bool   // older history can be paged in
	Loading              bool   // a page of older history is being read
}

// Closed is an open character gone.
type Closed struct{ Key string }

// Active is what's showing: a character's key, app.WorldSel(world) for a
// world's overview, or "".
type Active struct{ Key string }

// Status is the status line. Gen goes up with each new message.
type Status struct {
	Text string
	Err  bool
	Gen  int
}

// Confirm says whether an over-limit line is waiting for Enter again.
type Confirm struct{ On bool }

// SaveQuestion is the "save this password?" question, while Asking.
type SaveQuestion struct {
	World, Char string
	Asking      bool
}

// Op is how a Lines or LogLines event changes the page's lines.
type Op int

const (
	Reset   Op = iota // these are all the lines now
	Replace           // the same lines (same numbers), made again: their tags changed
	Append            // these go after the last
	Prepend           // these go before the first
)

// Lines changes a character's scrollback.
type Lines struct {
	Key   string
	Op    Op
	Lines []app.Line
}

// Log is a character's log mode. Lines are named by number: the marked
// range, the lines taken out of it, the lines the filter (or local_echo)
// hides, and the find's matches, oldest first.
type Log struct {
	Key                         string
	Open                        bool
	Find                        string
	Status                      string
	StatusErr                   bool
	Searching, Loading, HistDone bool
	Start, End                  *int
	Excluded, Hidden, Matches   []int
}

// LogLines changes a character's log-mode lines.
type LogLines struct {
	Key   string
	Op    Op
	Lines []app.Line
}

// Filter is log mode's filter panel list.
type Filter struct {
	Key     string
	Entries []FilterEntry
}

// FilterEntry is one item in Filter, with its buttons' state: Hidden and
// Only lit, Folded for a parent folded shut, FilteredUnder for a folded
// parent with something under it hidden or Only.
type FilterEntry struct {
	app.PanelEntry
	Hidden, Only, Folded, FilteredUnder bool
}

// Effect is something the core asks the page to do.
type Effect struct{ app.Effect }

func (Worlds) event()       {}
func (Order) event()        {}
func (Char) event()         {}
func (Closed) event()       {}
func (Active) event()       {}
func (Status) event()       {}
func (Confirm) event()      {}
func (SaveQuestion) event() {}
func (Lines) event()        {}
func (Log) event()          {}
func (LogLines) event()     {}
func (Filter) event()       {}
func (Effect) event()       {}
```

- [ ] **Step 4: The feed** — `internal/feed/feed.go`:

```go
// Package feed tells a page what changed. After each turn of a front
// end's loop it compares what the page shows with what it last sent and
// returns events for the differences, so no core code has to remember to
// announce a change. The TUI redraws from the core instead and doesn't
// use it.
package feed

import (
	"reflect"

	"github.com/latrani/Kiln/internal/app"
)

// Feed is one page's view of the core. Like the core, it isn't safe for
// concurrent use: it runs on the front end's loop.
type Feed struct {
	a     *app.App
	small map[string]Event // the small things as last sent, by identity
	lines map[string]*sent // each character's scrollback as sent
	logs  map[string]*sent // each character's log-mode lines as sent
}

// sent is how far a list of lines has been sent.
type sent struct {
	owner  any // the *app.Char or *app.Log they belong to
	gen    int // its rules generation
	lo, hi int // the lowest and highest numbers sent
	any    bool
}

// New starts a feed over a; its first Turn sends everything.
func New(a *app.App) *Feed {
	f := &Feed{a: a}
	f.forget()
	return f
}

func (f *Feed) forget() {
	f.small, f.lines, f.logs = map[string]Event{}, map[string]*sent{}, map[string]*sent{}
}

// All forgets what was sent and returns everything.
func (f *Feed) All() []Event {
	f.forget()
	return f.Turn(nil)
}

// Since is open character k's scrollback after line seq.
func (f *Feed) Since(k string, seq int) []app.Line {
	c := f.a.Char(k)
	if c == nil {
		return nil
	}
	var out []app.Line
	for i := len(c.Lines) - 1; i >= 0 && c.Lines[i].Seq > seq; i-- {
		out = append(out, *c.Lines[i])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Turn returns what changed since the last turn, then effs (all but
// app.Run, which is the loop's own).
func (f *Feed) Turn(effs []app.Effect) []Event {
	var evs []Event
	put := func(id string, e Event) {
		if old, ok := f.small[id]; !ok || !reflect.DeepEqual(old, e) {
			f.small[id] = e
			evs = append(evs, e)
		}
	}
	a := f.a
	put("worlds", worlds(a))
	put("order", Order{Keys: append([]string(nil), a.Order()...)})
	open := map[string]bool{}
	for _, k := range a.Order() {
		open[k] = true
		put(idChar+k, char(a.Char(k)))
	}
	for id, e := range f.small {
		if c, ok := e.(Char); ok && !open[c.Key] {
			delete(f.small, id)
			delete(f.small, idLog+c.Key)
			delete(f.small, idFilter+c.Key)
			delete(f.lines, c.Key)
			delete(f.logs, c.Key)
			evs = append(evs, Closed{Key: c.Key})
		}
	}
	put("active", Active{Key: a.Active()})
	st := a.Status()
	put("status", Status{Text: st.Text, Err: st.Err, Gen: st.Gen})
	put("confirm", Confirm{On: a.Confirming()})
	w, ch, asking := a.PendingSave()
	put("save", SaveQuestion{World: w, Char: ch, Asking: asking})
	for _, k := range a.Order() {
		evs = append(evs, f.scrollback(a.Char(k))...)
	}
	for _, k := range a.Order() {
		evs = append(evs, f.logMode(a.Char(k), put)...)
	}
	for _, e := range effs {
		if _, ok := e.(app.Run); !ok {
			evs = append(evs, Effect{e})
		}
	}
	return evs
}

func worlds(a *app.App) Worlds {
	var out Worlds
	if cfg := a.Config(); cfg != nil {
		for _, w := range cfg.Worlds {
			ws := World{ID: w.ID}
			for _, c := range w.Characters {
				ws.Characters = append(ws.Characters, Character{ID: c.ID, Name: c.Name})
			}
			out.Worlds = append(out.Worlds, ws)
		}
	}
	return out
}

func char(c *app.Char) Char {
	return Char{
		Key: c.Key, World: c.Ch.World, ID: c.Ch.ID, Name: c.Ch.Name,
		State: c.State, Unread: c.Unread, Attention: c.Attention,
		CertChanged: c.Pin != nil, Orphan: c.Orphan,
		Text: c.Text, Prompt: c.Prompt, NeedPW: c.NeedPW, More: c.More, Loading: c.Loading,
	}
}

// scrollback is Task 2's; logMode is Task 3's.
func (f *Feed) scrollback(*app.Char) []Event { return nil }

func (f *Feed) logMode(*app.Char, func(string, Event)) []Event { return nil }
```

The `TestFirstTurnSendsEverything` expectation includes `Lines`, which Task 2 adds: in this task, change that test's `want` to leave out `"Lines"`, and put it back in Task 2's Step 1 (it's listed there).

In `internal/app/app.go`, after `compile`:

```go
// RulesGen goes up whenever c's rules are compiled again, so lines made
// under the old ones may have been made again.
func (c *Char) RulesGen() int { return c.rulesGen }
```

- [ ] **Step 5: Run the feed tests**, then everything (Global Constraints). Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/app.go internal/feed docs/superpowers/plans/2026-10-11-web-api-3-change-feed.md
git commit -m "feed: a change feed for the web: small things and effects"
```

---

### Task 2: scrollback

**Files:**
- Modify: `internal/feed/feed.go`, `internal/feed/feed_test.go`

**Interfaces:**
- Consumes: Task 1's `Feed`, `sent`, `Lines`, `Op`, `app.Char.RulesGen`.
- Produces: `(*Feed).scrollback(c *app.Char) []Event`; `func (f *Feed) diff(m map[string]*sent, key string, owner any, gen, n int, at func(int) *app.Line) (Op, []app.Line, bool)`.

- [ ] **Step 1: Tests** — put `"Lines"` back at the end of `TestFirstTurnSendsEverything`'s `want`, and add:

```go
// writeLog puts lines in fm/kit's log, a minute apart from start.
func writeLog(t *testing.T, a *app.App, start time.Time, lines ...string) {
	t.Helper()
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, start, lines...)
}

func linesOf(evs []Event) []Lines {
	var out []Lines
	for _, e := range evs {
		if l, ok := e.(Lines); ok {
			out = append(out, l)
		}
	}
	return out
}

func seqs(ls []app.Line) []int {
	var out []int
	for _, l := range ls {
		out = append(out, l.Seq)
	}
	return out
}

func TestOneLineOneChange(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit", "fm/rook")
	s := attach(a, "fm/rook")
	f := New(a)
	f.Turn(nil)
	line(a, "fm/rook", s, "Hello")
	evs := f.Turn(nil)
	if got := kinds(evs); !reflect.DeepEqual(got, []string{"Char", "Lines"}) {
		t.Fatalf("kinds %v", got)
	}
	if c := evs[0].(Char); c.Key != "fm/rook" || c.Unread != 1 {
		t.Errorf("char %+v", c)
	}
	if l := evs[1].(Lines); l.Key != "fm/rook" || l.Op != Append || len(l.Lines) != 1 || l.Lines[0].Seq != 0 {
		t.Errorf("lines %+v", l)
	}
}

func TestScrollbackPrependsReplacesAndResets(t *testing.T) {
	a, dir := newApp(t)
	open(t, a, "fm/kit") // Kit's log is written after: Preload sees none yet
	many := make([]string, app.HistoryLines+50)
	for i := range many {
		many[i] = "[Wiki] edited"
	}
	a.Close("fm/kit")
	writeLogFor(t, dir, many...)
	open(t, a, "fm/kit")
	f := New(a)
	first := linesOf(f.Turn(nil))
	if len(first) != 1 || first[0].Op != Reset || first[0].Lines[len(first[0].Lines)-1].Seq != -1 {
		t.Fatalf("first: %+v", first)
	}
	msg := a.RequestOlder("fm/kit")[0].(app.Run).Func().(app.OlderMsg)
	page, _ := a.HandleOlder(msg)
	got := linesOf(f.Turn(nil))
	if len(got) != 1 || got[0].Op != Prepend || len(got[0].Lines) != len(page) || got[0].Lines[0].Seq != page[0].Seq {
		t.Fatalf("after an older page: %+v", got)
	}
	writeWorld(t, dir, fmWorld+"\n[[classify]]\ntag = \"wiki\"\npattern = '^\\[Wiki\\]'\n")
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	got = linesOf(f.Turn(nil))
	all := a.Char("fm/kit").Lines
	if len(got) != 1 || got[0].Op != Replace || len(got[0].Lines) != len(all) || got[0].Lines[0].Seq != all[0].Seq {
		t.Fatalf("after a rules change: %d events, op %v", len(got), got)
	}
	if tags := got[0].Lines[1].TagNames(); len(tags) == 0 || tags[0] != "wiki" {
		t.Errorf("replaced line tags %v", tags)
	}
}

// writeLogFor writes fm/kit's log under dir's logs, from day.
func writeLogFor(t *testing.T, dir string, lines ...string) {
	t.Helper()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range cfg.Worlds {
		for _, c := range w.Characters {
			if c.ID == "kit" {
				l, ok := (app.New(app.Deps{LogRoot: filepath.Join(dir, "logs")})).LogLayout(c)
				if !ok {
					t.Fatal("no log layout")
				}
				kilntest.WriteLog(t, l, day.AddDate(0, 0, -1), lines...)
			}
		}
	}
}

func TestReopenResets(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	line(a, "fm/kit", s, "one")
	line(a, "fm/kit", s, "two")
	f := New(a)
	f.Turn(nil)
	a.Close("fm/kit")
	open(t, a, "fm/kit")
	s = attach(a, "fm/kit")
	line(a, "fm/kit", s, "again")
	got := linesOf(f.Turn(nil))
	if len(got) != 1 || got[0].Op != Reset || len(got[0].Lines) != 1 || got[0].Lines[0].Seq != 0 {
		t.Errorf("after reopening: %+v", got)
	}
}

func TestSinceMatchesWhatWasSent(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	f := New(a)
	var sentLines []app.Line
	take := func() {
		for _, l := range linesOf(f.Turn(nil)) {
			sentLines = append(sentLines, l.Lines...)
		}
	}
	take()
	for _, text := range []string{"a", "b", "c", "d"} {
		line(a, "fm/kit", s, text)
		take()
	}
	for n := -1; n < 4; n++ {
		var want []int
		for _, l := range sentLines {
			if l.Seq > n {
				want = append(want, l.Seq)
			}
		}
		if got := seqs(f.Since("fm/kit", n)); !reflect.DeepEqual(got, want) {
			t.Errorf("Since(%d) = %v, sent after it %v", n, got, want)
		}
	}
}
```

(`writeLog` is for Task 3; leave it in now.)

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/feed -run 'FirstTurn|OneLine|Scrollback|Reopen|Since'`
Expected: FAIL (no `Lines` events yet).

- [ ] **Step 3: Implement** — in `internal/feed/feed.go`, replace the `scrollback` stub with:

```go
// scrollback is c's Lines event, if its lines changed.
func (f *Feed) scrollback(c *app.Char) []Event {
	op, ls, ok := f.diff(f.lines, c.Key, c, c.RulesGen(), len(c.Lines), func(i int) *app.Line { return c.Lines[i] })
	if !ok {
		return nil
	}
	return []Event{Lines{Key: c.Key, Op: op, Lines: ls}}
}

// diff works out how key's lines (n of them, at(i) the i'th, numbered in
// order) changed since they were last sent, and records them as sent.
// A new owner, or lines gone from either end, is a Reset; a new rules
// generation a Replace; otherwise new lines at the start or end are a
// Prepend or an Append. Only the ends are looked at. ok is false when
// nothing changed.
func (f *Feed) diff(m map[string]*sent, key string, owner any, gen, n int, at func(int) *app.Line) (Op, []app.Line, bool) {
	s := m[key]
	all := func() []app.Line {
		out := make([]app.Line, n)
		for i := range n {
			out[i] = *at(i)
		}
		return out
	}
	record := func() {
		ns := sent{owner: owner, gen: gen, any: n > 0}
		if n > 0 {
			ns.lo, ns.hi = at(0).Seq, at(n-1).Seq
		}
		m[key] = &ns
	}
	if s == nil || s.owner != owner {
		record()
		return Reset, all(), true
	}
	if n == 0 {
		if !s.any {
			return 0, nil, false
		}
		record()
		return Reset, nil, true
	}
	lo, hi := at(0).Seq, at(n-1).Seq
	switch {
	case !s.any || lo > s.lo || hi < s.hi:
		record()
		return Reset, all(), true
	case gen != s.gen:
		record()
		return Replace, all(), true
	case lo < s.lo && hi > s.hi:
		record() // both ends grew in one turn: rare; one Reset says it simply
		return Reset, all(), true
	case lo < s.lo:
		var out []app.Line
		for i := 0; i < n && at(i).Seq < s.lo; i++ {
			out = append(out, *at(i))
		}
		record()
		return Prepend, out, true
	case hi > s.hi:
		i := n
		for i > 0 && at(i-1).Seq > s.hi {
			i--
		}
		out := make([]app.Line, 0, n-i)
		for ; i < n; i++ {
			out = append(out, *at(i))
		}
		record()
		return Append, out, true
	}
	return 0, nil, false
}
```

- [ ] **Step 4: Run the feed tests**, then everything. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/feed
git commit -m "feed: scrollback goes out as reset, replace, prepend or append"
```

---

### Task 3: log mode and its filter panel

**Files:**
- Modify: `internal/feed/feed.go`, `internal/feed/feed_test.go`

**Interfaces:**
- Consumes: Task 2's `diff`; `app.Log` (`Lines`, `Find`, `Status`, `StatusErr`, `Searching`, `Loading`, `HistDone`, `Start`, `End`, `Excluded`, `Shows`, `Matches`, `Entries`, `Items`, `FilteredUnder`); `app.Char.Filter`, `Collapsed`.
- Produces: `(*Feed).logMode(c *app.Char, put func(string, Event)) []Event`.

- [ ] **Step 1: Tests** — add:

```go
func TestLogMode(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	writeLog(t, a, day.AddDate(0, 0, -1), "one", "two", "three")
	f := New(a)
	f.Turn(nil)
	g := a.OpenLog("fm/kit")
	evs := f.Turn(nil)
	if got := kinds(evs); !reflect.DeepEqual(got, []string{"Log", "LogLines", "Filter"}) {
		t.Fatalf("opening: kinds %v", got)
	}
	if l, _ := find[Log](evs); !l.Open || l.Start != nil {
		t.Errorf("log %+v", l)
	}
	if ll, _ := find[LogLines](evs); ll.Op != Reset || len(ll.Lines) != len(g.Lines) {
		t.Errorf("log lines %+v", ll)
	}
	g.Mark(g.Lines[0])
	g.Mark(g.Lines[2])
	g.ToggleExclude(g.Lines[1])
	evs = f.Turn(nil)
	l, _ := find[Log](evs)
	if l.Start == nil || *l.Start != g.Lines[0].Seq || *l.End != g.Lines[2].Seq || !reflect.DeepEqual(l.Excluded, []int{g.Lines[1].Seq}) {
		t.Errorf("range %+v", l)
	}
	g.SetFind("t", nil, nil)
	l, _ = find[Log](f.Turn(nil))
	if !reflect.DeepEqual(l.Matches, []int{g.Lines[1].Seq, g.Lines[2].Seq}) {
		t.Errorf("matches %v", l.Matches)
	}
	g.AddText("one")
	evs = f.Turn(nil)
	l, _ = find[Log](evs)
	if !reflect.DeepEqual(l.Hidden, []int{g.Lines[1].Seq, g.Lines[2].Seq}) {
		t.Errorf("hidden %v", l.Hidden)
	}
	if fl, _ := find[Filter](evs); len(fl.Entries) == 0 || !fl.Entries[len(fl.Entries)-1].Only {
		t.Errorf("filter %+v; want the text row with Only", fl)
	}
	a.CloseLog("fm/kit")
	evs = f.Turn(nil)
	if l, _ := find[Log](evs); l.Open || len(evs) != 1 {
		t.Errorf("closing: %v, %+v", kinds(evs), l)
	}
}
```

Check while writing that `Untagged` shows in the filter list for these lines (they have no tags), and that `writeLog`'s three lines are the only lines loaded (they're on the day before `day`; `OpenLog` loads whole days, so all three). Adjust the indexes if `g.Lines` holds anything else.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/feed -run LogMode`
Expected: FAIL (no `Log` events).

- [ ] **Step 3: Implement** — replace the `logMode` stub with:

```go
// logMode is c's log-mode events: Log (through put), then LogLines and
// Filter while it's open. Closing it sends Log with Open false once.
func (f *Feed) logMode(c *app.Char, put func(string, Event)) []Event {
	g := c.Log
	if g == nil {
		delete(f.logs, c.Key)
		delete(f.small, idFilter+c.Key)
		if _, ok := f.small[idLog+c.Key]; ok {
			put(idLog+c.Key, Log{Key: c.Key})
		}
		return nil
	}
	put(idLog+c.Key, logState(c.Key, g))
	var evs []Event
	if op, ls, ok := f.diff(f.logs, c.Key, g, 0, len(g.Lines), func(i int) *app.Line { return &g.Lines[i].Line }); ok {
		evs = append(evs, LogLines{Key: c.Key, Op: op, Lines: ls})
	}
	items := g.Items()
	only, hasOnly := c.Filter.Only()
	fl := Filter{Key: c.Key}
	for _, e := range g.EntriesOf(items) {
		fl.Entries = append(fl.Entries, FilterEntry{
			PanelEntry: e, Hidden: c.Filter.Hidden(e.Item), Only: hasOnly && only == e.Item,
			Folded:        e.Parent && c.Collapsed[e.Item.Name],
			FilteredUnder: e.Parent && c.Collapsed[e.Item.Name] && g.FilteredUnder(e.Item.Name, items),
		})
	}
	if old, ok := f.small[idFilter+c.Key]; !ok || !reflect.DeepEqual(old, fl) {
		f.small[idFilter+c.Key] = fl
		evs = append(evs, fl)
	}
	return evs
}

func logState(k string, g *app.Log) Log {
	l := Log{Key: k, Open: true, Find: g.Find, Status: g.Status, StatusErr: g.StatusErr,
		Searching: g.Searching, Loading: g.Loading, HistDone: g.HistDone}
	if g.Start != nil {
		s := g.Start.Seq
		l.Start = &s
	}
	if g.End != nil {
		e := g.End.Seq
		l.End = &e
	}
	for _, ln := range g.Lines {
		if g.Excluded[ln] {
			l.Excluded = append(l.Excluded, ln.Seq)
		}
		if !g.Shows(ln) {
			l.Hidden = append(l.Hidden, ln.Seq)
		}
	}
	for _, m := range g.Matches() {
		l.Matches = append(l.Matches, m.Seq)
	}
	return l
}
```

The Filter event goes out after LogLines on purpose (its own ordering, not through `put`, which would append it before them). `Turn`'s close handling already drops `"log "`/`"filter "` for a closed character.

- [ ] **Step 4: Run the feed tests**, then everything. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/feed
git commit -m "feed: log mode, its lines and its filter panel"
```

---

### Task 4: review and PR

- [ ] **Step 1: Fresh review** of the branch against `main` with this plan's Review Focus. Fix Critical and Important findings with a failing test first; log minors.
- [ ] **Step 2: Open the PR** "Web API step 3: the change feed", linking the spec, saying the TUI doesn't change and no strings change, and listing the event types. Then stop.
