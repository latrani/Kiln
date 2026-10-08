# Headless core, step 2a: the core owns characters — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `app.App` owns which characters are open, their sessions, the sidebar order, switching, unread and attention, and the status message; `ui.Model` drives it and keeps only what's about the screen.

**Architecture:** `app.App` is a plain struct run on the front end's loop. Actions are methods; session events come back through `Handle`; side effects go out as `[]app.Effect` (`Run` for blocking waits, `Quit`). `ui.charState` embeds `*app.Char` and keeps the view half (scrollback, input, log mode, highlighter, password prompt). Every core call that can open, close or switch is followed by the ui's own part (`closed`, `activated`) so editors, scroll positions and parked editors behave exactly as before.

**Tech Stack:** Go; `internal/app` (no Bubble Tea), `internal/ui` (Bubble Tea v2), `session`, `config`, `conn`, `logstore`, `str`.

**Spec:** `docs/superpowers/specs/2026-10-07-headless-core-design.md` — "Shape", "What moves" → "Characters and the sidebar", "Status", and "Order of work" step 2.

## Scope ruling (from the spec's step 2)

The spec's step 2 is one PR. It's split into three, each leaving Kiln working:
- **2a (this plan):** characters, sessions, sidebar order, switching, unread/attention, status.
- **2b:** scrollback data into `app` (lines, paging, unseen), and slimming `app.Line`'s memory.
- **2c:** input, history and drafts, submit, slash commands, the password prompt and save question.

Until 2b/2c, the scrollback, input, password prompt, `confirm` and notifications stay in `ui`.

## Global Constraints

- Nothing visible changes: golden screens in `internal/ui/testdata/golden` are NOT updated (never `-update`).
- Every existing test passes. Tests may change how they reach state (`h.m.a.Active()` instead of `h.m.active`, `.Unread` instead of `.unread`, writing logs where the harness's log root already is), never what they expect.
- `internal/app` doesn't depend on Bubble Tea or lipgloss and never paints (`TestNoTerminal`).
- `app.Char`'s exported fields are for front ends to read; only `App` changes them (doc comment says so).
- User-facing strings come from `internal/str`; this step adds none.
- `ui.Deps` keeps its fields, so `cmd/kiln` and `web/cmd/kiln-web` don't change.
- Run `go vet ./... && go test ./...` before each commit.

## Review Focus

1. **Closing the active character while an editor is open on it** — its edits wait as a draft, and an editor parked on the character that becomes active shows again (existing editor tests cover this; they must pass unchanged).
2. **A config reload that removes characters** — a disconnected one closes, a connected one stays as an orphan until it disconnects, then closes; the active one moves to the next character down. Task 1 tests the core half; existing ui tests the rest.
3. **Lines for a character you're not looking at** — count as unread (quiet ones don't), mark attention, and clear when you switch to it. Task 2 tests it.
4. **A changed certificate** — `Failed` with a pin mismatch sets the pin and the error status; the next `Connected` clears the pin. Task 2 tests it.
5. **Status timing** — a new status times out after 30s, typing dismisses only a status that was there before the key, and the quit hint disarms. Existing tests cover these; they must pass unchanged.

---

### Task 1: `app.App` — open, close, switch, status, config

**Files:**
- Create: `internal/app/app.go`
- Create: `internal/app/app_test.go`

**Interfaces:**
- Produces:
  - `type app.Deps struct { LogRoot string; Dial func(ctx context.Context, ch config.Character) (session.LineConn, error); NewLog func(l logstore.Layout) session.Appender; Password func(store, world, char string) (string, error); Now func() time.Time }`
  - `type app.Char struct { Key string; Ch config.Character; Sess *session.Session; State session.State; Rules Rules; Unread int; Attention bool; Pin *conn.PinMismatchError; Orphan bool; ConnectedAt time.Time; cancel context.CancelFunc }`
  - `type app.Status struct { Text string; Err bool; Gen int }`
  - `type app.ConfigResult struct { Closed []string; Errs map[string]error }`
  - `func app.Key(world, char string) string`, `func app.WorldSel(world string) string`, `func app.CompareChars(a, b config.Character) int`
  - `func app.New(d app.Deps) *app.App`
  - `(*App)`: `ApplyConfig(cfg *config.Config) ConfigResult`, `Config() *config.Config`, `Find(k string) (config.Character, bool)`, `AllChars() []config.Character`, `Open(k string) (*Char, error)`, `Close(k string)`, `Char(k string) *Char`, `Order() []string`, `Active() string`, `ActiveWorld() (string, bool)`, `WorldOpen(world string) bool`, `CanSwitch(k string) bool`, `Switch(k string) bool`, `Stops() []string`, `StepTarget(delta int) string`, `UnreadTarget(dir int) string`, `SetStatus(isErr bool, text string)`, `ClearStatus()`, `Status() Status`, `PasswordStore() string`, `LogLayout(ch config.Character) (logstore.Layout, bool)`

- [ ] **Step 1: Write the tests**

`internal/app/app_test.go`:

```go
package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/latrani/Kiln/internal/config"
)

const fmWorld = `host = "muck.test"
port = 8888
tls = true
use = ["fuzzball"]
login = "connect {name} {password}"

[[characters]]
id = "kit"
name = "Kit"

[[characters]]
id = "rook"
name = "Rook"
`

const zzWorld = `host = "zz.test"
port = 7777
tls = true

[[characters]]
id = "ash"
name = "Ash"
`

// configDir writes worlds into a fresh config dir and returns it.
func configDir(t *testing.T, worlds map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	writeWorlds(t, dir, worlds)
	return dir
}

func writeWorlds(t *testing.T, dir string, worlds map[string]string) {
	t.Helper()
	for name, body := range worlds {
		if err := os.WriteFile(filepath.Join(dir, "worlds", name+".toml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func load(t *testing.T, dir string) *config.Config {
	t.Helper()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// testApp is an App over fm (Kit, Rook) and zz (Ash), nothing open.
func testApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := configDir(t, map[string]string{"fm": fmWorld, "zz": zzWorld})
	a := New(Deps{LogRoot: filepath.Join(dir, "logs")})
	a.ApplyConfig(load(t, dir))
	return a, dir
}

func openAll(t *testing.T, a *App, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if c, err := a.Open(k); c == nil || err != nil {
			t.Fatalf("Open(%s) = %v, %v", k, c, err)
		}
	}
}

func TestOpenSortsAndActivatesTheFirst(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "zz/ash", "fm/rook", "fm/kit")
	if got, want := a.Order(), []string{"fm/kit", "fm/rook", "zz/ash"}; !slices.Equal(got, want) {
		t.Errorf("Order = %v, want %v", got, want)
	}
	if a.Active() != "zz/ash" {
		t.Errorf("Active = %q, want the first one opened", a.Active())
	}
	again, _ := a.Open("fm/kit")
	if again != a.Char("fm/kit") {
		t.Error("opening an open character made a new one")
	}
	if c, _ := a.Open("fm/nobody"); c != nil {
		t.Error("opened a character that isn't configured")
	}
}

func TestSwitchClearsUnreadAndRemembersWhereYouWere(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "fm/kit", "fm/rook")
	rook := a.Char("fm/rook")
	rook.Unread, rook.Attention = 3, true
	if !a.Switch("fm/rook") {
		t.Fatal("Switch to an open character failed")
	}
	if rook.Unread != 0 || rook.Attention {
		t.Errorf("unread %d attention %v after switching to it", rook.Unread, rook.Attention)
	}
	if got := a.UnreadTarget(1); got != "fm/kit" {
		t.Errorf("UnreadTarget with nothing unread = %q, want the one before (fm/kit)", got)
	}
	if a.Switch("fm/nobody") || a.Active() != "fm/rook" {
		t.Error("Switch to a closed character changed something")
	}
	if !a.Switch(WorldSel("fm")) {
		t.Fatal("Switch to an open world's overview failed")
	}
	if w, ok := a.ActiveWorld(); !ok || w != "fm" {
		t.Errorf("ActiveWorld = %q, %v", w, ok)
	}
	if a.CanSwitch(WorldSel("zz")) || a.Switch(WorldSel("zz")) {
		t.Error("switched to the overview of a world with nothing open")
	}
}

func TestStepTargetWalksWorldsAndCharacters(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "fm/kit", "fm/rook", "zz/ash")
	if got, want := a.Stops(), []string{WorldSel("fm"), "fm/kit", "fm/rook", WorldSel("zz"), "zz/ash"}; !slices.Equal(got, want) {
		t.Errorf("Stops = %v, want %v", got, want)
	}
	if got := a.StepTarget(1); got != "fm/rook" {
		t.Errorf("StepTarget(1) from Kit = %q", got)
	}
	if got := a.StepTarget(-1); got != WorldSel("fm") {
		t.Errorf("StepTarget(-1) from Kit = %q", got)
	}
	a.Switch("zz/ash")
	if got := a.StepTarget(1); got != WorldSel("fm") {
		t.Errorf("StepTarget(1) from the last stop = %q, want it to wrap", got)
	}
}

func TestUnreadTargetFindsTheNextUnread(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "fm/kit", "fm/rook", "zz/ash")
	a.Char("zz/ash").Unread = 1
	for _, dir := range []int{1, -1} {
		if got := a.UnreadTarget(dir); got != "zz/ash" {
			t.Errorf("UnreadTarget(%d) = %q, want zz/ash", dir, got)
		}
	}
	empty, _ := testApp(t)
	if got := empty.UnreadTarget(1); got != "" {
		t.Errorf("UnreadTarget with nothing open = %q", got)
	}
}

func TestCloseActivatesTheNextDown(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "fm/kit", "fm/rook", "zz/ash")
	a.Switch("fm/rook")
	a.Close("fm/rook")
	if a.Active() != "zz/ash" || a.Char("fm/rook") != nil {
		t.Errorf("after closing Rook: active %q, rook %v", a.Active(), a.Char("fm/rook"))
	}
	a.Close("zz/ash")
	if a.Active() != "fm/kit" {
		t.Errorf("closing the last one should activate the one above: %q", a.Active())
	}
	a.Close("fm/kit")
	if a.Active() != "" || len(a.Order()) != 0 {
		t.Errorf("all closed: active %q order %v", a.Active(), a.Order())
	}
}

// Closing a world's last character while its overview shows moves on, as
// closing the active character does.
func TestClosingTheOverviewsLastCharacter(t *testing.T) {
	a, _ := testApp(t)
	openAll(t, a, "fm/kit", "zz/ash")
	a.Switch(WorldSel("zz"))
	a.Close("zz/ash")
	if a.Active() != "fm/kit" {
		t.Errorf("Active = %q, want fm/kit", a.Active())
	}
}

func TestApplyConfigClosesRemovedCharacters(t *testing.T) {
	a, dir := testApp(t)
	openAll(t, a, "fm/kit", "zz/ash")
	if err := os.Remove(filepath.Join(dir, "worlds", "zz.toml")); err != nil {
		t.Fatal(err)
	}
	res := a.ApplyConfig(load(t, dir))
	if !slices.Equal(res.Closed, []string{"zz/ash"}) || a.Char("zz/ash") != nil {
		t.Errorf("Closed = %v, ash still open: %v", res.Closed, a.Char("zz/ash") != nil)
	}
	if len(res.Errs) != 0 {
		t.Errorf("Errs = %v", res.Errs)
	}
}

func TestStatusGenerations(t *testing.T) {
	a, _ := testApp(t)
	a.SetStatus(true, "first")
	g := a.Status().Gen
	a.SetStatus(false, "second")
	if s := a.Status(); s.Text != "second" || s.Err || s.Gen != g+1 {
		t.Errorf("Status = %+v after a second message", s)
	}
	a.ClearStatus()
	if s := a.Status(); s.Text != "" || s.Gen != g+1 {
		t.Errorf("Status = %+v after clearing; Gen shouldn't move", s)
	}
}

func TestPasswordStoreFollowsTheConfig(t *testing.T) {
	if got := New(Deps{}).PasswordStore(); got != config.DefaultPasswordStore {
		t.Errorf("before a config: %q", got)
	}
	a, _ := testApp(t)
	if got := a.PasswordStore(); got != a.Config().PasswordStore {
		t.Errorf("PasswordStore = %q, config says %q", got, a.Config().PasswordStore)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app`
Expected: build failure — `undefined: New`, `undefined: Deps`, `undefined: WorldSel`, etc.

- [ ] **Step 3: Write `internal/app/app.go`**

```go
package app

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/session"
)

// Deps are the core's connections to the outside world. Tests substitute
// fakes; each front end wires the real ones.
type Deps struct {
	LogRoot  string // where log_dir is relative to; "" (and no absolute log_dir): no logs to read
	Dial     func(ctx context.Context, ch config.Character) (session.LineConn, error)
	NewLog   func(l logstore.Layout) session.Appender // l from LogLayout
	Password func(store, world, char string) (string, error)
	Now      func() time.Time
}

// Char is one open character. Front ends read its fields; only App
// changes them.
type Char struct {
	Key         string
	Ch          config.Character
	Sess        *session.Session
	State       session.State
	Rules       Rules
	Unread      int
	Attention   bool
	Pin         *conn.PinMismatchError
	Orphan      bool      // removed from the config; closed when it disconnects
	ConnectedAt time.Time // when the current connection came up
	cancel      context.CancelFunc
}

// compile builds c's rules from its configuration. On an error c keeps
// the rules it had.
func (c *Char) compile() error {
	cls, err := classify.New(c.Ch.Rules.Classify, c.Ch.Name, c.Ch.Aliases)
	if err != nil {
		return err
	}
	c.Rules = Rules{Classifier: cls, Judge: rules.Judge{Attention: c.Ch.Rules.Attention, Quiet: c.Ch.Rules.Quiet}}
	return nil
}

// Status is the statusline's message. Gen goes up with each new one, so
// a front end can time out the one it saw.
type Status struct {
	Text string
	Err  bool
	Gen  int
}

// App is what Kiln decides. It isn't safe for concurrent use: it runs on
// its front end's loop.
type App struct {
	d            Deps
	cfg          *config.Config
	chars        map[string]*Char
	order        []string // open characters, in sidebar order
	active       string   // a character's key, WorldSel(world), or ""
	recent       []string // open characters by when last active, most recent first; not the active one
	status       Status
	pwStore      atomic.Pointer[string] // password_store; sessions read it off the loop
	paneW, paneH int                    // where server text is shown; 0×0 until known
}

// Key is a character's key: its world and id.
func Key(world, char string) string { return world + "/" + char }

// WorldSel is what Active is while world's overview shows.
func WorldSel(world string) string { return "=" + world }

// CompareChars orders characters alphabetically, ignoring case: by world
// id, then by name.
func CompareChars(a, b config.Character) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(a.World), strings.ToLower(b.World)),
		cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		cmp.Compare(Key(a.World, a.ID), Key(b.World, b.ID)),
	)
}

// New makes an App with nothing open and no config yet; ApplyConfig
// gives it one.
func New(d Deps) *App {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &App{d: d, chars: map[string]*Char{}}
}

// ConfigResult is what ApplyConfig did to the open characters.
type ConfigResult struct {
	Closed []string         // gone from the config and not connected, in sidebar order
	Errs   map[string]error // rules that didn't compile, by key; those characters keep their old ones
}

// ApplyConfig takes cfg and updates the open characters to match it. An
// open character that vanished from the config stays as an orphan while
// it's connected, until it next disconnects; otherwise it closes.
func (a *App) ApplyConfig(cfg *config.Config) ConfigResult {
	a.cfg = cfg
	store := cfg.PasswordStore
	a.pwStore.Store(&store)
	res := ConfigResult{Errs: map[string]error{}}
	for _, k := range slices.Clone(a.order) {
		c := a.chars[k]
		ch, ok := a.Find(k)
		if !ok {
			if c.Sess != nil && c.State != session.Disconnected && c.State != session.Failed {
				c.Orphan = true
			} else {
				a.Close(k)
				res.Closed = append(res.Closed, k)
			}
			continue
		}
		c.Ch, c.Orphan = ch, false
		if err := c.compile(); err != nil {
			res.Errs[k] = err
		}
		if c.Sess != nil {
			c.Sess.SetChar(ch)
		}
	}
	a.sortOrder() // names may have changed
	return res
}

// Config is the config last applied; nil before the first.
func (a *App) Config() *config.Config { return a.cfg }

// Find looks up a configured character by key.
func (a *App) Find(k string) (config.Character, bool) {
	if a.cfg != nil {
		for _, w := range a.cfg.Worlds {
			for _, ch := range w.Characters {
				if Key(ch.World, ch.ID) == k {
					return ch, true
				}
			}
		}
	}
	return config.Character{}, false
}

// AllChars is every configured character, in sidebar order.
func (a *App) AllChars() []config.Character {
	var all []config.Character
	if a.cfg != nil {
		for _, w := range a.cfg.Worlds {
			all = append(all, w.Characters...)
		}
	}
	slices.SortFunc(all, CompareChars)
	return all
}

func (a *App) sortOrder() {
	slices.SortFunc(a.order, func(x, y string) int { return CompareChars(a.chars[x].Ch, a.chars[y].Ch) })
}

// Open adds configured character k to the open ones, in sidebar order,
// and returns it, with the error if its rules didn't compile. An open
// character is returned as it is. It returns nil if k isn't configured.
// It doesn't connect. With nothing active, k becomes active.
func (a *App) Open(k string) (*Char, error) {
	if c := a.chars[k]; c != nil {
		return c, nil
	}
	ch, ok := a.Find(k)
	if !ok {
		return nil, nil
	}
	c := &Char{Key: k, Ch: ch}
	err := c.compile()
	a.chars[k] = c
	a.order = append(a.order, k)
	a.sortOrder()
	if a.active == "" {
		a.active = k
	}
	return c, err
}

// Close stops an open character's session and closes it. If it was
// active, or its world's overview was and it was the world's last
// character, the next one down becomes active, or the one above if it
// was last.
func (a *App) Close(k string) {
	i := slices.Index(a.order, k)
	if i < 0 {
		return
	}
	if c := a.chars[k]; c.cancel != nil {
		c.cancel()
	}
	delete(a.chars, k)
	a.order = slices.Delete(a.order, i, i+1)
	a.recent = slices.DeleteFunc(a.recent, func(r string) bool { return r == k })
	if w, ok := a.ActiveWorld(); ok && !a.WorldOpen(w) {
		k = a.active // its last character closed: the world's row goes too
	}
	if a.active != k {
		return
	}
	a.active = ""
	if len(a.order) > 0 {
		a.Switch(a.order[min(i, len(a.order)-1)])
	}
}

// Char is open character k; nil if it isn't open.
func (a *App) Char(k string) *Char { return a.chars[k] }

// Order is the open characters' keys in sidebar order. Don't change it.
func (a *App) Order() []string { return a.order }

// Active is the active sidebar item: a character's key, WorldSel(world)
// for a world's overview, or "" with nothing open.
func (a *App) Active() string { return a.active }

// ActiveWorld is the world whose overview is showing, when its sidebar
// row is the active one instead of a character.
func (a *App) ActiveWorld() (string, bool) { return strings.CutPrefix(a.active, WorldSel("")) }

// WorldOpen reports whether any of world's characters are open.
func (a *App) WorldOpen(world string) bool {
	return slices.ContainsFunc(a.order, func(k string) bool { return a.chars[k].Ch.World == world })
}

// CanSwitch reports whether Switch(k) would: k is an open character, or
// the overview of a world with one open.
func (a *App) CanSwitch(k string) bool {
	if a.chars[k] != nil {
		return true
	}
	w, isWorld := strings.CutPrefix(k, WorldSel(""))
	return isWorld && a.WorldOpen(w)
}

// Switch makes k active: a character's key, or WorldSel(world) for its
// overview. It reports false, changing nothing, if it can't (CanSwitch).
// Switching to a character clears its unread and attention; the
// character switched away from goes first in line for UnreadTarget.
func (a *App) Switch(k string) bool {
	if !a.CanSwitch(k) {
		return false
	}
	if a.chars[a.active] != nil && a.active != k {
		a.recent = slices.DeleteFunc(a.recent, func(r string) bool { return r == a.active })
		a.recent = append([]string{a.active}, a.recent...)
	}
	a.active = k
	if c := a.chars[k]; c != nil {
		a.recent = slices.DeleteFunc(a.recent, func(r string) bool { return r == k })
		c.Unread, c.Attention = 0, false
	}
	return true
}

// Stops are what stepping through the sidebar visits: each world with a
// character open, then its characters, in sidebar order, as values
// Active takes.
func (a *App) Stops() []string {
	var s []string
	last := ""
	for _, k := range a.order {
		if w := a.chars[k].Ch.World; w != last {
			last = w
			s = append(s, WorldSel(w))
		}
		s = append(s, k)
	}
	return s
}

// StepTarget is where stepping delta stops through Stops from the active
// one lands, wrapping around; "" with nothing open.
func (a *App) StepTarget(delta int) string {
	stops := a.Stops()
	if len(stops) == 0 {
		return ""
	}
	i := max(0, slices.Index(stops, a.active))
	return stops[(i+delta%len(stops)+len(stops))%len(stops)]
}

// UnreadTarget is the next character (dir 1) or previous one (dir -1) in
// sidebar order with unread lines, wrapping around. With nothing unread
// it's the character active before this one, so switching to it flips
// between the last two, like switching apps; "" when there's nowhere to go.
func (a *App) UnreadTarget(dir int) string {
	n := len(a.order)
	i := slices.Index(a.order, a.active) // -1 when no character is active
	for step := 1; step <= n; step++ {
		k := a.order[((i+dir*step)%n+n)%n]
		if k != a.active && a.chars[k].Unread > 0 {
			return k
		}
	}
	if len(a.recent) > 0 {
		return a.recent[0]
	}
	if n > 1 { // others are open but none has been active yet
		return a.order[((i+dir)%n+n)%n]
	}
	return ""
}

// SetStatus puts text on the statusline, as an error if isErr.
func (a *App) SetStatus(isErr bool, text string) {
	a.status = Status{Text: text, Err: isErr, Gen: a.status.Gen + 1}
}

// ClearStatus takes the message down. It isn't a new message, so Gen
// stays.
func (a *App) ClearStatus() { a.status.Text = "" }

// Status is the statusline's message.
func (a *App) Status() Status { return a.status }

// PasswordStore is the password_store setting. It's safe to call off the
// loop.
func (a *App) PasswordStore() string {
	if p := a.pwStore.Load(); p != nil && *p != "" {
		return *p
	}
	return config.DefaultPasswordStore
}

// LogLayout is where ch's logs go; ok is false when there's nowhere.
func (a *App) LogLayout(ch config.Character) (l logstore.Layout, ok bool) {
	l = logstore.Layout{Root: a.d.LogRoot, Dir: a.cfg.LogDir, Name: a.cfg.LogName,
		World: ch.World, Char: ch.ID, CharName: ch.Name}
	return l, a.d.LogRoot != "" || filepath.IsAbs(a.cfg.LogDir)
}
```

Note on `StepTarget`: the old `switchBy` computed `(i + delta + len) % len` with delta ±1. `delta%len(stops)` keeps that for ±1 and stays in range for any delta.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/app`
Expected: PASS (including Task 2 of step 1's `TestNoTerminal`).

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -m "app: App opens, closes and switches characters, and holds the status

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```

---

### Task 2: Sessions in the core — `Connect`, `Handle`, `Quit`, effects

**Files:**
- Create: `internal/app/effect.go`
- Create: `internal/app/session.go`
- Create: `internal/app/session_test.go`

**Interfaces:**
- Consumes: Task 1's `App`, `Char`, `Status`, `LogLayout`, `PasswordStore`, `Close`, `SetStatus`.
- Produces:
  - `type app.Effect interface{ effect() }`; `type app.Run struct{ Func func() any }`; `type app.Quit struct{}`
  - `type app.SessionMsg struct { Key string; Sess *session.Session; Ev session.Event; OK bool }`
  - `type app.Event struct { Key string; Ev session.Event; Line Line; Closed bool }`
  - `(*App)`: `Connect(k string) []Effect`, `Handle(msg SessionMsg) (ev Event, ok bool, effs []Effect)`, `Quit() []Effect`, `SetPane(w, h int)`, `ReportSize()`

- [ ] **Step 1: Write the tests**

`internal/app/session_test.go`:

```go
package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

type nopLog struct{}

func (nopLog) Append(logstore.Entry) error { return nil }

// lineConn is a connection that sends nothing until it's closed.
type lineConn struct {
	lines chan string
	once  sync.Once
}

func (c *lineConn) Lines() <-chan string { return c.lines }
func (c *lineConn) Err() error           { return nil }
func (c *lineConn) Send(string) error    { return nil }
func (c *lineConn) Close() error         { c.once.Do(func() { close(c.lines) }); return nil }

var now = time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local)

// sessionApp is an App over fm and zz whose sessions dial lineConns.
func sessionApp(t *testing.T, worlds map[string]string) *App {
	t.Helper()
	dir := configDir(t, worlds)
	a := New(Deps{
		LogRoot: filepath.Join(dir, "logs"),
		Dial: func(context.Context, config.Character) (session.LineConn, error) {
			return &lineConn{lines: make(chan string)}, nil
		},
		NewLog:   func(logstore.Layout) session.Appender { return nopLog{} },
		Password: func(string, string, string) (string, error) { return "", errors.New("none") },
		Now:      func() time.Time { return now },
	})
	a.ApplyConfig(load(t, dir))
	return a
}

// attach gives k a session that never runs, as Connect would, so events
// can be handed to Handle.
func attach(a *App, k string) *session.Session {
	c := a.Char(k)
	c.Sess = session.New(session.Options{Char: c.Ch, Log: nopLog{}})
	return c.Sess
}

func lineMsg(k string, s *session.Session, text string) SessionMsg {
	return SessionMsg{Key: k, Sess: s, OK: true, Ev: session.Event{Kind: session.EventLine, Entry: logstore.Entry{Dir: logstore.In, Text: text}}}
}

func stateMsg(k string, s *session.Session, st session.State, err error) SessionMsg {
	return SessionMsg{Key: k, Sess: s, OK: true, Ev: session.Event{Kind: session.EventState, State: st, Err: err}}
}

func TestHandleCountsUnreadOnlyOffScreen(t *testing.T) {
	// quiet goes above [[characters]], or TOML hands it to the last one.
	quiet := strings.Replace(fmWorld, "login = \"connect {name} {password}\"\n", "login = \"connect {name} {password}\"\nquiet = [\"wiki\"]\n", 1) +
		"\n[[classify]]\ntag = \"wiki\"\npattern = '^\\[Wiki\\]'\n"
	a := sessionApp(t, map[string]string{"fm": quiet})
	openAll(t, a, "fm/kit", "fm/rook")
	kit, rook := attach(a, "fm/kit"), attach(a, "fm/rook")
	for _, m := range []SessionMsg{lineMsg("fm/kit", kit, "hi"), lineMsg("fm/rook", rook, "hi"), lineMsg("fm/rook", rook, "[Wiki] edited")} {
		ev, ok, effs := a.Handle(m)
		if !ok || ev.Line.Entry.Text != m.Ev.Entry.Text {
			t.Fatalf("Handle(%q) = %+v, %v", m.Ev.Entry.Text, ev, ok)
		}
		if len(effs) != 1 {
			t.Errorf("Handle(%q) effects = %v, want the next wait", m.Ev.Entry.Text, effs)
		}
	}
	if u := a.Char("fm/kit").Unread; u != 0 {
		t.Errorf("the active character counted %d unread", u)
	}
	if u := a.Char("fm/rook").Unread; u != 1 {
		t.Errorf("Rook unread = %d, want 1 (the quiet line doesn't count)", u)
	}
}

func TestHandleIgnoresStaleSessions(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	attach(a, "fm/kit")
	old := session.New(session.Options{Char: a.Char("fm/kit").Ch, Log: nopLog{}})
	if _, ok, effs := a.Handle(lineMsg("fm/kit", old, "hi")); ok || effs != nil {
		t.Error("handled an event from a replaced session")
	}
	cur := a.Char("fm/kit").Sess
	if _, ok, effs := a.Handle(SessionMsg{Key: "fm/kit", Sess: cur, OK: false}); ok || effs != nil {
		t.Error("handled a shut-down session's last receive")
	}
}

func TestHandleClosesAnOrphanWhenItDisconnects(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit", "fm/rook")
	s := attach(a, "fm/rook")
	rook := a.Char("fm/rook")
	rook.State, rook.Orphan = session.Connected, true
	ev, ok, effs := a.Handle(stateMsg("fm/rook", s, session.Disconnected, nil))
	if !ok || !ev.Closed || effs != nil || a.Char("fm/rook") != nil {
		t.Errorf("ev %+v ok %v effs %v open %v; want closed with nothing more to wait for", ev, ok, effs, a.Char("fm/rook") != nil)
	}
}

func TestHandleTracksAChangedCertificate(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	pin := &conn.PinMismatchError{HostPort: "muck.test:8888", Pinned: "sha256:aa", Got: "sha256:bb"}
	a.Handle(stateMsg("fm/kit", s, session.Failed, pin))
	kit := a.Char("fm/kit")
	if kit.Pin != pin || kit.State != session.Failed {
		t.Errorf("pin %v state %v after a mismatch", kit.Pin, kit.State)
	}
	if st := a.Status(); !st.Err || st.Text != str.StatusCertChanged("Kit") {
		t.Errorf("status = %+v", st)
	}
	a.Handle(stateMsg("fm/kit", s, session.Connected, nil))
	if kit.Pin != nil || !kit.ConnectedAt.Equal(now) {
		t.Errorf("after connecting: pin %v, connected at %v", kit.Pin, kit.ConnectedAt)
	}
}

func TestConnectWaitsForTheSessionsEvents(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	effs := a.Connect("fm/kit")
	defer a.Quit()
	kit := a.Char("fm/kit")
	if kit.Sess == nil || kit.State != session.Connecting {
		t.Fatalf("after Connect: session %v state %v", kit.Sess, kit.State)
	}
	run, ok := effs[0].(Run)
	if len(effs) != 1 || !ok {
		t.Fatalf("Connect effects = %v, want one Run", effs)
	}
	msg, ok := run.Func().(SessionMsg)
	if !ok || msg.Key != "fm/kit" || msg.Sess != kit.Sess || !msg.OK {
		t.Errorf("the wait returned %+v", msg)
	}
	if again := a.Connect("fm/kit"); again != nil {
		t.Errorf("Connect on a live session = %v, want a reconnect and nothing to wait for", again)
	}
}

func TestQuitStopsEverySession(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	a.Connect("fm/kit")
	effs := a.Quit()
	if _, ok := effs[0].(Quit); len(effs) != 1 || !ok {
		t.Errorf("Quit effects = %v", effs)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app`
Expected: build failure — `undefined: SessionMsg`, `a.Handle undefined`, `undefined: Run`.

- [ ] **Step 3: Write `internal/app/effect.go`**

```go
package app

// Effect is something the front end does for the core: each front end
// performs them its own way.
type Effect interface{ effect() }

// Run is blocking work for the front end to do off its loop: Func's
// result goes back to the core (a SessionMsg goes to Handle).
type Run struct{ Func func() any }

// Quit ends the program.
type Quit struct{}

func (Run) effect()  {}
func (Quit) effect() {}
```

- [ ] **Step 4: Write `internal/app/session.go`**

```go
package app

import (
	"context"
	"errors"

	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

// SessionMsg is a session's next event, as a Run from Connect or Handle
// returns it. OK is false once the session has shut down.
type SessionMsg struct {
	Key  string
	Sess *session.Session
	Ev   session.Event
	OK   bool
}

// Event is what Handle made of a session event, for the front end's part.
type Event struct {
	Key    string
	Ev     session.Event
	Line   Line // for an EventLine, what Kiln made of its entry
	Closed bool // the character was an orphan and has closed
}

// wait is the Run that waits for s's next event.
func wait(k string, s *session.Session) Effect {
	return Run{Func: func() any {
		ev, ok := <-s.Events()
		return SessionMsg{Key: k, Sess: s, Ev: ev, OK: ok}
	}}
}

// Connect starts k's session, or restarts the one it has (and then there
// is nothing new to wait for). The Run it returns waits for the session's
// first event.
func (a *App) Connect(k string) []Effect {
	c := a.chars[k]
	if c == nil {
		return nil
	}
	if c.Sess != nil {
		c.Sess.Reconnect()
		return nil
	}
	l, _ := a.LogLayout(c.Ch)
	var s *session.Session
	s = session.New(session.Options{
		Char: c.Ch,
		Log:  a.d.NewLog(l),
		Dial: func(ctx context.Context) (session.LineConn, error) { return a.d.Dial(ctx, s.Char()) },
		Password: func() (string, error) {
			ch := s.Char()
			return a.d.Password(a.PasswordStore(), ch.World, ch.ID)
		},
	})
	if a.paneW > 0 {
		s.Resize(a.paneW, a.paneH)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.Sess, c.cancel = s, cancel
	c.State = session.Connecting // until the session says otherwise
	go s.Run(ctx)
	return []Effect{wait(k, s)}
}

// Handle takes a session's event. ok is false for one from a session
// that has since been replaced or has shut down: there's nothing to do.
// Otherwise the effects wait for the session's next event, unless the
// event closed the character.
func (a *App) Handle(msg SessionMsg) (ev Event, ok bool, effs []Effect) {
	c := a.chars[msg.Key]
	if c == nil || c.Sess != msg.Sess || !msg.OK {
		return Event{}, false, nil
	}
	ev = Event{Key: msg.Key, Ev: msg.Ev}
	switch msg.Ev.Kind {
	case session.EventLine:
		ev.Line = c.Rules.Line(msg.Ev.Entry)
		if msg.Key != a.active && msg.Ev.Entry.Dir == logstore.In && !ev.Line.Quiet {
			c.Unread++
			c.Attention = c.Attention || ev.Line.Attention
		}
	case session.EventState:
		c.State = msg.Ev.State
		if c.State == session.Connected {
			c.ConnectedAt = a.d.Now()
		}
		if c.Orphan && (c.State == session.Disconnected || c.State == session.Failed) {
			a.Close(msg.Key) // don't reconnect (and log in) a deleted character
			ev.Closed = true
			return ev, true, nil
		}
		var pin *conn.PinMismatchError
		if c.State == session.Failed && errors.As(msg.Ev.Err, &pin) {
			c.Pin = pin
			a.SetStatus(true, str.StatusCertChanged(c.Ch.Name))
		}
		if c.State == session.Connected {
			c.Pin = nil
		}
	case session.EventLogError:
		a.SetStatus(true, str.StatusLogWriteFailed(c.Ch.Name, msg.Ev.Err))
	}
	return ev, true, []Effect{wait(msg.Key, msg.Sess)}
}

// Quit stops every session and ends the program.
func (a *App) Quit() []Effect {
	for _, c := range a.chars {
		if c.cancel != nil {
			c.cancel()
		}
	}
	return []Effect{Quit{}}
}

// SetPane records the size of the pane server text shows in, for
// sessions that connect from now on. ReportSize tells the open ones.
func (a *App) SetPane(w, h int) { a.paneW, a.paneH = w, h }

// ReportSize tells every session the pane's size. Sessions only send it
// to servers that negotiated NAWS.
func (a *App) ReportSize() {
	for _, c := range a.chars {
		if c.Sess != nil {
			c.Sess.Resize(a.paneW, a.paneH)
		}
	}
}
```

Note: the old `reportSize` called `Resize(paneSize())` even when the pane was 0×0; `ReportSize` does the same with the stored size, which the ui sets on every `WindowSizeMsg` before the debounced report.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/app -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app
git commit -m "app: sessions connect, report events and quit through the core

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```

---

### Task 3: `ui.Model` drives `app.App`

**Files:**
- Modify: `internal/ui/model.go`, `internal/ui/sidebar.go`, `internal/ui/view.go`, `internal/ui/notify.go`, `internal/ui/picker.go`, `internal/ui/editor.go`, `internal/ui/browse.go` (and any other `ui` file the compiler names)
- Modify tests (access paths only): `internal/ui/*_test.go`

**Interfaces:**
- Consumes: everything Tasks 1–2 produce.
- Produces (for 2b/2c): `Model.a *app.App`; `charState` embeds `*app.Char`; `(*charState).compileLook() error`; `(*Model).run([]app.Effect) tea.Cmd`; `(*Model).closed(k, prev string)`; `(*Model).activated(prev string)`; `(*Model).showActive()`.

- [ ] **Step 1: Restructure `Model` and `charState` (model.go)**

`Model` loses `cfg`, `chars`'s core half, `order`, `active`, `status`, `statusErr`, `pwStore`, `recent`, `statusGen`; it gains `a *app.App`. It keeps `chars map[string]*charState` (the views, keyed like `app`'s characters), `statusOfLog`, `statusTimed` and everything else.

```go
type charState struct {
	*app.Char                        // the core's half; ui only reads it
	hl        *rules.Highlighter     // the character's look: the theme with its own looks on top
	sb        Scrollback
	in        *Input
	needPW    bool
	pwDraft   string           // input stashed while the password prompt is up
	browse    *browse          // non-nil while browse mode is open
	hidBrowse *browse          // browse mode as Ctrl+L left it, kept up to date; the next Ctrl+L brings it back
	filter    scene.Filter     // log mode's filter; outlasts a log-mode session
	collapsed map[string]bool  // filter panel parents folded shut, by tag
	foldSeen  map[string]bool  // parents the panel has already met; a new one starts folded
	hist      *history.Reader  // pages older log days into sb; only an in-flight sbOlderMsg read touches it
	leftover  []logstore.Entry // the preload's unshown start of its oldest day
	sentGen   int              // hereGen when the last notification went out; -1: none yet
	lastSent  time.Time        // when the last notification went out
}
```

Replace `compile` with:

```go
// compileLook builds cs's highlighter from the active theme with the
// character's own looks on top. Looks that don't resolve (a color nobody
// defines) leave the theme's own tag styles in use; either way the error
// is returned for the caller to show.
func (cs *charState) compileLook() error {
	th, err := theme.Active().With(cs.Ch.Looks...)
	if err != nil {
		th = theme.Active()
	}
	cs.hl = rules.New(th)
	return err
}
```

Keep `func key(world, char string) string { return app.Key(world, char) }` so tests and callers don't churn. In `picker.go`, `worldSel` becomes `func worldSel(world string) string { return app.WorldSel(world) }`. In `sidebar.go`, `compareChars` becomes `func compareChars(a, b config.Character) int { return app.CompareChars(a, b) }`; delete `allChars`, `find`, `sortOrder`, `activeWorld`, `worldOpen`, `stops` (callers use `m.a.AllChars()`, `m.a.Find`, `m.a.ActiveWorld()`, `m.a.WorldOpen`, `m.a.Stops()`).

`New`:

```go
func New(d Deps, cfg *config.Config) *Model {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Raw == nil {
		d.Raw = func(seq string) tea.Cmd { return tea.Raw(seq) }
	}
	m := &Model{d: d, chars: map[string]*charState{}, idle: NewInput()}
	m.a = app.New(app.Deps{
		LogRoot: d.LogRoot, Dial: d.Dial, NewLog: d.NewLog, Password: d.Password,
		Now: func() time.Time { return m.d.Now() }, // late-bound: tests swap the clock
	})
	m.focused, m.lastHere = true, d.Now()
	m.notifyOverrides = map[string]notify.Level{}
	m.applyConfig(cfg)
	m.shownPresence = m.presence()
	m.loadTheme() // at start a broken theme falls back to the built-in, and says so
	if m.themeErr != nil {
		m.setStatus(true, str.StatusThemeNotLoaded(m.themeErr))
		m.themeErr = nil
	}
	for _, ch := range m.a.AllChars() {
		if ch.Autoconnect && !d.NoAutoconnect {
			m.open(key(ch.World, ch.ID))
		}
	}
	return m
}
```

`Init`: loop `m.a.Order()`, `m.chars[k].Ch.Autoconnect`.

`run`, the effects bridge:

```go
// run performs the core's effects: blocking work as commands, quitting
// as tea.Quit.
func (m *Model) run(effs []app.Effect) tea.Cmd {
	var cmds []tea.Cmd
	for _, e := range effs {
		switch e := e.(type) {
		case app.Run:
			f := e.Func
			cmds = append(cmds, func() tea.Msg { return f() })
		case app.Quit:
			cmds = append(cmds, tea.Quit)
		}
	}
	return tea.Batch(cmds...)
}
```

Delete `waitEvent`, `eventMsg`, `passwordStore` (callers use `m.a.PasswordStore()`), `logLayout`, `mustLayout` (callers use `m.a.LogLayout`), `reportSize`.

`connect` and `quit`:

```go
// connect starts (or restarts) a character's session.
func (m *Model) connect(cs *charState) tea.Cmd { return m.run(m.a.Connect(cs.Key)) }

func (m *Model) quit() tea.Cmd { return m.run(m.a.Quit()) }
```

`update`: `case tea.WindowSizeMsg:` sets width/height, then `m.a.SetPane(m.paneSize())`, then the debounce as before; `case resizeMsg:` calls `m.a.ReportSize()`; `case eventMsg:` becomes `case app.SessionMsg: return m, m.handleEvent(msg)`; `case statusExpiredMsg:` is `if int(msg) == m.a.Status().Gen { m.a.ClearStatus() }`; the `KeyPressMsg` case reads `gen := m.a.Status().Gen` and dismisses with `m.a.ClearStatus()`.

`Update`: `if g := m.a.Status().Gen; g != m.statusTimed { m.statusTimed = g; cmd = tea.Batch(cmd, tea.Tick(statusTimeout, func(time.Time) tea.Msg { return statusExpiredMsg(g) })) }`.

`setStatus`, `takeLogStatus`, `clearLogStatus`:

```go
func (m *Model) setStatus(isErr bool, msg string) {
	m.a.SetStatus(isErr, msg)
	m.statusOfLog = false
}
```

`takeLogStatus` keeps its body (it calls `setStatus`); `clearLogStatus` does `if m.statusOfLog { m.a.ClearStatus(); m.statusOfLog = false }`. `disarmQuit` compares `m.a.Status().Text == quitHint(m.quitKey)` and clears with `m.a.ClearStatus()`. `openBrowse`'s two `m.status = ""` become `m.a.ClearStatus()`.

- [ ] **Step 2: Open, close, switch and config through the core**

```go
// open opens configured character k with its view, preloading its
// history, and returns it; an open character is returned as is. It
// returns nil if k isn't configured. It doesn't connect.
func (m *Model) open(k string) *charState {
	if cs := m.chars[k]; cs != nil {
		return cs
	}
	c, err := m.a.Open(k)
	if c == nil {
		return nil
	}
	cs := &charState{Char: c, in: NewInput(), sentGen: -1}
	if err == nil {
		err = cs.compileLook()
	}
	if err != nil {
		m.setStatus(true, str.StatusCharError(k, err))
	}
	m.chars[k] = cs
	m.preload(cs)
	cs.sb.MarkSeen() // history isn't news
	return cs
}

// close stops an open character's session and closes it.
func (m *Model) close(k string) {
	if m.chars[k] == nil {
		return
	}
	prev := m.a.Active()
	m.a.Close(k)
	m.closed(k, prev)
	m.activated(prev)
}

// closed does the screen's part of closing k, after the core has, when
// prev was active: an editor open on it keeps its edits as a draft, and
// its view goes.
func (m *Model) closed(k, prev string) {
	if k == prev {
		m.leaveEditor()
	}
	if e := m.parked[k]; e != nil {
		delete(m.parked, k)
		m.stashDraft(e)
	}
	delete(m.chars, k)
}

// showActive opens the active character's scrollback at the first line
// you haven't seen, measured at the pane's width; log mode keeps the pane
// to itself.
func (m *Model) showActive() {
	cs := m.cur()
	if cs == nil {
		return
	}
	l := m.layout()
	cs.sb.SetWidth(l.rw)
	cs.sb.Pause(l.sbH)
	if cs.browse != nil && m.picker != nil {
		m.leaveEditor()
		m.closePicker() // browse has the pane; the filter would be hidden
	}
}

// activated does the screen's part when the core has moved the active
// item away from prev: an overview starts at its top, the new character
// shows, and an editor left open on it comes back.
func (m *Model) activated(prev string) {
	if m.a.Active() == prev {
		return
	}
	m.ovTop = 0
	m.showActive()
	m.unparkEditor()
}

// switchTo makes k active: a character's key, or a world's selection key
// (worldSel) for its overview.
func (m *Model) switchTo(k string) {
	if !m.a.CanSwitch(k) {
		return
	}
	prev := m.a.Active()
	if prev != k {
		m.parkEditor()
		if old := m.cur(); old != nil {
			old.sb.MarkSeen() // you saw it up to now
		}
	}
	m.a.Switch(k)
	m.confirm = false
	if prev == k {
		m.showActive()
	} else {
		m.activated(prev)
	}
}

// switchBy moves through the sidebar's worlds and characters.
func (m *Model) switchBy(delta int) {
	if t := m.a.StepTarget(delta); t != "" {
		m.switchTo(t)
	}
}

// switchToUnread moves to the next character (dir 1) or previous one
// (dir -1) with unread lines; see app.UnreadTarget.
func (m *Model) switchToUnread(dir int) {
	if t := m.a.UnreadTarget(dir); t != "" {
		m.switchTo(t)
	}
}
```

`applyConfig`:

```go
// applyConfig hands cfg to the core, then updates the open characters'
// views: export settings, looks, and lines when what styles them changed.
func (m *Model) applyConfig(cfg *config.Config) {
	keys := slices.Clone(m.a.Order())
	before := map[string]config.Character{}
	for _, k := range keys {
		before[k] = m.chars[k].Ch
	}
	prev := m.a.Active()
	res := m.a.ApplyConfig(cfg)
	for _, k := range res.Closed {
		m.closed(k, prev)
	}
	m.activated(prev)
	for _, k := range keys { // in order, so which error shows is settled
		cs := m.chars[k]
		if cs == nil {
			continue
		}
		for _, b := range cs.browses() {
			b.setExport(cfg)
		}
		if cs.Orphan {
			continue
		}
		if err := res.Errs[k]; err != nil {
			m.setStatus(true, str.StatusCharError(k, err))
			continue
		}
		if err := cs.compileLook(); err != nil {
			m.setStatus(true, str.StatusCharError(k, err))
		}
		if !reflect.DeepEqual(styleInputs(before[k]), styleInputs(cs.Ch)) {
			cs.sb.Rerender(cs.Rules.Reline, func(l app.Line) string { return paint(cs.hl, l) })
		}
	}
	if m.picker != nil {
		m.fixPick()
	}
}
```

`loadTheme`'s per-character loop: `if err := cs.compileLook(); err != nil { m.setStatus(true, str.StatusCharError(k, err)) }` in place of `cs.compile()`, looping `m.a.Order()`. (A theme change no longer recompiles a character's classify rules: they don't depend on the theme. A rules error was already reported when the config loaded.)

`handleEvent`:

```go
func (m *Model) handleEvent(msg app.SessionMsg) tea.Cmd {
	prev := m.a.Active()
	ev, ok, effs := m.a.Handle(msg)
	if !ok {
		return nil // stale session, or it has shut down
	}
	if ev.Closed {
		m.closed(ev.Key, prev)
		m.activated(prev)
		return nil
	}
	cs := m.chars[ev.Key]
	next := m.run(effs)
	switch ev.Ev.Kind {
	case session.EventLine:
		text := paint(cs.hl, ev.Line)
		switch {
		case !cs.echoes(ev.Ev.Entry):
		case ev.Line.Quiet:
			cs.sb.append(lineOf(text, ev.Line))
		default:
			cs.sb.AppendLine(lineOf(text, ev.Line))
		}
		if ev.Key == m.a.Active() {
			l := m.layout()
			cs.sb.SetWidth(l.rw) // measure at the pane's width, even before a View
			cs.sb.Pause(l.sbH)
		}
		for _, b := range cs.browses() {
			b.appendLive(ev.Ev.Entry)
		}
		if n := m.notifyCmd(cs, ev.Line); n != nil {
			return tea.Batch(n, next)
		}
	case session.EventPrompt:
		cs.sb.SetPrompt(ansi.Sanitize(ev.Ev.Entry.Text))
	case session.EventState:
		if ev.Ev.State != session.Connected && cs.needPW {
			cs.endPassword()
		}
	case session.EventNeedPassword:
		cs.startPassword()
	}
	return next
}
```

Delete the old `render` helper if nothing uses it after this (the tests' `cs.render` calls in `theme_test.go` become `paint(cs.hl, cs.Rules.Line(e))`; keep `render` if simpler — it's `func (cs *charState) render(e logstore.Entry) (string, app.Line) { l := cs.Rules.Line(e); return paint(cs.hl, l), l }`).

- [ ] **Step 3: Rename access paths in the rest of `ui`**

Let the compiler list them (`go build ./internal/ui`). Each is a mechanical rename:

| Old | New |
|---|---|
| `cs.key`, `cs.ch`, `cs.sess`, `cs.state`, `cs.rules`, `cs.unread`, `cs.attention`, `cs.pin`, `cs.orphan`, `cs.connectedAt` (any `*charState`, e.g. `b.cs.ch`, `pc.ch`) | `.Key`, `.Ch`, `.Sess`, `.State`, `.Rules`, `.Unread`, `.Attention`, `.Pin`, `.Orphan`, `.ConnectedAt` |
| `m.order` | `m.a.Order()` |
| `m.active` (reads) | `m.a.Active()` |
| `m.cfg` | `m.a.Config()` |
| `m.status` (reads) / `m.statusErr` | `m.a.Status().Text` / `m.a.Status().Err` |
| `m.status = ""` | `m.a.ClearStatus()` |
| `m.passwordStore()` | `m.a.PasswordStore()` |
| `m.logLayout(ch)` | `m.a.LogLayout(ch)` |
| `m.find(k)`, `m.allChars()`, `m.worldOpen(w)`, `m.activeWorld()`, `m.stops()` | `m.a.Find(k)`, `m.a.AllChars()`, `m.a.WorldOpen(w)`, `m.a.ActiveWorld()`, `m.a.Stops()` |
| `m.sideShown = m.active` | `m.sideShown = m.a.Active()` |
| `m.parked[m.active]` | `m.parked[m.a.Active()]` |

`/connect` etc. in `command` keep calling `m.connect(cs)`; `/close` calls `m.close(cs.Key)`. Any remaining write to `m.active` outside `switchTo`/`close` is a bug in this plan — stop and ledger it rather than reintroduce the field.

Run: `go build ./... && go vet ./internal/ui`
Expected: `go build` passes; `go vet` reports only test files (next step).

- [ ] **Step 4: Rename access paths in the tests**

Same table, plus:

| Old (tests) | New |
|---|---|
| `h.m.active` | `h.m.a.Active()` |
| `h.m.status` / `h.m.statusErr` | `h.m.a.Status().Text` / `h.m.a.Status().Err` |
| `h.m.status = ""` | `h.m.a.ClearStatus()` |
| `h.m.order` | `h.m.a.Order()` |
| `h.m.cfg` | `h.m.a.Config()` |
| `eventMsg{key: k, sess: s, ev: ev, ok: ok}` | `app.SessionMsg{Key: k, Sess: s, Ev: ev, OK: ok}` |
| `h.m.d.LogRoot = root` followed by `h.m.preload(cs)` | write the test's log files under the harness's own `h.m.d.LogRoot` instead of a separate `root` (build the `logstore.Writer` with `Root: h.m.d.LogRoot`, after `newHarness`), and drop the assignment |

The `LogRoot` row is a setup change: `app` took its own copy of the log root at `New`, so reassigning the ui's copy afterwards no longer reaches the preload. What the tests assert doesn't change. The affected tests are in `model_test.go` (three) and `theme_test.go` (two).

Run: `go vet ./... && go test ./...`
Expected: PASS everywhere — every `internal/ui` test including the goldens, `internal/app`, `internal/str`, `TestNoHardCodedStyles`.
If a test fails, the migration changed behavior: compare the old function (in `git show HEAD:internal/ui/model.go`) with the new path; don't change the test's expectation.

- [ ] **Step 5: Check the other builds**

Run: `cd web && GOOS=js GOARCH=wasm go build -o /dev/null ./cmd/kiln-web && go test ./... && cd .. && go build ./cmd/kiln`
Expected: all succeed; `ui.Deps` didn't change, so neither front end needed edits.

- [ ] **Step 6: Commit**

```bash
git add internal/ui
git commit -m "ui: Model drives app.App for characters, sessions, switching and status

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01PC4WGuqjdnGDVGdiaL3Z7n"
```
