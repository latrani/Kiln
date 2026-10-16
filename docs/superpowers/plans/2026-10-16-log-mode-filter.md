# Log mode: filters always there Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Log mode stops having a filter submode. The filter panel always replaces the sidebar in log mode, keyboard focus moves between the log and the panel, each has its own hints footer, character-switching keys do nothing there, and both sidebars get a title (#202).

**Architecture:** Front-end only. The core already holds the filter and the folds per character (`app.Char.Filter`, `Collapsed`). The TUI's `browse.Panel` becomes always set while log mode is up, plus a `panelFocus` flag. Key routing in `browse.key` splits keys three ways: shared keys go to the focused side, and the other two groups of keys work from either side. The web does the same with real DOM focus, recorded in `logView`.

**Tech Stack:** Go (bubbletea v2), TypeScript/Preact web page, the string catalog (`internal/str`), themes (`internal/theme`).

**Spec:** this plan's Design section, settled in the #202 thread on 2026-10-16.

## Design

- **Panel always there:** entering log mode shows the filter panel in place of the connection sidebar, for as long as log mode is up. The top bar's Filter chip is gone, along with its string and roles.
- **Panel colors:** the panel takes the sidebar's colors (celadon), not cobalt.
- **Focus:** keyboard focus is on the log or the panel, and starts on the log.
  - `f` moves focus to the other side.
  - A click on either side focuses that side.
  - Esc (or Ctrl+C) in the panel moves focus back to the log.
  - Esc in the log backs out one step at a time, as today: marking, then find, then leaving log mode.
- **Keys:**
  - **Shared keys** go to the focused side: ↑↓, j/k, Space, Esc, Ctrl+C and Enter.
  - **Log keys** work whichever side has focus: `s` `/` `n` `N` `g` `c` `e` PgUp PgDn Home End.
  - **Panel keys** work whichever side has focus: `h` `o` ← → `x` Delete. They act on the panel's highlighted entry.
- **Footers:** the log's action bar and the panel's own footer show the keys that work right now. The unfocused side's footer drops the shared keys.
- **Switching keys:** Tab, Shift+Tab and Ctrl+↑↓ (on the web, Alt+↑↓ and Ctrl+↑↓) do nothing in log mode.
- **Leaving and coming back:**
  - The filter and the folds last until Kiln quits, as now.
  - Ctrl+L or the Log chip keeps log mode as it was left, focus included.
  - Esc out of log mode starts fresh next time: focus on the log, the highlight on the first entry.
- **Titles:**
  - The connection sidebar's first row is `Connections`, the picker's list's is `All connections`, and the panel's is `Filters`.
  - The title runs the full column width, underlined, so it reads as a rule between the title and the list.
- **Web:** copies all of it.

## Global Constraints

- Every user-facing string lives in `internal/str/locales/en.toml`; run `go generate ./internal/str` after editing it (regenerates Go and `web/ui/src/str.gen.ts`).
- Tests build expected text from the catalog, never a copy.
- Commits that touch `en.toml` end with a `Strings:` trailer listing the keys (new / changed / removed).
- New look → new theme role in `roles.go` (and `Roles`), `default.toml`, and the role list in `docs/themes.md`; removed roles leave all three. No escape codes in `internal/ui`.
- TUI goldens get `-update` only when the look changes on purpose; say so in the commit. No web PNG goldens exist yet.
- Below 1.0: removed things just go (no `status.filter` alias, no `view.filter_button`).
- Names in tests: Kit, Rook, Ash; world `fm`.
- Try Kiln only against a scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`.
- Web copies the TUI. On the web, use the browser's own focus, not a mimic of the TUI's.

## Review Focus

1. **The filter-text prompt** (`enter` on `+ Text`): while it's open it owns every key. `f`, `h` and `s` type letters; they don't move focus or act. Pinned in Task 2.
2. **Find is on and the panel has focus:** `browse.key` stops a search on any key (`StopSearch` at the top), so panel keys must still work after it, and Esc must only stop find. Pinned in Task 2.
3. **Ctrl+L away and back:** the side that had focus still has it, and the highlight and scroll are kept. Pinned in Task 2.
4. **A short or narrow screen** (`MinHeight` and `MinWidth`): the title, the panel list and the wrapped footer must fit without a panic or a negative height. With too little room the footer gives up rows before the list loses all of its rows. Pinned in Task 3.
5. **Web focus nowhere** (a click on the status line or the top bar takes focus off both): the footers fall back to the log as focused, and the next key reaches the log. Pinned in Task 5.

Side effects to keep: `togglePanel`'s close also ended the `+ Text` prompt's return path (the web's `LogBar.back` puts focus back on the panel after a filter prompt). After this change, Esc from the filter prompt returns focus to the panel. `hideBrowse`'s `StopSearch`/`ClearStatus` stays as is.

---

### Task 1: Sidebar titles (TUI)

**Files:**
- Modify: `internal/str/locales/en.toml` (`[view]`; `[filter]`)
- Modify: `internal/theme/roles.go`, `internal/theme/default.toml`, `docs/themes.md`
- Modify: `internal/ui/sidebar.go` (`sidebarView` ~155: lays out over `m.height-1` rows when titled), `internal/ui/view.go` (View's sidebar loop ~448), `internal/ui/model.go` (sidebar clicks ~1065 and wheel: `y-1` when titled)
- Test: `internal/ui/sidebar_test.go`; goldens

**Interfaces:**
- Produces:
  - Strings: `view.connections_title = "Connections"`, `picker.title = "All connections"`, `filter.title = "Filters"`.
  - Roles: `SidebarTitle Role = "sidebar.title"`, `PickerTitle Role = "picker.title"` and `FilterTitle Role = "filter.title"`, each `{ underline = true, bold = true }` in `default.toml`. Each inherits its area's colors.
  - Every sidebar column is titled, so every `y` that maps to a row is offset by one.
  - `func sideTitle(r theme.Role, label string, w int) string`: the label padded to `w` and painted in `r`, so the underline spans the column.

- [ ] **Step 1: Add the strings and roles**, run `go generate ./internal/str`.
- [ ] **Step 2: Write the failing tests** in `sidebar_test.go`:
  - `TestSidebarTitle`: row 0 of the sidebar column, stripped and trimmed, is `str.ViewConnectionsTitle()`. The raw row contains `theme.Paint(theme.SidebarTitle, …)` padded to `l.sw`. The first world row is on row 1.
  - `TestSidebarClickUnderTitle`: with Kit and Rook open, a click on Rook's row (found by searching the screen for its name, not computed) switches to Rook. A click on row 0 does nothing.
  - `TestPickerListTitle`: with the picker open, row 0 is `str.PickerTitle()` painted in `theme.PickerTitle`. A click on a world row, found by searching the screen, selects that world.
- [ ] **Step 3: Run to verify they fail**: `go test ./internal/ui -run 'TestSidebar(Title|ClickUnderTitle)|TestPickerListTitle'`: FAIL.
- [ ] **Step 4: Implement.** In the sidebar, every `y` that maps to `sv.at(y)` becomes `y-1`, both drawing and clicks, and `sidebarView` lays out over `m.height-1` rows. The wheel is unaffected.
- [ ] **Step 5: Run** `go test ./internal/ui`. Only goldens should fail; check that their diffs are just the title row and the list shifting down one, then `-update`.
- [ ] **Step 6: Commit**: `Sidebar: a Connections title (#202)`. Mention the golden change and add a `Strings: view.connections_title, picker.title, filter.title (new)` trailer.

---

### Task 2: Log mode: panel always up, focus, keys (TUI)

**Files:**
- Modify: `internal/ui/browse.go` (`browse` struct ~46: `panelFocus bool`; `key` ~88; `hints` ~176; the `b.Panel != nil` guards ~231)
- Modify: `internal/ui/filter.go` (`togglePanel` goes; `panelKey` returns whether it took the key; `panelClick` focuses)
- Modify: `internal/ui/model.go` (`openBrowse` ~933 creates the Panel; switching keys ~646-657; body click focuses the log; wheel/click `Panel != nil` guards ~1010, ~1065)
- Modify: `internal/ui/view.go` (`topBarFor`: drop the Filter chip and its `filterChip` return; `topClick` ~364)
- Modify: `internal/str/locales/en.toml` (drop `view.filter_button`), `internal/theme/roles.go`, `default.toml`, `docs/themes.md` (drop `status.filter`, `status.filter.on`)
- Test: `internal/ui/browse_test.go` (the filter tests near `kitFilter` ~841), `internal/ui/topbar_test.go`; goldens `log.txt`, `log-filter.txt`

**Interfaces:**
- Consumes: Task 1's titled sidebar (the panel draws under the `Filters` title: `panelView(w, h)` gets `h-1`, and clicks get `y-1`).
- Produces:
  - `browse.panelFocus bool`: false means the log has focus.
  - `func (b *browse) panelKey(k tea.KeyPressMsg) (cmd tea.Cmd, took bool)`: handles panel keys. It takes the shared ones only when `panelFocus` is set. Esc and Ctrl+C there clear `panelFocus`.
  - `func (b *browse) newPanel()`: sets `Panel` on the first entry with `follow`, and clears `panelFocus`. `openBrowse` calls it for a fresh log mode.
  - `topBar`/`topBarFor` return `(line string, logChip, presenceChip [2]int)`.
  - `var panelOnlyKeys = map[string]bool{"h", "o", "left", "right", "x", "delete"}` and `var sharedKeys = map[string]bool{"up", "down", "k", "j", "space", " ", "esc", "ctrl+c", "enter"}` in `keys.go`.
- Routing in `browse.key`, after `StopSearch` and the prompt:
  1. `f` flips `panelFocus` (follow on when it goes to the panel).
  2. Shared keys go to `panelKey` when the panel has focus, otherwise to the log's switch.
  3. `panelOnlyKeys` go to `panelKey` wherever focus is.
  4. Anything else goes to the log's switch.

- [ ] **Step 1: Update the strings and roles**, run `go generate ./internal/str`.
- [ ] **Step 2: Write the failing tests** in `browse_test.go`:
  - `TestLogModePanelAlwaysUp`: `ctrl+l`, then the sidebar column's row 0 is `str.FilterTitle()` and the panel's first entry shows. The top bar has no Filter chip, so assert on its spans.
  - `TestLogModeFocus`: focus starts on the log, and `down` moves the log cursor, not the highlight. `f`, then `down` moves the highlight. `esc` gives focus back to the log, still in log mode. One more `esc` (nothing marked, no find) leaves log mode.
  - `TestLogModeKeysFromEitherSide`: with the panel focused, `s` starts marking. With the log focused, `h` hides the highlighted entry (`kitFilter().Hidden(item)`).
  - `TestLogModeClickFocuses`: a click on a panel name row focuses the panel, and a click in the log body focuses the log.
  - `TestLogModePromptOwnsKeys` (Review Focus 1): with the panel focused on `+ Text`, `enter` opens the prompt. Typing `fhs` makes the prompt's value `fhs`, with focus and marking unchanged. Esc closes the prompt with focus still on the panel.
  - `TestLogModeFindThenPanel` (Review Focus 2): `/` with a term turns find on. `f` then focuses the panel and stops find. With find on and the panel focused, `esc` stops find only, and the panel keeps focus.
  - `TestLogModeHideKeepsFocus` (Review Focus 3): with the panel focused, `ctrl+l` twice still has the panel focused and the highlight unchanged.
  - `TestLogModeNoSwitching`: with Rook open and unread, `tab`, `shift+tab`, `ctrl+up` and `ctrl+down` in log mode leave Kit active.
- [ ] **Step 3: Run to verify they fail**: `go test ./internal/ui -run TestLogMode`: FAIL.
- [ ] **Step 4: Implement.** `model.go`'s switching-key cases skip when `cs != nil && cs.browse != nil`, and the key falls through to `browse.key`, which ignores it. Recolor `filter.*` in `default.toml` to the `sidebar.*` glazes: `filter` uses `glaze.celadon-ink`; `.button` and `.more` use `celadon-mid`; `.button.on` and `.add` use `celadon-bright` on `celadon-deep`.
- [ ] **Step 5: Run** `go test ./internal/ui ./internal/theme ./internal/str`. Fix the existing filter and top-bar tests that opened the panel with `f` or `togglePanel`: they now press `f` to focus, or just start in log mode. Check the golden diffs (log mode now shows the panel and its title) and `-update`.
- [ ] **Step 6: Commit**: `Log mode: the filter panel is always there, focus moves with f (#202)`. Mention the golden change and add a `Strings: view.filter_button (removed)` trailer.

---

### Task 3: Footers that follow focus (TUI)

**Files:**
- Modify: `internal/str/locales/en.toml` (`[browse]` hints ~304; `[filter]` hints ~323)
- Modify: `internal/ui/browse.go` (`hints`), `internal/ui/filter.go` (`panelView` draws the footer)
- Test: `internal/ui/browse_test.go`; goldens

**Interfaces:**
- Consumes: Task 2's `panelFocus`.
- Produces:
  - Strings, changed:
    - `browse.hints_none = "s select · / find · g date · f filters · esc back"`
    - `browse.hints_selected = "s select · space exclude · c copy · e export · / find · g date · f filters · esc back"`
    - `filter.hints = "h hide · o only · space ←→ fold · enter add · x remove · f esc log"`
  - Strings, new:
    - `browse.hints_none_away = "s select · / find · g date"`
    - `browse.hints_selecting_away = "s done · c copy · e export"`
    - `browse.hints_selected_away = "s select · c copy · e export · / find · g date"`
    - `filter.hints_away = "h hide · o only · ←→ fold · x remove"`
  - (Wording is a first pass; the PR asks Indi to look at it.)
  - `func (b *browse) panelHints() string`: `filter.hints` when the panel has focus, `filter.hints_away` otherwise.
  - `func footerRows(hints string, w int) []string`: splits `hints` on `str.Separator()` and packs the items into rows of width `w` without splitting an item. An item wider than `w` is cut with `…`.
  - The panel column, top to bottom: the title, the list, a `RuleFilter` rule row, then the footer rows. The footer takes at most `h/3` rows; with less room than that it drops its last items, so the list keeps at least 3 rows.
  - New role: `RuleFilter Role = "rule.filter"`, with the same look as `rule.input`.

- [ ] **Step 1: Update the strings and add the role**, run `go generate ./internal/str`.
- [ ] **Step 2: Write the failing tests:**
  - `TestLogFooters`: with the log focused, the log bar shows `str.BrowseHintsNone()` and the panel's footer rows, joined with `str.Separator()`, equal `str.FilterHintsAway()`. After `f` the log bar shows `str.BrowseHintsNoneAway()` and the panel footer shows `str.FilterHints()`. While marking with the panel focused, it's `str.BrowseHintsSelectingAway()`.
  - `TestFooterRows`: `footerRows("a b · c d · e", 5)` is `["a b", "c d", "e"]`. An item wider than `w` comes out cut with `…`.
  - `TestLogModeShortScreen` (Review Focus 4): at `MinWidth`×`MinHeight` in log mode, `View()` doesn't panic, the panel list has at least one entry row, and the column has exactly `m.height` rows.
- [ ] **Step 3: Run to verify they fail**: `go test ./internal/ui -run 'TestLogFooters|TestFooterRows|TestLogModeShortScreen'`: FAIL.
- [ ] **Step 4: Implement.**
- [ ] **Step 5: Run** `go test ./internal/ui ./internal/theme ./internal/str`. Check the golden diffs (the footer and the new hint text) and `-update`.
- [ ] **Step 6: Commit**: `Log mode: each side's footer shows the keys that work (#202)`. Mention the golden change and add a `Strings:` trailer listing the changed and new keys.

---

### Task 4: Web: sidebar titles and the always-there panel

**Files:**
- Modify: `web/ui/src/ui/Sidebar.tsx` (title row; FilterPanel always in log mode), `web/ui/src/ui/FilterPanel.tsx` (title, footer, focus tracking), `web/ui/src/ui/TopBar.tsx` (drop the Filter chip), `web/ui/src/ui/LogBar.tsx` (focus-aware hints; `back` after a filter prompt focuses the panel)
- Modify: `web/ui/src/filter.ts` (`togglePanel`/`closePanel` → `focusPanel(store)` / `focusLog(store)`), `web/ui/src/log.ts` (`logKey` routing, `toggleLog`'s fresh open sets `panel`), `web/ui/src/store.ts` (`LogView.panel` always set in log mode; new `LogView.focus?: "log" | "panel"`), `web/ui/src/presence.ts` (switching keys skipped in log mode), `web/ui/src/kiln.css` (title underline, footer)
- Test: `web/ui/src/ui/FilterPanel.test.tsx`, `web/ui/src/ui/LogView.test.tsx`, `web/ui/src/filter.test.ts`, `web/ui/src/log.test.ts`, `web/ui/src/presence.test.ts`, `web/ui/src/store.test.ts`

**Interfaces:**
- Consumes: Tasks 1-3's strings and roles (`str.viewConnectionsTitle()`, `str.pickerTitle()`, `str.filterTitle()`, `str.filterHints()`, `str.filterHintsAway()`, `str.browseHints*Away()`, `sidebar.title`, `picker.title`, `filter.title`, `rule.filter`).
- Produces:
  - `LogView.focus` is set by `focusin` on the panel (`"panel"`) and on the log (`"log"`). The footers read it, and treat `undefined` as `"log"`.
  - `focusPanel(store)` and `focusLog(store)` move DOM focus and record it. `f` calls whichever doesn't have focus.
  - In the panel's `onKeyDown`, a log key goes to `logAct`. In the log's key handler, a panel-only key goes to `panelAct`. Shared keys are left to the element that has focus, as the browser does.
  - `switchAction` gains a `logMode: boolean` argument and returns `undefined` when it's set.

- [ ] **Step 1: Write the failing tests:**
  - The sidebar's first child renders `str.viewConnectionsTitle()` with `roleClass("sidebar.title")`, and with the picker open `str.pickerTitle()` with `roleClass("picker.title")`. In log mode the sidebar renders the FilterPanel under `str.filterTitle()`, with no Filter chip in the top bar.
  - Footers: with `focus` undefined, the log bar shows `str.browseHintsNone()` and the panel footer `str.filterHintsAway()`. With `focus: "panel"` they show `str.browseHintsNoneAway()` and `str.filterHints()` (Review Focus 5: `undefined` means the log).
  - `switchAction({key: "ArrowDown", altKey: true, …}, true)` is `undefined`. With `false` it's `next_unread`.
  - `logKey`/panel routing: `filterKey({key: "h"})` from the log side gives `"hide"`. A log key from the panel gives the log action (e.g. `s` → select), and Esc in the panel gives `"close"`, which now means focus the log.
  - A fresh log mode has `panel` set on the first entry and `focus` unset.
- [ ] **Step 2: Run to verify they fail**: `cd web/ui && npm test`: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `cd web/ui && npm run check && npm test`, then `go test ./...`. PASS.
- [ ] **Step 5: Commit**: `Web: log mode's filters always there, as the TUI's (#202)`.

---

### Task 5: Docs, smoke, review, PR

- [ ] Update `docs/log-mode.md`. The filter section and the Esc/Ctrl+L paragraph (line 5, line 32) describe focus instead of opening and closing the panel. Also update `docs/using.md` wherever it mentions the Filter chip or `f` to open filters.
- [ ] Commit: `Docs: log mode's filters (#202)`.
- [ ] Smoke-test with a scratch `XDG_CONFIG_HOME` and `XDG_DATA_HOME` in tmux: go into log mode, move focus with `f`, hide with `h` from the log, start select mode with `s` from the panel, and check that Esc backs out one step at a time and that Tab does nothing.
- [ ] Run a fresh review of the whole branch against `main` with this Review Focus. Fix Critical and Important findings test-first, then triage the minors with Indi.
- [ ] Open the PR. Its description lists every string, new, changed or removed, with why. It says the goldens changed on purpose. It asks Indi to look at the hint wording. Don't merge.
