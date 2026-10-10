# Headless core, step 3: the core owns presence and notifications — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Whether you're here or away, `/away`, `/notify`, and whether an incoming line notifies live in `app`. The TUI reports focus and activity, draws the presence chip from `app.Presence()`, and turns a `Notify` effect into OSC 9 or a bell.

**Architecture:**
- **Presence:** `App` keeps focus, whether a focus event has been seen, when you were last here, `/away`, and the "here" generation that re-arms `first`. The TUI calls `Focus(bool)` for focus events and `Here()` for input, and reads `Presence()` / `Away()`.
- **Notifications:** `Handle` decides, for each incoming line, whether it notifies (away, level, grace after connecting, bursts) and returns a `Notify{Title, Body}` effect. The TUI's `run` performs it with `notify.Message` + `notify.Encode`.
- **Commands:** `/away` and `/notify` run in the core and stop being `Do` effects.
- Task 1 adds all of this to `app`, unused by the TUI (the TUI ignores effect types it doesn't know). Task 2 switches the TUI over and deletes its copies.

**Tech Stack:** Go; `internal/app`, `internal/ui`, `internal/notify`, `config`, `str`.

**Spec:** `docs/superpowers/specs/2026-10-07-headless-core-design.md`, section "Presence and notifications", and the effects table under "Shape".

## Global Constraints

- Nothing visible changes: golden screens are NOT updated (never `-update`).
- Every existing test passes. `ui` tests may change how they reach state (`h.m.a.Presence()`, `app.ConnectGrace`), never what they expect.
- `internal/app` doesn't depend on Bubble Tea or lipgloss and never paints (`TestNoTerminal`). Importing `internal/notify` is fine: it has no terminal dependencies.
- Strings come from `internal/str`; this step adds none (`StatusAway`, `NotifyLevel`, `NotifyLevelOverride`, `NotifyLevelUntilQuit` move with their code).
- Only `App` changes `app.Char` fields.
- Run `go vet ./... && go test ./...` before each commit.

## Rulings carried from the spec

- **`Notify.Body` is the line's raw text** (server colors and all). Cleaning it (`notify.Message`: strip ANSI and controls, `name: text`, truncate) is the front end's, since it's about the medium. Part 2 can reuse `notify.Message` or its own.
- **The presence chip's *shown* state stays in `ui`** (`shownPresence`): not repainting on blur is about tmux flagging windows, not about presence.
- **`ConnectGrace` and `BurstGap` are exported** so the TUI tests that step past them keep naming them instead of copying the numbers.

## What the old code did as a side effect (keep all of it)

- `here()`: sets `lastHere` to now, clears `/away`, bumps `hereGen` (re-arms `first` for *every* character).
- Key, paste and click set `focused = true` before `here()` (input implies focus, even if the focus-in was lost), but do **not** set `focusSeen`. Only focus-in and blur set `focusSeen`.
- A wheel event counts as here **only if already focused** (macOS scrolls background windows).
- Focus-in also asks for the background color (`askBackground`): stays in `ui`.
- A click captures `shownPresence` *before* `here()`, so clicking the chip while Away ends Away instead of re-setting it.
- `notifyCmd` updates `sentGen`/`lastSent` **only when it notifies**.
- A new character starts with `sentGen = -1`, so its first line under `first` notifies even though `hereGen` is 0. (On `main` this is set in `ui/sidebar.go`'s `charState` constructor.)
- `/notify` overrides are keyed by character key on the model, not the character: they survive closing and reopening the character, and config reloads, until Kiln quits.
- `/away` works with nothing open; `/notify` needs a character.
- `/away` and clicking the chip both say `str.StatusAway()`.

## Review Focus

1. **`first` on a new character.** `sentGen` must start at -1 in `App.Open`, or a fresh character's first line while away never notifies (`hereGen` starts at 0). Task 1's `TestNotifyFirstReArmsOnHere` pins it.
2. **Wheel while unfocused** must not count as here (`TestScrollWhileUnfocused…` in `ui/notify_test.go` covers it; the TUI must check `m.a.Focused()` before `m.a.Here()`).
3. **Input implies focus without `focusSeen`.** After typing with no focus event ever, presence is still Unknown, not Here. Task 1's `TestPresenceStates` pins it in `app`.
4. **`/notify` overrides outlive closing the character.** Task 2's `TestNotifyOverrideOutlivesClose` pins it.
5. **No double notification and no lost one.** After Task 2, the only notify path is `app.Notify` through `run`; `handleEvent` must not also call the old code, and a line that isn't shown (`ev.Shown` false) still notifies as before. Existing `ui/notify_test.go` covers both.

---

### Task 1: presence and the notification decision in `app`

**Files:**
- Create: `internal/app/presence.go`, `internal/app/presence_test.go`
- Modify: `internal/app/app.go` (fields, `New`, `Open`, `Char`), `internal/app/effect.go` (`Notify`), `internal/app/session.go` (`Handle`)

**Interfaces:**
- Produces:
  - `type app.Presence int` with `PresenceUnknown`, `PresenceHere`, `PresenceAway`
  - `func (a *App) Here()`, `Focus(in bool)`, `Focused() bool`, `Away() bool`, `Presence() Presence`, `SetAway()`
  - `func (a *App) NotifyLevel(k string) notify.Level` (`""` if k isn't open)
  - `const app.ConnectGrace = 5 * time.Second`, `const app.BurstGap = 250 * time.Millisecond`
  - `type app.Notify struct{ Title, Body string }` (an `Effect`)
  - unexported, for Task 2: `func (a *App) notifyCommand(c *Char, args []string)`
  - `Handle` appends a `Notify` after the wait effect when an incoming line notifies.

- [ ] **Step 1: Write the tests** — `internal/app/presence_test.go`:

```go
package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/str"
)

// clockApp is sessionApp with a clock the test moves.
func clockApp(t *testing.T, worlds map[string]string) (*App, *time.Time) {
	t.Helper()
	a := sessionApp(t, worlds)
	clk := now
	a.d.Now = func() time.Time { return clk }
	a.lastHere = clk
	return a, &clk
}

func withLevel(world, level string) string {
	return strings.Replace(world, "login = \"connect {name} {password}\"\n", "login = \"connect {name} {password}\"\nnotify = \""+level+"\"\n", 1)
}

func notes(effs []Effect) []Notify {
	var n []Notify
	for _, e := range effs {
		if x, ok := e.(Notify); ok {
			n = append(n, x)
		}
	}
	return n
}

func TestPresenceStates(t *testing.T) {
	a, clk := clockApp(t, map[string]string{"fm": fmWorld})
	if a.Presence() != PresenceUnknown {
		t.Errorf("at start: %v", a.Presence())
	}
	a.Here() // input with no focus event yet: still can't tell
	if a.Presence() != PresenceUnknown {
		t.Errorf("typing before any focus event: %v", a.Presence())
	}
	a.Focus(true)
	if a.Presence() != PresenceHere {
		t.Errorf("after focus-in: %v", a.Presence())
	}
	a.Focus(false)
	if a.Presence() != PresenceAway || a.Focused() {
		t.Errorf("after blur: %v, focused %v", a.Presence(), a.Focused())
	}
	a.Here()
	if a.Presence() != PresenceHere || !a.Focused() {
		t.Errorf("input after a blur: %v", a.Presence())
	}
	a.SetAway()
	if a.Presence() != PresenceAway || a.Status().Text != str.StatusAway() {
		t.Errorf("/away: %v, status %q", a.Presence(), a.Status().Text)
	}
	a.Here()
	if a.Away() {
		t.Error("input didn't end /away")
	}
	*clk = clk.Add(a.Config().NotifyIdle + time.Second)
	if a.Presence() != PresenceAway {
		t.Errorf("idle past notify_idle: %v", a.Presence())
	}
}

// A new character's first line while away notifies under "first"
// (hereGen starts at 0, so sentGen must start below it); the next
// doesn't, until you've been here again.
func TestNotifyFirstReArmsOnHere(t *testing.T) {
	a, clk := clockApp(t, map[string]string{"fm": withLevel(fmWorld, "first")})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	a.Focus(false)
	line := func(text string) []Notify {
		*clk = clk.Add(time.Second) // past the burst gap
		_, _, effs := a.Handle(lineMsg("fm/kit", s, text))
		return notes(effs)
	}
	if got := line("one"); !slices.Equal(got, []Notify{{Title: "Kit", Body: "one"}}) {
		t.Errorf("first line: %v", got)
	}
	if got := line("two"); got != nil {
		t.Errorf("second line: %v", got)
	}
	a.Here()
	a.Focus(false)
	if got := line("three"); len(got) != 1 {
		t.Errorf("after being here again: %v", got)
	}
}

func TestNotifyGraceAndBursts(t *testing.T) {
	a, clk := clockApp(t, map[string]string{"fm": withLevel(fmWorld, "all")})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	a.Char("fm/kit").ConnectedAt = *clk
	a.Focus(false)
	line := func(text string) int {
		_, _, effs := a.Handle(lineMsg("fm/kit", s, text))
		return len(notes(effs))
	}
	if n := line("banner"); n != 0 {
		t.Error("notified within the connect grace")
	}
	*clk = clk.Add(ConnectGrace)
	if n := line("hi"); n != 1 {
		t.Error("didn't notify past the grace")
	}
	if n := line("more of it"); n != 0 {
		t.Error("notified within a burst")
	}
	*clk = clk.Add(BurstGap)
	if n := line("again"); n != 1 {
		t.Error("didn't notify after the burst gap")
	}
}

func TestNoNotifyWhileHere(t *testing.T) {
	a, clk := clockApp(t, map[string]string{"fm": withLevel(fmWorld, "all")})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	a.Focus(true)
	*clk = clk.Add(time.Second)
	if _, _, effs := a.Handle(lineMsg("fm/kit", s, "hi")); notes(effs) != nil {
		t.Errorf("notified while here: %v", notes(effs))
	}
}

// The title says which world when another world has a character of the
// same name.
func TestNotifyTitleNamesTheWorldWhenAmbiguous(t *testing.T) {
	kitToo := strings.Replace(zzWorld, "name = \"Ash\"", "name = \"Kit\"", 1)
	a, clk := clockApp(t, map[string]string{"fm": withLevel(fmWorld, "all"), "zz": kitToo})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	a.Focus(false)
	*clk = clk.Add(time.Second)
	_, _, effs := a.Handle(lineMsg("fm/kit", s, "hi"))
	if got := notes(effs); len(got) != 1 || got[0].Title != "Kit@fm" {
		t.Errorf("got %v, want title Kit@fm", got)
	}
}
```

- [ ] **Step 2: Run them; they fail to compile**

Run: `go test ./internal/app/`
Expected: FAIL — `undefined: PresenceUnknown`, `a.Here undefined`, etc.

- [ ] **Step 3: Add the state to `App` and `Char`**

In `internal/app/app.go`, add to the `App` struct:

```go
	focused         bool                    // the front end has focus, as far as we know
	focusSeen       bool                    // a focus-in or focus-out has arrived, so the front end reports focus
	lastHere        time.Time               // latest focus-in or input; see Here
	awayNow         bool                    // set by /away until the next Here
	hereGen         int                     // bumped by each Here; re-arms "first"
	notifyOverrides map[string]notify.Level // from /notify, by character key, until Kiln quits
```

and to `Char` (unexported, after `rulesGen`):

```go
	sentGen     int       // App.hereGen when the last notification went out; -1: none yet
	lastSent    time.Time // when the last notification went out
```

In `New`, after the `d.Now` default:

```go
	return &App{d: d, chars: map[string]*Char{}, focused: true, lastHere: d.Now(),
		notifyOverrides: map[string]notify.Level{}}
```

In `Open`, `c := &Char{Key: k, Ch: ch, sentGen: -1}`.

- [ ] **Step 4: Write `internal/app/presence.go`**

```go
package app

import (
	"strings"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/str"
)

// ConnectGrace is how long after connecting only attention lines
// notify, so the login banner doesn't.
const ConnectGrace = 5 * time.Second

// BurstGap is how soon after a notification another line counts as part
// of the same burst (a multi-line description) and doesn't notify.
const BurstGap = 250 * time.Millisecond

// Presence is whether you're here, away, or Kiln can't tell.
type Presence int

const (
	PresenceUnknown Presence = iota // not away, and no focus event has ever arrived
	PresenceHere                    // not away, and the front end reports focus
	PresenceAway
)

// Here records that you're at the keyboard: a key, paste or click, or a
// wheel while focused. Input implies focus, even if the focus-in was
// lost. It ends /away and re-arms "first" for every character.
func (a *App) Here() {
	a.focused = true
	a.lastHere, a.awayNow = a.d.Now(), false
	a.hereGen++
}

// Focus reports the front end gaining (in) or losing focus. Gaining it
// counts as Here.
func (a *App) Focus(in bool) {
	a.focusSeen = true
	if in {
		a.Here()
	} else {
		a.focused = false
	}
}

// Focused reports whether the front end has focus, as far as Kiln knows.
func (a *App) Focused() bool { return a.focused }

// SetAway is /away: away until you're next Here. It says so.
func (a *App) SetAway() {
	a.awayNow = true
	a.SetStatus(false, str.StatusAway())
}

// notifyIdle is the notify_idle setting.
func (a *App) notifyIdle() time.Duration {
	if a.cfg != nil {
		return a.cfg.NotifyIdle
	}
	return config.DefaultNotifyIdle
}

// Away reports whether you've switched away, said /away, or been idle
// past notify_idle.
func (a *App) Away() bool {
	idle := a.notifyIdle()
	return !a.focused || a.awayNow || idle > 0 && a.d.Now().Sub(a.lastHere) > idle
}

// Presence is whether you're away, here, or Kiln can't tell: a terminal
// only reports focus when it changes, so until the first event one that
// reports it looks the same as one that never will.
func (a *App) Presence() Presence {
	switch {
	case a.Away():
		return PresenceAway
	case !a.focusSeen:
		return PresenceUnknown
	}
	return PresenceHere
}

// NotifyLevel is open character k's /notify override, or its configured
// level; "" if k isn't open.
func (a *App) NotifyLevel(k string) notify.Level {
	c := a.chars[k]
	if c == nil {
		return ""
	}
	if l := a.notifyOverrides[k]; l != "" {
		return l
	}
	return c.Ch.Notify
}

// notifyName is c's name, with its world when another world has a
// character of the same name.
func (a *App) notifyName(c *Char) string {
	for _, ch := range a.AllChars() {
		if ch.World != c.Ch.World && strings.EqualFold(ch.Name, c.Ch.Name) {
			return c.Ch.Name + "@" + c.Ch.World
		}
	}
	return c.Ch.Name
}

// notifyFor is the notification for an incoming line of c's, if you're
// away and c's level wants one. Only what arrives while you're away
// notifies: what came while you were here, you saw.
func (a *App) notifyFor(c *Char, l Line) []Effect {
	e := l.Entry
	if e.Dir != logstore.In || l.Quiet || !a.Away() {
		return nil
	}
	now := a.d.Now()
	if !l.Attention && (now.Sub(c.ConnectedAt) < ConnectGrace || now.Sub(c.lastSent) < BurstGap) {
		return nil
	}
	switch a.NotifyLevel(c.Key) {
	case notify.All:
	case notify.First:
		if !l.Attention && c.sentGen == a.hereGen {
			return nil
		}
	case notify.Attention:
		if !l.Attention {
			return nil
		}
	default:
		return nil
	}
	c.sentGen, c.lastSent = a.hereGen, now
	return []Effect{Notify{Title: a.notifyName(c), Body: e.Text}}
}

// notifyCommand is /notify: show the level, set an override until Kiln
// quits, or drop it with "default".
func (a *App) notifyCommand(c *Char, args []string) {
	if len(args) == 0 {
		shown := string(a.NotifyLevel(c.Key))
		if l := a.notifyOverrides[c.Key]; l != "" {
			shown = str.NotifyLevelOverride(l, c.Ch.Notify)
		}
		a.SetStatus(false, str.NotifyLevel(shown))
		return
	}
	if args[0] == "default" {
		delete(a.notifyOverrides, c.Key)
		a.SetStatus(false, str.NotifyLevel(c.Ch.Notify))
		return
	}
	l, err := notify.ParseLevel(args[0])
	if err != nil {
		a.SetStatus(true, err.Error())
		return
	}
	a.notifyOverrides[c.Key] = l
	a.SetStatus(false, str.NotifyLevelUntilQuit(c.Ch.Name, l))
}
```

Check the existing `str` signatures before relying on them (`grep -n "func NotifyLevel\|func StatusAway" internal/str/*.go`); the calls above are copied from `ui/notify.go` and `ui/model.go`, so they match.

- [ ] **Step 5: The `Notify` effect** — in `internal/app/effect.go`, add:

```go
// Notify is a notification to show. Title is the character's name (with
// @world when needed); Body is the line's text as it arrived, server
// colors and all, for the front end to clean up for its medium.
type Notify struct{ Title, Body string }
```

and `func (Notify) effect() {}` with the others.

- [ ] **Step 6: `Handle` decides** — in `internal/app/session.go`, `Handle`: declare `var note []Effect` before the `switch`; at the end of the `session.EventLine` case (after the unread count) add `note = a.notifyFor(c, *ev.Line)`; and make the final return:

```go
	return ev, true, append([]Effect{wait(msg.Key, msg.Sess)}, note...)
```

- [ ] **Step 7: Run the tests**

Run: `go vet ./... && go test ./internal/app/ ./internal/ui/`
Expected: PASS. (The TUI's `run` ignores `Notify` for now, and its own notify path still runs, so `ui` tests are unaffected.)

- [ ] **Step 8: Commit**

```bash
git add internal/app/
git commit -m "app: presence and the notification decision"
```

---

### Task 2: the TUI reports presence and performs `Notify`; `/away` and `/notify` run in the core

**Files:**
- Modify: `internal/app/submit.go` (`command`), `internal/app/effect.go` (`Do` doc), `internal/app/submit_test.go`, `internal/app/presence_test.go`
- Modify: `internal/ui/model.go`, `internal/ui/notify.go`, `internal/ui/view.go`, `internal/ui/sidebar.go`
- Modify (access paths only): `internal/ui/notify_test.go`, `internal/ui/pager_test.go`, `internal/ui/topbar_test.go`, and any other hit of the grep in Step 6

**Interfaces:**
- Consumes: everything Task 1 produces.
- Produces: `Do` is no longer returned for `/away` or `/notify`.

- [ ] **Step 1: Write the core tests** — append to `internal/app/presence_test.go`:

```go
func TestAwayAndNotifyCommands(t *testing.T) {
	a, _ := clockApp(t, map[string]string{"fm": fmWorld})
	a.SetInput("/away") // with nothing open
	if _, effs := a.Submit(); effs != nil || !a.Away() || a.Status().Text != str.StatusAway() {
		t.Errorf("/away with nothing open: effects %v, away %v, status %q", effs, a.Away(), a.Status().Text)
	}
	openAll(t, a, "fm/kit")
	a.SetInput("/notify none")
	if _, effs := a.Submit(); effs != nil || a.NotifyLevel("fm/kit") != "none" {
		t.Errorf("/notify none: effects %v, level %q", effs, a.NotifyLevel("fm/kit"))
	}
	a.SetInput("/notify bogus")
	if a.Submit(); !a.Status().Err {
		t.Error("/notify bogus wasn't an error")
	}
}

func TestNotifyOverrideOutlivesClose(t *testing.T) {
	a, _ := clockApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	a.SetInput("/notify all")
	a.Submit()
	a.Close("fm/kit")
	openAll(t, a, "fm/kit")
	if l := a.NotifyLevel("fm/kit"); l != "all" {
		t.Errorf("after reopening: %q, want the override", l)
	}
}
```

In `internal/app/submit_test.go`'s `TestCommandsInTheCore`, drop `{"/notify all", "/notify"}` and `{"/away", "/away"}` from the table: they're no longer `Do`s (the test above covers them).

Run: `go test ./internal/app/` — Expected: FAIL (`/away` and `/notify` still return `Do`).

- [ ] **Step 2: Run them in the core** — in `internal/app/submit.go`, `command`:

```go
	switch args[0] {
	case "/backup", "/restore": // need no character; the front end knows whether it has them
		return do()
	case "/away":
		a.SetAway()
		return nil
	}
```

and in the second switch, `case "/open", "/log", "/edit":` returns `do()`, plus:

```go
	case "/notify":
		a.notifyCommand(c, args[1:])
```

Update `Do`'s doc in `effect.go`: "one about its own screens (/open, /log, /edit), or one the core doesn't handle yet (/backup, /restore)".

Run: `go test ./internal/app/` — Expected: PASS.

- [ ] **Step 3: The TUI reports presence** — in `internal/ui/model.go`:
  - Delete the `Model` fields `focused`, `focusSeen`, `lastHere`, `awayNow`, `hereGen`, `notifyOverrides`; change `shownPresence` to `app.Presence`.
  - Delete `sentGen` and `lastSent` from `charState`; in `internal/ui/sidebar.go` the constructor becomes `&charState{Char: c, in: NewInput()}`.
  - `New`: delete `m.focused, m.lastHere = true, d.Now()` and `m.notifyOverrides = …`; `m.shownPresence = m.a.Presence()`.
  - `Update`: `m.shownPresence = m.a.Presence()`.
  - `update`:
    - `tea.FocusMsg`: `m.a.Focus(true)` then `return m, m.askBackground()`.
    - `tea.BlurMsg`: `m.a.Focus(false)`.
    - `tea.PasteMsg` and `tea.KeyPressMsg`: replace `m.focused = true; m.here()` with `m.a.Here()`.
    - `tea.MouseWheelMsg`: `if m.a.Focused() { m.a.Here() }` (keep the macOS comment).
    - `tea.MouseClickMsg`: `was := m.shownPresence` (keep its comment), then `m.a.Here()`.
  - `do`: delete the `/away` and `/notify` cases.
  - `run`: add

    ```go
		case app.Notify:
			cmds = append(cmds, m.encode(notify.Message(e.Title, e.Body)))
    ```
  - `handleEvent`: delete the `if n := m.notifyCmd(cs, *ev.Line); n != nil { … }` block; the notification now arrives in `effs` and `m.run(effs)` performs it.

- [ ] **Step 4: Slim `internal/ui/notify.go`** to the terminal's half: keep `encode` (it reads `NotifyMethod` and `d.Tmux`), delete `connectGrace`, `burstGap`, `notifyLevel`, `here`, `notifyIdle`, `presenceState` and its constants, `presence`, `away`, `notifyName`, `notifyCmd`, `notifyCommand`. Fix imports (`tea`, `notify` stay).

- [ ] **Step 5: The chip** — in `internal/ui/view.go`:
  - `topBarFor(w int, p app.Presence)`; its switch uses `app.PresenceHere` / `app.PresenceAway`.
  - `topClick(x int, was app.Presence)`; the chip case becomes `if was != app.PresenceAway { m.a.SetAway() }`.
  - `handleClick(msg tea.MouseClickMsg, was app.Presence)` in `model.go` likewise.

- [ ] **Step 6: Tests reach the core** — find every hit:

```bash
grep -rnE 'presence(State|Unknown|Here|Away)|\.presence\(\)|\.away\(\)|notifyLevel|connectGrace|burstGap|\.focused|hereGen|awayNow' internal/ui
```

Change access paths only: `m.presence()` → `m.a.Presence()`, `presenceX` → `app.PresenceX`, `h.m.away()` → `h.m.a.Away()`, `h.m.notifyLevel(h.m.chars["fm/kit"])` → `h.m.a.NotifyLevel("fm/kit")`, `connectGrace` → `app.ConnectGrace`, `burstGap` → `app.BurstGap`. Add the `app` import where needed. No expected value changes.

- [ ] **Step 7: Run everything**

Run: `go vet ./... && go test ./...` and `go test ./internal/ui -run Golden` (no `-update`).
Expected: PASS, golden screens untouched (`git status internal/ui/testdata` clean).

- [ ] **Step 8: Commit**

```bash
git add internal/app internal/ui
git commit -m "ui: presence and notifications go through the core"
```

Then: fresh-reviewer pass over the branch against `main` with this plan's Review Focus and side-effect list, fix Critical/Important with a failing test first, open the PR (no `Strings:` trailer: no catalog changes) and stop.
