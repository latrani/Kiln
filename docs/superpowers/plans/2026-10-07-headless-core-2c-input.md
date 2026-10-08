# Headless core, step 2c: the core owns input, submit, commands and the password prompt — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** What Enter does lives in `app`: sending lines, slash commands, the over-limit confirmation, logging in at the password prompt and the save-password question, plus each input's history and draft text. The TUI's `Input` stays the terminal's editing widget, kept in step with the core.

**Architecture:**
- **History** is an `app.History` the core owns, one per character plus one for the idle input. `ui.Input` holds a pointer to it (nil for form fields), so Up/Down at the edge and Commit go through the core's history.
- **Draft text:** the core keeps each input's text (`Char.Text`, the idle text). The TUI pushes the active widget's text after every key or paste (`SetInput`). After every core call that can change a text (`Submit`, `Handle`, `SkipLogin`), the TUI pulls it into the widget.
- **Commands:** `Submit` runs the commands that are about the core. For the ones about the TUI's own screens, or not in the core yet (`/open`, `/log`, `/edit`, `/notify`, `/away`, `/backup`, `/restore`), it returns a `Do` effect that the TUI carries out on its loop.

**Tech Stack:** Go; `internal/app`, `internal/ui`, `session`, `config`, `conn`, `str`.

**Spec:** `docs/superpowers/specs/2026-10-07-headless-core-design.md`, sections "Input" and "Submit, commands and the password prompt". 2a's plan has the scope ruling that splits step 2.

## Global Constraints

- Nothing visible changes: golden screens are NOT updated (never `-update`).
- Every existing test passes. Tests may change how they reach state (`.NeedPW`, `h.m.a.PendingSave()`, `in.hist.Lines()`) or set it up, never what they expect.
- `internal/app` doesn't depend on Bubble Tea or lipgloss and never paints.
- Only `App` changes `app.Char` fields; the TUI reads them and keeps its widgets in step.
- Strings come from `internal/str`; this step adds none (`StatusUnknownCommand` etc. move with their code).
- `ui.Deps` doesn't change: `ui.New` passes `ConfigDir`, `KnownHosts` and `SavePassword` on to `app.Deps`.
- Run `go vet ./... && go test ./...` before each commit.

## Rulings carried from the spec

- **The caret stays in the front end.** The spec has `SetInput(text, caret)`. The TUI's widget owns its cursor, and a browser's textarea owns its caret, so the core keeps text only (`SetInput(text)`). If part 2 (the web API) finds it needs the caret, it adds it then.
- **Pulling text from the core after `Handle`** covers the password prompt starting or ending on a character you aren't looking at: its widget follows its core text.

## Review Focus

1. **The over-limit confirmation.**
   - The first Enter warns and the second sends.
   - Any edit key, paste, Esc, or switching (including closing the active character) in between asks again.
   - Moving the cursor counts as an edit, as on `main`.
   - Covered by existing tests plus Task 3's rewritten `TestClosingResetsTheOverLimitConfirm`.
2. **The password prompt.**
   - The draft is stashed when the prompt starts and restored when it ends (submit, Esc, or the connection dropping).
   - A failed send keeps the prompt, with the field cleared.
   - The password never reaches history.
   - Covered by the existing `model_test.go` password tests.
3. **History browsing.** Up from a draft recalls older lines, and Down past the newest brings the draft back. Ctrl+C and sending stop browsing. Task 1 tests the core type; `input_test.go` covers the widget.
4. **`//foo` sends `/foo`, and history records `//foo`.** A redacted line (a typed password) isn't recorded. Covered by existing tests.
5. **Commands with nothing open.** `/quit`, `/open`, `/backup` and `/restore` work. Others say they need a character. `/backup` and `/restore` are "unknown" outside the web build. Covered by existing tests.

---

### Task 1: `app.History`, and `ui.Input` browses it

**Files:**
- Create: `internal/app/history.go`, `internal/app/history_test.go`
- Modify: `internal/ui/input.go`, `internal/ui/input_test.go` (setup/access only), `internal/ui/model_test.go` (access only)

**Interfaces:**
- Produces: `type app.History struct{…}`, with methods `Add(v string)`, `Older(cur string) (string, bool)`, `Newer() (string, bool)`, `Stop()`, `Pos() int`, `Lines() []string`. `ui.Input` gains `hist *app.History` (nil: no history) and loses `history`, `hist int`, `draft`.

- [ ] **Step 1: Write the tests**

`internal/app/history_test.go`:

```go
package app

import (
	"slices"
	"testing"
)

func TestHistoryBrowsesAndBringsTheDraftBack(t *testing.T) {
	var h History
	if _, ok := h.Older("draft"); ok {
		t.Error("Older with no history moved")
	}
	h.Add("look")
	h.Add("look") // a repeat of the last isn't recorded again
	h.Add("")
	h.Add("say hi")
	if got := h.Lines(); !slices.Equal(got, []string{"look", "say hi"}) {
		t.Errorf("Lines = %q", got)
	}
	if v, ok := h.Older("half typed"); !ok || v != "say hi" {
		t.Errorf("Older = %q, %v", v, ok)
	}
	if v, _ := h.Older("say hi"); v != "look" || h.Pos() != 0 {
		t.Errorf("Older again = %q at %d", v, h.Pos())
	}
	if _, ok := h.Older("look"); ok {
		t.Error("Older past the oldest moved")
	}
	h.Newer()
	if v, ok := h.Newer(); !ok || v != "half typed" {
		t.Errorf("Newer past the newest = %q, %v; want the draft back", v, ok)
	}
	if _, ok := h.Newer(); ok {
		t.Error("Newer while not browsing moved")
	}
	h.Older("x")
	h.Stop()
	if h.Pos() != len(h.Lines()) {
		t.Error("Stop didn't stop browsing")
	}
}
```

In `internal/ui/input_test.go`, the tests that rely on history give their `Input` one: after `in := NewInput()` in those tests (the one reading `in.history` near line 140, and the one reading `in.hist` near line 303), add `in.hist = &app.History{}`. Then `in.history` → `in.hist.Lines()` and `in.hist != 0` → `in.hist.Pos() != 0` (and its `%d` argument `in.hist.Pos()`). In `internal/ui/model_test.go`, `h.m.chars["fm/kit"].in.history` → `h.m.chars["fm/kit"].in.hist.Lines()` (this works once Task 3 gives character inputs the core's history; until then the char input gets one in `open`, below).

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app -run TestHistory`
Expected: build failure — `undefined: History`.

- [ ] **Step 3: Implement**

`internal/app/history.go`:

```go
package app

// History is one input's sent lines, oldest first, and where browsing
// them has got to. The zero value is empty and not browsing.
type History struct {
	lines []string
	pos   int    // the line shown while browsing; len(lines) = not browsing
	draft string // what was being typed when browsing started
}

// Add records v, unless it's empty or the same as the last line, and
// stops browsing.
func (h *History) Add(v string) {
	if v != "" && (len(h.lines) == 0 || h.lines[len(h.lines)-1] != v) {
		h.lines = append(h.lines, v)
	}
	h.Stop()
}

// Older is the line before the one shown, for Up at the top of the input.
// cur is the input's text: starting to browse keeps it as the draft that
// Newer brings back. ok is false when there's nothing older.
func (h *History) Older(cur string) (string, bool) {
	if h.pos == 0 || len(h.lines) == 0 {
		return "", false
	}
	if h.pos == len(h.lines) {
		h.draft = cur
	}
	h.pos--
	return h.lines[h.pos], true
}

// Newer is the line after the one shown, and past the newest, the draft.
// ok is false when not browsing.
func (h *History) Newer() (string, bool) {
	if h.pos >= len(h.lines) {
		return "", false
	}
	h.pos++
	if h.pos == len(h.lines) {
		return h.draft, true
	}
	return h.lines[h.pos], true
}

// Stop stops browsing: the input has been cleared or sent.
func (h *History) Stop() { h.pos = len(h.lines) }

// Pos is the line being shown while browsing; len(Lines()) when not.
func (h *History) Pos() int { return h.pos }

// Lines is the history, oldest first. Don't change it.
func (h *History) Lines() []string { return h.lines }
```

`internal/ui/input.go`:
- In `Input`, replace the `history []string`, `hist int` and `draft string` fields with `hist *app.History // the core's history for this input; nil for a field without one`.
- `NewInput` stays `&Input{lines: [][]rune{nil}, goal: -1}`.
- `Reset`:

```go
// Reset clears the text and stops browsing history.
func (in *Input) Reset() {
	in.SetValue("")
	if in.hist != nil {
		in.hist.Stop()
	}
}
```

- `Commit`:

```go
// Commit returns the text, records it in history, and clears the editor.
func (in *Input) Commit() string {
	v := in.Value()
	if in.hist != nil {
		in.hist.Add(v)
	}
	in.Reset()
	return v
}
```

- `Up`'s history half: after the screen-row move, `if in.hist == nil { return }; if v, ok := in.hist.Older(in.Value()); ok { in.SetValue(v) }`.
- `Down`'s history half: after the row move, `if in.hist == nil { return }; if v, ok := in.hist.Newer(); ok { in.SetValue(v) }`.

In `internal/ui/sidebar.go` `open`, build the input as `in := NewInput(); in.hist = &c.History` once Task 3 adds `Char.History`. Until then, give it its own: `in.hist = &app.History{}`. Do the same for `Model.idle` in `New` (`m.idle.hist = &app.History{}` until Task 3 points it at the core's idle history).

- [ ] **Step 4: Run everything**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app internal/ui
git commit -m "app: History is the core's; the input widget browses it

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```

---

### Task 2: Submit, commands, the password prompt and the save question in the core

**Files:**
- Create: `internal/app/submit.go`, `internal/app/submit_test.go`
- Modify: `internal/app/app.go` (`Deps` gains `ConfigDir`, `KnownHosts`, `SavePassword`; `Char` gains `History`, `Text`, `NeedPW`, `pwDraft`; `App` gains `idleText`, `idleHist`, `confirm`, `pending`; `Switch` clears `confirm`), `internal/app/session.go` (`Handle` starts and ends the password prompt), `internal/app/effect.go` (`Do`)

**Interfaces:**
- Consumes: Task 1's `History`; 2a/2b's `Connect`, `Close`, `Quit`, `Echo`, `SetStatus`, `ClearStatus`.
- Produces:
  - `type app.Do struct { Cmd string; Args []string; Key string }` (an `Effect`)
  - `type app.SubmitResult struct { Sent, Echoed bool }`
  - `(*App)`: `SetInput(text string)`, `Text() string`, `IdleHistory() *History`, `Unconfirm()`, `Confirming() bool`, `Submit() (SubmitResult, []Effect)`, `SkipLogin() bool`, `PendingSave() (world, char string, ok bool)`, `AnswerSave(save bool)`
  - `app.Char`: `History History`, `Text string`, `NeedPW bool`

- [ ] **Step 1: Write the tests**

`internal/app/submit_test.go`:

```go
package app

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

// sentConn records what's sent.
type sentConn struct {
	lineConn
	mu   sync.Mutex
	sent []string
}

func (c *sentConn) Send(l string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, l)
	return nil
}

func (c *sentConn) Sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.sent)
}

// submitApp is an App over fm with Kit connected to a sentConn; saved
// collects SavePassword calls.
func submitApp(t *testing.T, world string) (*App, *sentConn, map[string]string) {
	t.Helper()
	dir := configDir(t, map[string]string{"fm": world})
	c := &sentConn{lineConn: lineConn{lines: make(chan string)}}
	saved := map[string]string{}
	a := New(Deps{
		ConfigDir: dir,
		LogRoot:   filepath.Join(dir, "logs"),
		Dial:      func(context.Context, config.Character) (session.LineConn, error) { return c, nil },
		NewLog:    func(logstore.Layout) session.Appender { return nopLog{} },
		Password:  func(string, string, string) (string, error) { return "", errors.New("none") },
		SavePassword: func(_, world, char, pw string) error {
			saved[Key(world, char)] = pw
			return nil
		},
		Now: func() time.Time { return now },
	})
	a.ApplyConfig(load(t, dir))
	openAll(t, a, "fm/kit")
	effs := a.Connect("fm/kit")
	t.Cleanup(func() { a.Quit() })
	// Run the session's events through Handle until it's connected.
	msg := effs[0].(Run).Func().(SessionMsg)
	for a.Char("fm/kit").State != session.Connected {
		_, _, effs = a.Handle(msg)
		msg = effs[0].(Run).Func().(SessionMsg)
	}
	return a, c, saved
}

func TestSubmitSendsAndRecords(t *testing.T) {
	a, c, _ := submitApp(t, fmWorld)
	a.SetInput("//foo")
	res, _ := a.Submit()
	if !res.Sent || !slices.Equal(c.Sent(), []string{"/foo"}) {
		t.Errorf("sent %q, result %+v", c.Sent(), res)
	}
	kit := a.Char("fm/kit")
	if kit.Text != "" || !slices.Equal(kit.History.Lines(), []string{"//foo"}) {
		t.Errorf("after sending: text %q history %q", kit.Text, kit.History.Lines())
	}
}

func TestOverLimitAsksFirst(t *testing.T) {
	a, c, _ := submitApp(t, strings.Replace(fmWorld, "tls = true\n", "tls = true\nmax_line_bytes = 5\n", 1))
	a.SetInput("too long")
	if res, _ := a.Submit(); res.Sent || !a.Confirming() || a.Status().Text != str.StatusOverLimit(5) {
		t.Fatalf("first Enter: %+v confirming %v status %q", res, a.Confirming(), a.Status().Text)
	}
	a.Unconfirm()
	if res, _ := a.Submit(); res.Sent {
		t.Error("an edit in between should ask again")
	}
	if res, _ := a.Submit(); !res.Sent || !slices.Equal(c.Sent(), []string{"too long"}) {
		t.Errorf("second Enter: %+v sent %q", res, c.Sent())
	}
}

func TestCommandsInTheCore(t *testing.T) {
	a, _, _ := submitApp(t, fmWorld)
	for _, c := range []struct {
		text string
		do   string // a Do the front end carries out; "" for none
	}{{"/log", "/log"}, {"/edit world", "/edit"}, {"/notify all", "/notify"}, {"/away", "/away"}, {"/backup", "/backup"}, {"/frob", ""}} {
		a.SetInput(c.text)
		_, effs := a.Submit()
		var got string
		for _, e := range effs {
			if d, ok := e.(Do); ok {
				got = d.Cmd
				if d.Key != "fm/kit" {
					t.Errorf("%s: Do for %q", c.text, d.Key)
				}
			}
		}
		if got != c.do {
			t.Errorf("%s: Do %q, want %q", c.text, got, c.do)
		}
	}
	if a.Status().Text != str.StatusUnknownCommand("/frob") {
		t.Errorf("status = %q", a.Status().Text)
	}
	a.SetInput("/connect")
	a.Submit()
	if a.Status().Text != str.StatusAlreadyConnected("Kit") {
		t.Errorf("/connect while connected: %q", a.Status().Text)
	}
	a.SetInput("/close")
	a.Submit()
	if a.Char("fm/kit") != nil {
		t.Error("/close left Kit open")
	}
	a.SetInput("/connect")
	if _, effs := a.Submit(); effs != nil || a.Status().Text != str.StatusNeedsCharacter("/connect") {
		t.Errorf("with nothing open: effects %v status %q", effs, a.Status().Text)
	}
	a.SetInput("/open")
	if _, effs := a.Submit(); len(effs) != 1 || effs[0].(Do).Cmd != "/open" {
		t.Errorf("/open with nothing open: %v", effs)
	}
}

func TestPasswordPromptLogsInAndAsksToSave(t *testing.T) {
	a, c, saved := submitApp(t, fmWorld)
	kit := a.Char("fm/kit")
	a.SetInput("half a pose")
	a.Handle(SessionMsg{Key: "fm/kit", Sess: kit.Sess, OK: true, Ev: session.Event{Kind: session.EventNeedPassword}})
	if !kit.NeedPW || kit.Text != "" {
		t.Fatalf("prompt: needPW %v text %q; want the draft stashed", kit.NeedPW, kit.Text)
	}
	a.SetInput("s3cret")
	a.Submit()
	if kit.NeedPW || kit.Text != "half a pose" || slices.Contains(kit.History.Lines(), "s3cret") {
		t.Errorf("after logging in: needPW %v text %q history %q", kit.NeedPW, kit.Text, kit.History.Lines())
	}
	if sent := c.Sent(); len(sent) == 0 || !strings.Contains(sent[len(sent)-1], "s3cret") {
		t.Errorf("sent %q, want the login line", sent)
	}
	w, ch, ok := a.PendingSave()
	if !ok || w != "fm" || ch != "kit" {
		t.Fatalf("PendingSave = %q %q %v", w, ch, ok)
	}
	a.AnswerSave(true)
	if saved["fm/kit"] != "s3cret" || a.Status().Text != str.StatusPasswordSaved() {
		t.Errorf("saved %q status %q", saved, a.Status().Text)
	}
	if _, _, ok := a.PendingSave(); ok {
		t.Error("still pending after the answer")
	}
}

func TestSkippingTheLogin(t *testing.T) {
	a, _, _ := submitApp(t, fmWorld)
	kit := a.Char("fm/kit")
	a.SetInput("draft")
	a.Handle(SessionMsg{Key: "fm/kit", Sess: kit.Sess, OK: true, Ev: session.Event{Kind: session.EventNeedPassword}})
	if !a.SkipLogin() || kit.NeedPW || kit.Text != "draft" || a.Status().Text != str.StatusSkippedLogin() {
		t.Errorf("skip: needPW %v text %q status %q", kit.NeedPW, kit.Text, a.Status().Text)
	}
	if a.SkipLogin() {
		t.Error("SkipLogin with no prompt up did something")
	}
}
```

Check the str names against `internal/str/keys_gen.go` when writing (`StatusOverLimit`, `StatusUnknownCommand`, `StatusAlreadyConnected`, `StatusNeedsCharacter`, `StatusPasswordSaved`, `StatusSkippedLogin`); they're the ones `ui/model.go` calls today.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app -run 'TestSubmit|TestOverLimit|TestCommands|TestPassword|TestSkipping'`
Expected: build failure — `unknown field ConfigDir`, `a.SetInput undefined`, `undefined: Do`.

- [ ] **Step 3: Implement**

`internal/app/effect.go`: add

```go
// Do is a command for the front end to carry out: one about its own
// screens (/open, /log, /edit), or one the core doesn't handle yet
// (/notify, /away, /backup, /restore). Key is the active character's, or
// "" with none.
type Do struct {
	Cmd  string
	Args []string
	Key  string
}

func (Do) effect() {}
```

`internal/app/app.go`:
- `Deps` gains:

```go
	ConfigDir    string
	KnownHosts   conn.KnownHosts
	SavePassword func(store, world, char, password string) error // nil: never offer
```

- `Char` gains (after `Loading`):

```go
	History     History // what's been sent from this character's input
	Text        string  // the input's text, as the front end last said
	NeedPW      bool    // the password prompt is up
	pwDraft     string  // Text stashed while the password prompt is up
```

- `App` gains:

```go
	idleText string  // the input's text while nothing is open
	idleHist History // its history
	confirm  bool    // the next Submit sends an over-limit line anyway
	pending  *pendingSave
```

with

```go
// pendingSave is a password just used, awaiting the answer to "save it?".
type pendingSave struct{ world, char, pw string }
```

- `Switch` sets `a.confirm = false` (every call, as the TUI's `switchTo` did, including switching to the active item).

`internal/app/session.go`, in `Handle`:
- `EventState`: after the pin handling, `if c.State != session.Connected && c.NeedPW { c.endPassword() }`.
- add `case session.EventNeedPassword: c.startPassword()`.

`internal/app/submit.go`:

```go
package app

import (
	"errors"
	"strings"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

// SubmitResult is what Submit did that the front end shows: whether
// lines went to the server, and whether any of them echoed.
type SubmitResult struct {
	Sent, Echoed bool
}

// startPassword puts up the password prompt, stashing the draft so it
// neither becomes part of the password nor is lost.
func (c *Char) startPassword() {
	if c.NeedPW {
		return
	}
	c.NeedPW, c.pwDraft, c.Text = true, c.Text, ""
	c.History.Stop()
}

// endPassword takes the prompt down (submitted, skipped, or the
// connection dropped), bringing the draft back.
func (c *Char) endPassword() {
	c.NeedPW, c.Text, c.pwDraft = false, c.pwDraft, ""
	c.History.Stop()
}

// SetInput is the active input's text, as the front end has it now.
func (a *App) SetInput(text string) {
	if c := a.chars[a.active]; c != nil {
		c.Text = text
	} else {
		a.idleText = text
	}
}

// Text is the active input's text.
func (a *App) Text() string {
	if c := a.chars[a.active]; c != nil {
		return c.Text
	}
	return a.idleText
}

// IdleHistory is the history of the input used while nothing is open.
func (a *App) IdleHistory() *History { return &a.idleHist }

// Unconfirm is an edit: the next Submit of an over-limit line asks again.
func (a *App) Unconfirm() { a.confirm = false }

// Confirming reports whether the next Submit sends an over-limit line.
func (a *App) Confirming() bool { return a.confirm }

// SkipLogin takes down the active character's password prompt, if it's
// up, and says so; false if there was none.
func (a *App) SkipLogin() bool {
	c := a.chars[a.active]
	if c == nil || !c.NeedPW {
		return false
	}
	c.endPassword()
	a.SetStatus(false, str.StatusSkippedLogin())
	return true
}

// PendingSave is the character whose just-used password awaits "save
// it?"; ok is false when nothing's asked.
func (a *App) PendingSave() (world, char string, ok bool) {
	if a.pending == nil {
		return "", "", false
	}
	return a.pending.world, a.pending.char, true
}

// AnswerSave answers "save it?", saving to password_store for the
// character that was asked about, even if another is active now.
func (a *App) AnswerSave(save bool) {
	p := a.pending
	if p == nil {
		return
	}
	a.pending = nil
	if !save {
		a.SetStatus(false, str.StatusPasswordNotSaved())
		return
	}
	store := a.PasswordStore()
	switch err := a.d.SavePassword(store, p.world, p.char, p.pw); {
	case err != nil && store == "keychain":
		a.SetStatus(true, str.StatusKeychainFailed(err))
	case err != nil:
		a.SetStatus(true, str.StatusPasswordNotSavedErr(err))
	default:
		a.SetStatus(false, str.StatusPasswordSaved())
	}
}

func isLogErr(err error) bool {
	var le *session.LogError
	return errors.As(err, &le)
}

func isCommand(text string) bool { return strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "//") }

// overLimit reports whether any line of text is longer than limit bytes,
// or with joined whether its lines joined by spaces are.
func overLimit(text string, limit int, joined bool) bool {
	if limit <= 0 {
		return false
	}
	lines := strings.Split(text, "\n")
	if joined {
		return len(strings.Join(lines, " ")) > limit
	}
	for _, l := range lines {
		if len(l) > limit {
			return true
		}
	}
	return false
}

// Submit is Enter on the active input: a command, the password at the
// prompt, a connect when there's nothing to send, or lines for the
// server. A front end handles Enter on a world's overview, and on an
// empty input with nothing open, itself.
func (a *App) Submit() (SubmitResult, []Effect) {
	c := a.chars[a.active]
	if c == nil {
		text := a.idleText
		if isCommand(text) {
			a.idleHist.Add(text)
			a.idleText = ""
			return SubmitResult{}, a.command(nil, text)
		}
		a.SetStatus(true, str.StatusNothingOpen())
		return SubmitResult{}, nil
	}
	if c.NeedPW {
		pw := c.Text
		c.Text = ""
		c.History.Stop()
		e, err := c.Sess.Login(pw)
		if err != nil && !isLogErr(err) {
			a.SetStatus(true, str.StatusLoginNotSent(err))
			return SubmitResult{}, nil
		}
		c.endPassword()
		a.Echo(c.Key, e)
		if a.d.SavePassword != nil && pw != "" && a.PasswordStore() != "none" {
			a.pending = &pendingSave{world: c.Ch.World, char: c.Ch.ID, pw: pw}
		}
		return SubmitResult{}, nil
	}
	a.ClearStatus()
	if c.Text == "" && c.State != session.Connected {
		if c.State == session.Connecting || c.Pin != nil {
			return SubmitResult{}, nil // nothing Enter can do; the prompt says why
		}
		return SubmitResult{}, a.Connect(c.Key)
	}
	text := c.Text
	if isCommand(text) {
		c.History.Add(text)
		c.Text = ""
		return SubmitResult{}, a.command(c, text)
	}
	if c.Sess == nil || c.State != session.Connected {
		a.SetStatus(true, str.StatusNotConnected(c.Ch.Name))
		return SubmitResult{}, nil
	}
	flatten := c.Ch.NewlineMode == "flatten"
	if overLimit(text, c.Ch.MaxLineBytes, flatten) && !a.confirm {
		a.confirm = true
		a.SetStatus(true, str.StatusOverLimit(c.Ch.MaxLineBytes))
		return SubmitResult{}, nil
	}
	a.confirm = false
	lines := strings.Split(strings.TrimPrefix(text, "/"), "\n") // "//foo" sends "/foo"
	if flatten {
		lines = []string{strings.Join(lines, " ")}
	}
	res := SubmitResult{Sent: true}
	secret := false
	for _, line := range lines {
		e, err := c.Sess.Send(line)
		if err != nil && !isLogErr(err) {
			a.SetStatus(true, str.StatusNotSent(err))
			break
		}
		if err != nil {
			a.SetStatus(true, err.Error())
		}
		secret = secret || e.Text != line // the session redacted a typed password
		res.Echoed = a.Echo(c.Key, e) != nil
	}
	if !secret {
		c.History.Add(text)
	}
	c.Text = ""
	c.History.Stop()
	return res, nil
}

// command runs a slash command. text is the whole input line, so commands
// that take free text (/highlight) keep its spacing. c is nil with
// nothing open.
func (a *App) command(c *Char, text string) []Effect {
	args := strings.Fields(text)
	key := ""
	if c != nil {
		key = c.Key
	}
	do := func() []Effect { return []Effect{Do{Cmd: args[0], Args: args[1:], Key: key}} }
	switch args[0] {
	case "/backup", "/restore", "/away": // need no character; the front end knows whether it has them
		return do()
	}
	if c == nil && args[0] != "/quit" && args[0] != "/open" {
		a.SetStatus(true, str.StatusNeedsCharacter(args[0]))
		return nil
	}
	switch args[0] {
	case "/connect":
		if c.Sess != nil && c.State == session.Connected {
			a.SetStatus(false, str.StatusAlreadyConnected(c.Ch.Name))
			return nil
		}
		return a.Connect(c.Key)
	case "/reconnect":
		return a.Connect(c.Key)
	case "/disconnect":
		if c.Sess != nil {
			c.Sess.Disconnect()
		}
	case "/trust":
		if c.Pin == nil {
			a.SetStatus(true, str.StatusNoChangedCert())
			return nil
		}
		if err := a.d.KnownHosts.Trust(c.Pin.HostPort, c.Pin.Got); err != nil {
			a.SetStatus(true, str.StatusTrustFailed(err))
			return nil
		}
		a.SetStatus(false, str.StatusTrusted(c.Pin.HostPort))
		c.Pin = nil
		return a.Connect(c.Key)
	case "/close":
		a.Close(c.Key)
	case "/quit":
		return a.Quit()
	case "/open", "/log", "/edit", "/notify":
		return do()
	case "/highlight":
		text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), args[0]))
		if err := config.AppendHighlight(a.d.ConfigDir, c.Ch.World, text); err != nil {
			a.SetStatus(true, str.StatusHighlightFailed(err))
			return nil
		}
		a.SetStatus(false, str.StatusHighlightAdded(text))
	default:
		a.SetStatus(true, str.StatusUnknownCommand(args[0]))
	}
	return nil
}
```

Notes, each matching the ui's `submit`/`command` on `main`:
- The TUI's `Commit` recorded `in.Value()`, the text as typed (`//foo`); `CommitSecret` recorded nothing. `submit` above does the same.
- `/backup`/`/restore` used to be "unknown" when `Deps.Backup`/`Restore` were nil, and that check ran before `/away` and before the needs-a-character check. Returning `Do` for all three first keeps that order: the TUI says "unknown" when it has no hook.
- `/open` with nothing open was allowed and opened the picker. `/quit` likewise quits.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/app && go vet ./... && go test ./...`
Expected: PASS. (`ui` doesn't use these yet, so it's unaffected.)

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -m "app: Enter, commands, the password prompt and the save question live in the core

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```

---

### Task 3: The TUI submits through the core

**Files:**
- Modify: `internal/ui/model.go`, `internal/ui/sidebar.go`, `internal/ui/view.go`, `internal/ui/picker.go`, `internal/ui/notify.go` (if it reads `needPW`)
- Modify tests (access/setup): `internal/ui/*_test.go`

**Interfaces:**
- Consumes: Task 2's API.
- Produces: `(*Model).pushInput()`, `(*Model).pullInput(cs *charState)`, `(*Model).pullIdle()`, `(*Model).do(app.Do) tea.Cmd`, `(*Model).syncLines(cs *charState)`; `Model` loses `mode`, `pendingPW`, `pendingCh`, `confirm`; `charState` loses `needPW`, `pwDraft`, `startPassword`, `endPassword`.

- [ ] **Step 1: Wire the deps and histories**

`New`: `app.Deps` also gets `ConfigDir: d.ConfigDir, KnownHosts: d.KnownHosts, SavePassword: d.SavePassword`. `m.idle.hist = m.a.IdleHistory()`. In `open`: `cs.in.hist = &c.History`.

- [ ] **Step 2: Keeping widget and core in step**

```go
// pushInput tells the core the active input's text, as the widget has it.
func (m *Model) pushInput() { m.a.SetInput(m.input().Value()) }

// pullInput puts cs's core text in its widget, if the core changed it.
func (m *Model) pullInput(cs *charState) {
	if cs.in.Value() != cs.Text {
		cs.in.Reset()
		cs.in.SetValue(cs.Text)
	}
}

// pullIdle is pullInput for the input used while nothing is open.
func (m *Model) pullIdle() {
	if m.cur() == nil && m.idle.Value() != m.a.Text() {
		m.idle.Reset()
		m.idle.SetValue(m.a.Text())
	}
}

// syncLines shows lines the core has added to cs's scrollback since the
// view last caught up (what you sent, echoed).
func (m *Model) syncLines(cs *charState) {
	for _, l := range cs.Lines[cs.sb.Len():] {
		cs.sb.AppendLine(lineOf(paint(cs.hl, *l), l))
	}
}
```

- `update`, `tea.KeyPressMsg` case: after `cmd := m.handleKey(msg)`, call `m.pushInput()` (before the status-dismiss check, which still compares widget values). The `tea.PasteMsg` branch that inserts into `m.input()`: replace `m.confirm = false` with `m.a.Unconfirm()`, then `m.pushInput()`.
- `handleEvent`: after `ev, ok, effs := m.a.Handle(msg)` and the `!ok`/`Closed` returns, `m.pullInput(cs)` (the prompt may have started or ended). Delete the `EventState`/`EventNeedPassword` password lines (the core does it).

- [ ] **Step 3: Enter, Esc, the save question, and commands**

`submit`:

```go
// submit handles Enter: on a world's overview, or an empty input with
// nothing open, the picker; otherwise the core's Submit.
func (m *Model) submit() tea.Cmd {
	if m.overviewing() {
		m.openPicker() // as on an empty input with nothing open
		return nil
	}
	m.pushInput()
	if m.cur() == nil && m.a.Text() == "" {
		m.openPicker()
		return nil
	}
	prev := m.a.Active()
	res, effs := m.a.Submit()
	cmd := m.run(effs)
	m.afterCore(prev)
	if cs := m.cur(); cs != nil && res.Sent {
		cs.sb.ToBottomKeeping(res.Echoed) // what you send is where the pager picks up
	}
	return cmd
}

// afterCore catches the screen up with a core call that may have closed
// characters, moved the active item, added lines or changed texts.
func (m *Model) afterCore(prev string) {
	for k := range m.chars {
		if m.a.Char(k) == nil {
			m.closed(k, prev)
		}
	}
	m.activated(prev)
	if cs := m.cur(); cs != nil {
		m.syncLines(cs)
		m.pullInput(cs)
	}
	m.pullIdle()
}
```

`run` handles `app.Do` inline (it runs on the loop): `case app.Do: cmds = append(cmds, m.do(e))`. `do`:

```go
// do carries out a command the core hands back.
func (m *Model) do(d app.Do) tea.Cmd {
	cs := m.chars[d.Key]
	switch d.Cmd {
	case "/backup", "/restore":
		if (d.Cmd == "/backup" && m.d.Backup == nil) || (d.Cmd == "/restore" && m.d.Restore == nil) {
			m.setStatus(true, str.StatusUnknownCommand(d.Cmd)) // web-only, so unknown here
			return nil
		}
		if d.Cmd == "/backup" {
			return m.backupCmd()
		}
		return m.restoreCmd()
	case "/away":
		m.awayNow = true
		m.setStatus(false, str.StatusAway())
	case "/open":
		m.openPicker() // says why not
	case "/log":
		m.openBrowse(cs)
	case "/edit":
		m.editCommand(cs, strings.Join(d.Args, " "))
	case "/notify":
		m.notifyCommand(cs, d.Args)
	}
	return nil
}
```

Delete `command`, `isLogErr`, `startPassword`, `endPassword`.

`handleKey`:
- The `modeSavePassword` block becomes:

```go
	if _, _, asking := m.a.PendingSave(); asking {
		switch k.String() {
		case "enter", "y", "Y":
			m.a.AnswerSave(true)
		case "n", "N", "esc", "ctrl+c":
			m.a.AnswerSave(false)
		case openPickerKey:
			m.openPicker() // says why not
		}
		return nil
	}
```

- `esc`: `m.a.Unconfirm()`; `if !m.a.SkipLogin() && cs != nil && cs.sb.Scrolled() { cs.sb.ToBottom() }`; then `if cs != nil { m.pullInput(cs) }`.
- The tail where an edit key, `shift+enter`, `up` or `down` handled the input: replace `m.confirm = false` with `m.a.Unconfirm()`.
- `ctrl+c` with text keeps `m.input().Reset()` (the widget clears and stops browsing the core's history; `pushInput` after `handleKey` tells the core).

`switchTo` and `activated`: delete their `m.confirm = false` (the core's `Switch` does it, and `Close`'s switch with it).

Readers of the moved state:
- `m.mode == modeSavePassword` / `m.mode == modeNormal` (`picker.go`, `view.go` `prompt` and `overviewing`) → `_, _, asking := m.a.PendingSave()` / `!asking`.
- `view.go`'s save prompt reads `world, char, _ := m.a.PendingSave()` in place of `m.pendingCh[0]`, `[1]`.
- `cs.needPW` → `cs.NeedPW` everywhere (`view.go`, `model.go`).
- Delete `mode`, `modeNormal`, `modeSavePassword`, `pendingPW`, `pendingCh`, `confirm` from `Model`.

Run: `go build ./... && go vet ./internal/ui`
Expected: the build passes; vet names only test files.

- [ ] **Step 4: Tests' access paths and setup**

| Old (tests) | New |
|---|---|
| `.needPW` | `.NeedPW` |
| `h.m.mode != modeSavePassword` | `_, _, ok := h.m.a.PendingSave(); !ok` (a small helper `asking := func() bool { _, _, ok := h.m.a.PendingSave(); return ok }` in the tests that use it) |
| `h.m.mode != modeNormal` | `asking()` |
| `in.history` / `in.hist` (int) | `in.hist.Lines()` / `in.hist.Pos()` (done in Task 1) |
| `TestClosingResetsTheOverLimitConfirm` sets `h.m.confirm = true` | set it up for real: give Kit `max_line_bytes = 5` via its world, connect Kit (`h.init(); h.settle("fm/kit", h.connected("fm/kit"))`), open Rook, type "too long" and press Enter (the warning); then `h.m.close("fm/kit")` and assert `!h.m.a.Confirming()` as well as the active character |

Run: `go vet ./... && go test ./...`
Expected: PASS everywhere, goldens untouched. If a test fails, compare with `git show main:internal/ui/model.go`; don't change an expectation.

- [ ] **Step 5: Other builds**

Run: `cd web && GOOS=js GOARCH=wasm go build -o /dev/null ./cmd/kiln-web && go test ./... && cd .. && go build -o /dev/null ./cmd/kiln`
Expected: all succeed.

- [ ] **Step 6: Commit**

```bash
git add internal/ui
git commit -m "ui: Enter, Esc and the save question go through the core

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```
