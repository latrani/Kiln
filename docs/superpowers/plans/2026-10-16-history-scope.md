# History scope Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The history panel lists either this character's lines (as now) or every line sent from any input since Kiln started, switched by a Session/Character segmented control next to the History chip, by click or Tab; and the History chip and panel are gone while nothing is open (#201).

**Architecture:** The core (`internal/app`) owns one shared list of everything sent this run plus the global scope; every `History` points at it, and `Shown` reads whichever scope is set. Front ends only draw the scope and send "swap"/"set". The scope is in memory only, never saved.

**Tech Stack:** Go (bubbletea v2), TypeScript/Preact web page, the string catalog (`internal/str`), themes (`internal/theme`).

**Spec:** this plan's Design section, settled in the #201 thread on 2026-10-16.

## Design

- **Character scope** (the default): what the panel lists now, meaning this character's logs plus what it's sent this run.
- **Session scope:** every line added to any input's history since launch, in send order, including slash commands from the idle input. No world·character tags. Repeats collapse to their newest, as `Shown` already does.
- **Control:** while the panel is up, a segmented control of two touching chips, Session then Character, sits left of the History chip with the usual separator between them. The current scope's half is lit. Clicking a half sets that scope. Tab or Shift+Tab swaps it.
- **Persistence:** one global setting. It holds across opens and character switches, and lasts until Kiln quits.
- **Placeholder:** with the panel up and the query empty, the search row reads `type to search · tab to change scope`.
- **Nothing open:** no History chip, and ↑ doesn't open the panel (as the Log chip). Disconnected characters keep both.
- **Scope change** selects the newest shown line.

## Global Constraints

- Every user-facing string lives in `internal/str/locales/en.toml`; run `go generate ./internal/str` after editing it (regenerates Go and `web/ui/src/str.gen.ts`).
- Tests build expected text from the catalog (`str.ViewScopeSession()`), never a copy.
- Commits that touch `en.toml` end with a `Strings:` trailer listing the keys.
- New look → new theme role in `roles.go` (and `Roles`), `default.toml`, and the role list in `docs/themes.md`; never an escape code in `internal/ui`.
- Golden screens get `-update` only in the TUI task, and that commit says the look changed on purpose.
- Names in tests: Kit, Rook, Ash; world `fm`.
- Try Kiln only against a scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`.
- Below 1.0: no shims for removed behavior (the idle input's panel just goes).
- Web copies the TUI; differences only where the browser forces them.

## Review Focus

1. **Passwords:** a password typed at the prompt must never land in the session list (it goes through `Login`, not `Add`). Pinned in Task 1.
2. **Closing a character:** its lines stay in the session list after `Close`. Pinned in Task 1.
3. **A line sent from another input while a panel's up in session scope** (the web and TUI share one App): `Shown` must refresh without a reopen. Pinned in Task 1 (generation counter).
4. **Tab with the panel up** must swap scope, not jump to the next unread character; with the panel down, or in log mode or the picker, Tab keeps its meaning. Pinned in Task 3.
5. **Narrow statusline:** if the scope chips plus History don't fit beside the message, the scope chips drop first, then History (today's rule). Clicks must hit the spans actually drawn. Pinned in Task 3.

Side effects to keep: `HistoryOpen` today also calls `a.moreSent()` (pages in older logged lines); keep it for both scopes. `Close` resets query/draft/back, not the scope.

---

### Task 1: Core scope and shared list

**Files:**
- Modify: `internal/app/history.go` (struct `History`, `Add`, `Shown`; new `Scope`, `sentLog`)
- Modify: `internal/app/app.go` (App fields ~line 121; `Char` creation ~274 wires `History.all`)
- Modify: `internal/app/submit.go` (`History`, `HistoryOpen` ~90-106; new `HistoryScope`, `SetHistoryScope`, `SwapHistoryScope`)
- Test: `internal/app/history_test.go`, `internal/app/submit_test.go`

**Interfaces:**
- Produces:
  - `type Scope int` with `ScopeCharacter` (zero value) and `ScopeSession`.
  - `type sentLog struct { lines []Sent; scope Scope; gen int }`: App owns one (`a.all`); `gen` bumps on every append and every scope change.
  - `History` gains `all *sentLog` and `allGen int`. `Add` also appends to `h.all.lines` when `h.all != nil` (same skip-empty/skip-repeat rule, judged against `h.all`'s last). `Shown` lists `h.all.lines` in session scope, else `run`+`logged` as now. It recomputes when `stale || h.all.gen != h.allGen`.
  - `func (a *App) HistoryScope() Scope`
  - `func (a *App) SetHistoryScope(s Scope) []Effect`: sets scope, bumps gen, resets the active panel's selection to newest (`back = 0`), returns `a.moreSent()`.
  - `func (a *App) SwapHistoryScope() []Effect`: the other scope, through `SetHistoryScope`.
  - `HistoryOpen` returns nil and leaves the panel shut when `a.chars[a.active] == nil`.

- [ ] **Step 1: Write the failing tests**

In `history_test.go`:
- `TestHistorySessionScope`: two `History` values sharing one `*sentLog`; Add "look" to the first, "say hi" to the second, "look" to the first again. Open the first; in character scope `texts(Shown())` is `["look"]`; set `all.scope = ScopeSession` → `["say hi", "look"]` with no reopen needed (gen). Then Add "wave" to the second while open → Shown ends `"wave"`.

In `submit_test.go` (use `testApp`, `openAll`):
- `TestHistoryScopeAcrossChars`: open fm/kit and fm/rook, send a /command from each (commands reach `Add` without a connection) plus one from the idle input before opening (from nothing open). `SwapHistoryScope()`; open the panel on rook → Shown is all three in order. Switch active to kit, open → still session scope (persists). Close the panel and reopen → still session.
- `TestHistoryScopeKeepsClosedChar`: send from kit, `Close` kit, open rook's panel in session scope → kit's line is listed.
- `TestHistoryScopeNoPassword`: put kit at the password prompt (`startPassword`), `SetInput("hunter2")`, `Submit()`; session-scope Shown doesn't contain it.
- `TestHistoryScopeSelectsNewest`: with three lines shown, `HistoryMove(-2)`, then `SwapHistoryScope()` → `Sel() == len(Shown())-1`.
- `TestHistoryClosedWithNothingOpen`: nothing open, `HistoryOpen()` → `History().IsOpen()` is false.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/app -run 'TestHistory(SessionScope|ScopeAcrossChars|ScopeKeepsClosedChar|ScopeNoPassword|ScopeSelectsNewest|ClosedWithNothingOpen)'`
Expected: FAIL (undefined `sentLog`, `SwapHistoryScope`, …)

- [ ] **Step 3: Implement the interfaces above**

Wire `all: &a.all` into each new `Char`'s `History` and into `a.idleHist` (in `New`). `Shown`'s dedupe loop already takes a list of parts; session scope passes `[][]Sent{h.all.lines}`.

- [ ] **Step 4: Run the package**

Run: `go test ./internal/app`
Expected: PASS, the existing history tests included.

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -m "History: session scope, shared across inputs (#201)"
```

---

### Task 2: Feed and web API

**Files:**
- Modify: `internal/feed/events.go` (`History` ~line 66), `internal/feed/feed.go` (`history` ~365)
- Modify: `internal/webapi/actions.go` (~line 133)
- Test: `internal/feed/feed_test.go`, the webapi action tests (where `history_toggle` is tested; find with `grep -rn history_toggle internal/webapi`)

**Interfaces:**
- Consumes: Task 1's `HistoryScope`, `SetHistoryScope`, `SwapHistoryScope`, `ScopeSession`, `ScopeCharacter`.
- Produces:
  - `feed.History` gains `Scope string`: `"character"` or `"session"` (`//str:ok`: protocol tokens). It's set whether or not the panel is open.
  - Actions: `{Do: "history_scope", Text: "session"|"character"}` sets the scope. `{Do: "history_scope"}` with no Text swaps it.

- [ ] **Step 1: Write the failing tests**: in feed, open a char's panel, swap scope, `history(a).Scope == "session"`. In webapi, both action forms move `a.HistoryScope()` as described.
- [ ] **Step 2: Run to verify they fail**: `go test ./internal/feed ./internal/webapi`: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run**: `go test ./internal/feed ./internal/webapi`: PASS.
- [ ] **Step 5: Commit**: `git commit -m "Feed and web API: history scope (#201)"`

---

### Task 3: TUI: the control, Tab, placeholder, no chip with nothing open

**Files:**
- Modify: `internal/str/locales/en.toml` (`[view]`, beside `history_button` ~line 146)
- Modify: `internal/theme/roles.go` (~line 69), `internal/theme/default.toml` (~line 65), `docs/themes.md` (status role list, line 63)
- Modify: `internal/ui/view.go` (`statusBar` ~212; input rows with the panel ~76)
- Modify: `internal/ui/model.go` (Tab ~652; status click ~1107)
- Test: `internal/ui/historypanel_test.go`; golden `internal/ui/testdata/golden/history.txt` (and `history-scrolled.txt` if it changes)

**Interfaces:**
- Consumes: Task 1's API.
- Produces:
  - Strings: `view.scope_session = "Session"`, `view.scope_character = "Character"`, `view.history_placeholder = "type to search · tab to change scope"`.
  - Roles: `StatusScope Role = "status.scope"` (a half not chosen) and `StatusScopeOn Role = "status.scope.on"` (the chosen half). Defaults copy `status.history` / `status.history.on`.
  - `func (m *Model) statusBar(w int) (line string, chips statusChips)` where `type statusChips struct{ main, session, character [2]int }` (zero spans when not drawn). The click handler tests each span: `main` keeps today's behavior; `session`/`character` call `m.a.SetHistoryScope`, then `pullInput`/`pullIdle` as `historyToggle` does.

- [ ] **Step 1: Add the strings and roles**, run `go generate ./internal/str`.

- [ ] **Step 2: Write the failing tests** in `historypanel_test.go`:
  - `TestHistoryScopeChips`: open the panel. The bottom row ends with `chipText(str.ViewScopeSession())+chipText(str.ViewScopeCharacter())+str.Separator()+chipText(str.ViewHistoryButton())` once stripped, and Character is painted `StatusScopeOn` (compare with `theme.Paint`). Close the panel and the scope chips are gone.
  - `TestHistoryScopeTab`: with a second character holding unread lines, open the panel and press `tab`. The scope is session, and the active character is unchanged. Press `shift+tab` to swap back. Close the panel and `tab` switches characters as before.
  - `TestHistoryScopeClick`: a click on the Session span sets session, and a click on the Character span sets it back. Take the spans from `statusBar`.
  - `TestHistoryScopeNarrow`: at a width where all three chips don't fit beside the message but History does, only History is drawn and the scope spans are zero.
  - `TestHistoryPlaceholder`: open with an empty query and the query row is `glyph.Search + str.ViewHistoryPlaceholder()` (stripped, trimmed). Type `x` and the placeholder is gone.
  - `TestHistoryChipNothingOpen`: with nothing open, the bottom row has no `str.ViewHistoryButton()` and `up` leaves the panel shut. A disconnected character still shows the chip.

- [ ] **Step 3: Run to verify they fail**: `go test ./internal/ui -run 'TestHistory(Scope|Placeholder|ChipNothingOpen)'`: FAIL.

- [ ] **Step 4: Implement.**
  - `statusBar`'s `default:` case returns no chip when `cs == nil`. While `historyShows()`, it draws the scope pair (touching, no gap) plus `str.Separator()` before History. If that doesn't fit, it falls back to History alone.
  - Tab: in `model.go`'s `"tab", "shift+tab"` case, when `m.historyShows()` and there's no picker, `m.run(m.a.SwapHistoryScope())` and return.
  - Placeholder: in the panel loop in `view.go`, when `m.a.History().Query() == ""`, the query row is `glyph.Search + theme.Paint(theme.InputHint, fitName(str.ViewHistoryPlaceholder(), l.rw-gutterWidth))`. The cursor column stays at the start.

- [ ] **Step 5: Run**: `go test ./internal/ui ./internal/str ./internal/theme`: PASS except the goldens; inspect `go test ./internal/ui -run Golden` diffs, and if they're exactly the new chips/placeholder, `go test ./internal/ui -run Golden -update`.

- [ ] **Step 6: Commit**

```bash
git add internal docs/themes.md
git commit -m "TUI: Session/Character scope by the History chip (#201)

The history goldens change on purpose: the scope chips and the
search placeholder.

Strings: view.scope_session, view.scope_character, view.history_placeholder (new)"
```

---

### Task 4: Web: the same

**Files:**
- Modify: `web/ui/src/store.ts` (`HistoryPanel` ~86 gains `Scope: "character" | "session"`)
- Modify: `web/ui/src/ui/StatusLine.tsx` (~56-67)
- Modify: `web/ui/src/input.ts` (`KeyDo`, `keyAction` ~53), `web/ui/src/ui/Input.tsx` (~83, ~121, `placeholder`)
- Test: `web/ui/src/ui/HistoryPanel.test.tsx`, `web/ui/src/input.test.ts` (or wherever `keyAction` is tested), `web/ui/src/ui/LogView.test.tsx:98` neighbor for the chip

**Interfaces:**
- Consumes: Task 2's `Scope` and `history_scope`; Task 3's strings and the `status.scope`/`status.scope.on` roles (`roleClass`).
- Produces: `KeyDo` gains `"history_scope"`.

- [ ] **Step 1: Write the failing tests**:
  - While `historyShows`, the status line renders two buttons in order, `str.viewScopeSession()` then `str.viewScopeCharacter()`, with classes `status.scope(.on)` per `history.Scope`, before the History button. Clicking one sends `{Do: "history_scope", Text: "session"}`.
  - With no character open (`active: ""`), there's no History button.
  - `keyAction({key: "Tab"}, caret, true)` and Shift+Tab with `panel` both give `"history_scope"`. Without `panel`, Tab gives `undefined` (the browser keeps Tab).
  - With the panel up and an empty value, the textarea's `placeholder` is `str.viewHistoryPlaceholder()`.
- [ ] **Step 2: Run to verify they fail**: `cd web/ui && npm test`: FAIL.
- [ ] **Step 3: Implement.** The chip condition gains `!!state.chars[state.active]`. The segmented pair goes in a wrapper so the halves touch, with the same separator the TUI uses. Tab in the input sends `{Do: "history_scope"}` and `preventDefault`s **only while the panel is up**. That's a browser-forced call: Tab normally moves focus, and Esc still closes the panel and gives Tab back.
- [ ] **Step 4: Run**: `cd web/ui && npm run check && npm test`: PASS; then `go test ./...`.
- [ ] **Step 5: Commit**: `git commit -m "Web: history scope, as the TUI's (#201)"`

---

### Task 5: Smoke and PR

- [ ] With a scratch `XDG_CONFIG_HOME`/`XDG_DATA_HOME`, run Kiln (`go run ./cmd/kiln`) with fm open for Kit and Rook. Send a /command from each, open the panel, Tab, and check both scopes and the placeholder.
- [ ] Run a fresh review of the whole branch against `main`, with this Review Focus. Fix Critical and Important findings test-first, then triage the minors with Indi.
- [ ] Open the PR. The description lists the three strings and why each exists, and says the goldens changed on purpose. Don't merge.
