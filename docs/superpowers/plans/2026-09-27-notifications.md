# OS Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When you're away from Kiln, activity on your characters becomes a desktop notification (OSC 9 and/or BEL), including over ssh, tmux and mosh.

**Architecture:** A pure `internal/notify` package builds the message and the wire sequence. Config gains an inheritable `notify` level plus global `notify_idle` and `notify_method`. The UI tracks focus (DECSET 1004) and the last time you were present, decides per incoming line whether to notify, and writes the sequence with `tea.Raw` through a `Deps.Raw` seam.

**Tech Stack:** Go, Bubble Tea v2 (`charm.land/bubbletea/v2`), BurntSushi-style TOML config already in `internal/config`.

**Spec:** `docs/superpowers/specs/2026-09-27-notifications-design.md`

## Global Constraints

- Levels are exactly `"all"`, `"first"`, `"attention"`, `"none"`; default `"first"`.
- `notify_method` is exactly `"osc"`, `"bell"`, `"both"`; default `"osc"`.
- `notify_idle` is a Go duration string; default `"5m"`; `"0"` disables the idle fallback; negative or unparsable is a config error.
- OSC 9 only: `ESC ] 9 ; message BEL`. tmux wrap: `ESC P tmux ; <payload with every ESC doubled> ESC \`. A lone BEL is never wrapped.
- Message: `Name: line`, or `Name@world: line` when another configured character in a different world has the same name (case-insensitive). ANSI stripped, then every C0/C1 control and DEL removed, cut to 200 characters with a trailing `…`.
- Only incoming (`logstore.In`), non-quiet lines notify. System lines never do.
- `/notify` override lasts until Kiln quits (survives reconnects and config reloads).
- No emoji in UI text; glyphs one cell wide (`…` is fine).
- Test with scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`, never the real setup. Use Kit/Rook/Ash and world `fm` in fixtures.

## Review Focus

- **MUD text with escape bytes** (a line containing `ESC ] 0 ;`, a raw BEL, or a C1 `0x9b`) must not break out of the OSC or reach the terminal: pinned in Task 1 (`TestMessageSanitizes`) and Task 4 (`TestNotifySanitizesInjection`).
- **A burst of lines while away at level `first`** must produce one notification, not one per line: pinned in Task 4 (`TestNotifyFirstOnlyOnce`).
- **Coming back without typing** (focus-in only) must re-arm `first`: pinned in Task 4 (`TestNotifyFirstRearmsOnFocus`).
- **Mouse hover over an unfocused window** must not count as presence: pinned in Task 4 (`TestMouseMotionIsNotPresence`).
- **A config reload** must not drop a `/notify` override: pinned in Task 5 (`TestNotifyOverrideSurvivesReload`).

---

### Task 1: `internal/notify` package

**Files:**
- Create: `internal/notify/notify.go`
- Test: `internal/notify/notify_test.go`

**Interfaces:**
- Produces:
  - `type Level string`; consts `All Level = "all"`, `First = "first"`, `Attention = "attention"`, `None = "none"`; `func ParseLevel(s string) (Level, error)`.
  - `type Method string`; consts `OSC Method = "osc"`, `Bell = "bell"`, `Both = "both"`; `func ParseMethod(s string) (Method, error)`.
  - `const MaxLen = 200`
  - `func Message(name, line string) string`
  - `func Encode(msg string, m Method, tmux bool) string`

- [ ] **Step 1: Write the failing tests**

```go
package notify

import (
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	for _, s := range []string{"all", "first", "attention", "none"} {
		if l, err := ParseLevel(s); err != nil || string(l) != s {
			t.Errorf("ParseLevel(%q) = %q, %v", s, l, err)
		}
	}
	if _, err := ParseLevel("loud"); err == nil || !strings.Contains(err.Error(), `"all", "first", "attention" or "none"`) {
		t.Errorf("bad level error = %v", err)
	}
}

func TestParseMethod(t *testing.T) {
	for _, s := range []string{"osc", "bell", "both"} {
		if m, err := ParseMethod(s); err != nil || string(m) != s {
			t.Errorf("ParseMethod(%q) = %q, %v", s, m, err)
		}
	}
	if _, err := ParseMethod("smoke"); err == nil {
		t.Error("ParseMethod accepted smoke")
	}
}

func TestMessageSanitizes(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"Rook pages: hi", "Kit: Rook pages: hi"},
		{"\x1b[1;31mred\x1b[0m text", "Kit: red text"},
		{"a\x1b]0;pwned\x07b", "Kit: ab"},
		{"bell\x07 tab\t cr\r", "Kit: bell tab cr"},
		{"c1\u009b31m csi", "Kit: c131m csi"},
		{"del\x7f", "Kit: del"},
		{"bad\xffutf8", "Kit: badutf8"},
		{"  spaced  ", "Kit: spaced"},
	} {
		if got := Message("Kit", c.line); got != c.want {
			t.Errorf("Message(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestMessageTruncates(t *testing.T) {
	got := Message("Kit", strings.Repeat("é", 300))
	if r := []rune(got); len(r) != MaxLen || r[len(r)-1] != '…' {
		t.Errorf("len %d, ends %q", len(r), string(r[len(r)-1]))
	}
	short := Message("Kit", strings.Repeat("x", MaxLen-5))
	if len([]rune(short)) != MaxLen || strings.HasSuffix(short, "…") {
		t.Errorf("exactly MaxLen should not be cut: %q", short)
	}
}

func TestEncode(t *testing.T) {
	for _, c := range []struct {
		m    Method
		tmux bool
		want string
	}{
		{OSC, false, "\x1b]9;hi\x07"},
		{Bell, false, "\x07"},
		{Both, false, "\x1b]9;hi\x07\x07"},
		{OSC, true, "\x1bPtmux;\x1b\x1b]9;hi\x07\x1b\\"},
		{Bell, true, "\x07"},
		{Both, true, "\x1bPtmux;\x1b\x1b]9;hi\x07\x1b\\\x07"},
	} {
		if got := Encode("hi", c.m, c.tmux); got != c.want {
			t.Errorf("Encode(%s, tmux=%v) = %q, want %q", c.m, c.tmux, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/notify/`
Expected: FAIL (build fails: undefined `ParseLevel`, `Message`, …)

- [ ] **Step 3: Write the implementation**

```go
// Package notify builds desktop notifications as terminal escape
// sequences: OSC 9 (iTerm2, kitty, Ghostty, WezTerm, foot, Blink) and the
// bell, optionally wrapped for tmux passthrough.
package notify

import (
	"errors"
	"strings"

	"github.com/latrani/Kiln/internal/ansi"
)

// Level is how much a character notifies while you're away.
type Level string

const (
	All       Level = "all"       // every incoming, non-quiet line
	First     Level = "first"     // the first line since you left, then attention lines
	Attention Level = "attention" // lines an attention rule matched
	None      Level = "none"
)

// ParseLevel checks a level name.
func ParseLevel(s string) (Level, error) {
	switch l := Level(s); l {
	case All, First, Attention, None:
		return l, nil
	}
	return "", errors.New(`notify must be "all", "first", "attention" or "none"`)
}

// Method is how a notification reaches the terminal.
type Method string

const (
	OSC  Method = "osc"  // OSC 9 with the message
	Bell Method = "bell" // a bare BEL; survives mosh, which strips OSC 9
	Both Method = "both"
)

// ParseMethod checks a method name.
func ParseMethod(s string) (Method, error) {
	switch m := Method(s); m {
	case OSC, Bell, Both:
		return m, nil
	}
	return "", errors.New(`notify_method must be "osc", "bell" or "both"`)
}

// MaxLen is the longest message, in characters.
const MaxLen = 200

// Message is "name: line" with the line made safe to put inside an OSC:
// ANSI sequences and every other control character are removed, and the
// result is cut to MaxLen characters.
func Message(name, line string) string {
	line = ansi.Strip(strings.ToValidUTF8(line, ""))
	line = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, line)
	msg := []rune(name + ": " + strings.TrimSpace(line))
	if len(msg) > MaxLen {
		msg = append(msg[:MaxLen-1], '…')
	}
	return string(msg)
}

// Encode returns the bytes to write for msg. Inside tmux the OSC is
// wrapped in a DCS passthrough (which needs allow-passthrough on); the
// bell needs no wrapping.
func Encode(msg string, m Method, tmux bool) string {
	var b strings.Builder
	if m == OSC || m == Both {
		osc := "\x1b]9;" + msg + "\a"
		if tmux {
			osc = "\x1bPtmux;" + strings.ReplaceAll(osc, "\x1b", "\x1b\x1b") + "\x1b\\"
		}
		b.WriteString(osc)
	}
	if m == Bell || m == Both {
		b.WriteByte('\a')
	}
	return b.String()
}
```

Note on the `"  spaced  "` case: `name + ": " + TrimSpace(line)` gives `"Kit: spaced"`. For `"bell\x07 tab\t cr\r"`, removing BEL, TAB and CR leaves `"bell tab cr"`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/notify/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/notify
git commit -m "feat(notify): message sanitizing and OSC 9 / bell encoding"
```

---

### Task 2: Config keys

**Files:**
- Modify: `internal/config/config.go` (Character, Config, settings, overlay, globalFile, loadGlobal base, Load, loadWorldData, validate)
- Modify: `internal/config/edit.go` (WorldSettings, CharacterSettings, Inherited, inherited, ReadWorld, WriteWorld, ReadCharacter, WriteCharacter)
- Modify: `internal/config/defaults/config.toml`
- Modify: `README.md` (settings table)
- Test: `internal/config/config_test.go`, `internal/config/edit_test.go`

**Interfaces:**
- Consumes: `notify.Level`, `notify.ParseLevel`, `notify.Method`, `notify.ParseMethod`, `notify.First`, `notify.OSC` (Task 1).
- Produces:
  - `config.Character.Notify notify.Level`
  - `config.Config.NotifyIdle time.Duration`, `config.Config.NotifyMethod notify.Method`
  - `config.DefaultNotifyIdle = 5 * time.Minute`
  - `WorldSettings.Notify *string`, `CharacterSettings.Notify *string`, `Inherited.Notify string`

- [ ] **Step 1: Write the failing tests** (append to `internal/config/config_test.go`; add `"time"` and `"github.com/latrani/Kiln/internal/notify"` to its imports if missing)

```go
func TestNotifyInheritsAndDefaults(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"config.toml":   "[defaults]\nnotify = \"attention\"\n",
		"worlds/a.toml": "host = \"h\"\nport = 1\nnotify = \"all\"\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n[[characters]]\nid = \"rook\"\nname = \"Rook\"\nnotify = \"none\"\n",
		"worlds/b.toml": "host = \"h\"\nport = 1\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n",
	})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		world, char string
		want        notify.Level
	}{{"a", "kit", notify.All}, {"a", "rook", notify.None}, {"b", "ash", notify.Attention}} {
		ch, _ := cfg.Find(c.world, c.char)
		if ch.Notify != c.want {
			t.Errorf("%s/%s Notify = %q, want %q", c.world, c.char, ch.Notify, c.want)
		}
	}
}

func TestNotifyGlobalDefaults(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"worlds/b.toml": "host = \"h\"\nport = 1\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n"})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	ash, _ := cfg.Find("b", "ash")
	if ash.Notify != notify.First || cfg.NotifyIdle != 5*time.Minute || cfg.NotifyMethod != notify.OSC {
		t.Errorf("notify %q, idle %v, method %q", ash.Notify, cfg.NotifyIdle, cfg.NotifyMethod)
	}
}

func TestNotifyGlobalKeys(t *testing.T) {
	for _, c := range []struct {
		toml   string
		idle   time.Duration
		method notify.Method
		bad    bool
	}{
		{"notify_idle = \"90s\"\nnotify_method = \"both\"\n", 90 * time.Second, notify.Both, false},
		{"notify_idle = \"0\"\n", 0, notify.OSC, false},
		{"notify_idle = \"-1m\"\n", 0, "", true},
		{"notify_idle = \"soon\"\n", 0, "", true},
		{"notify_method = \"smoke\"\n", 0, "", true},
	} {
		dir := t.TempDir()
		write(t, dir, map[string]string{"config.toml": c.toml})
		cfg, err := Load(dir)
		if c.bad {
			if err == nil {
				t.Errorf("%q: loaded", c.toml)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.toml, err)
			continue
		}
		if cfg.NotifyIdle != c.idle || cfg.NotifyMethod != c.method {
			t.Errorf("%q: idle %v, method %q", c.toml, cfg.NotifyIdle, cfg.NotifyMethod)
		}
	}
}

func TestNotifyLevelValidated(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"worlds/a.toml": "host = \"h\"\nport = 1\nnotify = \"loud\"\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n"})
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "notify must be") {
		t.Errorf("err = %v", err)
	}
}
```

Append to `internal/config/edit_test.go`:

```go
func TestNotifyRoundTrips(t *testing.T) {
	dir := editDir(t, uglyWorld)
	s, inh, err := ReadWorld(dir, "fm")
	if err != nil {
		t.Fatal(err)
	}
	if s.Notify != nil || inh.Notify != "first" {
		t.Fatalf("world notify %v, inherited %q", s.Notify, inh.Notify)
	}
	all := "all"
	s.Notify = &all
	if err := WriteWorld(dir, "fm", s); err != nil {
		t.Fatal(err)
	}
	c, inh, err := ReadCharacter(dir, "fm", "Kit")
	if err != nil {
		t.Fatal(err)
	}
	if inh.Notify != "all" {
		t.Errorf("character inherits %q, want all", inh.Notify)
	}
	none := "none"
	c.Notify = &none
	if err := WriteCharacter(dir, "fm", "Kit", c); err != nil {
		t.Fatal(err)
	}
	got := readWorld(t, dir)
	if !strings.Contains(got, "notify = \"all\"") || !strings.Contains(got, "notify = \"none\"") {
		t.Errorf("file:\n%s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL (build: `ch.Notify undefined`, `cfg.NotifyIdle undefined`, `s.Notify undefined`)

- [ ] **Step 3: Implement in `internal/config/config.go`**

Add imports `"time"` and `"github.com/latrani/Kiln/internal/notify"`.

In `Character`, after `Reconnect`:

```go
	Notify       notify.Level // what notifies while you're away
```

In `Config`, after `PasswordStore`:

```go
	NotifyIdle    time.Duration // no input for this long counts as away; 0: only blur does
	NotifyMethod  notify.Method
```

In `settings`, after `Reconnect`:

```go
	Notify       *string `toml:"notify"`
```

In `overlay`, after the `Reconnect` block:

```go
	if o.Notify != nil {
		s.Notify = o.Notify
	}
```

In `globalFile`, after `PasswordStore`:

```go
	NotifyIdle    string   `toml:"notify_idle"`
	NotifyMethod  string   `toml:"notify_method"`
```

In the defaults `const` block:

```go
	DefaultNotifyIdle = 5 * time.Minute
```

(Put it in its own `var`-free const line; `time.Duration` constants are fine in a const block.)

In `loadGlobal`, add `Notify: ptr(string(notify.First))` to the `base := settings{...}` literal.

In `Load`, after the `export_format` switch:

```go
	idle := DefaultNotifyIdle
	if g.NotifyIdle != "" {
		if idle, err = time.ParseDuration(g.NotifyIdle); err != nil || idle < 0 {
			return nil, errors.New(`config.toml: notify_idle must be a duration like "5m" ("0" turns it off)`)
		}
	}
	method := notify.OSC
	if g.NotifyMethod != "" {
		if method, err = notify.ParseMethod(g.NotifyMethod); err != nil {
			return nil, fmt.Errorf("config.toml: %w", err)
		}
	}
```

and add `NotifyIdle: idle, NotifyMethod: method` to the `cfg := &Config{...}` literal.

In `loadWorldData`'s `ch := Character{...}`, add `Notify: notify.Level(*cs.Notify),`.

In `validate`, after the `newline_mode` check:

```go
	if _, err := notify.ParseLevel(string(ch.Notify)); err != nil {
		return err
	}
```

- [ ] **Step 4: Implement in `internal/config/edit.go`**

- `WorldSettings`: add `Notify *string` after `Reconnect`.
- `CharacterSettings`: add `Notify *string` after `Reconnect`.
- `Inherited`: add `Notify string` after `Reconnect`.
- `inherited`: add `Notify: *s.Notify` to the literal.
- `ReadWorld`: add `Notify: wf.Notify` to the returned literal.
- `WriteWorld` and `WriteCharacter`: after the `reconnect` change line, add
  `change("notify", tomlOptString(old.Notify), tomlOptString(s.Notify))`.
- `ReadCharacter`: add `Notify: cf.Notify` to the returned literal.

- [ ] **Step 5: Defaults file and README**

In `internal/config/defaults/config.toml`, before `[defaults]`:

```toml
# Notifications while you're away (see README): no input for notify_idle
# also counts as away ("0": only switching away does), and notify_method
# is "osc" (a notification with the line), "bell" (works over mosh) or
# "both".
# notify_idle = "5m"
# notify_method = "osc"
```

and under `[defaults]`, after `reconnect = true`:

```toml
notify = "first"        # "all", "first" (first line while away, then pages), "attention" or "none"
```

In `README.md`'s settings table, after the `password_store` row:

```markdown
| `notify_idle` | config.toml | No input for this long counts as away (default `"5m"`; `"0"`: only switching away counts), see [Notifications](#notifications) |
| `notify_method` | config.toml | `"osc"` (default): a notification with the line; `"bell"`: a bell (works over mosh); `"both"` |
```

and after the `reconnect` row:

```markdown
| `notify` | any level | What notifies while you're away: `"all"`, `"first"` (default: the first line, then attention lines), `"attention"` or `"none"` |
```

- [ ] **Step 6: Run tests**

Run: `gofmt -l internal && go test ./internal/config/ ./internal/ui/`
Expected: no gofmt output; PASS. (The UI tests load the bundled defaults, so they confirm the defaults file parses.)

- [ ] **Step 7: Commit**

```bash
git add internal/config README.md
git commit -m "feat(config): notify, notify_idle and notify_method"
```

---

### Task 3: Editor fields

**Files:**
- Modify: `internal/ui/editor.go` (worldForm, worldSettings, charForm, saveCharSettings)
- Test: `internal/ui/editor_test.go`

**Interfaces:**
- Consumes: `WorldSettings.Notify`, `CharacterSettings.Notify`, `Inherited.Notify` (Task 2).

- [ ] **Step 1: Write the failing test** (append to `internal/ui/editor_test.go`)

```go
func TestEditCharacterNotify(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/edit")
	h.enter()
	h.focusOn(extraLabel)
	h.enter()
	h.focusOn("Notify")
	if !strings.Contains(h.screen(), "default (first)") {
		t.Errorf("Notify should show what it inherits:\n%s", h.screen())
	}
	h.press(tea.KeyLeft, 0) // default → none, backwards
	h.focusOn(saveLabel)
	h.enter()
	if got := h.worldFile("fm"); !strings.Contains(got, "name = \"Kit\"\nautoconnect = true\nnotify = \"none\"\n") {
		t.Errorf("file:\n%s", got)
	}
	if kit := h.m.chars["fm/kit"]; kit.ch.Notify != "none" {
		t.Errorf("Notify = %q", kit.ch.Notify)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/ui/ -run TestEditCharacterNotify`
Expected: FAIL with `can't reach "Notify"`.

- [ ] **Step 3: Implement**

In `worldForm`, after the `Reconnect` inheritChoice line:

```go
		asExtra(inheritChoice("Notify", inh.Notify, "all", "first", "attention", "none")),
```

and after `f.setBool("Reconnect", s.Reconnect)`:

```go
	if s.Notify != nil {
		f.choose(f.field("Notify"), *s.Notify)
	}
```

In `worldSettings`, after the `Newlines` block:

```go
	if v := f.chosen(f.field("Notify")); v != "" {
		s.Notify = &v
	}
```

In `charForm`, the same inheritChoice line after `Reconnect`, and the same `if s.Notify != nil { f.choose(...) }` after `f.setBool("Reconnect", s.Reconnect)`.

In `saveCharSettings`, after building `s`:

```go
	if v := f.chosen(f.field("Notify")); v != "" {
		s.Notify = &v
	}
```

- [ ] **Step 4: Run the UI tests**

Run: `go test ./internal/ui/`
Expected: PASS. If `TestEditWorldFromPicker` fails with "extras didn't unfold" because the taller form pushed `Reconnect` off the 24-row screen, change that assertion's `"Reconnect"` to `"Autoconnect"` (it only checks that the extras unfolded).

- [ ] **Step 5: Commit**

```bash
git add internal/ui/editor.go internal/ui/editor_test.go
git commit -m "feat(ui): Notify field in the world and character editors"
```

---

### Task 4: Away tracking and sending notifications

**Files:**
- Create: `internal/ui/notify.go`
- Modify: `internal/ui/model.go` (Deps, Model, charState, New, update, handleEvent)
- Modify: `internal/ui/view.go` (View sets ReportFocus)
- Modify: `internal/ui/model_test.go` (harness clock and Raw capture)
- Modify: `cmd/kiln/main.go` (Deps.Tmux)
- Test: `internal/ui/notify_test.go`

**Interfaces:**
- Consumes: `notify.Message`, `notify.Encode`, `notify.Level` consts (Task 1); `Character.Notify`, `Config.NotifyIdle`, `Config.NotifyMethod` (Task 2).
- Produces:
  - `Deps.Tmux bool`, `Deps.Raw func(seq string) tea.Cmd`
  - `Model.focused bool`, `Model.lastHere time.Time`
  - `charState.firstSent time.Time`, `charState.notifyOverride notify.Level` (empty: none)
  - `func (cs *charState) notifyLevel() notify.Level`
  - `func (m *Model) here()`
  - `func (m *Model) away() bool`
  - `func (m *Model) notifyName(cs *charState) string`
  - `func (m *Model) notifyCmd(cs *charState, e logstore.Entry, res rules.Result) tea.Cmd`

- [ ] **Step 1: Give the harness a movable clock and capture raw writes** (`internal/ui/model_test.go`)

Add fields to `harness`:

```go
	now time.Time // what Deps.Now returns; tests may move it
	raw []string  // sequences written with Deps.Raw
```

In `newHarness`, set `h.now = time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local)` right after `h := &harness{...}`, and replace the `Now:` line in `Deps` with:

```go
		Now: func() time.Time {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.now
		},
		Raw: func(seq string) tea.Cmd {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.raw = append(h.raw, seq)
			return nil
		},
```

Add helpers:

```go
// advance moves the clock forward.
func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.now = h.now.Add(d)
}

// notified returns and clears what has been written with Deps.Raw.
func (h *harness) notified() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.raw
	h.raw = nil
	return r
}
```

- [ ] **Step 2: Write the failing tests** (`internal/ui/notify_test.go`)

```go
package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// notifyHarness has Kit connected with the given notify level (and
// optional extra world files), past its login and connect notices.
func notifyHarness(t *testing.T, level string, extra map[string]string) *harness {
	t.Helper()
	w := strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nnotify = \""+level+"\"", 1)
	worlds := map[string]string{"fm": w}
	for k, v := range extra {
		worlds[k] = v
	}
	h := newHarness(t, worlds)
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.notified()
	return h
}

// line feeds one incoming line for Kit and waits until it's shown.
func (h *harness) line(s string) {
	h.t.Helper()
	h.conn("fm/kit").lines <- s
	want := h.m.chars["fm/kit"].sb.Len() + 1
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].sb.Len() >= want })
}

func osc(msg string) string { return "\x1b]9;" + msg + "\x07" }

func TestNoNotifyWhileHere(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.line("Rook says, \"hi\"")
	if got := h.notified(); len(got) != 0 {
		t.Errorf("notified while focused: %q", got)
	}
}

func TestNotifyAllWhileBlurred(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \"hi\"")
	h.line("Rook says, \"again\"")
	want := []string{osc("Kit: Rook says, \"hi\""), osc("Kit: Rook says, \"again\"")}
	if got := h.notified(); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNotifyFirstOnlyOnce(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.m.Update(tea.BlurMsg{})
	h.advance(time.Second)
	h.line("Rook says, \"one\"")
	h.line("Rook says, \"two\"")
	h.line("Mira pages: you around?") // attention still gets through
	want := []string{osc("Kit: Rook says, \"one\""), osc("Kit: Mira pages: you around?")}
	if got := h.notified(); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNotifyFirstRearmsOnFocus(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.m.Update(tea.BlurMsg{})
	h.advance(time.Second)
	h.line("Rook says, \"one\"")
	h.advance(time.Second)
	h.m.Update(tea.FocusMsg{})
	h.advance(time.Second)
	h.m.Update(tea.BlurMsg{})
	h.advance(time.Second)
	h.line("Rook says, \"two\"")
	want := []string{osc("Kit: Rook says, \"one\""), osc("Kit: Rook says, \"two\"")}
	if got := h.notified(); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNotifyFirstRearmsOnKey(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.advance(10 * time.Minute) // idle: away without a blur
	h.line("Rook says, \"one\"")
	h.advance(time.Second) // come back strictly after the notification
	h.typeText("x")
	h.advance(10 * time.Minute)
	h.line("Rook says, \"two\"")
	if got := h.notified(); len(got) != 2 {
		t.Errorf("got %q, want two notifications", got)
	}
}

func TestNotifyAttentionOnly(t *testing.T) {
	h := notifyHarness(t, "attention", nil)
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \"hi\"")
	h.line("Mira pages: you around?")
	if got := h.notified(); !slices.Equal(got, []string{osc("Kit: Mira pages: you around?")}) {
		t.Errorf("got %q", got)
	}
}

func TestNotifyNone(t *testing.T) {
	h := notifyHarness(t, "none", nil)
	h.m.Update(tea.BlurMsg{})
	h.line("Mira pages: you around?")
	if got := h.notified(); len(got) != 0 {
		t.Errorf("got %q", got)
	}
}

func TestNotifyIdleFallback(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.advance(4 * time.Minute)
	h.line("Rook says, \"early\"")
	h.advance(2 * time.Minute) // 6 minutes without input
	h.line("Rook says, \"late\"")
	if got := h.notified(); !slices.Equal(got, []string{osc("Kit: Rook says, \"late\"")}) {
		t.Errorf("got %q", got)
	}
}

func TestNotifyIdleOff(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.m.cfg.NotifyIdle = 0
	h.advance(time.Hour)
	h.line("Rook says, \"hi\"")
	if got := h.notified(); len(got) != 0 {
		t.Errorf("idle 0 should never count as away: %q", got)
	}
}

func TestMouseMotionIsNotPresence(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.advance(10 * time.Minute)
	h.m.Update(tea.MouseMotionMsg{X: 40, Y: 5})
	h.line("Rook says, \"hi\"")
	if got := h.notified(); len(got) != 1 {
		t.Errorf("hover should not count as being here: %q", got)
	}
}

func TestQuietAndSentLinesDontNotify(t *testing.T) {
	quiet := strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nnotify = \"all\"", 1) +
		"\n[[highlight]]\nmatch = { pattern = '^Rook' }\nquiet = true\n"
	h := newHarness(t, map[string]string{"fm": quiet})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.notified()
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \"shh\"")
	if got := h.notified(); len(got) != 0 {
		t.Errorf("quiet line notified: %q", got)
	}
}

func TestNotifyNameDisambiguates(t *testing.T) {
	other := "host = \"h\"\nport = 1\n[[characters]]\nid = \"kit\"\nname = \"kit\"\n"
	h := notifyHarness(t, "all", map[string]string{"sp": other})
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \"hi\"")
	if got := h.notified(); !slices.Equal(got, []string{osc("Kit@fm: Rook says, \"hi\"")}) {
		t.Errorf("got %q", got)
	}
}

func TestNotifySanitizesInjection(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \x1b]0;pwned\x07\"hi\"\x07\u009b")
	got := h.notified()
	if !slices.Equal(got, []string{osc("Kit: Rook says, \"hi\"")}) {
		t.Errorf("got %q", got)
	}
}

func TestNotifyTmuxAndMethod(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.m.d.Tmux = true
	h.m.cfg.NotifyMethod = "both"
	h.m.Update(tea.BlurMsg{})
	h.line("hi")
	want := "\x1bPtmux;\x1b\x1b]9;Kit: hi\x07\x1b\\\x07"
	if got := h.notified(); !slices.Equal(got, []string{want}) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestViewReportsFocus(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	if !h.m.View().ReportFocus {
		t.Error("focus reporting is off")
	}
}
```

`Scrollback.Len()` already exists (`internal/ui/scrollback.go`). Also check the fuzzball pack's page rule matches `"Mira pages: you around?"`: `TestIncomingLinesHighlightAndBadges` already relies on it setting attention, so it does.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'Notify|MouseMotion|ReportsFocus|QuietAndSent'`
Expected: FAIL (build: `Deps.Raw` undefined, `tea.BlurMsg` not handled, …)

- [ ] **Step 4: Implement `internal/ui/notify.go`**

```go
package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/rules"
)

// notifyLevel is the /notify override, or the configured level.
func (cs *charState) notifyLevel() notify.Level {
	if cs.notifyOverride != "" {
		return cs.notifyOverride
	}
	return cs.ch.Notify
}

// here records that you're at the terminal: a focus-in, key, paste,
// click or wheel. It re-arms "first" for every character.
func (m *Model) here() { m.lastHere = m.d.Now() }

// away reports whether you've switched away, or been idle past
// notify_idle.
func (m *Model) away() bool {
	if !m.focused {
		return true
	}
	idle := config.DefaultNotifyIdle
	if m.cfg != nil {
		idle = m.cfg.NotifyIdle
	}
	return idle > 0 && m.d.Now().Sub(m.lastHere) > idle
}

// notifyName is the character's name, with its world when another
// world has a character of the same name.
func (m *Model) notifyName(cs *charState) string {
	for _, ch := range m.allChars() {
		if ch.World != cs.ch.World && strings.EqualFold(ch.Name, cs.ch.Name) {
			return cs.ch.Name + "@" + cs.ch.World
		}
	}
	return cs.ch.Name
}

// notifyCmd writes a notification for an incoming line if you're away
// and cs's level wants one; otherwise it returns nil.
func (m *Model) notifyCmd(cs *charState, e logstore.Entry, res rules.Result) tea.Cmd {
	if e.Dir != logstore.In || res.Quiet || !m.away() {
		return nil
	}
	switch cs.notifyLevel() {
	case notify.All:
	case notify.First:
		if !res.Attention && !cs.firstSent.Before(m.lastHere) {
			return nil
		}
	case notify.Attention:
		if !res.Attention {
			return nil
		}
	default:
		return nil
	}
	cs.firstSent = m.d.Now()
	method := notify.OSC
	if m.cfg != nil {
		method = m.cfg.NotifyMethod
	}
	return m.d.Raw(notify.Encode(notify.Message(m.notifyName(cs), e.Text), method, m.d.Tmux))
}
```

- [ ] **Step 5: Wire it into `internal/ui/model.go`**

In `Deps`, after `OpenURL`:

```go
	Tmux           bool                                            // inside tmux: wrap notifications for passthrough
	Raw            func(seq string) tea.Cmd                        // writes straight to the terminal; default tea.Raw
```

In `Model`, after `lastClick`'s block (before the closing brace):

```go
	focused  bool      // the terminal has focus, as far as we know
	lastHere time.Time // latest focus-in or input; see here
```

In `charState`, after `leftover`:

```go
	firstSent      time.Time    // when the last notification went out
	notifyOverride notify.Level // from /notify until Kiln quits; "" if none
```

Add the `notify` import to `model.go`.

In `New`, after the `d.Now` default:

```go
	if d.Raw == nil {
		d.Raw = func(seq string) tea.Cmd { return tea.Raw(seq) }
	}
```

and after `m := &Model{...}`:

```go
	m.focused, m.lastHere = true, d.Now()
```

In `update`, add cases:

```go
	case tea.FocusMsg:
		m.focused = true
		m.here()
	case tea.BlurMsg:
		m.focused = false
```

and call `m.here()` at the top of the existing `tea.PasteMsg`, `tea.KeyPressMsg`, `tea.MouseWheelMsg` and `tea.MouseClickMsg` cases (before their current bodies). Do not touch `tea.MouseMotionMsg`.

In `handleEvent`, in the `session.EventLine` case, after the unread/attention `if` block:

```go
		if n := m.notifyCmd(cs, ev.Entry, res); n != nil {
			return tea.Batch(n, waitEvent(msg.key, msg.sess))
		}
```

- [ ] **Step 6: Turn on focus reporting in `internal/ui/view.go`**

Change the first line of `View`:

```go
	v := tea.View{AltScreen: true, MouseMode: tea.MouseModeAllMotion, ReportFocus: true} // all motion: links light up on hover; focus: notifications
```

- [ ] **Step 7: Set `Tmux` in `cmd/kiln/main.go`**

In the `ui.Deps{...}` literal, add:

```go
		Tmux: os.Getenv("TMUX") != "",
```

- [ ] **Step 8: Run all tests**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output; PASS everywhere.

- [ ] **Step 9: Commit**

```bash
git add internal/ui cmd/kiln
git commit -m "feat(ui): desktop notifications while you're away"
```

---

### Task 5: `/notify` command and README

**Files:**
- Modify: `internal/ui/model.go` (command switch)
- Modify: `internal/ui/notify.go` (notifyCommand)
- Modify: `README.md` (commands table, Notifications section)
- Test: `internal/ui/notify_test.go`

**Interfaces:**
- Consumes: `charState.notifyOverride`, `charState.notifyLevel`, `notify.ParseLevel` (Tasks 1, 4).
- Produces: `func (m *Model) notifyCommand(cs *charState, args []string)`

- [ ] **Step 1: Write the failing tests** (append to `internal/ui/notify_test.go`)

```go
func TestNotifyCommand(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.typeText("/notify")
	h.enter()
	if !strings.Contains(h.screen(), "Kit notify: first") {
		t.Errorf("status:\n%s", h.screen())
	}
	h.typeText("/notify none")
	h.enter()
	if lvl := h.m.chars["fm/kit"].notifyLevel(); lvl != "none" {
		t.Errorf("level = %q", lvl)
	}
	h.typeText("/notify")
	h.enter()
	if !strings.Contains(h.screen(), "Kit notify: none (override; config says first)") {
		t.Errorf("status:\n%s", h.screen())
	}
	h.m.Update(tea.BlurMsg{})
	h.line("Mira pages: you around?")
	if got := h.notified(); len(got) != 0 {
		t.Errorf("override ignored: %q", got)
	}
	h.m.Update(tea.FocusMsg{})
	h.typeText("/notify default")
	h.enter()
	if lvl := h.m.chars["fm/kit"].notifyLevel(); lvl != "first" {
		t.Errorf("after default, level = %q", lvl)
	}
	h.typeText("/notify loud")
	h.enter()
	if !strings.Contains(h.screen(), `notify must be "all", "first", "attention" or "none"`) {
		t.Errorf("status:\n%s", h.screen())
	}
}

func TestNotifyOverrideSurvivesReload(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.typeText("/notify all")
	h.enter()
	if !h.m.reloadNow() {
		t.Fatal("reload failed")
	}
	if lvl := h.m.chars["fm/kit"].notifyLevel(); lvl != "all" {
		t.Errorf("after reload, level = %q", lvl)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/ui/ -run 'NotifyCommand|SurvivesReload'`
Expected: FAIL with status `unknown command /notify`.

- [ ] **Step 3: Implement**

Append to `internal/ui/notify.go` (add `"fmt"` to its imports):

```go
// notifyCommand is /notify: show the level, set an override until Kiln
// quits, or drop it with "default".
func (m *Model) notifyCommand(cs *charState, args []string) {
	if len(args) == 0 {
		shown := string(cs.notifyLevel())
		if cs.notifyOverride != "" {
			shown = fmt.Sprintf("%s (override; config says %s)", cs.notifyOverride, cs.ch.Notify)
		}
		m.setStatus(false, "%s notify: %s", cs.ch.Name, shown)
		return
	}
	if args[0] == "default" {
		cs.notifyOverride = ""
		m.setStatus(false, "%s notify: %s", cs.ch.Name, cs.ch.Notify)
		return
	}
	l, err := notify.ParseLevel(args[0])
	if err != nil {
		m.setStatus(true, "%v", err)
		return
	}
	cs.notifyOverride = l
	m.setStatus(false, "%s notify: %s until Kiln quits", cs.ch.Name, l)
}
```

In `model.go`'s `command` switch, before `default:`:

```go
	case "/notify":
		m.notifyCommand(cs, args[1:])
```

- [ ] **Step 4: README**

In the commands table, after the `/highlight` row:

```markdown
| `/notify [level]` | Show or set (until Kiln quits) what notifies for this character: `all`, `first`, `attention`, `none`, or `default` to go back to the config |
```

Add a section after `### Log mode` (before `## Where Kiln keeps things`):

```markdown
### Notifications

When you're away from Kiln, activity shows up as a desktop notification like `Kit: Rook pages: you around?` (`Kit@fm:` when two worlds have a Kit). You're away when you switch to another window or tab, or after `notify_idle` (default 5 minutes) without typing or clicking. The `notify` setting picks what notifies: `first` (the default) sends the first line since you left and then only lines that need attention (pages and whispers), `all` sends every line, `attention` only those, and `none` nothing. Lines hidden by a `quiet` rule never notify. `/notify` changes it for one character until Kiln quits.

Notifications work in iTerm2, kitty, Ghostty, WezTerm, foot and Blink, locally or over ssh. Inside tmux, add this to `~/.tmux.conf`:

    set -g allow-passthrough on
    set -g focus-events on

Mosh drops notifications, but it passes on the bell: set `notify_method = "both"` and turn on Blink's "Notification on background shell" to get an alert (without the line).
```

- [ ] **Step 5: Run all tests**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output; PASS.

- [ ] **Step 6: Manual smoke test** (scratch config, never the real one)

```bash
export XDG_CONFIG_HOME=$(mktemp -d) XDG_DATA_HOME=$(mktemp -d)
go run ./cmd/kiln
```

Add a world pointing at a MUCK you can reach, connect, set `/notify all`, switch to another app and have something happen in the room: an iTerm2 notification should appear. Repeat inside tmux with the two settings above. Over mosh from Blink, set `notify_method = "both"` and confirm Blink's bell alert (this checks mosh relays the bell).

- [ ] **Step 7: Commit**

```bash
git add internal/ui README.md
git commit -m "feat(ui): /notify command; document notifications"
```
