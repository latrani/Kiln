# Sidebar Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The sidebar shows only open characters (connected, or disconnected and not yet cleared), with an add-connection picker for everything else (#40), shaped so #41 can add world/character editors.

**Architecture:** `m.chars`/`m.order` hold only *open* characters; `open(k)`/`close(k)` manage that set and the model keeps the loaded `*config.Config` to list the rest. Sidebar rows get a `kind`. The picker swaps the sidebar's rows for every unopened configured character and puts a one-field `form` (a filter) in the input area's prompt state. With nothing open, an idle input shows an empty-state prompt.

**Tech Stack:** Go 1.27, Bubble Tea v2 (`charm.land/bubbletea/v2`), `github.com/charmbracelet/x/ansi`.

**Spec:** `docs/superpowers/specs/2026-09-25-sidebar-redesign-design.md`

## Global Constraints

- No new dependencies.
- Test fixtures use only the suite's names: worlds `fm`, `sp`, `big`; characters Kit, Rook, Ash, `bo`, C00–C29. Never real players' character names.
- Commit straight to `main`; never push or open PRs. Every commit message ends with the two attribution lines:
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E`
- Exact UI strings: `+ Add connection`; filter label `Filter`; filter hint `Enter to connect · Esc to close`; empty state `Nothing open · Enter or Ctrl+O to add a connection`; picker with nothing to offer `No matches`; badges: connected = blank, connecting = `…`, disconnected/failed = `✕`; activity = ` 3` or ` ● 3`.
- Sidebar and picker order: worlds A–Z by id, then characters A–Z by name, both case-insensitive.
- Names (characters, worlds, picker rows, `+ Add connection`) truncate with `…`; the activity text is never truncated.
- Run `gofmt -w` on changed Go files, then `gofmt -l .` (must print nothing) and `go test -race ./...` before each commit.

## Review Focus

- Ctrl+O or clicking `+ Add connection` while the active character is in browse mode: nothing opens; the status says `leave browse mode (Esc) to add a connection`. (Test in Task 4.)
- Picker with nothing to offer (all open, or no filter match): the sidebar shows `No matches` and Enter does nothing. (Task 4.)
- Closing the last open character: the empty-state prompt appears, and ↑, ↓, PgUp, Esc and typing don't panic. (Task 5.)
- A config reload removes the character highlighted in the picker: the highlight moves to the first match (or none), and Enter never opens a vanished character. (Task 4.)
- A late session event for a character that was closed: ignored, and it doesn't reappear. (Task 1.)

## File Structure

- Create `internal/ui/sidebar.go`: what the sidebar holds (`open`, `close`, `find`, `allChars`, `compareChars`, `sortOrder`) and how it's drawn (row kinds, `sidebarRows`, `sideView`, `sidebarView`, `scrollSidebar`, `sidebarLine`, `fitName`). Sidebar code moves here from `view.go`.
- Create `internal/ui/form.go`: `editKey` (shared line-editing keys), `field`, `form`.
- Create `internal/ui/picker.go`: the add-connection picker.
- Create tests `internal/ui/sidebar_test.go`, `internal/ui/form_test.go`, `internal/ui/picker_test.go`. They use the existing `harness` in `model_test.go` (same package).
- Modify `internal/ui/model.go` (lifecycle, keys, clicks, commands), `internal/ui/view.go` (layout, prompt, View), `internal/ui/keys.go` (`openPickerKey`), `internal/ui/model_test.go` and `internal/ui/browse_test.go` (fixtures that assumed every character is in the sidebar), `README.md`.

---

### Task 1: Open/close lifecycle and alphabetical order

**Files:**
- Create: `internal/ui/sidebar.go`
- Create: `internal/ui/sidebar_test.go`
- Modify: `internal/ui/model.go` (`Model` struct; `New`; `applyConfig`; delete `insertInWorld`, `drop`, `fixActive`; `handleEvent`'s orphan drop)
- Modify: `internal/ui/model_test.go`, `internal/ui/browse_test.go` (fixtures)

**Interfaces:**
- Produces:
  - `func compareChars(a, b config.Character) int`
  - `func (m *Model) allChars() []config.Character` (every configured character, sorted)
  - `func (m *Model) find(k string) (config.Character, bool)`
  - `func (m *Model) open(k string) *charState` (nil if `k` isn't configured; doesn't connect; makes `k` active if nothing is)
  - `func (m *Model) close(k string)`
  - `func (m *Model) sortOrder()`
  - field `Model.cfg *config.Config`
  - test helpers `func (h *harness) open(keys ...string)` and `func (h *harness) openAll()`

- [ ] **Step 1: Write the failing tests**

Create `internal/ui/sidebar_test.go`:

```go
package ui

import (
	"strings"
	"testing"
	"time"
)

func TestStartupOpensOnlyAutoconnect(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if got := strings.Join(h.m.order, " "); got != "fm/kit" {
		t.Errorf("order = %q, want only the autoconnect character", got)
	}
	if h.m.active != "fm/kit" {
		t.Errorf("active = %q", h.m.active)
	}
}

func TestOpenKeepsAlphabeticalOrder(t *testing.T) {
	// Case-sensitively these would sort Sp < fm and Kit < Rook < bo.
	fm := fmWorld + "\n[[characters]]\nid = \"bo\"\nname = \"bo\"\n"
	sp := "host = \"sp.test\"\nport = 1\n\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n"
	h := newHarness(t, map[string]string{"fm": fm, "Sp": sp})
	h.open("Sp/ash", "fm/rook", "fm/bo")
	if got, want := strings.Join(h.m.order, " "), "fm/bo fm/kit fm/rook Sp/ash"; got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
	if h.m.open("fm/nobody") != nil {
		t.Error("opened a character that isn't configured")
	}
}

func TestCloseHandsActiveToNeighbor(t *testing.T) {
	sp := "host = \"sp.test\"\nport = 1\n\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n"
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": sp})
	h.open("fm/rook", "sp/ash")
	h.m.switchTo("fm/rook")
	h.m.close("fm/rook") // the next one down
	if h.m.active != "sp/ash" || h.m.chars["fm/rook"] != nil {
		t.Fatalf("active = %q after closing rook", h.m.active)
	}
	h.m.close("sp/ash") // the last one: the one above
	if h.m.active != "fm/kit" {
		t.Fatalf("active = %q after closing ash", h.m.active)
	}
	h.m.close("fm/kit")
	if h.m.active != "" || len(h.m.order) != 0 || h.m.cur() != nil {
		t.Errorf("active = %q, order = %v; want nothing open", h.m.active, h.m.order)
	}
}

func TestLateEventFromClosedCharacterIgnored(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	sess := h.m.chars["fm/kit"].sess
	h.m.close("fm/kit")
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-sess.Events():
			h.m.Update(eventMsg{key: "fm/kit", sess: sess, ev: ev, ok: ok})
			if h.m.chars["fm/kit"] != nil {
				t.Fatal("a late event reopened the closed character")
			}
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("closed session still running")
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/ui -run 'TestStartupOpensOnlyAutoconnect|TestOpenKeepsAlphabeticalOrder|TestCloseHandsActiveToNeighbor|TestLateEventFromClosedCharacterIgnored'`
Expected: build failure: `h.open undefined`, `h.m.close undefined`.

- [ ] **Step 3: Add the lifecycle code**

Create `internal/ui/sidebar.go`:

```go
package ui

import (
	"cmp"
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/config"
)

// compareChars orders characters alphabetically, ignoring case: by world
// id, then by name. The sidebar and the picker both use it.
func compareChars(a, b config.Character) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(a.World), strings.ToLower(b.World)),
		cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		cmp.Compare(key(a.World, a.ID), key(b.World, b.ID)),
	)
}

// allChars is every configured character, in sidebar order.
func (m *Model) allChars() []config.Character {
	var all []config.Character
	if m.cfg != nil {
		for _, w := range m.cfg.Worlds {
			all = append(all, w.Characters...)
		}
	}
	slices.SortFunc(all, compareChars)
	return all
}

// find looks up a configured character by key.
func (m *Model) find(k string) (config.Character, bool) {
	if m.cfg != nil {
		for _, w := range m.cfg.Worlds {
			for _, ch := range w.Characters {
				if key(ch.World, ch.ID) == k {
					return ch, true
				}
			}
		}
	}
	return config.Character{}, false
}

// sortOrder puts the open characters in sidebar order.
func (m *Model) sortOrder() {
	slices.SortFunc(m.order, func(a, b string) int { return compareChars(m.chars[a].ch, m.chars[b].ch) })
}

// open adds configured character k to the sidebar, preloading its
// history, and returns it; an open character is returned as is. It
// returns nil if k isn't configured. It doesn't connect. With nothing
// active, k becomes active.
func (m *Model) open(k string) *charState {
	if cs := m.chars[k]; cs != nil {
		return cs
	}
	ch, ok := m.find(k)
	if !ok {
		return nil
	}
	cs := &charState{key: k, ch: ch, in: NewInput()}
	if err := cs.compile(); err != nil {
		m.setStatus(true, "%s: %v", k, err)
	}
	m.chars[k] = cs
	m.order = append(m.order, k)
	m.sortOrder()
	m.preload(cs)
	if m.active == "" {
		m.active = k
	}
	return cs
}

// close stops an open character's session and removes it from the
// sidebar. If it was active, the next one down becomes active, or the one
// above if it was last.
func (m *Model) close(k string) {
	i := slices.Index(m.order, k)
	if i < 0 {
		return
	}
	if cs := m.chars[k]; cs.cancel != nil {
		cs.cancel()
	}
	delete(m.chars, k)
	m.order = slices.Delete(m.order, i, i+1)
	if m.active != k {
		return
	}
	m.active = ""
	if len(m.order) > 0 {
		m.switchTo(m.order[min(i, len(m.order)-1)])
	}
}
```

In `internal/ui/model.go`:

1. Add to the `Model` struct, after `d Deps`:

```go
	cfg       *config.Config // the loaded config; the picker lists from it
```

2. Replace `New`'s body after the `d.Now` default with:

```go
	m := &Model{d: d, chars: map[string]*charState{}, collapsed: map[string]bool{}}
	m.applyConfig(cfg)
	for _, ch := range m.allChars() {
		if ch.Autoconnect {
			m.open(key(ch.World, ch.ID))
		}
	}
	return m
```

and change its doc comment to:

```go
// New builds the model from an already-loaded config and opens the
// autoconnect characters, preloading their recent history. Nothing
// connects until Init.
```

3. Replace `applyConfig` (and its doc comment) with:

```go
// applyConfig keeps cfg for the picker and updates open characters to
// match it. An open character that vanished from the config stays as an
// orphan while it's connected, until it next disconnects; otherwise it
// closes.
func (m *Model) applyConfig(cfg *config.Config) {
	m.cfg = cfg
	m.exportDir = cfg.ExportDir
	store := cfg.PasswordStore
	m.pwStore.Store(&store)
	for _, k := range slices.Clone(m.order) {
		cs := m.chars[k]
		if cs.browse != nil {
			cs.browse.exportDir = cfg.ExportDir
		}
		ch, ok := m.find(k)
		if !ok {
			if cs.sess != nil && cs.state != session.Disconnected && cs.state != session.Failed {
				cs.orphan = true
			} else {
				m.close(k)
			}
			continue
		}
		cs.ch, cs.orphan = ch, false
		if err := cs.compile(); err != nil {
			m.setStatus(true, "%s: %v", k, err)
		}
		if cs.sess != nil {
			cs.sess.SetChar(ch)
		}
	}
	m.sortOrder() // names may have changed
}
```

4. Delete `insertInWorld`, `drop` and `fixActive`.

5. In `handleEvent`, replace `m.drop(msg.key) // don't reconnect (and log in) a deleted character` with:

```go
			m.close(msg.key) // don't reconnect (and log in) a deleted character
```

- [ ] **Step 4: Add the harness helpers**

In `internal/ui/model_test.go`, after `func (h *harness) init()`:

```go
// open opens configured characters by key, as the picker would.
func (h *harness) open(keys ...string) {
	h.t.Helper()
	for _, k := range keys {
		if h.m.open(k) == nil {
			h.t.Fatalf("no character %s", k)
		}
	}
}

// openAll opens every configured character.
func (h *harness) openAll() {
	for _, ch := range h.m.allChars() {
		h.m.open(key(ch.World, ch.ID))
	}
}
```

- [ ] **Step 5: Run the new tests**

Run: `go test ./internal/ui -run 'TestStartupOpensOnlyAutoconnect|TestOpenKeepsAlphabeticalOrder|TestCloseHandsActiveToNeighbor|TestLateEventFromClosedCharacterIgnored'`
Expected: PASS.

- [ ] **Step 6: Fix fixtures that assumed every character is open**

Only autoconnect characters (Kit in `fmWorld`) are open now. Update:

- `TestLayoutShowsSidebarAndStatus` (`model_test.go`): drop `"✕ Rook"` from the `want` list, and after the loop add:
  ```go
  	if strings.Contains(s, "Rook") {
  		t.Errorf("rook isn't open but is in the sidebar:\n%s", s)
  	}
  ```
- `TestAutoconnectOnlyFlaggedCharacters`: replace the `h.m.chars["fm/rook"].sess != nil` check with
  ```go
  	if h.m.chars["fm/rook"] != nil {
  		t.Error("rook opened without autoconnect")
  	}
  ```
- `TestIncomingLinesHighlightAndBadges`, `TestSidebarClick`, `TestDoubleClickSidebarConnects`, and `TestBrowseCommandAndSwitching` (`browse_test.go`): add `h.open("fm/rook")` right after `newHarness(...)`.
- `TestSavePasswordAnswerBindsToPromptingCharacter`: add `h.open("fm/rook")` right after `h.init()`.
- `TestSidebarScrolls` and `TestSidebarNoHintsWhenItFits`: add `h.openAll()` right after `newHarness(...)`.
- `TestReloadAddsCharactersAndKeepsOldOnError`: new characters aren't opened by a reload any more. Replace the first screen check with
  ```go
  	if _, ok := h.m.find("fm/ash"); !ok {
  		t.Error("reload didn't pick up the new character")
  	}
  ```
  and in the second check replace `!strings.Contains(s, "Ash")` with `func() bool { _, ok := h.m.find("fm/ash"); return !ok }()`, so a failed reload keeps the old config.
- `TestRemovedConnectedCharacterLeavesOnDisconnect`: add `h.open("fm/rook", "sp/ash")` right after `h.settle("fm/kit", h.connected("fm/kit"))`, and change the order check to:
  ```go
  	if got := strings.Join(h.m.order, " "); got != "fm/kit fm/rook sp/ash" {
  		t.Errorf("order = %s, want the orphan kept in alphabetical order", got)
  	}
  ```

- [ ] **Step 7: Run everything**

Run: `gofmt -l . && go vet ./... && go test -race ./...`
Expected: no gofmt output; all packages `ok`.

- [ ] **Step 8: Commit**

```bash
git add internal/ui
git commit -m "refactor(ui): the sidebar holds only open characters

open/close manage the set; startup opens autoconnect characters; open
characters sort alphabetically (case-insensitive) by world, then name.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E"
```

---

### Task 2: Sidebar rows, badges, activity, truncation and closing

**Files:**
- Modify: `internal/ui/sidebar.go` (sidebar drawing moves here from `view.go`)
- Modify: `internal/ui/view.go` (delete moved code)
- Modify: `internal/ui/model.go` (`connect`, `handleClick`, `command`)
- Test: `internal/ui/sidebar_test.go`, `internal/ui/model_test.go`

**Interfaces:**
- Consumes: `open`, `close` (Task 1).
- Produces:
  - `type rowKind int` with `rowWorld`, `rowChar`, `rowAdd`
  - `type sidebarRow struct { kind rowKind; world, char string }`
  - `func fitName(s string, w int) string`
  - `const addLabel = "+ Add connection"`, `const badgeX = 2`
  - `/close` command

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/sidebar_test.go` (add `tea "charm.land/bubbletea/v2"`, `xansi "github.com/charmbracelet/x/ansi"` and `"github.com/latrani/Kiln/internal/session"` to its imports):

```go
// sideRow is sidebar row y as plain text, untrimmed.
func sideRow(h *harness, y int) string {
	return strings.SplitN(strings.Split(h.screen(), "\n")[y], "│", 2)[0]
}

func TestSidebarBadgesAndActivity(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	if got := strings.TrimSpace(sideRow(h, 1)); got != "Kit" {
		t.Errorf("connected row = %q, want no badge", got)
	}
	if got := strings.TrimSpace(sideRow(h, 2)); got != "✕ Rook" {
		t.Errorf("disconnected row = %q", got)
	}
	if got := strings.TrimSpace(sideRow(h, 3)); got != "+ Add connection" {
		t.Errorf("last row = %q", got)
	}
	h.m.chars["fm/rook"].state = session.Connecting
	if got := strings.TrimSpace(sideRow(h, 2)); got != "… Rook" {
		t.Errorf("connecting row = %q", got)
	}
	kit := h.m.chars["fm/kit"]
	h.m.switchTo("fm/rook")
	kit.unread, kit.attention = 3, true
	if got := sideRow(h, 1); !strings.HasPrefix(strings.TrimSpace(got), "Kit") || !strings.HasSuffix(got, " ● 3") {
		t.Errorf("attention row = %q, want the ● with the count on the right", got)
	}
	kit.attention = false
	if got := sideRow(h, 1); !strings.HasSuffix(got, " 3") || strings.Contains(got, "●") {
		t.Errorf("unread row = %q", got)
	}
}

func TestSidebarNamesTruncate(t *testing.T) {
	long := "host = \"h\"\nport = 1\n\n[[characters]]\nid = \"kit\"\nname = \"Kittenfluff-the-Magnificent\"\n"
	h := newHarness(t, map[string]string{"fm-with-a-long-name": long})
	h.openAll()
	sw := h.m.layout().sw
	if got := sideRow(h, 0); xansi.StringWidth(got) != sw || !strings.HasSuffix(strings.TrimSpace(got), "…") {
		t.Errorf("world row = %q, want cut with … at %d cells", got, sw)
	}
	cs := h.m.chars["fm-with-a-long-name/kit"]
	cs.unread, cs.attention = 12, true
	got := sideRow(h, 1)
	if xansi.StringWidth(got) != sw || !strings.HasSuffix(got, " ● 12") || !strings.Contains(got, "…") {
		t.Errorf("char row = %q, want the name cut with … and the activity whole", got)
	}
}

func TestClickXClosesAndNameDoesnt(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	click := func(x, y int) { h.m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}) }
	click(badgeX, 1) // kit is connected: no ✕ to hit
	if h.m.chars["fm/kit"] == nil {
		t.Fatal("clicking a connected character's badge cell closed it")
	}
	click(6, 2) // rook's name: switch, not close
	click(6, 2) // double-click: reconnect, not close
	if h.m.chars["fm/rook"] == nil || h.m.chars["fm/rook"].sess == nil {
		t.Fatal("double-clicking the name should connect rook")
	}
	h.m.close("fm/rook")
	h.open("fm/rook")
	click(badgeX, 2)
	if h.m.chars["fm/rook"] != nil {
		t.Errorf("clicking ✕ didn't close rook:\n%s", h.screen())
	}
}

func TestCloseCommand(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("/close")
	h.enter()
	if h.m.chars["fm/kit"] != nil || h.m.active != "fm/rook" {
		t.Errorf("active = %q; kit should be closed", h.m.active)
	}
}

func TestOpeningShowsConnectingNotX(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.connect(h.m.chars["fm/kit"])
	if got := strings.TrimSpace(sideRow(h, 1)); got != "… Kit" {
		t.Errorf("row = %q right after connecting", got)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/ui -run 'TestSidebarBadgesAndActivity|TestSidebarNamesTruncate|TestClickXClosesAndNameDoesnt|TestCloseCommand|TestOpeningShowsConnectingNotX'`
Expected: build failure: `undefined: badgeX`.

- [ ] **Step 3: Move and rewrite the sidebar drawing**

Cut these from `internal/ui/view.go` and paste them into `internal/ui/sidebar.go`: `sidebarRow`, `sidebarRows`, `sideView`, `sideView.at`, `sidebarView`, `scrollSidebar`, `sidebarLine`. Then replace `sidebarRow`, `sidebarRows` and `sidebarLine` with the code below. In `sidebarView`, change the scroll-into-view lookup to only match character rows:

```go
		if a := slices.IndexFunc(sv.rows, func(r sidebarRow) bool { return r.kind == rowChar && r.char == m.active }); a >= 0 {
```

New code in `sidebar.go` (add `"fmt"`, `xansi "github.com/charmbracelet/x/ansi"`, `"github.com/latrani/Kiln/internal/session"` and `"github.com/latrani/Kiln/internal/style"` to its imports):

```go
// rowKind says what a sidebar row is. #41 adds rows for adding a
// character or a world to the picker.
type rowKind int

const (
	rowWorld rowKind = iota // a world header
	rowChar                 // a character
	rowAdd                  // "+ Add connection"
)

type sidebarRow struct {
	kind  rowKind
	world string
	char  string // character key, for rowChar
}

// addLabel is the sidebar's last row.
const addLabel = "+ Add connection"

// badgeX is the column of a character row's connection badge; clicking a
// ✕ there closes the character.
const badgeX = 2

// attentionMark prefixes the unread count when a line needed attention.
var attentionMark = style.SGR(config.HighlightStyle) + "●" + style.Reset

// sidebarRows lists the open characters under their worlds, then the
// add-connection row.
func (m *Model) sidebarRows() []sidebarRow {
	var rows []sidebarRow
	lastWorld := ""
	for _, k := range m.order {
		w := m.chars[k].ch.World
		if w != lastWorld {
			lastWorld = w
			rows = append(rows, sidebarRow{kind: rowWorld, world: w})
		}
		if !m.collapsed[w] {
			rows = append(rows, sidebarRow{kind: rowChar, world: w, char: k})
		}
	}
	return append(rows, sidebarRow{kind: rowAdd})
}

// closable reports whether cs shows a ✕ that closes it.
func closable(cs *charState) bool {
	return cs.state == session.Disconnected || cs.state == session.Failed
}

// sidebarLine draws one row: the connection badge on the left (blank
// when connected), and activity (unread count, "●" for attention) on the
// right, which the name gives way to.
func (m *Model) sidebarLine(r sidebarRow, w int) string {
	switch r.kind {
	case rowWorld:
		arrow := "▾ "
		if m.collapsed[r.world] {
			arrow = "▸ "
		}
		return bold + fitName(arrow+r.world, w) + style.Reset
	case rowAdd:
		return style.Dim(fitName(addLabel, w))
	}
	cs := m.chars[r.char]
	badge := " "
	switch {
	case cs.state == session.Connecting:
		badge = "…"
	case closable(cs):
		badge = "✕"
	}
	activity, shown := "", ""
	if cs.unread > 0 {
		activity = fmt.Sprintf(" %d", cs.unread)
		shown = activity
		if cs.attention {
			activity = " ●" + activity
			shown = " " + attentionMark + shown
		}
	}
	line := fitName("  "+badge+" "+cs.ch.Name, w-xansi.StringWidth(activity)) + shown
	if r.char == m.active {
		return reverse + line + style.Reset
	}
	return line
}

// fitName is fit for names: too long, they're cut with "…".
func fitName(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return fit(xansi.Truncate(s, w, "…"), w)
}
```

Remove imports from `view.go` that are now unused (`slices` if nothing else uses it); `go vet` will say.

- [ ] **Step 4: Show connecting right away**

In `internal/ui/model.go` `connect`, after `cs.sess, cs.cancel = s, cancel`:

```go
	cs.state = session.Connecting // until the session says otherwise; no ✕ flash
```

- [ ] **Step 5: Close on ✕ clicks and `/close`**

In `handleClick`, replace the sidebar `switch` with:

```go
		switch r, hint := sv.at(msg.Y); {
		case hint != 0:
			m.scrollSidebar(hint * max(1, sv.avail-1))
		case r == nil, r.kind == rowAdd:
		case r.kind == rowWorld:
			m.collapsed[r.world] = !m.collapsed[r.world]
		case msg.X == badgeX && closable(m.chars[r.char]):
			m.close(r.char)
		default:
			m.switchTo(r.char)
			m.sideShown = r.char // clicked, so already in view
			now := m.d.Now()
			double := m.lastClick.char == r.char && now.Sub(m.lastClick.at) <= doubleClick
			m.lastClick.char, m.lastClick.at = r.char, now
			if cs := m.chars[r.char]; double && cs.state != session.Connected && cs.state != session.Connecting {
				m.lastClick.char = "" // a third click starts over
				return m.connect(cs)
			}
		}
```

In `command`, add before `case "/quit":`:

```go
	case "/close":
		m.close(cs.key)
```

- [ ] **Step 6: Run the new tests**

Run: `go test ./internal/ui -run 'TestSidebarBadgesAndActivity|TestSidebarNamesTruncate|TestClickXClosesAndNameDoesnt|TestCloseCommand|TestOpeningShowsConnectingNotX'`
Expected: PASS.

- [ ] **Step 7: Update fixtures for the new rows and badges**

- `TestLayoutShowsSidebarAndStatus`: add `"+ Add connection"` to the `want` list.
- `TestAutoconnectOnlyFlaggedCharacters`: replace `!strings.Contains(s, "○ Kit")` with `strings.Contains(s, "✕ Kit") || strings.Contains(s, "○")`, so it fails if the connected row still has a badge.
- `TestIncomingLinesHighlightAndBadges`: replace the `"● Kit        2"` check with
  ```go
  	if row := sideRow(h, 1); !strings.HasSuffix(row, " ● 2") {
  		t.Errorf("sidebar activity missing: %q", row)
  	}
  ```
  and replace `strings.Contains(s, "● Kit") || !strings.Contains(s, "○ Kit")` with `strings.Contains(sideRow(h, 1), "●")`.
- `TestSidebarScrolls` (32 rows now: header, 30 characters, `+ Add connection`): expect `side(23) == "▾ 9 more"` at the start; after 25 × Ctrl+↓, `side(23) == "▾ 5 more"`; after two wheel-downs, `side(22) == "✕ C29"`, `side(23) == "+ Add connection"` and `side(0) == "▴ 9 more"`.
- `TestSidebarNoHintsWhenItFits`: use `manyChars(22)` (24 rows) and check for `"C21"`.

- [ ] **Step 8: Run everything**

Run: `gofmt -l . && go vet ./... && go test -race ./...`
Expected: no gofmt output; all `ok`.

- [ ] **Step 9: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): sidebar badges, activity and closing

Connected characters show no badge; ✕ marks disconnected ones and
clicking it closes them, as does /close. Unread and attention sit on the
right (● 3), and names truncate with … around them. The sidebar ends
with + Add connection.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E"
```

---

### Task 3: The form, and shared editing keys

**Files:**
- Create: `internal/ui/form.go`
- Create: `internal/ui/form_test.go`
- Modify: `internal/ui/model.go` (`handleKey`'s editing switch)

**Interfaces:**
- Produces:
  - `func editKey(in *Input, k tea.KeyPressMsg) bool`
  - `type field struct { label string; in *Input }`
  - `type form struct { fields []field; focus int; hint string }`
  - `func newForm(hint string, labels ...string) *form`
  - `func (f *form) value(i int) string`
  - `func (f *form) key(k tea.KeyPressMsg) bool`
  - `func (f *form) paste(s string)`
  - `func (f *form) row() (text string, col int)`

- [ ] **Step 1: Write the failing tests**

Create `internal/ui/form_test.go`:

```go
package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
)

func TestFormEditsFocusedField(t *testing.T) {
	f := newForm("Enter to go", "Filter")
	for _, r := range "rok" {
		if !f.key(tea.KeyPressMsg{Code: r, Text: string(r)}) {
			t.Fatalf("typing %q not handled", r)
		}
	}
	f.key(tea.KeyPressMsg{Code: tea.KeyLeft})
	f.key(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if f.value(0) != "rook" {
		t.Errorf("value = %q", f.value(0))
	}
	for _, k := range []tea.KeyPressMsg{{Code: tea.KeyUp}, {Code: tea.KeyDown}, {Code: tea.KeyEnter}, {Code: tea.KeyEscape}} {
		if f.key(k) {
			t.Errorf("%s should be left to the form's owner", k.String())
		}
	}
	f.paste("a\nb")
	if f.value(0) != "rooa bk" {
		t.Errorf("paste = %q, want one line", f.value(0))
	}
}

func TestFormRow(t *testing.T) {
	f := newForm("Enter to connect · Esc to close", "Filter")
	f.key(tea.KeyPressMsg{Code: 'm', Text: "m"})
	f.key(tea.KeyPressMsg{Code: 'a', Text: "a"})
	text, col := f.row()
	if got := ansi.Strip(text); got != "Filter: ma  · Enter to connect · Esc to close" {
		t.Errorf("row = %q", got)
	}
	if col != len("Filter: ma") {
		t.Errorf("cursor col = %d", col)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/ui -run 'TestForm'`
Expected: build failure: `undefined: newForm`.

- [ ] **Step 3: Write `form.go`**

```go
package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/style"
)

// editKey applies a line-editing key to in: cursor movement, deletion
// and typed text. It reports false for any other key, including Enter,
// Up and Down, which mean different things in the chat input and in a
// form.
func editKey(in *Input, k tea.KeyPressMsg) bool {
	switch k.String() {
	case "left":
		in.Left()
	case "right":
		in.Right()
	case "ctrl+left", "alt+left", "alt+b":
		in.WordLeft()
	case "ctrl+right", "alt+right", "alt+f":
		in.WordRight()
	case "home", "ctrl+a":
		in.Home()
	case "end", "ctrl+e":
		in.End()
	case "backspace":
		in.Backspace()
	case "delete", "ctrl+d":
		in.Delete()
	case "ctrl+w", "alt+backspace", "ctrl+backspace":
		in.DeleteWordBack()
	case "alt+delete", "ctrl+delete", "alt+d":
		in.DeleteWordForward()
	case "ctrl+u":
		in.KillToStart()
	case "ctrl+k":
		in.KillToEnd()
	default:
		if k.Text == "" || k.Mod&(tea.ModCtrl|tea.ModAlt) != 0 {
			return false
		}
		in.InsertText(k.Text)
	}
	return true
}

// field is one labeled, single-line input in a form.
type field struct {
	label string
	in    *Input
}

// form is a set of fields shown in the input area as a prompt, owning
// the keys while it's open. The picker's filter is a one-field form;
// #41's world and character editors add fields.
type form struct {
	fields []field
	focus  int    // the field being edited
	hint   string // what the keys do, shown dim after the field
}

func newForm(hint string, labels ...string) *form {
	f := &form{hint: hint}
	for _, l := range labels {
		f.fields = append(f.fields, field{label: l, in: NewInput()})
	}
	return f
}

// value is field i's text.
func (f *form) value(i int) string { return f.fields[i].in.Value() }

// key edits the focused field; it reports false for keys that aren't
// editing keys, which the form's owner handles.
func (f *form) key(k tea.KeyPressMsg) bool { return editKey(f.fields[f.focus].in, k) }

// paste inserts s into the focused field, as one line.
func (f *form) paste(s string) {
	f.fields[f.focus].in.InsertText(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", " "), "\n", " "))
}

// row draws the focused field as the input area's prompt row, and
// returns the cursor's column.
func (f *form) row() (text string, col int) {
	fl := f.fields[f.focus]
	rows, _, c := fl.in.Render(1<<20, 0, false, false)
	label := fl.label + ": "
	text = style.Dim(label) + strings.TrimPrefix(rows[0], "> ") + style.Dim("  · "+f.hint)
	return text, c - 2 + xansi.StringWidth(label)
}
```

- [ ] **Step 4: Use `editKey` in `handleKey`**

In `internal/ui/model.go` `handleKey`, replace the whole second `switch k.String()` (from `case "shift+enter", "alt+enter":` through its `default:` block) with:

```go
	switch k.String() {
	case "shift+enter", "alt+enter":
		cs.in.Newline()
	case "up":
		cs.in.Up()
	case "down":
		cs.in.Down()
	default:
		if !editKey(cs.in, k) {
			return nil
		}
	}
```

- [ ] **Step 5: Run everything**

Run: `gofmt -l . && go vet ./... && go test -race ./...`
Expected: all `ok` (the existing input and key tests cover the refactor).

- [ ] **Step 6: Commit**

```bash
git add internal/ui
git commit -m "refactor(ui): shared editing keys, and a form for the input area

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E"
```

---

### Task 4: The add-connection picker

**Files:**
- Create: `internal/ui/picker.go`
- Create: `internal/ui/picker_test.go`
- Modify: `internal/ui/keys.go` (`openPickerKey`)
- Modify: `internal/ui/model.go` (`Model.picker`; `update` paste; `handleKey`; `handleClick`; `applyConfig`)
- Modify: `internal/ui/sidebar.go` (`sidebarView` rows and focus)
- Modify: `internal/ui/view.go` (`prompt`; View's sidebar loop)

**Interfaces:**
- Consumes: `open`, `allChars`, `find` (Task 1); `sidebarRow`, `rowKind`, `fitName`, `addLabel` (Task 2); `form`, `newForm` (Task 3).
- Produces:
  - `type picker struct { form *form; sel string }`, field `Model.picker *picker`
  - `func (m *Model) openPicker()`, `func (m *Model) closePicker()`
  - `func (m *Model) pickerRows() []sidebarRow`
  - `func (m *Model) fixPick()`, `func (m *Model) movePick(delta int)`
  - `func (m *Model) pick(k string) tea.Cmd`
  - `func (m *Model) pickerKey(k tea.KeyPressMsg) tea.Cmd`
  - `func (m *Model) pickerLine(r sidebarRow, w int) string`
  - `const openPickerKey = "ctrl+o"`, `const pickerHint`, `const noMatches = "No matches"`

- [ ] **Step 1: Write the failing tests**

Create `internal/ui/picker_test.go`:

```go
package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

const spWorld = "host = \"sp.test\"\nport = 1\n\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\naliases = [\"Cinder\"]\n"

// sideRows is the sidebar's text, trimmed, down to the first blank row.
func sideRows(h *harness) []string {
	var out []string
	for _, row := range strings.Split(h.screen(), "\n") {
		s := strings.TrimSpace(strings.SplitN(row, "│", 2)[0])
		if s == "" {
			break
		}
		out = append(out, s)
	}
	return out
}

func TestPickerOpensAndConnects(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.press('o', tea.ModCtrl)
	if got := strings.Join(sideRows(h), "|"); got != "fm|Rook|sp|Ash" {
		t.Errorf("picker rows = %q, want unopened characters only", got)
	}
	if s := h.screen(); !strings.Contains(s, "│Filter:   · Enter to connect · Esc to close") {
		t.Errorf("no filter prompt:\n%s", s)
	}
	h.enter()
	rook := h.m.chars["fm/rook"]
	if h.m.picker != nil || rook == nil || rook.sess == nil || h.m.active != "fm/rook" {
		t.Fatalf("Enter should open, connect and switch to rook:\n%s", h.screen())
	}
}

func TestPickerFilter(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.press('o', tea.ModCtrl)
	for filter, want := range map[string]string{
		"cin":  "sp|Ash",   // alias
		"SP":   "sp|Ash",   // world, any case
		"ro":   "fm|Rook",  // name
		"zzz":  noMatches,  // nothing
		"":     "fm|Rook|sp|Ash",
	} {
		h.m.picker.form.fields[0].in.SetValue("")
		h.typeText(filter)
		if got := strings.Join(sideRows(h), "|"); got != want {
			t.Errorf("filter %q: rows = %q, want %q", filter, got, want)
		}
	}
	h.m.picker.form.fields[0].in.SetValue("")
	h.typeText("zzz")
	h.enter() // nothing to open
	if h.m.picker == nil || len(h.m.order) != 1 {
		t.Errorf("Enter with no matches changed something: order %v", h.m.order)
	}
}

func TestPickerArrowsSkipHeaders(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.press('o', tea.ModCtrl)
	if h.m.picker.sel != "fm/rook" {
		t.Fatalf("sel = %q, want the first character", h.m.picker.sel)
	}
	h.press(tea.KeyDown, 0)
	h.press(tea.KeyDown, 0) // clamps at the last
	if h.m.picker.sel != "sp/ash" {
		t.Errorf("sel = %q after ↓↓", h.m.picker.sel)
	}
	h.press(tea.KeyUp, 0)
	if h.m.picker.sel != "fm/rook" {
		t.Errorf("sel = %q after ↑", h.m.picker.sel)
	}
	h.typeText("a") // "a" matches Ash only (not Rook): the highlight follows
	if h.m.picker.sel != "sp/ash" {
		t.Errorf("sel = %q after filtering", h.m.picker.sel)
	}
}

func TestPickerClicks(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	click := func(y int) tea.Cmd {
		_, cmd := h.m.Update(tea.MouseClickMsg{X: 4, Y: y, Button: tea.MouseLeft})
		return cmd
	}
	click(2) // rows: fm, Kit, + Add connection
	if h.m.picker == nil {
		t.Fatalf("clicking %s didn't open the picker:\n%s", addLabel, h.screen())
	}
	click(0) // a world header: nothing
	if h.m.picker == nil {
		t.Fatal("clicking a header closed the picker")
	}
	if click(1) == nil || h.m.chars["fm/rook"] == nil || h.m.picker != nil {
		t.Errorf("clicking Rook should open and connect it:\n%s", h.screen())
	}
}

func TestPickerEscAndReload(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.press('o', tea.ModCtrl)
	h.typeText("ro")
	h.press(tea.KeyEscape, 0)
	if h.m.picker != nil || h.m.chars["fm/rook"] != nil {
		t.Fatal("Esc should close the picker and open nothing")
	}
	h.press('o', tea.ModCtrl)
	h.typeText("ro")
	noRook := strings.Replace(fmWorld, "[[characters]]\nid = \"rook\"\nname = \"Rook\"\n", "", 1)
	os.WriteFile(filepath.Join(h.dir, "worlds", "fm.toml"), []byte(noRook), 0o600)
	h.m.Update(reloadMsg{})
	if h.m.picker == nil || h.m.picker.form.value(0) != "ro" {
		t.Fatal("reload should keep the picker and its filter")
	}
	if h.m.picker.sel != "" || !strings.Contains(h.screen(), noMatches) {
		t.Errorf("sel = %q; the removed character should be gone:\n%s", h.m.picker.sel, h.screen())
	}
	h.enter()
	if h.m.chars["fm/rook"] != nil {
		t.Error("Enter opened a character that's no longer configured")
	}
}

func TestPickerBlockedInBrowse(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/browse")
	h.enter()
	h.press('o', tea.ModCtrl)
	if h.m.picker != nil {
		t.Fatal("picker opened over browse mode")
	}
	h.m.openPicker() // as a click on + Add connection would
	if h.m.picker != nil || !strings.Contains(h.screen(), "leave browse mode (Esc) to add a connection") {
		t.Errorf("screen:\n%s", h.screen())
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/ui -run 'TestPicker'`
Expected: build failure: `h.m.picker undefined`.

- [ ] **Step 3: Write `picker.go`**

```go
package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/style"
)

// pickerHint follows the filter in the input area.
const pickerHint = "Enter to connect · Esc to close"

// noMatches fills the picker when it has nothing to offer.
const noMatches = "No matches"

// picker is the add-connection list shown in the sidebar: every
// configured character that isn't open, narrowed by a one-field form.
type picker struct {
	form *form
	sel  string // key of the highlighted character; "" when nothing matches
}

// openPicker shows the picker, unless the active character is in browse
// mode, which has the pane (and the input area) to itself.
func (m *Model) openPicker() {
	if cs := m.cur(); cs != nil && cs.browse != nil {
		m.setStatus(true, "leave browse mode (Esc) to add a connection")
		return
	}
	m.picker = &picker{form: newForm(pickerHint, "Filter")}
	m.sideTop, m.sideShown = 0, ""
	m.fixPick()
}

// closePicker goes back to the open characters.
func (m *Model) closePicker() {
	m.picker = nil
	m.sideShown = "" // scroll the active character back into view
}

// matches reports whether ch fits the lowercased filter f: its name, an
// alias or its world contains it.
func matches(ch config.Character, f string) bool {
	if f == "" {
		return true
	}
	for _, s := range append([]string{ch.Name, ch.World}, ch.Aliases...) {
		if strings.Contains(strings.ToLower(s), f) {
			return true
		}
	}
	return false
}

// pickerRows lists the characters the picker offers, under their worlds.
func (m *Model) pickerRows() []sidebarRow {
	f := strings.ToLower(strings.TrimSpace(m.picker.form.value(0)))
	var rows []sidebarRow
	for _, ch := range m.allChars() {
		k := key(ch.World, ch.ID)
		if m.chars[k] != nil || !matches(ch, f) {
			continue
		}
		if len(rows) == 0 || rows[len(rows)-1].world != ch.World {
			rows = append(rows, sidebarRow{kind: rowWorld, world: ch.World})
		}
		rows = append(rows, sidebarRow{kind: rowChar, world: ch.World, char: k})
	}
	return rows
}

// pickable is the keys of the characters the picker offers, in order.
func (m *Model) pickable() []string {
	var keys []string
	for _, r := range m.pickerRows() {
		if r.kind == rowChar {
			keys = append(keys, r.char)
		}
	}
	return keys
}

// fixPick keeps the highlight on its character if it's still offered,
// and otherwise moves it to the first one (or none).
func (m *Model) fixPick() {
	keys := m.pickable()
	if slices.Contains(keys, m.picker.sel) {
		return
	}
	m.picker.sel = ""
	if len(keys) > 0 {
		m.picker.sel = keys[0]
	}
}

// movePick moves the highlight by delta characters, stopping at the ends.
func (m *Model) movePick(delta int) {
	keys := m.pickable()
	i := slices.Index(keys, m.picker.sel)
	if i < 0 {
		return
	}
	m.picker.sel = keys[min(max(0, i+delta), len(keys)-1)]
}

// pick opens, connects and switches to k, closing the picker.
func (m *Model) pick(k string) tea.Cmd {
	cs := m.open(k)
	if cs == nil {
		return nil
	}
	m.closePicker()
	m.switchTo(k)
	return m.connect(cs)
}

// pickerKey handles a key while the picker is open.
func (m *Model) pickerKey(k tea.KeyPressMsg) tea.Cmd {
	page := max(1, m.sidebarView().avail-1)
	switch k.String() {
	case "esc", "ctrl+c", openPickerKey:
		m.closePicker()
	case "enter":
		return m.pick(m.picker.sel)
	case "up":
		m.movePick(-1)
	case "down":
		m.movePick(1)
	case "pgup":
		m.movePick(-page)
	case "pgdown":
		m.movePick(page)
	default:
		if m.picker.form.key(k) {
			m.fixPick()
		}
	}
	return nil
}

// pickerLine draws one picker row: a world, or a character indented
// under it, highlighted when selected.
func (m *Model) pickerLine(r sidebarRow, w int) string {
	if r.kind == rowWorld {
		return bold + fitName(r.world, w) + style.Reset
	}
	ch, _ := m.find(r.char)
	line := fitName("  "+ch.Name, w)
	if r.char == m.picker.sel {
		return reverse + line + style.Reset
	}
	return line
}
```

In `internal/ui/keys.go`, after `openBrowseKey`:

```go
// openPickerKey opens the add-connection picker.
const openPickerKey = "ctrl+o"
```

- [ ] **Step 4: Wire the picker into the sidebar view**

In `internal/ui/sidebar.go` `sidebarView`, replace the first line and the scroll-into-view block so rows and focus come from the picker while it's open:

```go
func (m *Model) sidebarView() sideView {
	rows, focus := m.sidebarRows(), m.active
	if m.picker != nil {
		rows, focus = m.pickerRows(), m.picker.sel
	}
	sv := sideView{rows: rows}
```

and

```go
	if focus != m.sideShown {
		m.sideShown = focus
		if a := slices.IndexFunc(sv.rows, func(r sidebarRow) bool { return r.kind == rowChar && r.char == focus }); a >= 0 {
```

(the rest of the block is unchanged).

In `internal/ui/view.go` `View`, replace the sidebar loop's `case r != nil:` and `default:` with:

```go
		case r != nil && m.picker != nil:
			b.WriteString(m.pickerLine(*r, l.sw))
		case r != nil:
			b.WriteString(m.sidebarLine(*r, l.sw))
		case y == 0 && m.picker != nil:
			b.WriteString(style.Dim(fitName(noMatches, l.sw)))
		default:
```

In `prompt`, add right after the `case m.mode == modeSavePassword:` case:

```go
	case m.picker != nil:
		text, col := m.picker.form.row()
		return text, col, true
```

- [ ] **Step 5: Wire keys, clicks, paste and reload**

In `internal/ui/model.go`:

1. `Model` struct, after `cfg`:

```go
	picker    *picker        // non-nil while the add-connection picker is open
```

2. `handleKey`: right after the `ctrl+up`/`ctrl+down` switch (before the browse check):

```go
	if m.picker != nil {
		return m.pickerKey(k)
	}
```

and in the switch that has `case openBrowseKey:`, add:

```go
	case openPickerKey:
		m.openPicker()
		return nil
```

3. `update`'s `tea.PasteMsg` case: make the picker's branch first:

```go
	case tea.PasteMsg:
		if m.picker != nil {
			m.picker.form.paste(msg.Content)
			m.fixPick()
		} else if cs := m.cur(); cs != nil && cs.browse != nil {
```

(the rest unchanged).

4. `handleClick`'s sidebar branch: replace the `switch` from Task 2 with:

```go
		r, hint := sv.at(msg.Y)
		switch {
		case hint != 0:
			m.scrollSidebar(hint * max(1, sv.avail-1))
		case r == nil:
		case m.picker != nil:
			if r.kind == rowChar {
				return m.pick(r.char)
			}
		case r.kind == rowAdd:
			m.openPicker()
		case r.kind == rowWorld:
			m.collapsed[r.world] = !m.collapsed[r.world]
		case msg.X == badgeX && closable(m.chars[r.char]):
			m.close(r.char)
		default:
			m.switchTo(r.char)
			m.sideShown = r.char // clicked, so already in view
			now := m.d.Now()
			double := m.lastClick.char == r.char && now.Sub(m.lastClick.at) <= doubleClick
			m.lastClick.char, m.lastClick.at = r.char, now
			if cs := m.chars[r.char]; double && cs.state != session.Connected && cs.state != session.Connecting {
				m.lastClick.char = "" // a third click starts over
				return m.connect(cs)
			}
		}
```

5. End of `applyConfig`:

```go
	if m.picker != nil {
		m.fixPick()
	}
```

- [ ] **Step 6: Run the picker tests**

Run: `go test ./internal/ui -run 'TestPicker'`
Expected: PASS.

- [ ] **Step 7: Run everything**

Run: `gofmt -l . && go vet ./... && go test -race ./...`
Expected: all `ok`.

- [ ] **Step 8: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): add-connection picker

Ctrl+O or + Add connection swaps the sidebar for every configured
character that isn't open, filtered from the input area. Enter or a
click opens, connects and switches to one.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E"
```

---

### Task 5: Empty state

**Files:**
- Modify: `internal/ui/model.go` (`Model.idle`; `New`; `input`; `update` paste; `handleKey`; `submit`; `command`)
- Modify: `internal/ui/view.go` (`layout`; `prompt`; `View`'s right pane)
- Test: `internal/ui/sidebar_test.go`

**Interfaces:**
- Consumes: `openPicker` (Task 4), `close` (Task 1), `editKey` (Task 3).
- Produces: field `Model.idle *Input`; `func (m *Model) input() *Input`; `const emptyHint = "Nothing open · Enter or Ctrl+O to add a connection"`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/sidebar_test.go`:

```go
func TestEmptyState(t *testing.T) {
	h := newHarness(t, map[string]string{"sp": spWorld}) // nothing autoconnects
	if s := h.screen(); !strings.Contains(s, "│"+emptyHint) {
		t.Fatalf("no empty-state prompt:\n%s", s)
	}
	for _, k := range []rune{tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown, tea.KeyEscape} {
		h.press(k, 0) // must not panic with nothing open
	}
	h.typeText("hello")
	h.enter()
	if !strings.Contains(h.screen(), "nothing open to send to") {
		t.Errorf("screen:\n%s", h.screen())
	}
	h.press('c', tea.ModCtrl) // clear "hello"
	h.typeText("/browse")
	h.enter()
	if !strings.Contains(h.screen(), "/browse needs an open character") {
		t.Errorf("screen:\n%s", h.screen())
	}
	h.enter() // empty input: open the picker
	if h.m.picker == nil {
		t.Fatal("Enter on the empty state should open the picker")
	}
	h.press(tea.KeyEscape, 0)
	h.typeText("/quit")
	if cmd := h.enter(); cmd == nil {
		t.Fatal("/quit returned no command")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("/quit didn't quit")
	}
}

func TestClosingLastCharacterShowsEmptyState(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/close")
	h.enter()
	if s := h.screen(); !strings.Contains(s, "│"+emptyHint) || strings.Contains(s, "Kit") {
		t.Errorf("screen:\n%s", s)
	}
}
```

(`spWorld` is defined in `picker_test.go`; test files in a package share it.)

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/ui -run 'TestEmptyState|TestClosingLastCharacterShowsEmptyState'`
Expected: build failure: `undefined: emptyHint`.

- [ ] **Step 3: Add the idle input**

In `internal/ui/model.go`:

1. `Model` struct, after `picker`:

```go
	idle      *Input         // the input box while nothing is open
```

2. `New`: `m := &Model{d: d, chars: map[string]*charState{}, collapsed: map[string]bool{}, idle: NewInput()}`.

3. After `func (m *Model) cur()`:

```go
// input is the active character's input box, or the idle one.
func (m *Model) input() *Input {
	if cs := m.cur(); cs != nil {
		return cs.in
	}
	return m.idle
}
```

4. `update`'s paste case: change `} else if cs := m.cur(); cs != nil && m.mode == modeNormal {` and its body to:

```go
		} else if m.mode == modeNormal {
			m.input().InsertText(msg.Content)
			m.confirm = false
```

5. `handleKey`: change the `ctrl+c` case to:

```go
	case "ctrl+c":
		if m.input().Empty() {
			return m.quit()
		}
		m.input().Reset()
```

and replace the `if cs == nil { return nil }` guard plus the editing switch after it with:

```go
	in := m.input()
	switch k.String() {
	case "shift+enter", "alt+enter":
		in.Newline()
	case "up":
		in.Up()
	case "down":
		in.Down()
	default:
		if !editKey(in, k) {
			return nil
		}
	}
```

6. `submit`: replace

```go
	cs := m.cur()
	if cs == nil {
		return nil
	}
```

with

```go
	cs := m.cur()
	if cs == nil {
		switch text := m.idle.Value(); {
		case text == "":
			m.openPicker()
		case strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "//"):
			m.idle.Commit()
			return m.command(nil, text)
		default:
			m.setStatus(true, "nothing open to send to")
		}
		return nil
	}
```

7. `command`: right after `args := strings.Fields(text)`:

```go
	if cs == nil && args[0] != "/quit" {
		m.setStatus(true, "%s needs an open character", args[0])
		return nil
	}
```

- [ ] **Step 4: Draw the empty state**

In `internal/ui/view.go`:

1. Add near the top:

```go
// emptyHint is the input area's prompt while nothing is open.
const emptyHint = "Nothing open · Enter or Ctrl+O to add a connection"
```

2. `layout`: replace the `} else if cs == nil {` branch and the `} else {` branch with one branch that renders whichever input is live:

```go
	} else {
		limit, flatten := 0, false
		if cs != nil {
			limit, flatten = cs.ch.MaxLineBytes, cs.ch.NewlineMode == "flatten"
		}
		rows, r, c := m.input().Render(l.rw, limit, flatten, false)
		maxIn := max(1, m.height/3)
		top := 0
		if len(rows) > maxIn {
			top = min(max(0, r-maxIn+1), len(rows)-maxIn)
		}
		l.inRows = rows[top:min(len(rows), top+maxIn)]
		l.inTop, l.curRow, l.curCol = top, r-top, c
	}
```

3. `prompt`: replace `case cs == nil: return "", 0, false` with:

```go
	case cs == nil && m.idle.Empty():
		return hint(emptyHint)
	case cs == nil:
		return "", 0, false
```

4. `View`: in the `} else if cs == nil {` branch, only show the setup message when there's nothing configured at all:

```go
	} else if cs == nil {
		right = append(right, make([]string, l.sbH)...)
		if len(m.allChars()) == 0 {
			right[0] = style.Dim("No characters yet: add one in " + m.d.ConfigDir + "/worlds/")
		}
```

5. `handleClick`'s input-area click (end of the function) uses `cs.in`; change it to `m.input()` and drop the `if cs == nil { return nil }` guard above it, guarding the pill check with `cs != nil &&` instead:

```go
	cs := m.cur()
	if cs != nil && cs.sb.Scrolled() && msg.Y == l.sbH-1 && msg.X >= m.width-l.pillW {
		cs.sb.ToBottom()
	}
	if y := msg.Y - l.sbH - 1; !l.prompt && y >= 0 && y < len(l.inRows) && msg.X > l.sw {
		m.input().Click(msg.X-l.sw-3, l.inTop+y) // past the separator and gutter
	}
	return nil
```

- [ ] **Step 5: Run the new tests**

Run: `go test ./internal/ui -run 'TestEmptyState|TestClosingLastCharacterShowsEmptyState'`
Expected: PASS.

- [ ] **Step 6: Run everything**

Run: `gofmt -l . && go vet ./... && go test -race ./...`
Expected: all `ok`.

- [ ] **Step 7: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): empty state when nothing is open

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E"
```

---

### Task 6: README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the docs**

1. Features list: replace the "Many worlds, many characters" bullet with:

```markdown
- **Many worlds, many characters, all at once.** The sidebar shows the characters you have open, grouped under their worlds; everything else is a Ctrl+O away. Each one has its own scrollback, draft and history.
```

2. Getting started step 4: replace with:

```markdown
4. **Run `kiln`.** Characters with `autoconnect = true` open and log in. Press `Ctrl+O` (or click `+ Add connection`) to open another.
```

3. Keys table: add rows after the `Ctrl+↑` / `Ctrl+↓` row:

```markdown
| `Ctrl+O` | Add a connection: type to filter, `Enter` to connect, `Esc` to close |
```

and change the `Ctrl+↑` / `Ctrl+↓` row's text to `Switch between open characters`.

4. Replace the paragraph starting "Click a character in the sidebar" with:

```markdown
The sidebar lists the characters you have open. Click one to switch to it, double-click a disconnected one to reconnect, or click its `✕` to close it. Click a world header to collapse it. Connected characters have no mark; `…` means connecting and `✕` disconnected. On the right, a number counts unread lines, and `●` means one of them needs your attention (a page or whisper, by default).
```

5. Commands table: add after `/disconnect`:

```markdown
| `/close` | Disconnect and remove the character from the sidebar |
```

- [ ] **Step 2: Check it reads right**

Run: `grep -n "Ctrl+O\|/close\|+ Add connection" README.md`
Expected: the lines above, and no leftover mention of `○`.

Run: `grep -n "○" README.md`
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: sidebar, picker and /close

Closes #40

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01X5RzCDj8AoLniuQdFBZC4E"
```
