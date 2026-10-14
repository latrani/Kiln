# Web UI step 4b: the editor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The page can add and edit worlds and edit and delete characters the way the TUI does. The editor is a real form in the bottom pane, opened from the picker, Ctrl+T/Alt+T, `/edit` or a world's overview. Its edits survive hiding, and an editor opened from the main view stays with what it was opened on.

**Architecture:**
- **Go:** gains one action, `settings`. It reads a world's or character's settings, plus what they inherit, into a `Settings` event. Everything else already exists in part 2 (`add_world`, `save_world`, `save_character`, `delete_world`, `delete_character`, `Saved`/`Problems`, `check()`).
- **`editor.ts`:** the TUI's form rules (which fields, values to and from settings, unsaved, errors) as pure functions.
- **The store:** holds the open editor (in the picker, as the TUI's `picker.edit`), the hidden drafts and the parked editors. It settles Go's answers in `apply`, so a save, a close and a switch in one batch happen in the TUI's order.
- **`EditorForm.tsx`:** draws it with native controls.

**Tech Stack:** Go (internal/app, internal/webapi), Preact + TypeScript (web/ui), node:test with preact-render-to-string, headless Chrome smoke (web/smoke).

**Spec:** `docs/superpowers/specs/2026-10-12-web-ui-design.md`. This plan covers step 4, "The bottom pane's forms", second half. The relevant parts are "Controls", "Keys" and "For you to check" 2 and 3. 4a (the picker) is merged.

## For you to check

These are the calls this plan makes that change what someone sees. Everything else copies the TUI (`internal/ui/editor.go`, `form.go`, `picker.go`).

1. **Enter saves, Tab moves.** The spec decided this (its item 2). The hint is a new web string: `Enter to save · Tab moves between fields · Esc to cancel`. ↑/↓ don't move between fields as they do in the TUI: in a choice they change the choice (the browser's radio keys).
2. **Errors show beside their field, every one.** The TUI shows only the first problem, on the hint line. The spec asks for each beside its field. An error about no one field goes at the top of the form. After a refused save, focus goes to the first field with an error, and `More settings` opens if that field is inside it.
3. **Live checks don't refuse keystrokes.** The TUI throws away a keystroke its check refuses. The web lets you type it and shows the check's message beside the field until you fix it (spec item 6). Save still refuses it.
4. **Delete asks twice with "Press again to delete fm".** The TUI's message says "Enter again", but on the web you can click as well. It's a new web string. The character version drops "and forget the password", because the web never stores one.
5. **No `Forget saved password` button on the web.** The web never stores a password (kiln-web gives the core no password store), so there's nothing to forget.
6. **Focus always shows, in the TUI's `form.focus` look,** on the focused button, `More settings`, checkbox box or choice (with its `◄ ►`). A text field shows its caret, as the TUI's.
7. **Clicking a picker row while an editor is open** keeps the editor's edits as a draft, as the TUI's. The picker stays open, even for an editor opened from the main view: the click was in the picker.

## Global Constraints

- **The TUI is the reference** where the spec doesn't say otherwise: `internal/ui/editor.go` (fields, order, drafts, parking), `form.go` (looks), `picker.go` (world rows, `+ World`, Ctrl+T in the picker).
- **Go decides data; the page decides looks,** from theme roles (`roleClass`, CSS variables), never a hard-coded color.
- **Every string a person reads comes from the catalog** (`internal/str/locales/en.toml` → `go generate ./internal/str` → `str.gen.ts`). A commit that changes the catalog carries a `Strings:` trailer.
- **Tests build expected text from the catalog** (`str.*`), never a copy of it.
- **Focus:** opening a form moves focus into it, and Esc puts focus back. The dimmed scrollback stays readable (not `inert`).
- **Native elements first.** ARIA only where none fits. No `<select>`.
- **Pre-1.0:** no compatibility shims for API shape changes.
- **The TUI's goldens never move.**
- **Config values** (`batch`, `pin`, `on`, …) and config keys are vocabulary and are never translated. `on` and `off` are shown through `editor.on`/`editor.off`, as in the TUI.

## Review Focus

1. **A save, a close and a switch in one batch.** Deleting the active character closes it, which changes `Active` in the same batch as `Saved`. The editor must close, not get parked under a character that's gone or stashed as a draft that reappears. → Task 4's "deleting the edited character closes the editor; nothing's parked or drafted".
2. **A draft for a world that's deleted** mustn't reappear when a new world of the same name is added. → Task 4's test.
3. **An answer that isn't the editor's,** such as the name form's `add_character` or an old `Saved` from before it opened, mustn't close or mark the editor. → Task 4's "an answer to something else leaves the editor be".
4. **Values round-trip.** Opening and saving untouched writes back exactly what was read: an unset setting stays unset (`null`), not `false` or `""`. → Task 3's round-trip test.
5. **Typing in an editor field never reaches the chat input,** the picker filter, or the global keys. Ctrl+O is eaten while an editor is open. → Task 6's `pickerKey` test and Task 7's smoke step.

---

## File Structure

- `internal/app/edit.go`: `NewWorldSettings`, `WorldSettings`, `CharacterSettings` (reads for the editors). Test in `edit_test.go`.
- `internal/ui/editor.go`: `openWorldEditor` uses `a.NewWorldSettings` (one place says what a new world starts from). No look changes.
- `internal/webapi/settings.go` (new): the `Settings` event and the `settings` action. Test in `loop_test.go`.
- `internal/webapi/actions.go`: routes `settings`.
- `web/ui/src/picker.ts`: world rows and `+ World` can be highlighted; `choose` opens editors; `editorKey` (Ctrl+T/Alt+T). Test in `picker.test.ts`.
- `web/ui/src/editor.ts` (new): the forms' fields, values, actions, errors. Test in `editor.test.ts`.
- `web/ui/src/store.ts`: the editor, drafts, parked editors, and settling Go's answers. Test in `store.test.ts`.
- `web/ui/src/ui/EditorForm.tsx` (new): the form. Test in `ui/EditorForm.test.tsx`.
- `web/ui/src/ui/Sidebar.tsx`: world rows as options; `+ World`.
- `web/ui/src/ui/BottomPane.tsx`: shows the editor.
- `web/ui/src/ui/PickerForm.tsx`: Ctrl+T on the highlighted row.
- `web/ui/src/theme.ts`, `kiln.css`: the controls' looks.
- `web/ui/src/main.tsx`: Ctrl+T/Alt+T, `/edit`.
- `internal/str/locales/en.toml`: `web.form_hint`, `web.delete_again` (new).
- `web/smoke/editor_test.go` (new): end to end.
- `docs/web-dev.md`: one line on the editor.

---

### Task 1: Go reads settings for the editor (`settings` → `Settings`)

**Files:**
- Modify: `internal/app/edit.go`
- Modify: `internal/ui/editor.go:225-240` (`openWorldEditor`'s new-world branch)
- Create: `internal/webapi/settings.go`
- Modify: `internal/webapi/actions.go` (`doSettings`)
- Test: `internal/app/edit_test.go`, `internal/webapi/loop_test.go`

**Interfaces:**
- Produces (Go): `func (a *App) NewWorldSettings() (config.WorldSettings, config.Inherited, error)`, `func (a *App) WorldSettings(world string) (config.WorldSettings, config.Inherited, error)`, `func (a *App) CharacterSettings(world, char string) (config.CharacterSettings, config.Inherited, error)`.
- Produces (page API): the action `{"Do":"settings","World":w,"Char":c}`. World `""` asks for a new world; Char `""` asks for the world itself.
  - The answer is an event `{"type":"Settings","World","Char","Name","WorldSettings"?,"CharSettings"?,"Inherited"}`.
  - A file that can't be read puts the error on the status line (`Status`, `Err: true`) and sends no `Settings`.
  - `WorldSettings` and `CharSettings` use Go's field names (`Host`, `Port`, `TLS`, `TLSTrust`, `Use`, `Login`, `MaxLineBytes`, `NewlineMode`, `Autoconnect`, `Reconnect`, `Notify`, `LocalEcho`; `Aliases`), with `null` for unset.

- [ ] **Step 1: Write the failing app test** (append to `internal/app/edit_test.go`; use the file's existing helper for an App on a temp config dir with `fm/kit`. If it's named differently from `newApp`, use that name.)

```go
func TestSettingsReadsForTheEditors(t *testing.T) {
	a, _ := newApp(t)
	s, inh, err := a.NewWorldSettings()
	if err != nil || !slices.Equal(s.Use, []string{"fuzzball"}) || inh.NewlineMode == "" {
		t.Errorf("new world: %+v %+v %v", s, inh, err)
	}
	w, _, err := a.WorldSettings("fm")
	if err != nil || w.Host == "" {
		t.Errorf("fm: %+v %v", w, err)
	}
	if _, _, err := a.CharacterSettings("fm", "kit"); err != nil {
		t.Errorf("fm/kit: %v", err)
	}
	if _, _, err := a.WorldSettings("nope"); err == nil {
		t.Error("a world that isn't there read fine")
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/app -run TestSettingsReadsForTheEditors`
Expected: FAIL to compile, `a.NewWorldSettings undefined`.

- [ ] **Step 3: Implement** (in `internal/app/edit.go`, after `DeleteWarning`; add the `os` and `path/filepath` imports)

```go
// NewWorldSettings is what the editor for a new world starts from: the
// fuzzball pack when there is one, as AddWorld uses, and what a world
// inherits.
func (a *App) NewWorldSettings() (config.WorldSettings, config.Inherited, error) {
	var s config.WorldSettings
	if _, err := os.Stat(filepath.Join(a.d.ConfigDir, "packs", "fuzzball.toml")); err == nil { //str:ok
		s.Use = []string{"fuzzball"} //str:ok
	}
	inh, err := config.Defaults(a.d.ConfigDir)
	return s, inh, err
}

// WorldSettings is what world's file sets, and what it inherits.
func (a *App) WorldSettings(world string) (config.WorldSettings, config.Inherited, error) {
	return config.ReadWorld(a.d.ConfigDir, world)
}

// CharacterSettings is what world's character char sets, and what it
// inherits.
func (a *App) CharacterSettings(world, char string) (config.CharacterSettings, config.Inherited, error) {
	return config.ReadCharacter(a.d.ConfigDir, world, char)
}
```

In `internal/ui/editor.go` `openWorldEditor`, replace the new-world branch's `var s config.WorldSettings` / `os.Stat` / `config.Defaults` lines with:

```go
		s, inh, err := m.a.NewWorldSettings()
		if err != nil {
			m.setStatus(true, err.Error())
			return
		}
```

Drop the `os` and `path/filepath` imports from `editor.go` if nothing else there uses them.

- [ ] **Step 4: Run app and ui tests**

Run: `go test ./internal/app ./internal/ui`
Expected: PASS (the goldens unchanged).

- [ ] **Step 5: Write the failing webapi tests** (append to `internal/webapi/loop_test.go`)

```go
func TestSettingsAreWhatTheEditorStartsFrom(t *testing.T) {
	h := start(t)
	w := event(h.act(`{"Do":"settings","World":"fm"}`), "Settings")
	if w == nil || w["World"] != "fm" || w["CharSettings"] != nil {
		t.Fatalf("fm: %v", w)
	}
	ws := w["WorldSettings"].(map[string]any)
	if ws["Host"] != "muck.test" || ws["Port"] != 8888.0 || ws["Login"] != nil {
		t.Errorf("fm's settings: %v (an unset setting is null)", ws)
	}
	if w["Inherited"].(map[string]any)["NewlineMode"] == "" {
		t.Errorf("fm inherits: %v", w["Inherited"])
	}
	c := event(h.act(`{"Do":"settings","World":"fm","Char":"kit"}`), "Settings")
	if c == nil || c["Char"] != "kit" || c["Name"] != "Kit" || c["CharSettings"] == nil || c["WorldSettings"] != nil {
		t.Errorf("fm/kit: %v", c)
	}
	n := event(h.act(`{"Do":"settings"}`), "Settings")
	if n == nil || n["World"] != "" || fmt.Sprint(n["WorldSettings"].(map[string]any)["Use"]) != "[fuzzball]" {
		t.Errorf("a new world: %v", n)
	}
}

func TestSettingsThatCantBeReadSayWhy(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"settings","World":"nope"}`)
	if s := event(b, "Settings"); s != nil {
		t.Errorf("settings for a world that isn't there: %v", s)
	}
	if st := event(b, "Status"); st == nil || st["Err"] != true {
		t.Errorf("status: %v", st)
	}
}
```

(Add `"fmt"` to the test file's imports if it isn't there.)

- [ ] **Step 6: Run them to see them fail**

Run: `go test ./internal/webapi -run 'TestSettings(AreWhat|ThatCant)'`
Expected: FAIL. `settings` is an unknown action, so the status says a bad action and no `Settings` event comes.

- [ ] **Step 7: Implement**

Create `internal/webapi/settings.go`:

```go
package webapi

import "github.com/latrani/Kiln/internal/config"

// Settings is a world's or character's settings, for the page's editor:
// what its file sets, and what it inherits for what it leaves unset.
// World "" is a new world; Char "" the world itself. Name is the
// character's, for the form's title.
type Settings struct {
	World, Char, Name string
	WorldSettings     *config.WorldSettings     `json:",omitempty"`
	CharSettings      *config.CharacterSettings `json:",omitempty"`
	Inherited         config.Inherited
}

// settings reads what the editor for world's char starts from. One that
// can't be read is said on the status line, as the TUI's, and there's
// nothing to send.
func (l *Loop) settings(world, char string) []any {
	a := l.a
	out := Settings{World: world, Char: char}
	var err error
	switch {
	case world == "":
		var s config.WorldSettings
		s, out.Inherited, err = a.NewWorldSettings()
		out.WorldSettings = &s
	case char == "":
		var s config.WorldSettings
		s, out.Inherited, err = a.WorldSettings(world)
		out.WorldSettings = &s
	default:
		var s config.CharacterSettings
		s, out.Inherited, err = a.CharacterSettings(world, char)
		out.CharSettings = &s
		if ch, ok := a.Find(world + "/" + char); ok {
			out.Name = ch.Name
		}
	}
	if err != nil {
		a.SetStatus(true, err.Error())
		return nil
	}
	return []any{out}
}
```

In `internal/webapi/actions.go` `doSettings`, before `a := l.a`:

```go
	if act.Do == "settings" { //str:ok
		return l.settings(act.World, act.Char), true
	}
```

(Use `app.Key(world, char)` in place of `world + "/" + char` if `app` is already imported in `settings.go`'s neighbors. `app.Key` is the canonical join.)

- [ ] **Step 8: Run the webapi tests, then all Go**

Run: `go test ./internal/webapi && go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/app/edit.go internal/app/edit_test.go internal/ui/editor.go internal/webapi/settings.go internal/webapi/actions.go internal/webapi/loop_test.go
git commit -m "webapi: the settings action, for the page's editor"
```

---

### Task 2: World rows and `+ World` in the picker

**Files:**
- Modify: `web/ui/src/picker.ts`
- Modify: `web/ui/src/ui/Sidebar.tsx` (`PickerList`, `groups`)
- Test: `web/ui/src/picker.test.ts`

**Interfaces:**
- Produces: `PickRow` gains `{ kind: "add_world" }`, always the last row. `selKey` is `"=" + world` for a world row and `"+"` for `+ World`. `fixPick` never returns `""` (there's always `+ World`).
- Consumes: Task 4's `store.openEditor(t: Target, closeAll: boolean)` and `store.leaveEditor()`. Until Task 4 lands, `choose` calls them, so this task's `choose` changes and their tests come in Task 4 (see its Step 1). This task changes only the rows.

- [ ] **Step 1: Update the tests to the TUI's rows** (in `picker.test.ts`, replace the first six tests' expectations)

```ts
test("characters not open, under their worlds, each world ending with + Character, then + World", () => {
  const rows = pickerRows(worlds, { "fm/kit": {} }, "");
  assert.equal(kinds(rows), "world char add_char world char add_char add_world");
  assert.deepEqual(pickable(rows), ["=Bay", "Bay/ash", "+Bay", "=fm", "fm/rook", "+fm", "+"]);
});

test("the filter matches a name, an alias or the world, ignoring case and spaces around it", () => {
  assert.deepEqual(pickable(pickerRows(worlds, {}, " CIN ")), ["=Bay", "Bay/ash", "+Bay", "+"]);
  assert.deepEqual(pickable(pickerRows(worlds, {}, "fm")), ["=fm", "fm/kit", "fm/rook", "+fm", "+"]);
});

test("a world whose name matches is listed with nothing to offer; one that doesn't, isn't", () => {
  const rows = pickerRows(worlds, { "fm/kit": {}, "fm/rook": {} }, "f");
  assert.equal(kinds(rows), "world add_char add_world");
});

test("the highlight stays on its row, else goes to the first character, else the first row", () => {
  const rows = pickerRows(worlds, {}, "");
  assert.equal(fixPick(rows, "fm/kit"), "fm/kit");
  assert.equal(fixPick(rows, "=fm"), "=fm", "a world's row can be highlighted");
  assert.equal(fixPick(rows, "gone/x"), "Bay/ash");
  assert.equal(fixPick(pickerRows(worlds, { "Bay/ash": {}, "fm/kit": {}, "fm/rook": {} }, ""), ""), "=Bay");
});

test("nothing matches: only + World", () => {
  const rows = pickerRows(worlds, {}, "zzz");
  assert.equal(kinds(rows), "add_world");
  assert.equal(fixPick(rows, "fm/kit"), "+");
  assert.equal(movePick(rows, "+", 1), "+");
});

test("↑/↓ move a row, stopping at the ends; PgUp/PgDn a page of rows", () => {
  const rows = pickerRows(worlds, {}, "");
  assert.equal(movePick(rows, "Bay/ash", -1), "=Bay");
  assert.equal(movePick(rows, "=Bay", -1), "=Bay");
  assert.equal(movePick(rows, "+fm", 1), "+");
  assert.equal(movePick(rows, "+", 1), "+");
  assert.equal(pagePick(rows, "Bay/ash", 1, 3), "fm/kit", "three rows on: + Character, fm, then Kit");
  assert.equal(pagePick(rows, "+", -1, 100), "=Bay");
});
```

- [ ] **Step 2: Run to see them fail**

Run: `cd web/ui && npm test`
Expected: those six FAIL (no `add_world`; world rows aren't pickable).

- [ ] **Step 3: Implement** (in `picker.ts`)

- Update the file's header comment. Drop "A world's own row can be highlighted once its editor comes (step 4b), and "+ World" comes with it." and say the rows end with `+ World`.
- `PickRow` gains `| { kind: "add_world" }`.
- In `pickerRows`, `return rows;` becomes `rows.push({ kind: "add_world" }); return rows;`, with the doc comment saying ", then a row to add a world".
- `selKey`:

```ts
// selKey is what the highlight holds when r is highlighted: a
// character's key, "=world" for a world's own row (its editor), "+world"
// for adding a character to world, "+" for adding a world.
export function selKey(r: PickRow): string {
  switch (r.kind) {
    case "char": return r.key;
    case "world": return "=" + r.world;
    case "add_char": return "+" + r.world;
    case "add_world": return "+";
  }
}
```

- `fixPick`'s comment: "otherwise moves it to the first character, or the first row there is (+ World, at least)".

Every `PickRow` but `add_world` has `world`. `groups` in `Sidebar.tsx` and anything else reading `r.world` must narrow on `kind` first: `tsc` will name them.

- [ ] **Step 4: The list draws them** (`Sidebar.tsx` `PickerList`)

`groups` keeps the `add_world` row out of the world groups:

```ts
// groups splits the picker's rows by world (each world's row, then its
// rows), leaving + World, which is in no world, for after them.
function groups(rows: PickRow[]): { world: string; rows: PickRow[] }[] {
  const out: { world: string; rows: PickRow[] }[] = [];
  for (const r of rows) {
    if (r.kind === "world") out.push({ world: r.world, rows: [r] });
    else if (r.kind !== "add_world") out.at(-1)?.rows.push(r);
  }
  return out;
}
```

Inside the map over a group's rows:
- The world row becomes an option, not the `aria-hidden` header span. Its text is the world's name, in `picker.world.selected` when highlighted, else `picker.world`. The group keeps `aria-label={world}`.
- After the groups, one more `<li>` for `+ World`, drawn as the TUI's: a chip at the left edge, `picker.selected` when highlighted, else `picker.add`.

```tsx
                if (r.kind === "world") {
                  return <li {...opts} class={"pick " + roleClass(on ? "picker.world.selected" : "picker.world")}>{r.world}</li>;
                }
```

and after the groups' `.map(...)`, inside the listbox `<ul>`:

```tsx
        {(() => {
          const on = p.sel === "+";
          return (
            <li key="+" id="pick-+" role="option" aria-selected={on} class="pick" onClick={() => choose(store, "+")}>
              <span class={roleClass(on ? "picker.selected" : "picker.add")}> {str.pickerAddWorld()} </span>
            </li>
          );
        })()}
```

Delete the old `<span aria-hidden="true" class={"pick " + roleClass("picker.world")}>{world}</span>` header. The world's option now draws its name.

- [ ] **Step 5: Run the page's tests and type check**

Run: `cd web/ui && npm run check && npm test`
Expected: PASS. `choose` still treats `"+"` as `+ Character` for world `""`; Task 4 fixes that, and nothing reaches it yet.

- [ ] **Step 6: Commit**

```bash
git add web/ui/src/picker.ts web/ui/src/picker.test.ts web/ui/src/ui/Sidebar.tsx
git commit -m "web/ui: the picker's world rows and + World, as the TUI's"
```

---

### Task 3: The editor's forms (`editor.ts`)

**Files:**
- Create: `web/ui/src/editor.ts`
- Test: `web/ui/src/editor.test.ts`

**Interfaces:**
- Produces:

```ts
export type Target = { kind: "new_world" } | { kind: "world"; world: string } | { kind: "char"; world: string; char: string };
export const targetKey: (t: Target) => string; // "+", "=world", "world/char": the picker's selection keys
export interface Inherited { Login: string; MaxLineBytes: number; NewlineMode: string; Autoconnect: boolean; Reconnect: boolean; Notify: string; LocalEcho: boolean }
export interface SettingsEvent { World: string; Char: string; Name: string; WorldSettings?: any; CharSettings?: any; Inherited: Inherited }
export type Field =
  | { kind: "text"; key: string; label: string; hint?: string; extra?: boolean }
  | { kind: "toggle"; key: string; label: string; extra?: boolean }
  | { kind: "choice"; key: string; label: string; options: { value: string; shown: string }[]; extra?: boolean };
export type Values = Record<string, string | boolean>;
export interface Problem { Field: string; Msg: string }
export interface Editor {
  target: Target; title: string; fields: Field[]; values: Values;
  saved: string; more: boolean; armed: boolean; problems: Problem[];
  live: Record<string, string>; refused: number; pending?: string; closeAll: boolean;
}
export function targetOf(e: { World: string; Char: string }): Target;
export function newEditor(e: SettingsEvent, closeAll: boolean): Editor;
export function edit(ed: Editor, key: string, value: string | boolean, check?: (field: string, value: string) => string): Editor;
export function unsaved(ed: Editor): boolean;
export function saveAction(ed: Editor): { Do: string; [k: string]: unknown };
export function deleteAction(ed: Editor): { Do: string; [k: string]: unknown } | undefined;
export function errorOf(ed: Editor, key: string): string | undefined;
export function formError(ed: Editor): string | undefined;
export function firstError(ed: Editor): string | undefined;
export function refuse(ed: Editor, problems: Problem[]): Editor;
```

Field `key`s are the config keys Go's `FieldError.Field` and `check()` use: `id host port tls tls_trust use login max_line_bytes newline_mode autoconnect reconnect notify local_echo aliases`.

- [ ] **Step 1: Write the failing tests** (`editor.test.ts`)

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { deleteAction, edit, errorOf, firstError, formError, newEditor, refuse, saveAction, targetKey, unsaved } from "./editor.ts";
import * as str from "./str.gen.ts";

const inh = { Login: "", MaxLineBytes: 4096, NewlineMode: "batch", Autoconnect: false, Reconnect: true, Notify: "first", LocalEcho: true };
const fmSettings = { Host: "muck.test", Port: 8888, TLS: true, TLSTrust: "", Use: ["fuzzball"], Login: null, MaxLineBytes: 2048,
  NewlineMode: null, Autoconnect: true, Reconnect: null, Notify: null, LocalEcho: null };
const fm = () => newEditor({ World: "fm", Char: "", Name: "", WorldSettings: fmSettings, Inherited: inh }, false);
const kit = () => newEditor({ World: "fm", Char: "kit", Name: "Kit", CharSettings: { Aliases: ["kitty"], Autoconnect: null, Reconnect: false, Notify: "all", LocalEcho: null }, Inherited: inh }, true);
const fresh = () => newEditor({ World: "", Char: "", Name: "", WorldSettings: { Host: "", Port: 0, TLS: false, TLSTrust: "", Use: ["fuzzball"], Login: null, MaxLineBytes: null, NewlineMode: null, Autoconnect: null, Reconnect: null, Notify: null, LocalEcho: null }, Inherited: inh }, false);

test("a world's form: the TUI's fields, in order, the rest behind More settings", () => {
  const ed = fm();
  assert.equal(ed.title, str.editorEditing("fm"));
  assert.deepEqual(ed.fields.filter((f) => !f.extra).map((f) => f.key), ["host", "port", "tls"]);
  assert.deepEqual(ed.fields.filter((f) => f.extra).map((f) => f.key),
    ["tls_trust", "use", "login", "max_line_bytes", "newline_mode", "autoconnect", "reconnect", "notify", "local_echo"]);
  assert.equal(fresh().fields[0].key, "id", "a new world asks for its id first");
  assert.equal(fresh().title, str.editorNewWorld());
});

test("a choice offers what it inherits first, as the TUI's", () => {
  const nl = fm().fields.find((f) => f.key === "newline_mode")!;
  assert.ok(nl.kind === "choice");
  assert.deepEqual(nl.options.map((o) => o.shown), [str.editorDefault("batch"), "batch", "flatten"]);
  const rc = fm().fields.find((f) => f.key === "reconnect")!;
  assert.ok(rc.kind === "choice");
  assert.deepEqual(rc.options.map((o) => o.shown), [str.editorDefault(str.editorOn()), str.editorOn(), str.editorOff()]);
  const mb = fm().fields.find((f) => f.key === "max_line_bytes")!;
  assert.ok(mb.kind === "text" && mb.hint === str.editorDefaultHint(4096));
});

test("a character's form: aliases and what it can set, nothing hidden", () => {
  const ed = kit();
  assert.equal(ed.title, str.editorEditing("fm/Kit"));
  assert.deepEqual(ed.fields.map((f) => f.key), ["aliases", "autoconnect", "reconnect", "notify", "local_echo"]);
  assert.ok(ed.fields.every((f) => !f.extra));
});

test("saving untouched writes back what was read: unset stays unset", () => {
  assert.deepEqual(saveAction(fm()), { Do: "save_world", World: "fm", WorldSettings: fmSettings });
  assert.deepEqual(saveAction(kit()), { Do: "save_character", World: "fm", Char: "kit",
    CharSettings: { Aliases: ["kitty"], Autoconnect: null, Reconnect: false, Notify: "all", LocalEcho: null } });
});

test("edits read as the TUI reads them: lists split on commas and spaces, numbers or 0", () => {
  let ed = fresh();
  for (const [k, v] of [["id", "zz"], ["host", "zz.test"], ["port", "77x"], ["use", "a, b c"], ["max_line_bytes", ""], ["reconnect", "off"]] as const) ed = edit(ed, k, v);
  const a = saveAction(ed) as any;
  assert.equal(a.Do, "add_world");
  assert.equal(a.ID, "zz");
  assert.equal(a.WorldSettings.Port, 0, "an unreadable port is 0, which the save refuses");
  assert.deepEqual(a.WorldSettings.Use, ["a", "b", "c"]);
  assert.equal(a.WorldSettings.MaxLineBytes, null);
  assert.equal(a.WorldSettings.Reconnect, false);
});

test("unsaved: the title says so once a value differs from what was read", () => {
  const ed = fm();
  assert.equal(unsaved(ed), false);
  assert.equal(unsaved(edit(ed, "host", "x")), true);
  assert.equal(unsaved(edit(edit(ed, "host", "x"), "host", "muck.test")), false);
});

test("a live check shows beside its field until fixed; an edit clears that field's save problem and disarms Delete", () => {
  const check = (f: string, v: string) => (f === "port" && /\D/.test(v) ? str.editorPortNumber() : "");
  let ed = edit(fm(), "port", "88a", check);
  assert.equal(errorOf(ed, "port"), str.editorPortNumber());
  ed = edit(ed, "port", "88", check);
  assert.equal(errorOf(ed, "port"), undefined);
  ed = { ...refuse(ed, [{ Field: "host", Msg: "bad host" }, { Field: "", Msg: "disk full" }]), armed: true };
  assert.equal(errorOf(ed, "host"), "bad host");
  assert.equal(formError(ed), "disk full");
  ed = edit(ed, "host", "h");
  assert.equal(errorOf(ed, "host"), undefined);
  assert.equal(ed.armed, false);
});

test("a refusal counts, opens More settings for a field inside it, and names the first field to fix", () => {
  const ed = refuse(fm(), [{ Field: "notify", Msg: "x" }, { Field: "port", Msg: "y" }]);
  assert.equal(ed.refused, 1);
  assert.equal(ed.more, true);
  assert.equal(firstError(ed), "port", "in the form's order, not Go's");
  assert.equal(refuse(fm(), [{ Field: "", Msg: "z" }]).more, false);
});

test("delete: a world or a character; a new world has nothing to delete", () => {
  assert.deepEqual(deleteAction(fm()), { Do: "delete_world", World: "fm" });
  assert.deepEqual(deleteAction(kit()), { Do: "delete_character", World: "fm", Char: "kit" });
  assert.equal(deleteAction(fresh()), undefined);
});

test("targets key as the picker's rows do", () => {
  assert.equal(targetKey(fm().target), "=fm");
  assert.equal(targetKey(kit().target), "fm/kit");
  assert.equal(targetKey(fresh().target), "+");
});
```

- [ ] **Step 2: Run to see them fail**

Run: `cd web/ui && npm test`
Expected: FAIL, `editor.ts` not found (the bundle step errors).

- [ ] **Step 3: Implement `editor.ts`**

```ts
// The editor's forms, as the TUI's (internal/ui/editor.go): which fields
// a world or character has, their values to and from Go's settings, and
// what's wrong with them. Values are the form's: text as typed, a
// toggle's on, a choice's option ("" for inheriting).
import * as str from "./str.gen.ts";

// Target is what an editor adds or edits.
export type Target = { kind: "new_world" } | { kind: "world"; world: string } | { kind: "char"; world: string; char: string };

// targetKey names a target as the picker's rows do: "+" for a new
// world, "=world", "world/char". One editor (or draft) per key.
export const targetKey = (t: Target): string => (t.kind === "new_world" ? "+" : t.kind === "world" ? "=" + t.world : `${t.world}/${t.char}`);

// targetOf is the target Go's Settings (or a settings action) is for.
export function targetOf(e: { World: string; Char: string }): Target {
  return e.Char ? { kind: "char", world: e.World, char: e.Char } : e.World ? { kind: "world", world: e.World } : { kind: "new_world" };
}

// Inherited is what a world or character gets for settings it leaves
// unset (config.Inherited).
export interface Inherited { Login: string; MaxLineBytes: number; NewlineMode: string; Autoconnect: boolean; Reconnect: boolean; Notify: string; LocalEcho: boolean }

// SettingsEvent is Go's Settings (internal/webapi/settings.go).
export interface SettingsEvent { World: string; Char: string; Name: string; WorldSettings?: any; CharSettings?: any; Inherited: Inherited }

// Field is one of a form's fields, keyed by its config key (what Go's
// problems and check() name).
export type Field =
  | { kind: "text"; key: string; label: string; hint?: string; extra?: boolean }
  | { kind: "toggle"; key: string; label: string; extra?: boolean }
  | { kind: "choice"; key: string; label: string; options: { value: string; shown: string }[]; extra?: boolean };

export type Values = Record<string, string | boolean>;

// Problem is one of Go's Problems: a field ("" for none) and why.
export interface Problem { Field: string; Msg: string }

// Editor is an open (or hidden, or parked) editor.
export interface Editor {
  target: Target;
  title: string;
  fields: Field[];
  values: Values;
  saved: string; // the values as read, to tell unsaved edits
  more: boolean; // More settings open
  armed: boolean; // Delete pressed once: pressed again, it deletes
  problems: Problem[]; // why Go refused the last save or delete
  live: Record<string, string>; // what check() says of a field as typed
  refused: number; // how many refusals, so the form can move focus on each
  pending?: string; // the action sent (its Do), until Go answers
  closeAll: boolean; // opened from the main view: closing it closes the picker
}

const onOff = (b: boolean) => (b ? "on" : "off"); //str:ok config values

// shown is how a choice's option reads: on and off in words, config
// values (batch, pin, …) as they're written in the file.
const shown = (v: string) => (v === "on" ? str.editorOn() : v === "off" ? str.editorOff() : v);

// inheritChoice is a choice between inheriting (showing what that
// gives) and the given options.
function inheritChoice(key: string, label: string, inherits: string, options: string[], extra = false): Field {
  return { kind: "choice", key, label, extra, options: [{ value: "", shown: str.editorDefault(shown(inherits)) }, ...options.map((o) => ({ value: o, shown: shown(o) }))] };
}

function worldFields(add: boolean, inh: Inherited): Field[] {
  const f: Field[] = add ? [{ kind: "text", key: "id", label: str.editorWorld() }] : [];
  f.push(
    { kind: "text", key: "host", label: str.editorHost() },
    { kind: "text", key: "port", label: str.editorPort() },
    { kind: "toggle", key: "tls", label: str.editorTls() },
    inheritChoice("tls_trust", str.editorCertTrust(), "pin", ["pin", "ca"], true),
    { kind: "text", key: "use", label: str.editorPacks(), hint: str.editorNone(), extra: true },
    { kind: "text", key: "login", label: str.editorLogin(), hint: str.editorDefaultHint(inh.Login || str.editorNone()), extra: true },
    { kind: "text", key: "max_line_bytes", label: str.editorMaxBytes(), hint: str.editorDefaultHint(inh.MaxLineBytes), extra: true },
    inheritChoice("newline_mode", str.editorNewlines(), inh.NewlineMode, ["batch", "flatten"], true),
    ...charChoices(inh, true),
  );
  return f;
}

function charChoices(inh: Inherited, extra: boolean): Field[] {
  return [
    inheritChoice("autoconnect", str.editorAutoconnect(), onOff(inh.Autoconnect), ["on", "off"], extra),
    inheritChoice("reconnect", str.editorReconnect(), onOff(inh.Reconnect), ["on", "off"], extra),
    inheritChoice("notify", str.editorNotify(), inh.Notify, ["all", "first", "priority", "none"], extra),
    inheritChoice("local_echo", str.editorLocalEcho(), onOff(inh.LocalEcho), ["on", "off"], extra),
  ];
}

const optBool = (b: boolean | null | undefined) => (b == null ? "" : onOff(b));
const boolOf = (v: string | boolean) => (v === "on" ? true : v === "off" ? false : null);
const text = (v: Values, k: string) => String(v[k] ?? "");
// list splits a comma- or space-separated field, as app.List.
const list = (s: string) => s.split(/[, ]/).filter((x) => x);
// num reads a number as the TUI's Atoi: anything else is 0, which the
// save refuses.
const num = (s: string) => (/^\d+$/.test(s) ? Number(s) : 0);
const orNull = (s: string) => (s === "" ? null : s);

function valuesOf(e: SettingsEvent): Values {
  if (e.CharSettings) {
    const s = e.CharSettings;
    return { aliases: (s.Aliases ?? []).join(", "), autoconnect: optBool(s.Autoconnect), reconnect: optBool(s.Reconnect),
      notify: s.Notify ?? "", local_echo: optBool(s.LocalEcho) };
  }
  const s = e.WorldSettings;
  const v: Values = { host: s.Host ?? "", port: s.Port > 0 ? String(s.Port) : "", tls: !!s.TLS, tls_trust: s.TLSTrust ?? "",
    use: (s.Use ?? []).join(", "), login: s.Login ?? "", max_line_bytes: s.MaxLineBytes == null ? "" : String(s.MaxLineBytes),
    newline_mode: s.NewlineMode ?? "", autoconnect: optBool(s.Autoconnect), reconnect: optBool(s.Reconnect),
    notify: s.Notify ?? "", local_echo: optBool(s.LocalEcho) };
  if (!e.World) v.id = "";
  return v;
}

// newEditor is the editor for what Go's Settings e is for, filled in.
export function newEditor(e: SettingsEvent, closeAll: boolean): Editor {
  const target = targetOf(e);
  const fields = target.kind === "char" ? [{ kind: "text", key: "aliases", label: str.editorAliases(), hint: str.editorNone() } as Field, ...charChoices(e.Inherited, false)]
    : worldFields(target.kind === "new_world", e.Inherited);
  const title = target.kind === "new_world" ? str.editorNewWorld() : str.editorEditing(target.kind === "char" ? `${e.World}/${e.Name}` : e.World);
  const values = valuesOf(e);
  return { target, title, fields, values, saved: JSON.stringify(values), more: false, armed: false, problems: [], live: {}, refused: 0, closeAll };
}

// edit sets field key to value: check (Go's, for text) says what's
// wrong with it as typed, the field's refusal from the last save goes,
// and Delete has to be pressed twice again.
export function edit(ed: Editor, key: string, value: string | boolean, check?: (field: string, value: string) => string): Editor {
  const live = { ...ed.live };
  const msg = typeof value === "string" && check ? check(key, value) : "";
  if (msg) live[key] = msg;
  else delete live[key];
  return { ...ed, values: { ...ed.values, [key]: value }, live, problems: ed.problems.filter((p) => p.Field !== key), armed: false };
}

// unsaved reports whether the values differ from what was read.
export const unsaved = (ed: Editor): boolean => JSON.stringify(ed.values) !== ed.saved;

// saveAction is the action that saves ed: add_world, save_world or
// save_character, with the settings as Go reads them (unset is null).
export function saveAction(ed: Editor): { Do: string; [k: string]: unknown } {
  const v = ed.values;
  const t = ed.target;
  if (t.kind === "char") {
    return { Do: "save_character", World: t.world, Char: t.char, CharSettings: { Aliases: list(text(v, "aliases")),
      Autoconnect: boolOf(v.autoconnect), Reconnect: boolOf(v.reconnect), Notify: orNull(text(v, "notify")), LocalEcho: boolOf(v.local_echo) } };
  }
  const s = { Host: text(v, "host"), Port: num(text(v, "port")), TLS: v.tls === true, TLSTrust: text(v, "tls_trust"), Use: list(text(v, "use")),
    Login: orNull(text(v, "login")), MaxLineBytes: text(v, "max_line_bytes") === "" ? null : num(text(v, "max_line_bytes")),
    NewlineMode: orNull(text(v, "newline_mode")), Autoconnect: boolOf(v.autoconnect), Reconnect: boolOf(v.reconnect),
    Notify: orNull(text(v, "notify")), LocalEcho: boolOf(v.local_echo) };
  return t.kind === "new_world" ? { Do: "add_world", ID: text(v, "id"), WorldSettings: s } : { Do: "save_world", World: t.world, WorldSettings: s };
}

// deleteAction is the action that deletes what ed edits; none for a new
// world.
export function deleteAction(ed: Editor): { Do: string; [k: string]: unknown } | undefined {
  const t = ed.target;
  if (t.kind === "char") return { Do: "delete_character", World: t.world, Char: t.char };
  if (t.kind === "world") return { Do: "delete_world", World: t.world };
  return undefined;
}

const has = (ed: Editor, key: string) => ed.fields.some((f) => f.key === key);

// errorOf is what's wrong with field key: as typed, or else as Go
// refused it.
export const errorOf = (ed: Editor, key: string): string | undefined => ed.live[key] ?? ed.problems.find((p) => p.Field === key)?.Msg;

// formError is what Go refused about no one field of the form.
export const formError = (ed: Editor): string | undefined => ed.problems.find((p) => !has(ed, p.Field))?.Msg;

// firstError is the first field, in the form's order, with something
// wrong.
export const firstError = (ed: Editor): string | undefined => ed.fields.find((f) => errorOf(ed, f.key) !== undefined)?.key;

// refuse is ed after Go refused its save or delete: every problem
// shown, More settings open if one is inside it.
export function refuse(ed: Editor, problems: Problem[]): Editor {
  const more = ed.more || problems.some((p) => ed.fields.find((f) => f.key === p.Field)?.extra);
  return { ...ed, problems, more, armed: false, pending: undefined, refused: ed.refused + 1 };
}
```

- [ ] **Step 4: Run the tests and type check**

Run: `cd web/ui && npm run check && npm test`
Expected: PASS. If `str.editorDefaultHint` takes `unknown`, the number argument is fine.

- [ ] **Step 5: Commit**

```bash
git add web/ui/src/editor.ts web/ui/src/editor.test.ts
git commit -m "web/ui: the editor's forms, as the TUI's"
```

---

### Task 4: The store holds editors: open, hide, settle, park

**Files:**
- Modify: `web/ui/src/store.ts`
- Modify: `web/ui/src/picker.ts` (`choose`)
- Test: `web/ui/src/store.test.ts`, `web/ui/src/picker.test.ts`

**Interfaces:**
- Consumes: Task 3's `Editor`, `Target`, `targetKey`, `targetOf`, `newEditor`, `edit`, `refuse`, `saveAction`, `deleteAction`, `SettingsEvent`.
- Produces:
  - **State:** `Picker` gains `edit?: Editor`. `State` gains `drafts: Record<string, Editor>` (by `targetKey`), `parked: Record<string, Editor>` (by the active key it was left on), and `opening?: { key: string; closeAll: boolean }`.
  - **Store methods:**
    - `openEditor(t: Target, closeAll: boolean): void`
    - `closeEditor(): void` (Esc)
    - `hideEditor(): void` (Ctrl+T)
    - `leaveEditor(): void` (going elsewhere in the picker: a draft, the picker stays)
    - `setEditor(patch: Partial<Editor>): void`
    - `editField(key: string, value: string | boolean): void` (with Go's live check)
    - `sendEditor(action: { Do: string }): void`
  - **`choose(store, sel)`:** `"+"` opens a new world's editor and `"=w"` world w's. Both use `closeAll: false`, leaving any open editor first.

- [ ] **Step 1: Write the failing tests** (append to `store.test.ts`, and add the `choose` cases to `picker.test.ts`)

`store.test.ts`. Use the file's existing imports; add `newEditor` from `./editor.ts` if needed. Every Go event here is a plain object, as in the file's other tests.

```ts
const inh = { Login: "", MaxLineBytes: 4096, NewlineMode: "batch", Autoconnect: false, Reconnect: true, Notify: "first", LocalEcho: true };
const fmSettings = { type: "Settings", World: "fm", Char: "", Name: "", Inherited: inh,
  WorldSettings: { Host: "muck.test", Port: 8888, TLS: true, TLSTrust: "", Use: [], Login: null, MaxLineBytes: null, NewlineMode: null, Autoconnect: null, Reconnect: null, Notify: null, LocalEcho: null } };
const kitSettings = { type: "Settings", World: "fm", Char: "kit", Name: "Kit", Inherited: inh,
  CharSettings: { Aliases: [], Autoconnect: null, Reconnect: null, Notify: null, LocalEcho: null } };
const running = () => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ phase: "running", act: (o: object) => sent.push(o) });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }, { ID: "rook", Name: "Rook" }] }] },
    { type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Order", Keys: ["fm/kit"] }, { type: "Active", Key: "fm/kit" }]);
  return { s, sent };
};

test("opening an editor asks Go for its settings, and opens it, over the picker, when they come", () => {
  const { s, sent } = running();
  s.openEditor({ kind: "world", world: "fm" }, true);
  assert.deepEqual(sent.at(-1), { Do: "settings", World: "fm", Char: "" });
  assert.equal(s.state.picker, undefined, "nothing until the settings come");
  s.apply([fmSettings]);
  assert.equal(s.state.picker?.edit?.title, str.editorEditing("fm"));
  assert.equal(s.state.picker?.edit?.closeAll, true);
  s.apply([kitSettings]);
  assert.equal(s.state.picker?.edit?.title, str.editorEditing("fm"), "settings nobody asked for open nothing");
});

test("Esc drops the edits; Ctrl+T hides them for next time; closeAll takes the picker too", () => {
  const { s } = running();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.editField("host", "changed");
  s.hideEditor();
  assert.ok(s.state.picker && !s.state.picker.edit, "back to the picker");
  s.openEditor({ kind: "world", world: "fm" }, false);
  assert.equal(s.state.picker?.edit?.values.host, "changed", "the draft, with no trip to Go");
  s.closeEditor();
  s.openEditor({ kind: "world", world: "fm" }, true);
  assert.equal(s.state.picker?.edit, undefined, "Esc dropped it: Go is asked again");
  s.apply([fmSettings]);
  s.closeEditor();
  assert.equal(s.state.picker, undefined, "opened from the main view: closing it closes the picker");
});

test("a save sends the form and waits; Go's refusal shows; Saved closes it", () => {
  const { s, sent } = running();
  s.openEditor({ kind: "world", world: "fm" }, true);
  s.apply([fmSettings]);
  s.sendEditor({ Do: "save_world", World: "fm" });
  assert.equal(s.state.picker?.edit?.pending, "save_world");
  assert.deepEqual(sent.at(-1), { Do: "save_world", World: "fm" });
  s.apply([{ type: "Problems", Op: "save_world", Problems: [{ Field: "port", Msg: "no" }] }]);
  assert.deepEqual(s.state.picker?.edit?.problems, [{ Field: "port", Msg: "no" }]);
  assert.equal(s.state.picker?.edit?.pending, undefined);
  s.sendEditor({ Do: "save_world", World: "fm" });
  s.apply([{ type: "Saved", Op: "save_world" }]);
  assert.equal(s.state.picker, undefined);
});

test("an answer to something else leaves the editor be", () => {
  const { s } = running();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.apply([{ type: "Saved", Op: "save_world" }]);
  assert.ok(s.state.picker?.edit, "nothing was sent: not its answer");
  s.sendEditor({ Do: "save_world", World: "fm" });
  s.apply([{ type: "Saved", Op: "add_character", Key: "fm/x" }]);
  assert.equal(s.state.picker?.edit?.pending, "save_world");
});

test("a new world saved: back to the picker, on its + Character", () => {
  const { s } = running();
  s.openPicker();
  s.openEditor({ kind: "new_world" }, false);
  s.apply([{ ...fmSettings, World: "" }]);
  s.editField("id", "zz");
  s.sendEditor({ Do: "add_world" });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [] }, { ID: "zz", Characters: [] }] }, { type: "Saved", Op: "add_world" }]);
  assert.equal(s.state.picker?.edit, undefined);
  assert.equal(s.state.picker?.sel, "+zz");
});

test("deleting the edited character closes the editor; nothing's parked or drafted", () => {
  const { s } = running();
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, true);
  s.apply([kitSettings]);
  s.sendEditor({ Do: "delete_character", World: "fm", Char: "kit" });
  s.apply([{ type: "Closed", Key: "fm/kit" }, { type: "Active", Key: "" }, { type: "Saved", Op: "delete_character" }]);
  assert.equal(s.state.picker, undefined);
  assert.deepEqual(s.state.parked, {});
  assert.deepEqual(s.state.drafts, {});
});

test("a deleted world's drafts go, so a new world of its name starts fresh", () => {
  const { s } = running();
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, false);
  s.apply([kitSettings]);
  s.editField("aliases", "x");
  s.hideEditor();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.sendEditor({ Do: "delete_world", World: "fm" });
  s.apply([{ type: "Saved", Op: "delete_world" }]);
  assert.deepEqual(s.state.drafts, {});
});

test("an editor opened from the main view stays with its character when you switch, and comes back with it", () => {
  const { s } = running();
  s.apply([{ type: "Char", Key: "fm/rook", World: "fm", ID: "rook", Name: "Rook", State: 2 }, { type: "Order", Keys: ["fm/kit", "fm/rook"] }]);
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, true);
  s.apply([kitSettings]);
  s.editField("aliases", "x");
  s.apply([{ type: "Active", Key: "fm/rook" }]);
  assert.equal(s.state.picker, undefined, "parked: the picker goes with it");
  assert.equal(s.state.parked["fm/kit"]?.values.aliases, "x");
  s.apply([{ type: "Active", Key: "fm/kit" }]);
  assert.equal(s.state.picker?.edit?.values.aliases, "x", "back, as it was");
  assert.deepEqual(s.state.parked, {});
});

test("a character closed with an editor parked on it keeps the edits as a draft", () => {
  const { s } = running();
  s.apply([{ type: "Char", Key: "fm/rook", World: "fm", ID: "rook", Name: "Rook", State: 2 }]);
  s.openEditor({ kind: "char", world: "fm", char: "kit" }, true);
  s.apply([kitSettings]);
  s.editField("aliases", "x");
  s.apply([{ type: "Active", Key: "fm/rook" }]);
  s.apply([{ type: "Closed", Key: "fm/kit" }]);
  assert.deepEqual(s.state.parked, {});
  assert.equal(s.state.drafts["fm/kit"]?.values.aliases, "x");
});

test("an editor opened in the picker stays open when the active character changes", () => {
  const { s } = running();
  s.apply([{ type: "Char", Key: "fm/rook", World: "fm", ID: "rook", Name: "Rook", State: 2 }]);
  s.openPicker();
  s.openEditor({ kind: "world", world: "fm" }, false);
  s.apply([fmSettings]);
  s.apply([{ type: "Active", Key: "fm/rook" }]);
  assert.ok(s.state.picker?.edit);
});
```

`picker.test.ts`, appended:

```ts
test("choosing a world's row or + World opens its editor (Go is asked); leaving one keeps its edits", () => {
  const sent: any[] = [];
  const s = new Store();
  s.set({ phase: "running", act: (o: object) => sent.push(o) });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }] }] }]);
  s.openPicker();
  choose(s, "=fm");
  assert.deepEqual(sent.at(-1), { Do: "settings", World: "fm", Char: "" });
  choose(s, "+");
  assert.deepEqual(sent.at(-1), { Do: "settings", World: "", Char: "" });
});
```

- [ ] **Step 2: Run to see them fail**

Run: `cd web/ui && npm test`
Expected: FAIL. `openEditor` etc. are not functions, and `choose(s, "+")` opens a name form instead.

- [ ] **Step 3: Implement**

`store.ts`:

```ts
import { edit, newEditor, refuse, targetKey, targetOf } from "./editor.ts";
import type { Editor, SettingsEvent, Target } from "./editor.ts";
```

- **`Picker`** gains `edit?: Editor; // an editor open over the picker (editor.ts)`.
- **`State`** gains:

```ts
  drafts: Record<string, Editor>; // hidden editors' edits, by targetKey, for the next time
  parked: Record<string, Editor>; // editors opened from the main view, by the item they were left on
  opening?: { key: string; closeAll: boolean }; // an editor waiting for Go's settings
```

- **`fresh`** sets `drafts: {}, parked: {}`. They go on `Start` with everything of Go's: Kiln restarted, and the TUI's drafts don't outlive it either.

- **`step`:**

```ts
      case "Settings": {
        const o = s.opening;
        if (!o || o.key !== targetKey(targetOf(e as SettingsEvent))) return s;
        return show({ ...s, opening: undefined }, newEditor(e as SettingsEvent, o.closeAll));
      }
```

- **Module-level pure helpers** (after `without`):

```ts
const pickerOf = (s: State): Picker => s.picker ?? { filter: "", sel: fixPick(pickerRows(s.worlds, s.chars, ""), "") };

// stash keeps ed's edits for the next time its target's editor opens.
const stash = (s: State, ed: Editor): State => ({ ...s, drafts: { ...s.drafts, [targetKey(ed.target)]: ed } });

// show opens ed over the picker (opening it), the editor open there
// before kept as a draft, as the TUI's setEditor.
function show(s: State, ed: Editor): State {
  const old = s.picker?.edit;
  if (old && targetKey(old.target) !== targetKey(ed.target)) s = stash(s, old);
  return { ...s, picker: { ...pickerOf(s), adding: undefined, edit: ed } };
}

// shut closes the open editor: back to the picker, or with closeAll out
// of it, as the TUI's closeEditor.
function shut(s: State): State {
  if (!s.picker?.edit) return s;
  return s.picker.edit.closeAll ? { ...s, picker: undefined } : { ...s, picker: { ...s.picker, edit: undefined } };
}

// exists reports whether k (a character's key or "=world") is still
// something to show.
const exists = (s: State, k: string): boolean =>
  k.startsWith("=") ? Object.values(s.chars).some((c: any) => "=" + c.World === k) : !!s.chars[k];

// settle is s after Go's answer to the editor's save or delete, as the
// TUI's save and delete calls: refused, it shows why; saved, it closes (a
// new world's to the picker, on its + Character), and a deleted world's
// drafts go with it.
function settle(s: State, a: Answer): State {
  const ed = s.picker!.edit!;
  if (a.Problems) return { ...s, picker: { ...s.picker!, edit: refuse(ed, a.Problems) } };
  if (a.Op === "delete_world" && ed.target.kind === "world") {
    const w = ed.target.world;
    s = { ...s, drafts: Object.fromEntries(Object.entries(s.drafts).filter(([, d]) => d.target.kind === "new_world" || d.target.world !== w)) };
  }
  if (a.Op === "add_world") return { ...s, picker: { ...s.picker!, edit: undefined, filter: "", sel: "+" + String(ed.values.id) } };
  return shut(s);
}

// follow moves editors with the active item, as the TUI's parkEditor
// and unparkEditor: one opened from the main view stays on the item it
// was opened on (parked) when you go elsewhere, or becomes a draft if
// that item went; a parked one on an item that went becomes a draft;
// and the one parked on the item now active comes back.
function follow(s: State, prev: string): State {
  const ed = s.picker?.edit;
  if (s.active !== prev && ed?.closeAll) {
    s = exists(s, prev) ? { ...s, parked: { ...s.parked, [prev]: ed } } : stash(s, ed);
    s = { ...s, picker: undefined };
  }
  for (const [k, p] of Object.entries(s.parked)) {
    if (!exists(s, k)) s = stash({ ...s, parked: without(s.parked, k) }, p);
  }
  const back = s.parked[s.active];
  if (back && s.active !== prev) {
    if (s.picker?.edit) s = stash(s, s.picker.edit);
    s = show({ ...s, picker: undefined, parked: without(s.parked, s.active) }, back);
  }
  return s;
}
```

(`Answer` is already declared in this file; `without` is generic.)

- **`apply`:** record `const prev = this.state.active; const seen = this.state.answer?.n ?? 0;` before the loop. After the loop, before the existing `if (s.picker)` fix-up:

```ts
    const ed = s.picker?.edit;
    if (ed?.pending && s.answer && s.answer.n > seen && s.answer.Op === ed.pending) s = settle(s, s.answer);
    s = follow(s, prev);
```

The existing fix-up stays after them, so a new world's `+zz` is kept once `Worlds` lists it.

- **Methods:**

```ts
  // openEditor opens target's editor over the picker (opening the picker
  // too, and with closeAll closing with it): its draft, or one left
  // parked, as it was; otherwise Go's settings are asked for, and it
  // opens when they come.
  openEditor(t: Target, closeAll: boolean): void {
    const k = targetKey(t);
    let s = this.state;
    let ed: Editor | undefined;
    for (const [at, p] of Object.entries(s.parked)) {
      if (targetKey(p.target) === k) {
        s = { ...s, parked: without(s.parked, at) };
        ed = p;
      }
    }
    ed ??= s.drafts[k];
    if (ed) {
      this.state = show({ ...s, drafts: without(s.drafts, k) }, { ...ed, armed: false, pending: undefined, closeAll });
      this.notify();
      return;
    }
    this.set({ opening: { key: k, closeAll } });
    const w = t.kind === "new_world" ? "" : t.world;
    this.state.act?.({ Do: "settings", World: w, Char: t.kind === "char" ? t.char : "" });
  }

  // closeEditor drops the editor's edits (Esc).
  closeEditor(): void {
    this.state = shut(this.state);
    this.notify();
  }

  // hideEditor closes the editor but keeps its edits for the next time
  // (Ctrl+T, the key that opened it).
  hideEditor(): void {
    const ed = this.state.picker?.edit;
    if (!ed) return;
    this.state = shut(stash(this.state, ed));
    this.notify();
  }

  // leaveEditor keeps the editor's edits as a draft and goes back to the
  // picker, which stays open: a row of it was chosen.
  leaveEditor(): void {
    const ed = this.state.picker?.edit;
    if (!ed) return;
    this.state = { ...stash(this.state, ed), picker: { ...this.state.picker!, edit: undefined } };
    this.notify();
  }

  // setEditor changes the open editor.
  setEditor(patch: Partial<Editor>): void {
    const p = this.state.picker;
    if (p?.edit) this.set({ picker: { ...p, edit: { ...p.edit, ...patch } } });
  }

  // editField sets one of the editor's fields, with Go's check of what's
  // typed.
  editField(key: string, value: string | boolean): void {
    const p = this.state.picker;
    if (!p?.edit) return;
    const check = (globalThis as any).kiln?.check as ((f: string, v: string) => string) | undefined;
    this.set({ picker: { ...p, edit: edit(p.edit, key, value, check) } });
  }

  // sendEditor sends the editor's save or delete; its answer settles it
  // (apply).
  sendEditor(action: { Do: string; [k: string]: unknown }): void {
    this.setEditor({ pending: action.Do });
    this.state.act?.(action);
  }
```

- **`closePicker`** drops `opening` too: `this.set({ picker: undefined, opening: undefined })`. Closing the picker while Go's settings are on their way means the editor shouldn't pop up after.

`picker.ts` `choose`:

```ts
// choose acts on the picker's row sel, as Enter or a click does: a
// character opens, connects and shows (pick), closing the picker; "+
// Character" asks for the new character's name; a world's row opens its
// editor, and + World a new world's. An editor open over the picker is
// left first, its edits kept, as the TUI's. Nothing for "".
export function choose(store: Store, sel: string): void {
  store.leaveEditor();
  if (sel === "+") store.openEditor({ kind: "new_world" }, false);
  else if (sel.startsWith("=")) store.openEditor({ kind: "world", world: sel.slice(1) }, false);
  else if (sel.startsWith("+")) store.setPicker({ adding: sel.slice(1), name: "", error: undefined });
  else if (sel) {
    store.state.act?.({ Do: "pick", Key: sel });
    store.closePicker();
  }
}
```

- [ ] **Step 4: Run the tests and type check**

Run: `cd web/ui && npm run check && npm test`
Expected: PASS, all, including 4a's picker and store tests. Anything building a `State` literal by hand (tests) needs `drafts: {}, parked: {}`; `tsc` names them.

- [ ] **Step 5: Commit**

```bash
git add web/ui/src/store.ts web/ui/src/store.test.ts web/ui/src/picker.ts web/ui/src/picker.test.ts
git commit -m "web/ui: the store holds editors: drafts, parking, and Go's answers"
```

---

### Task 5: The editor form (`EditorForm.tsx`) and its looks

**Files:**
- Create: `web/ui/src/ui/EditorForm.tsx`
- Modify: `web/ui/src/ui/BottomPane.tsx`
- Modify: `web/ui/src/theme.ts` (glyph and role variables), `web/ui/src/kiln.css`
- Modify: `internal/str/locales/en.toml` (`[web]`: `form_hint`, `delete_again`), then regenerate
- Test: `web/ui/src/ui/EditorForm.test.tsx`

**Interfaces:**
- Consumes: Task 3's `Editor`, `Field`, `errorOf`, `formError`, `firstError`, `unsaved`, `saveAction`, `deleteAction`, `targetKey`. Task 4's store methods.
- Produces: `EditorForm({ state, store })`, drawn by `BottomPane` while `state.picker.edit` is set, keyed by `targetKey` so a different editor remounts (and takes focus).
- New strings: `str.webFormHint(hint)`, `str.webDeleteAgain(what)`.

- [ ] **Step 1: Add the strings** (`internal/str/locales/en.toml`, in `[web]`, after `picker`)

```toml
# An editor's keys (web): Enter saves wherever you are, as forms do in a
# browser; {hint} is the editor's own (Esc to cancel).
form_hint = "Enter to save · Tab moves between fields · {hint}"
# Delete, pressed once (web): you can click as well as press Enter, and
# the web never stores a password, so nothing else is deleted.
delete_again = "Press again to delete {what}"
```

Run: `go generate ./internal/str && go test ./internal/str`
Expected: PASS; `str.gen.ts` has `webFormHint(hint)` and `webDeleteAgain(what)`.

- [ ] **Step 2: Write the failing tests** (`ui/EditorForm.test.tsx`)

```tsx
import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { Store } from "../store.ts";
import { roleClass } from "../theme.ts";
import * as str from "../str.gen.ts";

const inh = { Login: "", MaxLineBytes: 4096, NewlineMode: "batch", Autoconnect: false, Reconnect: true, Notify: "first", LocalEcho: true };
const fm = { type: "Settings", World: "fm", Char: "", Name: "", Inherited: inh,
  WorldSettings: { Host: "muck.test", Port: 8888, TLS: true, TLSTrust: "", Use: [], Login: null, MaxLineBytes: null, NewlineMode: null, Autoconnect: null, Reconnect: null, Notify: null, LocalEcho: null } };
const open = (patch?: (s: Store) => void) => {
  const s = new Store();
  s.set({ phase: "running", act: () => {} });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [] }] }]);
  s.openEditor({ kind: "world", world: "fm" }, true);
  s.apply([fm]);
  patch?.(s);
  return render(<App store={s} />);
};

test("the editor is a form named by its title, in the bottom pane, with the web's hint", () => {
  const html = open();
  assert.match(html, /<div class="bottom form[^"]*">\s*<form[^>]*aria-labelledby="ed-title"/);
  assert.ok(html.includes(`id="ed-title"`) && html.includes(str.editorEditing("fm")));
  assert.ok(html.includes(str.webFormHint(str.editorHint())));
});

test("fields are real controls with labels: text, a checkbox, radios; no select", () => {
  const html = open();
  assert.match(html, /<label[^>]*for="ed-host"[^>]*>/);
  assert.match(html, /<input[^>]*id="ed-host"[^>]*value="muck.test"/);
  assert.match(html, /<input[^>]*type="checkbox"[^>]*id="ed-tls"[^>]*checked/);
  assert.match(html, /role="radiogroup"[^>]*aria-labelledby="ed-reconnect-label"/);
  assert.match(html, /<input[^>]*type="radio"[^>]*name="ed-reconnect"/);
  assert.ok(!html.includes("<select"));
});

test("More settings is a details element, closed, with the TUI's ►", () => {
  const html = open();
  assert.match(html, /<details(?![^>]*\sopen)[^>]*>\s*<summary[^>]*>[^<]*►/);
  assert.ok(html.includes(str.editorExtra()));
});

test("unsaved shows in the title", () => {
  const html = open((s) => s.editField("host", "x"));
  assert.ok(html.includes(str.editorEditing("fm") + str.separator() + str.editorUnsaved()));
});

test("an error sits beside its field, tied to it; one about no field, at the top", () => {
  const html = open((s) => {
    s.sendEditor({ Do: "save_world" });
    s.apply([{ type: "Problems", Op: "save_world", Problems: [{ Field: "port", Msg: "bad port" }, { Field: "", Msg: "disk full" }] }]);
  });
  assert.match(html, /<input[^>]*id="ed-port"[^>]*aria-invalid="true"[^>]*aria-describedby="ed-port-err"/);
  assert.match(html, new RegExp(`id="ed-port-err"[^>]*class="${roleClass("form.error")}"[^>]*>[^<]*bad port`));
  assert.match(html, /role="alert"[^>]*>[^<]*disk full/);
});

test("Save is the primary button; Delete, once pressed, asks again", () => {
  const html = open((s) => s.setEditor({ armed: true }));
  assert.match(html, new RegExp(`<button[^>]*type="submit"[^>]*class="${roleClass("form.button")}"[^>]*>${str.editorSave()}`));
  assert.match(html, new RegExp(`<button[^>]*type="button"[^>]*class="${roleClass("form.button.secondary")}"[^>]*>${str.editorDeleteWorld()}`));
  assert.ok(html.includes(str.webDeleteAgain("fm")));
  assert.ok(!html.includes(str.editorForgetPassword()), "the web never stores a password");
});
```

Attribute order in the regexes follows the JSX in Step 4. If preact-render-to-string orders them differently, reorder the JSX attributes, not the tests' meaning.

- [ ] **Step 3: Run to see them fail**

Run: `cd web/ui && npm test`
Expected: FAIL. The bottom pane still shows the picker's filter.

- [ ] **Step 4: Implement**

`theme.ts`: extend `glyphs` and `lineRoles`:

```ts
const glyphs = `--g-echo:${JSON.stringify(str.glyph.echo)};--g-sys:${JSON.stringify(str.glyph.sys)};--g-rule:${JSON.stringify(str.glyph.rule)}` +
  `;--g-prev:${JSON.stringify(str.glyph.choicePrev)};--g-next:${JSON.stringify(str.glyph.choiceNext)}`;
```

and add `"form.hint"` to `lineRoles` (a text field's hint is its placeholder), updating that comment ("…and the input's and forms' hints (placeholders)").

`ui/EditorForm.tsx`:

```tsx
import { useEffect, useRef, useState } from "preact/hooks";
import { deleteAction, errorOf, firstError, formError, saveAction, unsaved } from "../editor.ts";
import type { Editor, Field } from "../editor.ts";
import { composing } from "../input.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// EditorForm is an editor over the picker, as the TUI's: its title (and
// whether it's unsaved), its fields with labels lined up, More settings
// (a world's extras), Save, Delete, and the keys. Each TUI field is a
// real control (spec, Controls). Enter in a field saves (spec: a
// browser's forms); Tab moves; Esc cancels. Errors sit by their field,
// and after a refusal focus goes to the first. Focus is drawn in
// form.focus, as the TUI draws its focused button.
export function EditorForm({ state, store }: { state: State; store: Store }) {
  const ed = state.picker!.edit!;
  const root = useRef<HTMLFormElement>(null);
  const [focus, setFocus] = useState("");
  useEffect(() => root.current?.querySelector<HTMLElement>("input, button")?.focus(), []);
  useEffect(() => {
    const k = firstError(ed);
    if (ed.refused && k) document.getElementById("ed-" + k)?.focus();
  }, [ed.refused]);
  const off = state.phase === "stopped";
  const save = () => {
    if (!ed.pending) store.sendEditor(saveAction(ed));
  };
  const del = deleteAction(ed);
  const what = ed.target.kind === "char" ? `${ed.target.world}/${ed.target.char}` : ed.target.kind === "world" ? ed.target.world : "";
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.defaultPrevented) return;
    const t = e.target as HTMLElement;
    if (e.key === "Escape") {
      e.preventDefault();
      store.closeEditor();
    } else if (e.key === "Enter" && t.tagName === "INPUT" && !composing(e) && !e.shiftKey && !e.altKey && !e.ctrlKey && !e.metaKey) {
      e.preventDefault();
      save();
    }
  };
  const main = ed.fields.filter((f) => !f.extra);
  const extra = ed.fields.filter((f) => f.extra);
  const top = formError(ed);
  // The keys, or Delete's question; one live region, so the question is
  // read out when it comes.
  const hint = (
    <>
      <span class={roleClass("form.hint")}>{" " + str.separator()}</span>
      <span aria-live="polite" class={roleClass(ed.armed ? "form.error" : "form.hint")}>{ed.armed ? str.webDeleteAgain(what) : str.webFormHint(str.editorHint())}</span>
    </>
  );
  return (
    <form ref={root} class="editor" aria-labelledby="ed-title" onKeyDown={onKeyDown}
      onSubmit={(e) => { e.preventDefault(); save(); }}
      onFocusIn={(e) => setFocus((e.target as HTMLElement).dataset.f ?? "")} onFocusOut={() => setFocus("")}>
      <div id="ed-title" class={roleClass("form.title")}>{ed.title}{unsaved(ed) ? str.separator() + str.editorUnsaved() : ""}</div>
      {top && <div role="alert" class={roleClass("form.error")}>{top}</div>}
      <div class="fields">{main.map((f) => <Row key={f.key} f={f} ed={ed} store={store} focus={focus} off={off} />)}</div>
      {extra.length > 0 && (
        <details open={ed.more} onToggle={(e) => store.setEditor({ more: (e.currentTarget as HTMLDetailsElement).open })}>
          <summary data-f="more" class={focus === "more" ? roleClass("form.focus") : undefined}>
            {(ed.more ? str.glyph.open : str.glyph.collapsed) + " " + str.editorExtra()}
          </summary>
          <div class="fields">{extra.map((f) => <Row key={f.key} f={f} ed={ed} store={store} focus={focus} off={off} />)}</div>
        </details>
      )}
      {/* The buttons a row each, as the TUI's; the keys after the last. */}
      <div class="buttons">
        <span class="row">
          <button type="submit" data-f="save" class={roleClass(focus === "save" ? "form.focus" : "form.button")} disabled={off}>{str.editorSave()}</button>
          {!del && hint}
        </span>
        {del && (
          <span class="row">
            <button type="button" data-f="delete" class={roleClass(focus === "delete" ? "form.focus" : "form.button.secondary")} disabled={off}
              onClick={() => (ed.armed ? store.sendEditor(del) : store.setEditor({ armed: true }))}>
              {ed.target.kind === "char" ? str.editorDeleteCharacter() : str.editorDeleteWorld()}
            </button>
            {hint}
          </span>
        )}
      </div>
    </form>
  );
}

// Row is one field: its label, its control, and what's wrong with it.
function Row({ f, ed, store, focus, off }: { f: Field; ed: Editor; store: Store; focus: string; off: boolean }) {
  const id = "ed-" + f.key;
  const err = errorOf(ed, f.key);
  const desc = err ? id + "-err" : undefined;
  const v = ed.values[f.key];
  const lit = focus === f.key ? roleClass("form.focus") : "";
  let control;
  if (f.kind === "text") {
    control = (
      <input id={id} data-f={f.key} type="text" value={String(v ?? "")} placeholder={f.hint} autocomplete="off" spellcheck={false} disabled={off}
        aria-invalid={err ? "true" : undefined} aria-describedby={desc}
        onInput={(e) => store.editField(f.key, (e.currentTarget as HTMLInputElement).value)} />
    );
  } else if (f.kind === "toggle") {
    // The TUI's [x]: the checkbox is read aloud, the box drawn.
    control = (
      <span class="box">
        <input id={id} data-f={f.key} type="checkbox" class="sr-only" checked={v === true} disabled={off}
          aria-invalid={err ? "true" : undefined} aria-describedby={desc}
          onChange={(e) => store.editField(f.key, (e.currentTarget as HTMLInputElement).checked)} />
        <span aria-hidden="true" class={lit}>{v === true ? "[x]" : "[ ]"}</span>
      </span>
    );
  } else {
    // The TUI's ◄ v ►: a radio group showing only the chosen option (the
    // arrows choose, as a radio group's do), ◄ ► while focused.
    control = (
      <span role="radiogroup" class={"choice " + lit} aria-labelledby={id + "-label"} aria-invalid={err ? "true" : undefined} aria-describedby={desc}>
        {f.options.map((o, i) => (
          <label key={o.value}>
            <input id={i === 0 ? id : undefined} data-f={f.key} type="radio" class="sr-only" name={id} value={o.value}
              checked={v === o.value} disabled={off} onChange={() => store.editField(f.key, o.value)} />
            <span>{o.shown}</span>
          </label>
        ))}
      </span>
    );
  }
  return (
    <>
      {f.kind === "choice"
        ? <span id={id + "-label"} class={roleClass("form.label")}>{f.label + ": "}</span>
        : <label for={id} class={roleClass("form.label")}>{f.label + ": "}</label>}
      <span class="control">
        {control}
        {err && <span id={id + "-err"} class={roleClass("form.error")}>{" " + err}</span>}
      </span>
    </>
  );
}
```

Ruling to make if the browser's radio group misbehaves: focusing the group should land on the checked radio, as browsers do. `id` sits on the first radio only so `ed-<key>` exists for "focus the first error". If the first radio isn't the checked one, focusing it doesn't change the choice: focus alone never checks a radio. Verify in the smoke test (Task 7).

`BottomPane.tsx`: inside `if (state.picker)`:

```tsx
        {state.picker.edit ? <EditorForm key={targetKey(state.picker.edit.target)} state={state} store={store} /> : <PickerForm state={state} store={store} />}
```

(import `EditorForm` and `targetKey`). Update the comment: "…or with the picker open its filter, or an editor over it (in the form's area…)".

`kiln.css`, after the `.pick-row` rules:

```css
/* An editor, as the TUI's: the title, then a row per field with the
   labels lined up, More settings, the buttons a row each, the keys after
   the last. Text fields are bare; a toggle is the TUI's [x] and a choice
   shows only what's chosen, ◄ ► while focused (the controls themselves
   are the browser's, out of sight). */
.editor .fields { display: grid; grid-template-columns: max-content 1fr; white-space: pre; }
.editor .control { display: flex; min-width: 0; }
.editor input[type="text"] { flex: 1; min-width: 1ch; font: inherit; color: inherit; background: transparent; border: 0; padding: 0; outline: none; }
.editor input::placeholder { color: var(--role-form-hint-fg); opacity: 1; }
.editor input:-webkit-autofill { -webkit-text-fill-color: var(--term-fg); box-shadow: inset 0 0 0 100vmax var(--term-bg); }
.editor .box, .editor .choice { position: relative; cursor: pointer; }
.editor .choice input:not(:checked) + span { display: none; }
.editor .choice span::before { content: "  "; }
.editor .choice:focus-within span::before { content: var(--g-prev) " "; }
.editor .choice:focus-within span::after { content: " " var(--g-next); }
.editor summary { list-style: none; cursor: pointer; white-space: pre; }
.editor summary::-webkit-details-marker { display: none; }
.editor .buttons .row { display: flex; white-space: pre; }
```


- [ ] **Step 5: Run the tests and type check**

Run: `cd web/ui && npm run check && npm test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/str/locales/en.toml internal/str web/ui/src/str.gen.ts web/ui/src/ui/EditorForm.tsx web/ui/src/ui/EditorForm.test.tsx web/ui/src/ui/BottomPane.tsx web/ui/src/theme.ts web/ui/src/kiln.css
git commit -m "web/ui: the editor form, real controls in the TUI's look

Strings: web.form_hint, web.delete_again (new)"
```

---

### Task 6: Ways in: Ctrl+T/Alt+T, `/edit`, the picker's Ctrl+T

**Files:**
- Modify: `web/ui/src/picker.ts` (`pickerKey`, `editorKey`)
- Modify: `web/ui/src/main.tsx` (the capture listener, the `/edit` effect)
- Modify: `web/ui/src/ui/PickerForm.tsx` (`useKeys` must not steal keys from an editor; it's unmounted then, so only check)
- Test: `web/ui/src/picker.test.ts`

**Interfaces:**
- Consumes: Task 4's `openEditor`, `hideEditor`; Task 3's `Target`.
- Produces: `editorKey(e: EditorKeyish, s: Pick<State, "active" | "phase" | "picker" | "chars">): { open: Target; closeAll: boolean } | "hide" | "eat" | undefined`. `EditorKeyish` is `PickerKeyish` plus `code: string`. `pickerKey` returns `"eat"` for Ctrl+O while an editor is open.

The TUI (`model.go` `openEditorKey`, `picker.go` `pickerKey`) is the reference:
- In the picker, Ctrl+T on a highlighted world or character opens its editor (`closeAll: false`). On an add row it does nothing.
- In an editor, Ctrl+T hides it.
- Outside the picker, Ctrl+T opens the active character's editor, or on an overview its world's (`closeAll: true`).
- Ctrl+T is the Mac's key: Windows and Linux browsers keep it, which the page can't help. Alt+T works everywhere. It's matched by `code === "KeyT"`, since Alt+T on a Mac types `†`.

- [ ] **Step 1: Write the failing tests** (`picker.test.ts`; extend the `key` helper to carry `code`, defaulting to `"Key" + key.toUpperCase()` for one letter)

```ts
const keyT = (mods: { ctrlKey?: boolean; altKey?: boolean }) => ({ ...key("t", mods), code: "KeyT" });
const altT = () => ({ ...key("†", { altKey: true }), code: "KeyT" });
const chars = { "fm/kit": { World: "fm", ID: "kit" } };
const on = (active: string, picker?: any) => ({ active, phase: "running" as const, picker, chars });

test("Ctrl+T or Alt+T: the active character's editor, or its world's from the overview, as the TUI's", () => {
  assert.deepEqual(editorKey(keyT({ ctrlKey: true }), on("fm/kit")), { open: { kind: "char", world: "fm", char: "kit" }, closeAll: true });
  assert.deepEqual(editorKey(altT(), on("=fm")), { open: { kind: "world", world: "fm" }, closeAll: true });
  assert.equal(editorKey(keyT({ ctrlKey: true }), on("")), "eat", "nothing open: nothing, and not the browser's");
});

test("in the picker: the highlighted world or character's editor; in an editor, Ctrl+T hides it", () => {
  assert.deepEqual(editorKey(keyT({ ctrlKey: true }), on("", { filter: "", sel: "=fm" })), { open: { kind: "world", world: "fm" }, closeAll: false });
  assert.deepEqual(editorKey(keyT({ ctrlKey: true }), on("", { filter: "", sel: "fm/rook" })), { open: { kind: "char", world: "fm", char: "rook" }, closeAll: false });
  assert.equal(editorKey(keyT({ ctrlKey: true }), on("", { filter: "", sel: "+fm" })), "eat");
  assert.equal(editorKey(keyT({ ctrlKey: true }), on("", { filter: "", sel: "", edit: {} })), "hide");
  assert.equal(editorKey(keyT({ ctrlKey: true }), on("", { filter: "", sel: "+fm", adding: "fm" })), "eat");
});

test("not Ctrl+T: Shift, Meta, or Kiln not running", () => {
  assert.equal(editorKey(keyT({}), on("fm/kit")), undefined);
  assert.equal(editorKey({ ...keyT({ ctrlKey: true }), shiftKey: true }, on("fm/kit")), undefined);
  assert.equal(editorKey(keyT({ ctrlKey: true }), { ...on("fm/kit"), phase: "stopped" as const }), "eat");
});

test("Ctrl+O while an editor is open does nothing, as the TUI's", () => {
  assert.equal(pickerKey(key("o", { ctrlKey: true }), at("", { picker: { filter: "", sel: "", edit: {} } as any })), "eat");
});
```

- [ ] **Step 2: Run to see them fail**

Run: `cd web/ui && npm test`
Expected: FAIL, `editorKey` not exported, and the Ctrl+O case returns `"close"`.

- [ ] **Step 3: Implement** (`picker.ts`)

In `pickerKey`, the Ctrl+O guard becomes `if (s.phase !== "running" || s.picker?.adding !== undefined || s.picker?.edit) return "eat";`, and its comment adds "or in an editor".

Add (`import type { Target } from "./editor.ts";`):

```ts
// EditorKeyish is a keydown as editorKey reads it: code too, since Alt+T
// on a Mac types a character, not "t".
interface EditorKeyish extends PickerKeyish {
  code: string;
}

// splitKey is a character's key as its world and id.
const splitKey = (k: string): [string, string] => {
  const i = k.indexOf("/");
  return [k.slice(0, i), k.slice(i + 1)];
};

// editorKey is what Ctrl+T or Alt+T does, as the TUI's Ctrl+T: in an
// editor, hide it (its edits kept); in the picker, open the highlighted
// world's or character's editor; elsewhere the active character's, or
// on an overview its world's, closing with the picker. Where it does
// nothing it's still eaten, so the browser doesn't take it (Windows and
// Linux browsers keep Ctrl+T anyway; Alt+T works everywhere).
export function editorKey(e: EditorKeyish, s: Pick<State, "active" | "phase" | "picker" | "chars">): { open: Target; closeAll: boolean } | "hide" | "eat" | undefined {
  const ctrlT = e.ctrlKey && !e.altKey && e.key.toLowerCase() === "t";
  const altT = e.altKey && !e.ctrlKey && e.code === "KeyT";
  if (!(ctrlT || altT) || e.metaKey || e.shiftKey) return undefined;
  if (s.phase !== "running" || s.picker?.adding !== undefined) return "eat";
  if (s.picker?.edit) return "hide";
  if (s.picker) {
    const sel = s.picker.sel;
    if (sel.startsWith("=")) return { open: { kind: "world", world: sel.slice(1) }, closeAll: false };
    if (sel.includes("/")) {
      const [world, char] = splitKey(sel);
      return { open: { kind: "char", world, char }, closeAll: false };
    }
    return "eat";
  }
  if (s.active.startsWith("=")) return { open: { kind: "world", world: s.active.slice(1) }, closeAll: true };
  if (s.chars[s.active]) {
    const [world, char] = splitKey(s.active);
    return { open: { kind: "char", world, char }, closeAll: true };
  }
  return "eat";
}
```

`main.tsx`: the capture listener also asks `editorKey`. `e.code` goes into the keyish.

```ts
  const ed = editorKey({ key: e.key, code: e.code, ctrlKey: e.ctrlKey, altKey: e.altKey, metaKey: e.metaKey, shiftKey: e.shiftKey, onBody: !t || t === document.body }, store.state);
  if (ed) {
    e.preventDefault();
    if (ed === "hide") store.hideEditor();
    else if (ed !== "eat") store.openEditor(ed.open, ed.closeAll);
    return;
  }
```

Put this before the existing `pickerKey` handling. In `effect`, add `/edit`, as the TUI's `editCommand`: the active character's editor, or with `world` its world's, closing with the picker. Update the comment's "/log and /edit come with…" to "/log comes with log mode (step 5)".

```ts
  } else if (e.effect === "Do" && e.Cmd === "/edit" && e.Key) {
    const [world, char] = [String(e.Key).slice(0, String(e.Key).indexOf("/")), String(e.Key).slice(String(e.Key).indexOf("/") + 1)];
    store.openEditor(e.Args?.[0] === "world" ? { kind: "world", world } : { kind: "char", world, char }, true);
```

(Export `splitKey` from `picker.ts` and use it here instead of the inline slicing.)

`PickerForm.tsx`: the filter's `onKeyDown` gets no Ctrl+T case. The capture listener has handled it before the filter sees it.

- [ ] **Step 4: Run the tests and type check**

Run: `cd web/ui && npm run check && npm test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/ui/src/picker.ts web/ui/src/picker.test.ts web/ui/src/main.tsx web/ui/src/ui/PickerForm.tsx
git commit -m "web/ui: Ctrl+T, Alt+T and /edit open the editor, as the TUI's"
```

---

### Task 7: End to end, and the docs

**Files:**
- Create: `web/smoke/editor_test.go`
- Modify: `docs/web-dev.md`

**Interfaces:**
- Consumes: the smoke helpers in `web/smoke/smoke_test.go` and `scroll_test.go`: `launch`, `evaluator`, `waitFor`, `pressKey`, `ctrlO`, `devtools`, `mustUnquote`, `jsString`.

- [ ] **Step 1: Write the smoke test**

The page's test handle (`kilnPage`, `boot.ts`) has `act`, `events` and `sent`, but no store. The highlight is moved with keys, as `TestPicker` does. The fake world (`config` in `smoke_test.go`) is `fake`: Tester, Second and Historian, on the fake MUCK's port. A save keeps that port, so later connections still work.

```go
package smoke

import (
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

// TestEditor edits the way a person does: a world from the picker, a
// check and a refusal fixed, a save; a choice by arrow keys; a new world;
// a character's editor from Alt+T, hidden and back with its edit; and a
// delete pressed twice.
func TestEditor(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Worlds")`, "the worlds")
	field := func(k string) string { return `document.querySelector("#ed-` + k + `")` }
	selected := `document.querySelector('nav [aria-selected="true"]').id`
	filter := `document.querySelector('.bottom input[role="combobox"]')`

	// The picker, ↑ from the first character to the world's row, Enter:
	// its editor, focus in Host, the scrollback dimmed.
	ctrlO(t, ws)
	waitFor(t, ev, selected+` === "pick-fake/historian"`, "the picker, on the first character")
	pressKey(t, ws, "ArrowUp", 38)
	waitFor(t, ev, selected+` === "pick-=fake"`, "the world's row")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.activeElement === `+field("host"), "the world's editor, Host focused")
	if got := ev(`document.querySelector(".scroll.dim") !== null`); got != "true" {
		t.Error("the scrollback isn't dimmed behind the editor")
	}

	// A bad port shows its check by the field; Ctrl+O is eaten; Enter is
	// refused by Go, focus going to Port; nothing typed reached the input.
	port := mustUnquote(t, ev(field("port")+`.value`))
	ev(field("port") + `.select()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "99x"})
	waitFor(t, ev, field("port")+`.getAttribute("aria-invalid") === "true"`, "the port's check")
	ctrlO(t, ws)
	if got := ev(field("port") + ` !== null`); got != "true" {
		t.Error("Ctrl+O closed the editor")
	}
	ev(field("host") + `.focus()`)
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Problems" && e.Op === "save_world") && document.activeElement === `+field("port"), "refused, focus on Port")
	if got := ev(`kilnPage.sent.some(a => a.Do === "input")`); got != "false" {
		t.Error("typing in the editor went to the chat input")
	}

	// A choice: ↓ on the chosen radio picks the next (the browser's radio
	// keys). Then the port put back, Enter: saved, back at the filter.
	ev(`document.querySelector("details").open = true`)
	waitFor(t, ev, `document.querySelector("details[open]")`, "More settings open")
	ev(`document.querySelector('[name="ed-reconnect"]:checked').focus()`)
	pressKey(t, ws, "ArrowDown", 40)
	waitFor(t, ev, `document.querySelector('[name="ed-reconnect"]:checked').value === "on"`, "↓ chose on")
	ev(field("port") + `.select()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": port})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Saved" && e.Op === "save_world") && !`+field("port")+` && document.activeElement === `+filter, "saved, back at the filter")
	waitFor(t, ev, `document.querySelector('[role="status"]').textContent.includes(`+jsString(str.EditorSaved("fake"))+`)`, "the status says saved")

	// + World, the last row: a new world saved lands on its + Character.
	for range 5 {
		pressKey(t, ws, "PageDown", 34)
	}
	waitFor(t, ev, selected+` === "pick-+"`, "+ World")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.activeElement === `+field("id"), "a new world's editor, World focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "zz"})
	ev(field("host") + `.focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "zz.test"})
	ev(field("port") + `.focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "7777"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, selected+` === "pick-+zz"`, "the new world's + Character highlighted")

	// Second, picked; then from the main view Alt+T, an edit, Alt+T hides
	// it (the picker with it), Alt+T brings it back, unsaved.
	ev(filter + `.focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "sec"})
	waitFor(t, ev, selected+` === "pick-fake/second"`, "Second highlighted")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.querySelector("header").textContent.includes("Second")`, "Second showing")
	altT := func() {
		for _, typ := range []string{"rawKeyDown", "keyUp"} {
			devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": typ, "key": "†", "code": "KeyT", "windowsVirtualKeyCode": 84, "modifiers": 1})
		}
	}
	altT()
	waitFor(t, ev, `document.activeElement === `+field("aliases"), "Second's editor")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "twoey"})
	altT()
	waitFor(t, ev, `!`+field("aliases")+` && !document.querySelector(".scroll.dim")`, "hidden, the picker closed with it")
	altT()
	unsaved := jsString(str.EditorEditing("fake/Second") + str.Separator() + str.EditorUnsaved())
	waitFor(t, ev, field("aliases")+`.value === "twoey" && document.querySelector("#ed-title").textContent === `+unsaved, "back, unsaved")

	// Delete, pressed twice: Second's gone, and so is the editor.
	ev(`[...document.querySelectorAll(".editor button")].at(-1).focus()`)
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.querySelector(".editor [aria-live]").textContent === `+jsString(str.WebDeleteAgain("fake/second")), "asked again")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Saved" && e.Op === "delete_character") && !document.querySelector(".editor")`, "deleted, the editor gone")
}
```

If the first character the picker highlights isn't Historian (the order is `app.CompareChars`, see 4a), start from the one it is: the step only needs ↑ to land on `=fake`.

- [ ] **Step 2: Run it**

Run: `web/build.sh >/dev/null && cd web && KILN_SMOKE=1 go test -count=1 -run TestEditor ./smoke`
Expected: PASS. A failure is a finding: debug it (superpowers:systematic-debugging) before changing the test.

- [ ] **Step 3: Mutation-check two steps**
  1. In `store.ts`'s `apply`, comment out `s = follow(s, prev);`. Expect the store tests' parking cases to fail. Restore it.
  2. In `EditorForm.tsx`, change the refusal focus effect's dependency from `[ed.refused]` to `[]`. Expect the smoke test's "refused, focus on Port" to fail. Restore it.

  Undo by a reverse edit or a backup copy, never `git checkout`.

- [ ] **Step 4: The docs** (`docs/web-dev.md`): where it lists what the page does, add the editor:
  - opened from the picker (Enter on a world or `+ World`), Ctrl+T (Mac) or Alt+T, `/edit` and `/edit world`;
  - Enter saves; Esc cancels; Ctrl+T/Alt+T hides it and keeps the edits.

- [ ] **Step 5: The whole suite**

Run: `go vet ./... && go test ./... && (cd web && go vet ./... && go test ./... && GOOS=js GOARCH=wasm go vet ./...) && (cd web/ui && npm run check && npm test) && web/build.sh >/dev/null && (cd web && KILN_SMOKE=1 go test -count=1 ./smoke)`
Expected: PASS, every package; smoke 3 tests.

- [ ] **Step 6: Commit**

```bash
git add web/smoke/editor_test.go docs/web-dev.md
git commit -m "web: the editor end to end; web-dev.md says how to open it"
```
