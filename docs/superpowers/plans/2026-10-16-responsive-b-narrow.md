# Responsive B: narrow mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Under 64 columns (64ch on the web) there's no sidebar column. The sidebar is a toggle that starts closed. When open, it takes the top bar and scrollback area at full width, while the input area and status line stay. This is part B of #204.

**Architecture:** Front end only; the core is untouched. Each front end decides `narrow` from its width and keeps a `narrowOpen` flag that isn't saved. The TUI first gets a **side box**, the rectangle the sidebar is drawn into and clicked in. That box is the column in wide mode and the top area in narrow, so drawing and clicks don't care which mode they're in. In log mode, narrow, the panel is open exactly while it has focus, so `f` does both.

**Tech Stack:** Go (bubbletea v2), TypeScript/Preact web page.

**Spec:** `docs/superpowers/specs/2026-10-16-responsive-design.md`, the narrow-mode parts. Settled since: the breakpoint is 64; narrow has no column, only the toggle; when open it takes the top bar and scrollback while the input and status line stay; narrow always starts closed and saves nothing; resizing across 64 switches modes.

## Global Constraints

- Every user-facing string lives in `internal/str/locales/en.toml`. None are expected here.
- No escape codes in `internal/ui`. Existing roles cover everything here.
- TUI goldens: add `narrow` (closed) and `narrow-open`. Existing goldens must not change: wide mode is untouched.
- Names in tests: Kit, Rook, Ash; world `fm`.
- Try Kiln only against a scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`.
- Web copies the TUI and uses the browser's own focus.

## Design calls (beyond the spec)

- **Esc closes the open narrow sidebar** (outside log mode, with no picker up), before Esc does anything else. It's one step back, as everywhere.
- **The open sidebar replaces the top bar,** so its title row (with `‹` to close) sits where the top bar was.
- **Ctrl+↑/↓ switch characters with the narrow sidebar open and leave it open.** That's browsing, not picking. A click on a row is picking, and closes it.
- **Becoming narrow closes it** (a resize from wide). Becoming wide just uses the saved collapse state.
- **The pane's width (NAWS) doesn't change when it opens:** it covers the scrollback rather than resizing it.

## Review Focus

1. **Picking closes it, in every way of picking:**
   - A character row, a world row, `+ Connection` (which opens the picker, so it stays shown), a picker choice, and Esc out of the picker.
   - Double-click to connect: the first click picks and closes, so the second lands on the main view. That's fine, but it must not misfire on whatever is drawn there.
   - Pinned in Task 2.
2. **Log mode, narrow:**
   - `f`, Ctrl+B, `≡` and `‹` all open and close the panel together with focus.
   - Esc in the panel closes it.
   - The panel's footer and the log's bar stay visible.
   - Panel keys from the log (`h` while closed) still act on the hidden highlight.
   - Pinned in Task 2.
3. **Clicks in the open narrow sidebar** land on the rows drawn at full width: the `badgeX` close `×`, the picker's add-character field and its cursor, and the scroll hints. Pinned in Task 2.
4. **Resize:** wide with the sidebar collapsed → narrow → wide again. Collapsed comes back, and narrowOpen doesn't leak into wide. Pinned in Task 2.
5. **Web focus:** opening moves focus into the sidebar and closing gives it to the input. Esc with focus in the open sidebar closes it. Pinned in Task 3.

---

### Task 1: TUI: the side box (refactor)

**Files:**
- Modify: `internal/ui/view.go`: the sidebar draw loop (~441-494) becomes `func (m *Model) sideRows(w, h int) (rows []string, cursor *tea.Cursor)`, with the loop writing those rows beside the right pane.
- Modify: `internal/ui/sidebar.go`: `sidebarView()` lays out over the box's height, not `m.height`.
- Modify: `internal/ui/model.go`: the sidebar click block (~1067-1117) becomes `func (m *Model) sideClick(x, y int) tea.Cmd`, and the wheel block becomes `func (m *Model) sideWheel(b tea.MouseButton)`. Both take box coordinates.
- Test: none new. The whole suite and every golden must pass unchanged.

**Interfaces:**
- Produces:
  - `func (m *Model) sideBox() (w, h int)`: the sidebar's rectangle from screen (0,0). Wide and shown, it's `(l.sw, m.height)`. Otherwise it's `(0, 0)` until Task 2 adds narrow.
  - `sideRows(w, h)` draws the title rows, then the list, panel or picker rows, each filled in its area's role to `w`. Its cursor is in box coordinates.
  - `sidebarView()` and `panelView` take the box's `h - sideTitleH`.
  - Callers in `View` and `handleClick`/the wheel test "in the box" as `x < w && y < h`.

- [ ] **Step 1: Extract** as above, with no logic changes.
- [ ] **Step 2: Run** `go test ./internal/ui`. Expected: PASS, and `git status` shows no testdata.
- [ ] **Step 3: Commit**: `TUI: the side box, the sidebar's rectangle (#204)`.

---

### Task 2: TUI: narrow mode

**Files:**
- Create: `internal/ui/narrow.go`
- Modify: `internal/ui/collapse.go` (`toggleSidebar`, `menuPrefix`, `sidebarShown`), `internal/ui/view.go` (`layout()`, `View`), `internal/ui/model.go` (WindowSizeMsg; Esc; the click/wheel dispatch), `internal/ui/picker.go` (`closePicker`)
- Test: `internal/ui/narrow_test.go`; goldens `narrow`, `narrow-open` in `golden_test.go`

**Interfaces:**
- Consumes: Task 1's `sideBox`, `sideRows`, `sideClick`, `sideWheel`.
- Produces:
  - `const NarrowWidth = 64` (exported beside `MinWidth`).
  - `func (m *Model) narrow() bool`: `m.width < NarrowWidth`.
  - `Model.narrowOpen bool`: not saved; false on start and on becoming narrow.
  - `func (m *Model) narrowShown() bool`: narrow and (`narrowOpen`, or `m.listing()`, or log mode with `panelFocus`).
  - `sideBox()` in narrow: `(m.width, H)` while `narrowShown()`, else `(0, 0)`. H is the rows above the bottom area: `l.top + l.sbH` outside log mode, `m.height - 4` in log mode (rule, bar, rule, statusline).
  - `layout()` in narrow: `sw = 0`, `rx = 0` always.
  - `toggleSidebar()` in narrow:
    - Log mode: as `f`, so `focusPanel(!panelFocus)`.
    - Otherwise: flips `narrowOpen`.
    - `canToggle` still applies. Nothing is saved.
  - In narrow log mode, `f` is unchanged: `panelFocus` alone opens and closes it, since `narrowShown` reads it.
  - `menuPrefix` shows `≡` (and its badge) in narrow while `!narrowShown()`, with the same `canToggle` rule.
  - Picking closes it: `sideClick` sets `narrowOpen = false` after switching on a character or world row; `closePicker` sets it too. A picker choice closes the picker, so it's covered.
  - Esc: with `narrowOpen` outside log mode and no picker, Esc closes it and does nothing else.
  - `‹` in the open narrow title closes it, through `toggleSidebar`.

- [ ] **Step 1: Write the failing tests** in `narrow_test.go`. The harness is at 60×24; add `h.m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})` in a `narrowHarness`.
  - `TestNarrowStartsClosed`: no sidebar column (`layout().sw == 0`), and the top bar starts with `glyph.Menu`.
  - `TestNarrowOpenAndPick` (Review Focus 1):
    - `ctrl+b` opens it: screen row 0 is the connections title with `‹` at column 59, and the rows below are the sidebar's at full width. The input row is still drawn.
    - A click on Rook's row (found by searching the screen) switches to Rook and closes it.
    - `ctrl+b` then a click on the world row shows the overview and closes it.
    - `ctrl+b` then `esc` closes it.
  - `TestNarrowPicker`: `ctrl+o` shows the picker's list at full width, and `esc` closes both. `narrowOpen` is false after.
  - `TestNarrowLogMode` (Review Focus 2):
    - `ctrl+l` shows the log with no panel.
    - `f` opens the panel at full width, with its footer drawn and the log bar's row still there.
    - `esc` closes it, with focus on the log.
    - `ctrl+b` opens it with focus, and `ctrl+b` closes it.
    - With it closed, `h` hides the highlighted entry.
  - `TestNarrowClicksLand` (Review Focus 3): opened with Kit and Rook (Rook disconnected), a click on Rook's `×` at `badgeX` closes Rook.
  - `TestNarrowResize` (Review Focus 4):
    - Wide, collapse with `ctrl+b`, resize to 60: closed.
    - `ctrl+b` opens, resize to 100: wide and collapsed, with `layout().sw == 0` and `SidebarCollapsed()` true.
    - Resize to 60: closed again.
- [ ] **Step 2: Run to verify they fail**: `go test ./internal/ui -run TestNarrow`: FAIL.
- [ ] **Step 3: Implement.** Add `TestGoldenNarrow` (`main`'s scenario at 60×24) and `TestGoldenNarrowOpen` (the same, `ctrl+b`), then `-update` only those two.
- [ ] **Step 4: Run** `go test ./...`: PASS, with no other golden changing.
- [ ] **Step 5: Commit**: `TUI: narrow mode, the sidebar a toggle under 64 columns (#204)`. Mention the two new goldens.

---

### Task 3: Web: narrow mode

**Files:**
- Modify: `web/ui/src/store.ts` (`narrow?: boolean`, `narrowOpen?: boolean`; `closePicker` and the switch paths set `narrowOpen: false`), `web/ui/src/sidebar.ts` (`narrowShown`, `toggleSidebar`, `sideShown`), `web/ui/src/ui/App.tsx` (classes `narrow`, `open`; a ResizeObserver on `.kiln` measuring with `paneSize`, `cols < 64`), `web/ui/src/kiln.css`, `web/ui/src/ui/Sidebar.tsx` (row clicks close it; Esc on the nav closes it), `web/ui/src/ui/TopBar.tsx` (`≡` in narrow)
- Test: `web/ui/src/ui/Narrow.test.tsx`; `web/smoke/collapse_test.go` gains `TestNarrow`, using `Emulation.setDeviceMetricsOverride` at 400×800

**Interfaces:**
- Consumes: plan A's `sideForced`, `canToggle`, `toggleSidebar`, `Menu`.
- Produces:
  - `narrowShown(s)`: `s.narrow && (s.narrowOpen || listing(s) || logShows(s, s.active) && s.logView[s.active]?.focus === "panel")`.
  - `sideShown(s)` in narrow is `narrowShown(s)`.
  - `toggleSidebar` in narrow: in log mode, `focusPanel`/`focusLog`; otherwise it flips `narrowOpen`. Opening focuses the sidebar's `‹`; closing focuses the input.
  - Layout:
    - `.kiln.narrow { grid-template-columns: 1fr; grid-template-areas: "top" "body" "bottom" "status"; }`.
    - `.kiln.narrow.open` makes the areas `"side" "side" "bottom" "status"` and doesn't display `.top` or the body.
    - The width is measured in the page font's `ch`, not a media query, whose `ch` is the default font's.
  - Esc with focus in the open narrow sidebar closes it.

- [ ] **Step 1: Write the failing tests** in `Narrow.test.tsx`, setting `narrow: true` in the store:
  - Closed: no `.side`, and the `≡` button shows.
  - `toggleSidebar` sets `narrowOpen`, and `.kiln` has `narrow open`.
  - `.side` renders with `‹`, and `header` doesn't render.
  - `closePicker` and a `switch` from a row clear `narrowOpen`.
  - In log mode, `toggleSidebar` calls `focusPanel` (focus `"panel"`), and the panel shows.
  - Smoke `TestNarrow` at 400px: `≡` shows, a click opens, a click on a row closes, and focus is on `#kiln-input` (or the input's textarea).
- [ ] **Step 2: Run to verify they fail**: `cd web/ui && npm test`: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run check && npm test`, `KILN_SMOKE=1 go test ./smoke` (in `web/`), and `go test ./...`. All PASS.
- [ ] **Step 5: Commit**: `Web: narrow mode, as the TUI's (#204)`.

---

### Task 4: Docs, smoke, review, PR

- [ ] `docs/using.md`: narrow mode in the collapsing paragraph (under 64 columns: no column, `≡`/Ctrl+B open it over the main view, picking closes it). `docs/log-mode.md`: narrow, `f` opens and closes the panel.
- [ ] Commit: `Docs: narrow mode (#204)`.
- [ ] Run the TUI in tmux at 60 columns with a scratch config. Ctrl+B opens it; a click or Esc closes it; and in log mode `f` opens and closes the panel.
- [ ] Run a fresh review of the whole branch against `main` with this Review Focus. Fix Critical and Important findings test-first, then triage the minors with Indi.
- [ ] Open the PR (`Closes #204`). It says the two new goldens are on purpose and lists the design calls above. Don't merge.
