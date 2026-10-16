# Responsive A: collapsing the sidebar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In wide mode, the sidebar (world list, picker, or filter panel) collapses with `‹` or Ctrl+B to a `≡` at the top bar's left, which carries the activity badge, and the choice is saved. This is part A of #204; narrow mode is plan B.

**Architecture:** The core (`internal/app`) owns the saved preference, read from and written to a state file through `Deps.StateFile`, and the badge's sum (`Activity`). Each front end decides when the sidebar is actually shown. It shows when the preference says so, or for now while the picker is up, or while log mode's panel has focus. Each front end draws `‹` and `≡`. The TUI first gets a `layout.rx` (where the right pane starts), so a zero-width sidebar is one change.

**Tech Stack:** Go (bubbletea v2), TypeScript/Preact web page, the string catalog (`internal/str`), themes (`internal/theme`).

**Spec:** `docs/superpowers/specs/2026-10-16-responsive-design.md`. This plan covers its wide mode: Collapsing and the ≡, the badge, Ctrl+B, log mode's wide case, the picker's wide case, and what's saved. Narrow mode is plan B.

## Global Constraints

- Every user-facing string lives in `internal/str/locales/en.toml`; run `go generate ./internal/str` after editing it (regenerates Go and `web/ui/src/str.gen.ts`).
- Tests build expected text from the catalog, never a copy.
- Commits that touch `en.toml` end with a `Strings:` trailer listing the keys.
- New look → new theme role in `roles.go` (and `Roles`), `default.toml`, and the role list in `docs/themes.md`. No escape codes in `internal/ui`.
- Glyphs: `‹` (U+2039) and `≡` (`glyph.Menu`) are one cell, EAW Neutral or Ambiguous, and in WGL4. Add `glyph.Collapse = "‹"` to `internal/glyph` (and `All`).
- TUI goldens: `-update` only where the look changes on purpose, and say so in the commit. Add a `collapsed` golden scenario.
- Below 1.0: no migration. A missing state file means expanded.
- Names in tests: Kit, Rook, Ash; world `fm`.
- Try Kiln only against a scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`.
- Web copies the TUI, using the browser's own focus and buttons.

## Review Focus

1. **Forced-shown states:**
   - The picker opened while collapsed must show, and must be collapsed again when it closes.
   - In log mode, `f` to the panel while collapsed must show it, and `f`/Esc back must collapse it again.
   - Neither may change the saved preference.
   - Pinned in Tasks 1, 4 and 5.
2. **Ctrl+B in log mode with the panel focused and collapsing:** focus goes to the log first, so the panel doesn't stay shown by focus and make Ctrl+B look dead. Pinned in Task 4.
3. **Clicks after collapse:** every right-pane click and wheel position (scrollback, input, log body, top bar, statusline chips) must land where it's drawn once the sidebar is zero wide. This is the `rx` refactor. Pinned in Tasks 3 and 4.
4. **The state file:**
   - Missing: expanded, no message.
   - Unreadable or malformed: expanded, with a statusline message.
   - A failed write: the preference still changes for this run, with a statusline message.
   - Pinned in Task 1.
5. **The badge with nothing open, and in log mode:** `≡` alone. In log mode it's the filter panel's `≡`, which has no badge. Pinned in Tasks 4 and 5.

---

### Task 1: Core: the saved preference and the badge's sum

**Files:**
- Create: `internal/app/state.go`
- Modify: `internal/app/app.go` (`Deps` ~27; `New` ~154 reads the file)
- Modify: `internal/ui/model.go` (`ui.Deps` ~33 gains `StateFile`; `New` passes it), `cmd/kiln/main.go` (~276: `StateFile: filepath.Join(dataDir, "state.json")`), `web/cmd/kiln-web/main.go` (~99: the same under its `dataDir`)
- Modify: `internal/str/locales/en.toml` (`[status]`)
- Test: `internal/app/state_test.go`

**Interfaces:**
- Produces:
  - `Deps.StateFile string`: the state file's path. `""` means nothing is saved.
  - The file is JSON: `{"sidebar_collapsed": true}`. Unknown fields are kept when it's rewritten.
  - `func (a *App) SidebarCollapsed() bool`
  - `func (a *App) SetSidebarCollapsed(on bool)`: writes the file. A failed write sets an error status, and the value changes anyway.
  - `func (a *App) Activity() (unread int, priority bool)`: the sum over open characters of `Unread`, and whether any has `Priority && Unread > 0` (what its row shows as `●`).
  - Strings:
    - `status.state_unreadable = "Kiln's saved state couldn't be read: {err}"`, set by `New` for a file that exists but can't be read or parsed.
    - `status.state_not_saved = "Kiln's state couldn't be saved: {err}"`

- [ ] **Step 1: Add the strings**, run `go generate ./internal/str`.
- [ ] **Step 2: Write the failing tests** in `state_test.go`:
  - `TestSidebarCollapsedSaved`: an App over a temp `StateFile` starts expanded. `SetSidebarCollapsed(true)`, then a new App over the same file starts collapsed.
  - `TestStateFileKeepsOtherFields`: a file holding `{"other": 1}`, after `SetSidebarCollapsed(true)`, still has `other` when parsed.
  - `TestStateFileUnreadable`: a file holding `not json` starts expanded, with `Status().Err` and text `str.StatusStateUnreadable(…)` (compare with `upTo`/`mark`).
  - `TestStateFileUnwritable`: with `StateFile` in a directory that doesn't exist and can't be created (under a file, say), `SetSidebarCollapsed(true)` leaves `SidebarCollapsed()` true and the status is `StatusStateNotSaved`.
  - `TestStateFileMissing`: no file, no status.
  - `TestActivity`: Kit `Unread 3`, Rook `Unread 2, Priority true` gives `(5, true)`. Rook's priority with `Unread 0` gives `priority` false.
- [ ] **Step 3: Run them to verify they fail**: `go test ./internal/app -run 'TestSidebarCollapsed|TestStateFile|TestActivity'`: FAIL.
- [ ] **Step 4: Implement**, and wire `StateFile` in both mains and `ui.Deps`.
- [ ] **Step 5: Run** `go test ./internal/app ./internal/ui ./cmd/... ./web/cmd/...`: PASS.
- [ ] **Step 6: Commit**: `App: the sidebar's collapsed preference, saved; Activity (#204)` with `Strings: status.state_unreadable, status.state_not_saved (new)`.

---

### Task 2: Feed and web API

**Files:**
- Modify: `internal/feed/events.go`, `internal/feed/feed.go` (the `put` list ~152), `internal/webapi/actions.go`
- Test: `internal/webapi/loop_test.go`

**Interfaces:**
- Consumes: Task 1's `SidebarCollapsed`, `SetSidebarCollapsed`, `Activity`.
- Produces:
  - Event `feed.Sidebar{Collapsed bool; Unread int; Priority bool}`, sent when any of the three changes.
  - Action `{Do: "sidebar", Shut: bool}` calls `SetSidebarCollapsed(Shut)`.

- [ ] **Step 1: Write the failing test**: the first batch has a `Sidebar` event with `Collapsed: false`. `{"Do":"sidebar","Shut":true}` sends `Sidebar` with `Collapsed: true`. A line arriving for a character in the background raises `Unread`.
- [ ] **Step 2: Run to verify it fails**: `go test ./internal/webapi ./internal/feed`: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run**: PASS. Commit: `Feed and web API: the sidebar's state (#204)`.

---

### Task 3: TUI: `layout.rx`, where the right pane starts (refactor)

**Files:**
- Modify: `internal/ui/view.go` (`layout` struct ~24; `layout()` ~45; cursor positions ~389, ~427), `internal/ui/model.go` (every `msg.X … l.sw` at ~1003-1186)
- Test: none new. The whole suite and every golden must pass unchanged.

**Interfaces:**
- Produces: `layout.rx int`, the screen column of the right pane's first cell: `sw+1` with a sidebar, `0` without. Right-pane coordinates become `msg.X - l.rx`. "Is this in the right pane" becomes `msg.X >= l.rx`, and "in the sidebar" becomes `msg.X < l.sw`, which is false when `sw` is 0.

- [ ] **Step 1: Replace** every `l.sw+1`, `l.sw + 1`, `msg.X-l.sw-1`, `msg.X > l.sw` and `msg.X <= l.sw` in non-test code with the `rx` forms. Don't touch tests: with a sidebar, `rx == sw+1`.
- [ ] **Step 2: Run** `go test ./internal/ui`. Expected: PASS with no golden changes (`git status` shows no testdata).
- [ ] **Step 3: Commit**: `TUI: layout.rx, where the right pane starts (#204)`.

---

### Task 4: TUI: collapse, `≡` and the badge, Ctrl+B

**Files:**
- Modify: `internal/glyph/glyph.go` (`Collapse`)
- Modify: `internal/theme/roles.go`, `default.toml`, `docs/themes.md`:
  - `status.menu` for `≡`, the same look as `status.log`.
  - `sidebar.collapse` for `‹`, the same as `sidebar.more`, and inherited by `picker.collapse` and `filter.collapse`. Each list's own, so it takes its area's color.
- Modify: `internal/ui/view.go` (`layout()`, `topBarFor`, the sidebar loop's title row, `topClick`), `internal/ui/sidebar.go` (`sideTitle`), `internal/ui/model.go` (global keys beside `ctrl+o` ~640; clicks), `internal/ui/keys.go` (`toggleSidebarKey = "ctrl+b"`)
- Test: `internal/ui/collapse_test.go`; goldens (a new `collapsed` scenario in `golden_test.go`)

**Interfaces:**
- Consumes: Task 1, Task 3.
- Produces:
  - `func (m *Model) sideShown() bool`: `!m.a.SidebarCollapsed() || m.picker != nil || (cs != nil && cs.browse != nil && cs.browse.panelFocus)`. `layout()` uses `sw = 0`, `rx = 0` when it's false.
  - `func (m *Model) sideForced() bool`: shown only because of the picker or panel focus. `‹` isn't drawn then.
  - `func (m *Model) toggleSidebar()`: Ctrl+B, `‹` and `≡`. With the picker or an editor up it does nothing. When collapsing in log mode with the panel focused, it first gives the log focus.
  - The top bar: when `!sideShown()`, the line starts `≡` (`status.menu`). Outside log mode, `Activity()` adds a space, then `●` (`sidebar.priority`) if `priority`, then the count (`sidebar.unread`) if `unread > 0`. Then a space and the name as now. `topBarFor` returns the `≡`'s span `menuChip [2]int` too, and every caller and test is updated.
  - The title row: `sideTitle` puts `‹` in the list's collapse role at the last column, unless `sideForced()`. A click on it calls `toggleSidebar()`.

- [ ] **Step 1: Add the glyph and roles.**
- [ ] **Step 2: Write the failing tests** in `collapse_test.go`:
  - `TestCollapseByChevron`: Kit and Rook open, a click on `‹` (the title row's last sidebar column). The sidebar is gone (screen row 1 starts with the top bar's rule), the top bar starts `glyph.Menu`, and `SidebarCollapsed()` is true. A click on `≡` (the `menuChip` span) brings it back.
  - `TestCtrlBToggles`: `ctrl+b` collapses and `ctrl+b` expands.
  - `TestBadge`: collapsed, with Kit `Unread 3` and Rook `Unread 2, Priority true`. The top bar starts `glyph.Menu + " " + glyph.Priority + "5 "`, the `●` is painted `sidebar.priority`, and `5` is painted `sidebar.unread`. With no priority it's `≡ 5`, and with nothing unread just `≡ ` then the name.
  - `TestCollapsedClicksLand` (Review Focus 3): collapsed, a click on a link in the scrollback and one in the input land where they're drawn. Use the existing scrollback-click and input-click test patterns, with `X` measured from column 0.
  - `TestPickerShowsWhileCollapsed` (Review Focus 1): collapsed, `ctrl+o` shows the sidebar with the picker's title and no `‹`. Esc collapses it again, and the preference stays collapsed throughout.
  - `TestLogPanelShowsWhileFocused` (Review Focus 1, 5): collapsed, `ctrl+l` shows the log with the top bar `≡` and no badge. `f` shows the panel and `f` collapses it again.
  - `TestCtrlBWithPanelFocused` (Review Focus 2): expanded in log mode, `f`, then `ctrl+b` collapses, and the log has focus.
- [ ] **Step 3: Run to verify they fail**: `go test ./internal/ui -run 'TestCollapse|TestCtrlB|TestBadge|TestCollapsedClicks|TestPickerShows|TestLogPanelShows'`: FAIL.
- [ ] **Step 4: Implement.** Add `TestGoldenCollapsed` (`main`'s scenario, collapsed, with Rook unread and a priority). Run `-update` for it alone, then check every other golden for the `‹` in title rows. That's an intended change: `-update` and say so.
- [ ] **Step 5: Run** `go test ./...`: PASS.
- [ ] **Step 6: Commit**: `TUI: the sidebar collapses to ≡, with the activity badge; Ctrl+B (#204)`. Mention the goldens.

---

### Task 5: Web: the same

**Files:**
- Modify: `web/ui/src/store.ts` (`Sidebar` event → `state.sidebar`), `web/ui/src/ui/App.tsx` (no `--side` column while hidden: `kiln collapsed` class), `web/ui/src/kiln.css` (`.kiln.collapsed { grid-template-columns: 0 0 1fr }`, sidebar and divider not displayed), `web/ui/src/ui/Sidebar.tsx` (`SideTitle` gains the `‹` button), `web/ui/src/ui/TopBar.tsx` (`≡` button and badge), `web/ui/src/page.tsx` (Ctrl+B in the global keys), `web/ui/src/layout.ts` or a new `web/ui/src/sidebar.ts` (`sideShown(state)`, `toggleSidebar(store)`)
- Modify: `internal/str/locales/en.toml` (`[web]`)
- Test: `web/ui/src/ui/Collapse.test.tsx`, plus a step in `web/smoke` (`TestFilter` or a new `TestCollapse`)

**Interfaces:**
- Consumes: Task 2's event and action, Task 4's roles and glyph.
- Produces:
  - `sideShown(s)`, the same rule as the TUI's: not collapsed, or the picker's up, or the active log has `logView.focus === "panel"`.
  - `toggleSidebar(store)`: sends `{Do: "sidebar", Shut: !collapsed}`. When collapsing with the panel focused, it calls `focusLog` first.
  - Strings:
    - `web.sidebar_menu = "Sidebar"`, the `≡` button's label. Its accessible name adds the badge via `web.sidebar_activity = "{n} unread"` and `web.sidebar_priority = "priority"`, joined with `str.separator()`.
    - `web.collapse_sidebar = "Collapse sidebar"`, `‹`'s label.
  - `≡` is a `<button aria-expanded="false">` at the top bar's start. `‹` is a button at the title's end.

- [ ] **Step 1: Add the strings**, run `go generate ./internal/str`.
- [ ] **Step 2: Write the failing tests** in `Collapse.test.tsx`:
  - Expanded: the sidebar has `‹` labelled `str.webCollapseSidebar()`, and the top bar has no `≡`.
  - Collapsed (a `Sidebar` event with `Collapsed: true`): there's no `.side`, `.kiln` has the `collapsed` class, and the top bar's first button is `≡`, labelled with the badge.
  - The badge renders `glyph.priority` (in `sidebar.priority`) then `5` (in `sidebar.unread`), with the same cases as the TUI's.
  - The picker open while collapsed shows `.side` with no `‹`. Log mode with `focus: "panel"` shows the panel, and its `≡` has no badge.
  - `toggleSidebar` sends `{Do: "sidebar", Shut: true}`. With the panel focused, `focus` is cleared first.
  - Smoke: Ctrl+B collapses (no `.side`), and a click on `≡` expands.
- [ ] **Step 3: Run to verify they fail**: `cd web/ui && npm test`: FAIL.
- [ ] **Step 4: Implement.**
- [ ] **Step 5: Run**: `npm run check && npm test`, `KILN_SMOKE=1 go test ./smoke` (in `web/`), and `go test ./...`. All PASS.
- [ ] **Step 6: Commit**: `Web: the sidebar collapses to ≡, as the TUI's (#204)` with a `Strings:` trailer.

---

### Task 6: Docs, smoke, review, PR

- [ ] `docs/using.md`: Ctrl+B in the key table, and a paragraph on `‹`/`≡` and the badge. `docs/log-mode.md`: the panel collapses too, and shows while it has focus.
- [ ] Commit: `Docs: collapsing the sidebar (#204)`.
- [ ] Run the TUI in tmux with a scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`. Collapse it, quit, restart: still collapsed. `state.json` is in the scratch data dir.
- [ ] Run a fresh review of the whole branch against `main` with this Review Focus. Fix Critical and Important findings test-first, then triage the minors with Indi.
- [ ] Open the PR. It lists the strings and roles, says the goldens changed on purpose (`‹` in every title row, plus the new `collapsed` golden), and says plan B (narrow mode) is next. Don't merge.
