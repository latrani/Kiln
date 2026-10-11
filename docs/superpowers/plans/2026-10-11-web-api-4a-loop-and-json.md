# Web API, step 4a: the loop and the JSON API — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A front-end-neutral loop that owns the core, takes the page's actions as JSON, and hands back each turn's events as one JSON batch, tested natively. Step 4b puts it in wasm behind a test page and deletes the xterm shim.

**Architecture:**
- **`internal/webapi`** (plain Go, no `syscall/js`): `Loop` owns an `*app.App` and a `*feed.Feed` and drains one channel. Into it go the page's actions (`Act(json)`), the results of `app.Run` work, and a ticker. Each message is one turn: apply it, perform the effects that are the loop's (`Run` → a goroutine posting its result back), call `feed.Turn`, and send the events as one JSON batch through `out func([]byte)`. The first batch is `feed.All()`.
- **Actions** are one JSON shape, `{"do": "...", …fields}`, decoded into `Action` and mapped onto `app` calls. An unknown `do` or bad JSON says so on the status line (it's a bug in the page, not something a person did).
- **Events** go out as `{"type": "<Go type name>", …fields}`. Lines go out in a page-friendly form (`Line`: number, kind, day, time, direction, sanitized text with the server's SGR kept, tags with spans, attention, quiet). Effects go out as `{"type": "Effect", "effect": "<Copy|Notify|SaveFile|Do|Quit>", …}`.
- **Settings results.** The loop adds its own event after a settings action: `Saved{Op}` or `Problems{Op, Problems []Problem}` with `Problem{Field, Msg}` (`Field` "" for one about no field). One form is open at a time, so there's no request id.
- **Feed changes (from #141 and for the loop):**
  - `Input{Key, Text, Prompt, NeedPW}` replaces `Char.Text/Prompt/NeedPW` and the `Idle` event (the idle input is key `""`). The loop calls `Feed.Told(key)` right after it sets text the page sent, so the page's own text is never echoed back over newer typing; only text the core changes (submit, history, the password prompt) goes out.
  - `Char` gains `MaxLineBytes` and `NewlineMode`, so the page can call `OverLimit` without asking the core.
  - `All()` starts with `Start{}`: the page drops everything it holds.
  - `Replace` only when both ends are unchanged; `Reset` otherwise.
  - `logMode` syncs the filter (`g.Items()`) before building `Log`.
- **Core additions:** `App.HistoryOlder(cur string) bool` and `App.HistoryNewer() bool` on the active input (the TUI keeps driving `History` through its own input).

**Tech Stack:** Go; `internal/webapi` (new), `internal/feed`, `internal/app`; `encoding/json`.

**Spec:** `docs/superpowers/specs/2026-10-11-web-api-design.md`, "How the page and Kiln talk" and "The wasm side". Issues #141 (carry-overs) and #139 (numbering ui test lines; see Task 5).

## Global Constraints

- The TUI doesn't change; golden screens NOT updated.
- `internal/webapi` imports no `syscall/js`, Bubble Tea or lipgloss, and nothing from `internal/ui`; a test pins it.
- Strings: the only person-readable text the loop makes is the bad-action status, which goes in the catalog as `status.bad_action = "the page sent something Kiln can't use: {err}"`. Field names, `do` values and event type names are protocol vocabulary (`//str:ok`). The commit that adds it ends `Strings: status.bad_action (new)`.
- Run `go vet ./... && go test ./...` and the web module's tests (`cd web && go test ./... && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...`) before each commit. No `-race`.
- End each commit with the session's attribution lines.

## Review Focus

1. **The page's typing is never clobbered.** An `input` action followed by turns that don't touch the text sends no `Input` for that key; a `submit` sends `Input{Text: ""}`; a history recall sends the recalled text. `TestInputIsNotEchoed`.
2. **One batch per turn, in order, and nothing touches the core off the loop.** `Act` only enqueues; `Run` work runs in goroutines that only post back. The reviewer should check every path from `Act`/`Run` funcs to the `App`.
3. **Every effect is performed or forwarded exactly once.** `Run` by the loop; `Quit` forwarded and ends the loop after that batch; the rest forwarded.
4. **Bad input from the page** (bad JSON, unknown `do`, a key that isn't open, a line number not loaded) never panics: it's a status message, or ignored where the TUI would ignore it.
5. **Settings actions report every problem** as `Problems`, and `Saved` reloads the config like the TUI does after an edit (`Reload`), so the feed's `Worlds` follows.

---

### Task 1: the feed and core changes

**Files:**
- Modify: `internal/app/submit.go` (`HistoryOlder`, `HistoryNewer`), `internal/app/submit_test.go`
- Modify: `internal/feed/events.go`, `internal/feed/feed.go`, `internal/feed/feed_test.go`

**Interfaces:**
- Produces (in `app`): `func (a *App) HistoryOlder(cur string) bool`, `func (a *App) HistoryNewer() bool`.
- Produces (in `feed`): `type Input struct{ Key, Text, Prompt string; NeedPW bool }`; `type Start struct{}`; `Char` loses `Text`, `Prompt`, `NeedPW`, gains `MaxLineBytes int`, `NewlineMode string`; `Idle` is removed; `func (f *Feed) Told(key string)`.

- [ ] **Step 1: Tests**

`internal/app/submit_test.go`:

```go
func TestHistoryRecallOnTheActiveInput(t *testing.T) {
	a, _, _ := submitApp(t, fmWorld)
	a.SetInput("first")
	a.Submit()
	a.SetInput("draft")
	if !a.HistoryOlder("draft") || a.Text() != "first" {
		t.Fatalf("older: %q", a.Text())
	}
	if a.HistoryOlder("first") {
		t.Error("older past the oldest")
	}
	if !a.HistoryNewer() || a.Text() != "draft" {
		t.Errorf("newer: %q; want the draft back", a.Text())
	}
}
```

(Check `submitApp` opens and connects a character so `Submit` sends; if its first submit doesn't add to history, use a command like `/notify` that does. `grep -n "History.Add" internal/app/submit.go`.)

`internal/feed/feed_test.go`: in `TestFirstTurnSendsEverything` the want list becomes `{"Worlds", "Order", "Char", "Input", "Input", "Presence", "Active", "Status", "Confirm", "SaveQuestion", "Lines"}` (Kit's input, then the idle input), and its `Char` check stays. In `TestPresenceIdleTextConnectedAtAndStore`, `find[Idle]` becomes:

```go
	if i, _ := find[Input](evs); i.Key != "" || i.Text != "/open fm/kit" {
		t.Errorf("idle input %+v", i)
	}
```

In `TestAllSendsEverythingAgain`, `All` now sends `Start` first: compare `kinds(again)[1:]` with `kinds(first)` and check `kinds(again)[0] == "Start"`. Add:

```go
func TestInputIsNotEchoed(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	f := New(a)
	f.Turn(nil)
	a.SetInput("typed by the page")
	f.Told("fm/kit")
	if evs := f.Turn(nil); len(evs) != 0 {
		t.Errorf("the page's own text came back: %v", kinds(evs))
	}
	a.SetInput("") // the core clearing it, as Submit does
	if i, ok := find[Input](f.Turn(nil)); !ok || i.Text != "" || i.Key != "fm/kit" {
		t.Errorf("a core change: %+v, %v", i, ok)
	}
}

func TestCharCarriesTheLimit(t *testing.T) {
	a, _ := newApp(t)
	open(t, a, "fm/kit")
	c, _ := find[Char](New(a).Turn(nil))
	if c.MaxLineBytes != a.Char("fm/kit").Ch.MaxLineBytes || c.NewlineMode != a.Char("fm/kit").Ch.NewlineMode {
		t.Errorf("char %+v", c)
	}
}
```

- [ ] **Step 2: Run them to see them fail** — `go test ./internal/app ./internal/feed` → build failures.

- [ ] **Step 3: Implement**

`internal/app/submit.go`, after `IdleHistory`:

```go
// history is the active input's history: the active character's, or the
// idle input's.
func (a *App) history() *History {
	if c := a.chars[a.active]; c != nil {
		return &c.History
	}
	return &a.idleHist
}

// HistoryOlder puts the line sent before the one shown into the active
// input, cur (its text now) kept as the draft Newer comes back to. false
// if there's nothing older.
func (a *App) HistoryOlder(cur string) bool {
	v, ok := a.history().Older(cur)
	if ok {
		a.SetInput(v)
	}
	return ok
}

// HistoryNewer puts the next newer line, or the draft, into the active
// input. false if it isn't browsing history.
func (a *App) HistoryNewer() bool {
	v, ok := a.history().Newer()
	if ok {
		a.SetInput(v)
	}
	return ok
}
```

`internal/feed/events.go`:
- `Char`: delete `Text`, `Prompt`, `NeedPW`; add `MaxLineBytes int // max_line_bytes` and `NewlineMode string // newline_mode`.
- Replace `Idle` with:

```go
// Input is one input as the core holds it: a character's (Key), or with
// Key "" the input used while nothing is open. Text goes out only when
// the core changed it (a submit, a history recall, the password prompt):
// what the page sent is never echoed back (see Feed.Told).
type Input struct {
	Key, Text string
	Prompt    string // the server's unterminated prompt, sanitized; "" for none
	NeedPW    bool   // the password prompt is up
}

// Start begins everything (All): the page drops what it holds first.
type Start struct{}
```

  and the `event()` methods for `Input` and `Start` in place of `Idle`'s.

`internal/feed/feed.go`:
- A constant `idInput = "input " //str:ok: a map key` beside the others.
- `char` drops the three fields and sets `MaxLineBytes: c.Ch.MaxLineBytes, NewlineMode: c.Ch.NewlineMode`.
- In `Turn`, after each `put(idChar+k, …)`, `put(idInput+k, input(a, k))`; replace the `idle` put with `put(idInput, input(a, ""))`. When a character is gone, also `delete(f.small, idInput+k)`.
- Add:

```go
func input(a *app.App, k string) Input {
	if c := a.Char(k); c != nil {
		return Input{Key: k, Text: c.Text, Prompt: c.Prompt, NeedPW: c.NeedPW}
	}
	return Input{Text: a.IdleText()}
}

// Told records that the page set key's input text itself ("" for the
// idle input), so the next turn doesn't send it back.
func (f *Feed) Told(key string) { f.small[idInput+key] = input(f.a, key) }
```

- `All`: `return append([]Event{Start{}}, f.Turn(nil)...)` after `forget`; add `func (Start) event() {}` with the others.
- `diff`: the `gen != s.gen` case becomes `case gen != s.gen && lo == s.lo && hi == s.hi:` and a new case right after it, `case gen != s.gen:` → `record(true); return Reset, all(), true`.
- `logMode`: move `items := g.Items()` above `put(idLog+c.Key, logState(c.Key, g))`.

- [ ] **Step 4: Run everything.** Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app internal/feed docs/superpowers/plans/2026-10-11-web-api-4a-loop-and-json.md
git commit -m "feed: inputs are their own events and never echo the page; Start on All; history recall in the core"
```

---

### Task 2: events as JSON

**Files:**
- Create: `internal/webapi/encode.go`, `internal/webapi/encode_test.go`

**Interfaces:**
- Consumes: every `feed` event type.
- Produces: `type Line struct{ Seq int; Kind app.Kind; Day string; Time time.Time; Out bool; Text string; Tags []classify.Tag; Attention, Quiet bool }`; `func lineOf(l app.Line) Line`; `func Encode(evs []any) ([]byte, error)` — a JSON array, each element an object with `"type"` first; `feed.Lines`/`feed.LogLines` encoded with `[]Line`; `feed.Effect` encoded as `{"type":"Effect","effect":"<name>",…the effect's fields}`.

- [ ] **Step 1: Tests** — `internal/webapi/encode_test.go`:

```go
package webapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/feed"
	"github.com/latrani/Kiln/internal/logstore"
)

func decode(t *testing.T, b []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	return out
}

func TestEncodeTagsEachEventWithItsType(t *testing.T) {
	when := time.Date(2026, 9, 24, 21, 0, 0, 0, time.UTC)
	l := app.Line{Kind: app.Server, Seq: 3, Day: "2026-09-24", Entry: logstore.Entry{Time: when, Dir: logstore.In, Text: "\x1b[1mhi\x1b[0m\x07"}}
	b, err := Encode([]any{
		feed.Active{Key: "fm/kit"},
		feed.Lines{Key: "fm/kit", Epoch: 2, Op: feed.Append, Lines: []app.Line{l}},
		feed.Effect{Effect: app.Copy{Text: "x"}},
		Problems{Op: "save_world", Problems: []Problem{{Field: "port", Msg: "bad"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	evs := decode(t, b)
	if evs[0]["type"] != "Active" || evs[0]["Key"] != "fm/kit" {
		t.Errorf("active %v", evs[0])
	}
	lines := evs[1]["Lines"].([]any)
	got := lines[0].(map[string]any)
	if evs[1]["type"] != "Lines" || got["Seq"] != 3.0 || got["Text"] != "\x1b[1mhi\x1b[0m" || got["Out"] != false {
		t.Errorf("lines %v", evs[1])
	}
	if evs[2]["type"] != "Effect" || evs[2]["effect"] != "Copy" || evs[2]["Text"] != "x" {
		t.Errorf("effect %v", evs[2])
	}
	if evs[3]["type"] != "Problems" {
		t.Errorf("problems %v", evs[3])
	}
}
```

(The BEL is sanitized away; the SGR stays — check `app.Line.Text()` does exactly that, `ansi.Sanitize`.)

- [ ] **Step 2: Run** — fails to build.

- [ ] **Step 3: Implement** — `internal/webapi/encode.go`:

```go
// Package webapi is Kiln's core behind a page: a loop that owns the core,
// takes the page's actions as JSON, and hands back what changed each turn
// as one JSON batch. It's plain Go, tested natively; the wasm build
// connects it to the page.
package webapi

import (
	"bytes"
	"encoding/json"
	"reflect"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/feed"
	"github.com/latrani/Kiln/internal/logstore"
)

// Line is a line as the page gets it: Text sanitized, the server's SGR
// kept for the page to style.
type Line struct {
	Seq       int
	Kind      app.Kind
	Day       string
	Time      time.Time
	Out       bool // something you sent
	Text      string
	Tags      []classify.Tag
	Attention bool
	Quiet     bool
}

func lineOf(l app.Line) Line {
	return Line{Seq: l.Seq, Kind: l.Kind, Day: l.Day, Time: l.Entry.Time, Out: l.Entry.Dir == logstore.Out,
		Text: l.Text(), Tags: l.Tags, Attention: l.Attention, Quiet: l.Quiet}
}

func linesOf(ls []app.Line) []Line {
	out := make([]Line, len(ls))
	for i, l := range ls {
		out[i] = lineOf(l)
	}
	return out
}

// Encode is evs as a JSON array of objects, each with "type" (its Go
// type's name) first.
func Encode(evs []any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, e := range evs {
		if i > 0 {
			b.WriteByte(',')
		}
		typ := reflect.TypeOf(e).Name()
		var extra map[string]string
		switch v := e.(type) {
		case feed.Lines:
			e = struct {
				Key   string
				Epoch int
				Op    feed.Op
				Lines []Line
			}{v.Key, v.Epoch, v.Op, linesOf(v.Lines)}
		case feed.LogLines:
			e = struct {
				Key   string
				Epoch int
				Op    feed.Op
				Lines []Line
			}{v.Key, v.Epoch, v.Op, linesOf(v.Lines)}
		case feed.Effect:
			extra = map[string]string{"effect": reflect.TypeOf(v.Effect).Name()} //str:ok
			e = v.Effect
		}
		body, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		b.WriteString(`{"type":`) //str:ok
		t, _ := json.Marshal(typ)
		b.Write(t)
		for k, v := range extra {
			kb, _ := json.Marshal(k)
			vb, _ := json.Marshal(v)
			b.WriteByte(',')
			b.Write(kb)
			b.WriteByte(':')
			b.Write(vb)
		}
		if len(body) > 2 { // not "{}"
			b.WriteByte(',')
			b.Write(body[1 : len(body)-1])
		}
		b.WriteByte('}')
	}
	b.WriteByte(']')
	return b.Bytes(), nil
}
```

`Problems`/`Problem`/`Saved` are Task 4's; for this task, declare them in `encode.go` now:

```go
// Saved says a settings action worked; Op is the action's do.
type Saved struct{ Op string }

// Problems is what was wrong with a settings action: every problem, each
// naming its field ("" for one about no field).
type Problems struct {
	Op       string
	Problems []Problem
}

// Problem is one of Problems.
type Problem struct{ Field, Msg string }
```

- [ ] **Step 4: Run** — PASS. Then everything.

- [ ] **Step 5: Commit** — `git commit -m "webapi: events as JSON, lines in a page-friendly form"`.

---

### Task 3: the loop and the core actions

**Files:**
- Create: `internal/webapi/loop.go`, `internal/webapi/actions.go`, `internal/webapi/loop_test.go`
- Modify: `internal/str/locales/en.toml` (`status.bad_action`), regenerate

**Interfaces:**
- Consumes: `Encode`, `feed`, the `app` API.
- Produces: `type Loop`; `func New(a *app.App, out func([]byte)) *Loop`; `func (l *Loop) Act(js []byte)`; `func (l *Loop) Run(ctx context.Context, autoconnect bool)`; `type Action struct{…}` (fields below); `func OverLimit(text string, maxLineBytes int, newlineMode string) bool`.

`Action` fields (JSON names are the Go names): `Do string; Key, Text, Term, Tag, Day, Format, Name, ID, World, Char string; Seq, Start, End, Epoch, Cols, Rows int; In, Shut, Save bool; Item *scene.Item; WorldSettings *config.WorldSettings; CharSettings *config.CharacterSettings`.

Core actions this task maps (`do` → calls):

| do | does |
|---|---|
| `open` {Key} | `Open`; a load error goes on the status line (`str.StatusCharError`); a character that was closed and has `autoconnect` isn't connected by opening — the page connects with `submit` |
| `close` {Key} | `Close` |
| `switch` {Key} | `Switch` |
| `input` {Key, Text} | `SetInput(Text)` on the active input if Key is active (else on that character: `Char(Key).Text = Text` is the core's field — use `Switch`-free `SetInputFor`? no: ignore a Key that isn't active), then `Told(Key)` |
| `submit` {Text} | `SetInput(Text)`, `Told`, `Submit` |
| `older` {Text} / `newer` | `SetInput(Text)`, `Told`, `HistoryOlder(Text)` / `HistoryNewer()` |
| `cancel` | Esc: `Unconfirm`, then `SkipLogin` |
| `here`, `scrolled`, `focus` {In} | `Here`, `Scrolled`, `Focus(In)` |
| `page` {Key} | `RequestOlder(Key)` |
| `answer_save` {Save} | `AnswerSave(Save)` |
| `resize` {Cols, Rows} | `SetPane(Cols, Rows)`, `ReportSize()` |
| `all` | the batch is `feed.All()` instead of a turn |
| `since` {Key, Epoch, Seq} | a `feed.Lines{Op: Append}` of `Since` if ok, else the batch is `feed.All()` |
| `quit` | `Quit()` |

- [ ] **Step 1: Tests** — `internal/webapi/loop_test.go`. Build the `App` the way `internal/feed/feed_test.go`'s `newApp` does (copy `newApp`, `writeWorld`, `attach`), and a harness:

```go
// harness runs a Loop and collects its batches.
type harness struct {
	t       *testing.T
	a       *app.App
	l       *Loop
	batches chan []map[string]any
	cancel  func()
}

func start(t *testing.T) *harness {
	t.Helper()
	a, _ := newApp(t)
	h := &harness{t: t, a: a, batches: make(chan []map[string]any, 100)}
	h.l = New(a, func(b []byte) { h.batches <- decode(t, b) })
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go h.l.Run(ctx, false)
	t.Cleanup(cancel)
	h.next() // the first batch: everything
	return h
}

// next is the next batch, or fails after a second.
func (h *harness) next() []map[string]any {
	h.t.Helper()
	select {
	case b := <-h.batches:
		return b
	case <-time.After(time.Second):
		h.t.Fatal("no batch")
		return nil
	}
}

// act sends an action and returns its turn's batch.
func (h *harness) act(js string) []map[string]any {
	h.t.Helper()
	h.l.Act([]byte(js))
	return h.next()
}

func types(b []map[string]any) []string {
	var out []string
	for _, e := range b {
		out = append(out, e["type"].(string))
	}
	return out
}
```

Tests:

```go
func TestFirstBatchIsEverything(t *testing.T) {
	a, _ := newApp(t)
	got := make(chan []map[string]any, 1)
	l := New(a, func(b []byte) { got <- decode(t, b) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.Run(ctx, false)
	b := <-got
	if types(b)[0] != "Start" {
		t.Errorf("first batch %v", types(b))
	}
}

func TestOpenSubmitAndInput(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"open","Key":"fm/kit"}`)
	if !slices.Contains(types(b), "Char") || !slices.Contains(types(b), "Active") {
		t.Fatalf("open: %v", types(b))
	}
	b = h.act(`{"Do":"input","Key":"fm/kit","Text":"hello"}`)
	if slices.Contains(types(b), "Input") {
		t.Errorf("the page's own text came back: %v", b)
	}
	b = h.act(`{"Do":"submit","Text":"/notify off"}`) // a command: no connection needed
	var cleared bool
	for _, e := range b {
		if e["type"] == "Input" && e["Key"] == "fm/kit" && e["Text"] == "" {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("submit should clear the input: %v", b)
	}
}

func TestBadActionsSaySo(t *testing.T) {
	h := start(t)
	for _, js := range []string{`not json`, `{"Do":"frobnicate"}`} {
		b := h.act(js)
		var said bool
		for _, e := range b {
			if e["type"] == "Status" && e["Err"] == true && strings.HasPrefix(e["Text"].(string), upTo(str.StatusBadAction(mark), mark)) {
				said = true
			}
		}
		if !said {
			t.Errorf("%s: %v", js, b)
		}
	}
	h.act(`{"Do":"close","Key":"nobody/here"}`) // ignored, as the TUI would
}

func TestRunWorkComesBackOnTheLoop(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"submit","Text":""}`) // connect: a Run waiting on the session
	deadline := time.After(2 * time.Second)
	for {
		for _, e := range h.next() {
			if e["type"] == "Char" && e["State"] == float64(session.Connected) {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatal("never connected")
		default:
		}
	}
}

func TestQuitEndsTheLoop(t *testing.T) {
	a, _ := newApp(t)
	got := make(chan []map[string]any, 10)
	l := New(a, func(b []byte) { got <- decode(t, b) })
	done := make(chan struct{})
	go func() { l.Run(context.Background(), false); close(done) }()
	<-got
	l.Act([]byte(`{"Do":"quit"}`))
	b := <-got
	if types(b)[len(b)-1] != "Effect" || b[len(b)-1]["effect"] != "Quit" {
		t.Errorf("last batch %v", types(b))
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("the loop kept running")
	}
}

func TestOverLimit(t *testing.T) {
	if OverLimit("12345", 5, "batch") || !OverLimit("123456", 5, "batch") || !OverLimit("123\n45", 5, "flatten") {
		t.Error("OverLimit")
	}
}

// webapi is plain Go: no js, no terminal, no UI package.
func TestWebapiDependsOnNoFrontEnd(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "syscall/js" || strings.HasSuffix(dep, "/internal/ui") || strings.Contains(dep, "bubbletea") || strings.Contains(dep, "lipgloss") {
			t.Errorf("webapi depends on %s", dep)
		}
	}
}
```

`mark` and `upTo` follow the catalog-test convention (`grep -rn "func upTo\|mark =" internal/*/catalog_test.go`); add a `catalog_test.go` in `webapi` with the same helpers. `newApp`'s `Dial` returns `kilntest.NewConn()`, so connecting succeeds; `fmWorld` needs no login line for the connect to settle (check `session` doesn't wait on a password when `login` is unset).

- [ ] **Step 2: Run** — fails to build.

- [ ] **Step 3: The string** — add to `[status]` in `internal/str/locales/en.toml`: `bad_action = "the page sent something Kiln can't use: {err}"`, then `go generate ./internal/str`.

- [ ] **Step 4: The loop** — `internal/webapi/loop.go`:

```go
package webapi

import (
	"context"
	"encoding/json"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/feed"
	"github.com/latrani/Kiln/internal/str"
)

// tickEvery is how often a turn runs with nothing else happening, so
// what changes with time alone (presence going away) reaches the page.
const tickEvery = 30 * time.Second

// Loop owns the core for one page. Everything that touches the core
// happens in Run, one message at a time.
type Loop struct {
	a   *app.App
	f   *feed.Feed
	in  chan any
	out func([]byte)
}

type tick struct{}

// New makes a loop over a that sends each turn's batch to out.
func New(a *app.App, out func([]byte)) *Loop {
	return &Loop{a: a, f: feed.New(a), in: make(chan any, 64), out: out}
}

// Act hands the loop one action from the page, as JSON. It never blocks
// on the core.
func (l *Loop) Act(js []byte) { l.in <- js }

// Run drains the loop until ctx is done or the core quits. It first
// opens and connects autoconnect characters if asked, then sends
// everything.
func (l *Loop) Run(ctx context.Context, autoconnect bool) {
	var effs []app.Effect
	if autoconnect {
		for _, ch := range l.a.AllChars() {
			if ch.Autoconnect {
				k := app.Key(ch.World, ch.ID)
				if _, err := l.a.Open(k); err != nil {
					l.a.SetStatus(true, str.StatusCharError(k, err))
				}
				effs = append(effs, l.a.Connect(k)...)
			}
		}
	}
	l.perform(effs)
	l.send(l.f.All())
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		var msg any
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			msg = tick{}
		case msg = <-l.in:
		}
		evs, effs, all := l.step(msg)
		quit := l.perform(effs)
		if all {
			evs = append(l.f.All(), evs...)
		} else {
			evs = append(l.f.Turn(effs), evs...)
		}
		l.send(evs)
		if quit {
			return
		}
	}
}

// perform runs the loop's own effects (Run, in a goroutine that posts its
// result back) and reports whether one was Quit. The rest go to the page
// through the feed.
func (l *Loop) perform(effs []app.Effect) (quit bool) {
	for _, e := range effs {
		switch e := e.(type) {
		case app.Run:
			go func() { l.in <- e.Func() }()
		case app.Quit:
			quit = true
		}
	}
	return quit
}

func (l *Loop) send(evs []feed.Event) {
	anys := make([]any, len(evs))
	for i, e := range evs {
		anys[i] = e
	}
	b, err := Encode(anys)
	if err != nil {
		return // a Go value that won't marshal is a bug here; nothing to tell the page
	}
	l.out(b)
}

// step handles one message: the page's action, a Run's result, or a
// tick. extra are events the loop adds (settings results); all asks for
// everything instead of a turn.
func (l *Loop) step(msg any) (extra []feed.Event, effs []app.Effect, all bool) {
	switch m := msg.(type) {
	case []byte:
		var act Action
		if err := json.Unmarshal(m, &act); err != nil {
			l.a.SetStatus(true, str.StatusBadAction(err))
			return nil, nil, false
		}
		return l.do(act)
	case app.SessionMsg:
		_, _, effs := l.a.Handle(m)
		return nil, effs, false
	case app.OlderMsg:
		l.a.HandleOlder(m)
	case app.LogOlderMsg:
		return nil, l.a.HandleLogOlder(m), false
	}
	return nil, nil, false
}

// OverLimit reports whether text is too long to send under these
// settings: app.OverLimit for a page that has a Char event's.
func OverLimit(text string, maxLineBytes int, newlineMode string) bool {
	return app.OverLimit(text, config.Character{MaxLineBytes: maxLineBytes, NewlineMode: newlineMode})
}
```

(`feed.Event` must be the extra events' type: `Saved` and `Problems` need an `event()` method, which is unexported in `feed`. So make `extra` `[]any` and `send` take `[]any`: change `send`'s parameter to `[]any`, convert the feed's events with a helper `func anys(evs []feed.Event) []any`. Adjust the code above accordingly. Add the `config` import.)

`internal/webapi/actions.go`: `type Action struct{…}` with the fields listed, and `func (l *Loop) do(act Action) (extra []any, effs []app.Effect, all bool)` with a `switch act.Do` implementing the table. Unknown `do`: `l.a.SetStatus(true, str.StatusBadAction(fmt.Errorf("%q", act.Do)))`. Each `do` string is `//str:ok`. For `input`: only when `act.Key == l.a.Active()` (the idle input when both are ""); otherwise ignore. For `since`: `if ls, ok := l.f.Since(act.Key, act.Epoch, act.Seq); ok { extra = append(extra, feed.Lines{Key: act.Key, Epoch: act.Epoch, Op: feed.Append, Lines: ls}) } else { all = true }`.

- [ ] **Step 5: Run** — PASS; then everything.

- [ ] **Step 6: Commit** — `git commit -m "webapi: a loop that owns the core and talks JSON" -m "Strings: status.bad_action (new)"` (one message with the trailer as its last paragraph).

---

### Task 4: log mode, the filter panel and settings

**Files:**
- Modify: `internal/webapi/actions.go`, `internal/webapi/loop_test.go`

**Interfaces:**
- Consumes: Task 3's `do`; the `app.Log` API (by line number: look lines up with a helper `lineAt(g *app.Log, seq int) *app.LogLine` that scans `g.Lines` for `Seq == seq`, nil if not loaded).
- Produces: the actions below.

| do | does |
|---|---|
| `log` {Key} | `OpenLog(Key)`; `Shown` stays nil (find matches `Line.Plain`, which is what the page shows) |
| `log_close` {Key} | `CloseLog(Key)` |
| `log_older` {Key} | `Log.Older(nil)` |
| `mark` {Key, Seq} | `Mark(lineAt)` |
| `range` {Key, Start, End} | `NewRange(lineAt(Start), lineAt(End))`, then `RangeStatus()` |
| `extend` {Key, Seq} | `ExtendTo(lineAt)` |
| `exclude` {Key, Seq} | `ToggleExclude(lineAt)` |
| `find` {Key, Term, Seq} | `SetFind(Term, lineAt(Seq), nil)` (the page steps through `Log.Matches` itself; a match found by paging shows up in `Matches`) |
| `find_older` {Key, Seq} | `FindOlder(lineAt(Seq), false, nil)` |
| `stop_search` {Key} | `StopSearch()` |
| `goto_date` {Key, Day} | `GotoDate(Day, nil)` |
| `copy` {Key} | `Copy()` |
| `export` {Key, Format, Name} | `Export(Format, Name)` |
| `hide` / `only` {Key, Item} | `ToggleHide(*Item)` / `PressOnly(*Item)`, then `Refilter()` |
| `fold` {Key, Tag, Shut} | `Fold(Tag, Shut)` |
| `add_text` / `remove_text` {Key, Term} | `AddText(Term)` / `RemoveText(Term)`, then `Refilter()` |
| `add_world` {ID, WorldSettings} | `AddWorld`; ok → `Reload()` and `Saved`; else `Problems` |
| `save_world` {World, WorldSettings} | `SaveWorld`, same |
| `add_character` {World, Name} | `AddCharacter`, same |
| `save_character` {World, Char, CharSettings} | `SaveCharacter`, same |
| `delete_world` {World} / `delete_character` {World, Char} / `forget_password` {World, Char} | the app call, same; a kept password's error (`DeleteCharacter`'s `pwErr`) goes on the status line as the TUI does (`str.EditorDeletedPasswordKept`) |

Log actions on a character without log mode open, or a `Seq` not loaded (`lineAt` nil), do nothing (the core's calls already treat nil as nothing, e.g. `Mark(nil)`; `ToggleExclude(nil)` says why on log mode's status, which is fine).

`Problems` from an error: `for _, p := range app.Problems(err)`, `Problem{Msg: p.Error()}` with `Field` from `errors.As(p, &fe)`.

- [ ] **Step 1: Tests** — add to `loop_test.go`: a log-mode test (write Kit's log with three lines before `open`; `log` → a batch with `Log` and `LogLines`; `mark` the first and third by their `Seq` from the `LogLines` batch → the next batch's `Log` has `Start`/`End`; `add_text` → `Filter` with the text row and `Log.Hidden`), and a settings test (`add_world` with `{"ID":"fm","WorldSettings":{"Host":"","Port":0}}` → a `Problems` event naming `port`, `host`, `id`; `add_world` with a good new id → `Saved` and a `Worlds` event listing it).

- [ ] **Step 2–4:** run (fail), implement the table, run everything.

- [ ] **Step 5: Commit** — `git commit -m "webapi: log mode, filter panel and settings actions"`.

---

### Task 5: review and PR

- [ ] **Step 1: Fresh review** of the branch against `main` with this plan's Review Focus. Fix Critical and Important findings with a failing test first; log minors.
- [ ] **Step 2: Open the PR** "Web API step 4a: the loop and the JSON API". The body lists the actions and events, says the TUI doesn't change, lists `Strings: status.bad_action (new)` and why it exists, and ticks off what it closes from #141 (input split, Start on All, one batch per turn, Replace only with unchanged ends, filter synced first, presence timer). #141's cost item stays open (not done); #139 is step 4b's (log mode by number in the TUI's tests isn't touched here). Then stop.
