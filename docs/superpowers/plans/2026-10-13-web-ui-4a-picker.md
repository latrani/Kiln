# Web UI step 4a: the picker — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The page can open a connection the way the TUI does: the picker in the sidebar, its filter in the bottom pane, adding a character, and the scrollback dimmed behind it.

**Architecture:** Go sends the worlds in the picker's order with their aliases, and gains a `pick` action (open, switch, connect) and a live check for a character's name. The page keeps the picker's state (filter, highlight, the add-character form) in the store; `picker.ts` holds the TUI's row and highlight rules as pure functions; the Sidebar draws the picker's list, the BottomPane its filter or the name form, and the Scrollback dims behind it by CSS.

**Tech Stack:** Go (internal/feed, internal/webapi, internal/app), Preact + TypeScript (web/ui), node:test, headless Chrome smoke (web/smoke).

**Spec:** `docs/superpowers/specs/2026-10-12-web-ui-design.md` (step 4, "The bottom pane's forms"; this is its first half: the editor forms are 4b).

## Global Constraints

- Where the spec doesn't say otherwise, the web does what the TUI does (`internal/ui/picker.go` is the reference for rows, highlight and keys).
- Go decides data and order; the page decides looks, from theme roles (`roleClass`, CSS variables), never a hard-coded color.
- Every string a person reads comes from the catalog (`internal/str/locales/en.toml` → `go generate ./internal/str` → `str.gen.ts`); a commit that changes it carries a `Strings:` trailer.
- Tests build expected text from the catalog (`str.*`), never a copy of it.
- Opening a form or the picker moves focus into it; Esc puts focus back. The dimmed scrollback stays readable (not `inert`).
- Native elements first; ARIA only where none fits.
- Pre-1.0: no compatibility shims for API shape changes.
- The TUI's goldens never move.

## Review Focus

1. **Picking a character that's already open** (autoconnect or another tab's turn opened it meanwhile): it should switch to it, never drop or reconnect a live session. → Task 2's `TestPickAnOpenCharacterOnlySwitches`.
2. **A filter that matches nothing**: Enter does nothing, nothing throws, and clearing the filter brings the highlight back to the first character. → Task 3's "nothing matches" test.
3. **Focus after the picker closes** (Esc, Ctrl+O again, a pick): it lands in the input of whatever is showing, so typing just works. → Task 8's smoke steps.
4. **Typing in the filter or the name field never reaches the chat input's drafts** (Typing) or Go's input. → Task 5's test that keys in the filter send no `input`.
5. **The picker while Kiln stops or another tab takes over**: it closes, as everything else does on `Start`, and nothing is left focused in a vanished form. → Task 4's store test (`Start` drops the picker).

---

## File Structure

- `internal/feed/feed.go` — `worlds()`: picker order, aliases. Test in `feed_test.go`.
- `internal/webapi/actions.go` — `pick` action. Test in `loop_test.go`.
- `internal/app/edit.go` — `name` in `liveChecks`. Test in `edit_test.go`.
- `web/ui/src/picker.ts` (new) — rows, selection keys, highlight moves. Test `picker.test.ts`.
- `web/ui/src/store.ts` — `picker` and `answer` state; `Saved`/`Problems` events.
- `web/ui/src/sidebar.ts` — `+ Connection` and the gap row.
- `web/ui/src/ui/Sidebar.tsx` — the `+ Connection` chip; the picker's list when open.
- `web/ui/src/ui/PickerForm.tsx` (new) — the filter and the name form in the bottom pane.
- `web/ui/src/ui/BottomPane.tsx` — shows PickerForm while the picker is open.
- `web/ui/src/ui/Scrollback.tsx`, `ui/Overview.tsx`, `kiln.css` — dimming.
- `web/ui/src/ui/Input.tsx`, `main.tsx` — Enter with nothing open, Ctrl+O, `/open`, Enter on an overview.
- `web/smoke/picker_test.go` (new) — end to end.
- `docs/web-dev.md` — brought up to date (it still says the page is empty and backup isn't on it).

---

### Task 1: Worlds in the picker's order, with aliases

**Files:**
- Modify: `internal/feed/feed.go` (`worlds`, `Character`)
- Test: `internal/feed/feed_test.go`

**Interfaces:**
- Produces: `Worlds` event `{Worlds: [{ID, Characters: [{ID, Name, Aliases}]}]}`, worlds sorted by lowercased ID, characters by `app.CompareChars`. `Aliases` is omitted when empty (`omitempty`).

- [ ] **Step 1: Write the failing test**

```go
func TestWorldsAreInThePickersOrderWithAliases(t *testing.T) {
	a, dir := newApp(t)
	writeWorld(t, dir, fmWorld+"\n[[characters]]\nid = \"ash\"\nname = \"ash\"\naliases = [\"cinder\"]\n")
	if err := os.WriteFile(filepath.Join(dir, "worlds", "Bay.toml"), []byte("host = \"b.test\"\nport = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	w, _ := find[Worlds](New(a).Turn(nil))
	var ids []string
	for _, x := range w.Worlds {
		ids = append(ids, x.ID)
	}
	if !reflect.DeepEqual(ids, []string{"Bay", "fm"}) {
		t.Errorf("worlds %v, want Bay before fm (case-insensitive)", ids)
	}
	fm := w.Worlds[1].Characters
	if len(fm) != 3 || fm[0].Name != "ash" || fm[1].Name != "Kit" || fm[2].Name != "Rook" {
		t.Errorf("fm's characters %+v, want by name, case-insensitive", fm)
	}
	if !reflect.DeepEqual(fm[0].Aliases, []string{"cinder"}) {
		t.Errorf("ash's aliases %v", fm[0].Aliases)
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/feed -run TestWorldsAreInThePickersOrder`
Expected: FAIL (build error: no field `Aliases`).

- [ ] **Step 3: Implement**

In `feed.go`, `Character` becomes:

```go
// Character is one character in a World: aliases are what the picker's
// filter matches besides its name and world.
type Character struct {
	ID, Name string
	Aliases  []string `json:",omitempty"`
}
```

and `worlds`:

```go
// worlds is every configured world and its characters, in the picker's
// order: worlds by id, characters by name, ignoring case.
func worlds(a *app.App) Worlds {
	var out Worlds
	if cfg := a.Config(); cfg != nil {
		for _, w := range cfg.Worlds {
			ws := World{ID: w.ID}
			chars := slices.Clone(w.Characters)
			slices.SortFunc(chars, app.CompareChars)
			for _, c := range chars {
				ws.Characters = append(ws.Characters, Character{ID: c.ID, Name: c.Name, Aliases: c.Aliases})
			}
			out.Worlds = append(out.Worlds, ws)
		}
	}
	slices.SortFunc(out.Worlds, func(x, y World) int { return strings.Compare(strings.ToLower(x.ID), strings.ToLower(y.ID)) })
	return out
}
```

(`Character` lives in `internal/feed/events.go`; edit it there.)

- [ ] **Step 4: Run**

Run: `go test ./internal/feed ./internal/webapi`
Expected: PASS.

- [ ] **Step 5: Commit** — `feed: worlds in the picker's order, with aliases`

---

### Task 2: The `pick` action, and a name's live check

**Files:**
- Modify: `internal/webapi/actions.go`, `internal/app/edit.go`
- Test: `internal/webapi/loop_test.go`, `internal/app/edit_test.go`

**Interfaces:**
- Produces: action `{Do: "pick", Key}` — opens Key if it isn't, switches to it, and connects it only if it wasn't open; a Key that isn't configured does nothing. `kiln.check("name", v)` returns `config.NameChars`'s message.

- [ ] **Step 1: Write the failing tests**

`loop_test.go`:

```go
func TestPickOpensSwitchesAndConnects(t *testing.T) {
	h := start(t)
	b := h.act(`{"Do":"pick","Key":"fm/rook"}`)
	if a := event(b, "Active"); a == nil || a["Key"] != "fm/rook" {
		t.Errorf("active %v", a)
	}
	deadline := time.After(2 * time.Second)
	for {
		if c := event(b, "Char"); c != nil && c["Key"] == "fm/rook" && c["State"] != float64(session.Disconnected) {
			return
		}
		select {
		case b = <-h.batches:
		case <-deadline:
			t.Fatal("never connecting")
		}
	}
}

func TestPickAnOpenCharacterOnlySwitches(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	h.act(`{"Do":"open","Key":"fm/rook"}`)
	h.act(`{"Do":"switch","Key":"fm/kit"}`)
	h.act(`{"Do":"pick","Key":"fm/rook"}`)
	if got := h.a.Active(); got != "fm/rook" {
		t.Errorf("active %q", got)
	}
	if h.a.Char("fm/rook").Sess != nil {
		t.Error("picking an open character connected it")
	}
}

func TestPickSomeoneUnknownDoesNothing(t *testing.T) {
	h := start(t)
	h.act(`{"Do":"pick","Key":"fm/nobody"}`)
	if h.a.Char("fm/nobody") != nil || h.a.Active() != "" {
		t.Error("picked a character that isn't configured")
	}
}
```

(The second test's `Sess != nil` check is a pin: `open` never connects, so a pick that connected would give Rook a session.)

`internal/app/edit_test.go`:

```go
func TestCheckFieldName(t *testing.T) {
	if CheckField("name", "Kit") != nil {
		t.Error("Kit refused")
	}
	if err := CheckField("name", "Kit Two"); err == nil || err.Error() != str.ConfigNameHasSpaces() {
		t.Errorf("a space: %v", err)
	}
}
```

- [ ] **Step 2: Run them**

Run: `go test ./internal/webapi ./internal/app -run 'Pick|CheckFieldName'`
Expected: FAIL: an unknown action (status "bad action"), and the name check returns nil.

- [ ] **Step 3: Implement**

`edit.go`, in `liveChecks`: add `"name": config.NameChars,`.

`actions.go`, beside `switch`:

```go
	case "pick": //str:ok
		// The picker's Enter on a character, as the TUI's pick: open it,
		// show it and connect it. One already open (autoconnect, or opened
		// since the page drew the picker) is only shown.
		was := a.Char(act.Key) != nil
		l.open(act.Key)
		if a.Char(act.Key) != nil {
			a.Switch(act.Key)
			if !was {
				effs = a.Connect(act.Key)
			}
		}
```

- [ ] **Step 4: Run**

Run: `go test ./internal/...`
Expected: PASS.

- [ ] **Step 5: Commit** — `webapi: pick opens, shows and connects a character; a name has a live check`

---

### Task 3: The picker's rows and highlight (`picker.ts`)

**Files:**
- Create: `web/ui/src/picker.ts`, `web/ui/src/picker.test.ts`

**Interfaces:**
- Consumes: the `Worlds` event's shape (Task 1).
- Produces:

```ts
export type PickRow =
  | { kind: "world"; world: string }
  | { kind: "char"; world: string; key: string; name: string }
  | { kind: "add_char"; world: string };
export function pickerRows(worlds: WorldInfo[], open: Record<string, unknown>, filter: string): PickRow[];
export function selKey(r: PickRow): string;            // "" for a row that can't be highlighted
export function pickable(rows: PickRow[]): string[];
export function fixPick(rows: PickRow[], sel: string): string;   // keep, else first character, else first row, else ""
export function movePick(rows: PickRow[], sel: string, delta: number): string;
export function pagePick(rows: PickRow[], sel: string, dir: number, page: number): string;
export interface WorldInfo { ID: string; Characters?: { ID: string; Name: string; Aliases?: string[] }[] }
```

Selection keys as the TUI's: a character's is its key (`world/id`); `+ Character`'s is `+world`. A world's header row can't be highlighted in 4a (its editor is 4b): `selKey` gives `""`. There's no `+ World` row until 4b.

- [ ] **Step 1: Write the failing tests**

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { fixPick, movePick, pagePick, pickable, pickerRows } from "./picker.ts";

const worlds = [
  { ID: "Bay", Characters: [{ ID: "ash", Name: "Ash", Aliases: ["cinder"] }] },
  { ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }, { ID: "rook", Name: "Rook" }] },
];
const kinds = (rows: { kind: string }[]) => rows.map((r) => r.kind).join(" ");

test("characters not open, under their worlds, each world ending with + Character", () => {
  const rows = pickerRows(worlds, { "fm/kit": {} }, "");
  assert.equal(kinds(rows), "world char add_char world char add_char");
  assert.deepEqual(pickable(rows), ["Bay/ash", "+Bay", "fm/rook", "+fm"]);
});

test("the filter matches a name, an alias or the world, ignoring case and spaces around it", () => {
  assert.deepEqual(pickable(pickerRows(worlds, {}, " CIN ")), ["Bay/ash", "+Bay"]);
  assert.deepEqual(pickable(pickerRows(worlds, {}, "fm")), ["fm/kit", "fm/rook", "+fm"]);
});

test("a world whose name matches is listed with nothing to offer; one that doesn't, isn't", () => {
  const rows = pickerRows(worlds, { "fm/kit": {}, "fm/rook": {} }, "f");
  assert.equal(kinds(rows), "world add_char");
});

test("the highlight stays on its row, else goes to the first character, else the first row", () => {
  const rows = pickerRows(worlds, {}, "");
  assert.equal(fixPick(rows, "fm/kit"), "fm/kit");
  assert.equal(fixPick(rows, "gone/x"), "Bay/ash");
  assert.equal(fixPick(pickerRows(worlds, { "Bay/ash": {}, "fm/kit": {}, "fm/rook": {} }, ""), ""), "+Bay");
});

test("nothing matches: nothing to highlight", () => {
  const rows = pickerRows(worlds, {}, "zzz");
  assert.deepEqual(rows, []);
  assert.equal(fixPick(rows, "fm/kit"), "");
  assert.equal(movePick(rows, "", 1), "");
});

test("↑/↓ move a row, stopping at the ends; PgUp/PgDn a page of rows", () => {
  const rows = pickerRows(worlds, {}, "");
  assert.equal(movePick(rows, "Bay/ash", -1), "Bay/ash");
  assert.equal(movePick(rows, "Bay/ash", 1), "+Bay");
  assert.equal(movePick(rows, "+fm", 1), "+fm");
  assert.equal(pagePick(rows, "Bay/ash", 1, 3), "fm/kit", "three rows on: the world row, then Kit");
  assert.equal(pagePick(rows, "+fm", -1, 100), "Bay/ash");
});
```

- [ ] **Step 2: Run**

Run: `cd web/ui && npm test`
Expected: FAIL (no `picker.ts`).

- [ ] **Step 3: Implement** `picker.ts`, porting `internal/ui/picker.go`'s `matches`, `pickerRows`, `pickable`, `fixPick`, `movePick` and `pagePick` (same rules; `page` is the count of rows a page moves, the TUI's `avail-1`):

```ts
// The picker's rows and highlight, as the TUI's (internal/ui/picker.go):
// every configured character that isn't open, under its world, each
// world ending with a row to add a character. Go sends the worlds in
// this order (feed.worlds). A world's own row is highlightable once its
// editor comes (step 4b), and "+ World" comes with it.

export interface WorldInfo { ID: string; Characters?: { ID: string; Name: string; Aliases?: string[] }[] }

export type PickRow =
  | { kind: "world"; world: string }
  | { kind: "char"; world: string; key: string; name: string }
  | { kind: "add_char"; world: string };

// matches reports whether a character fits the lowercased filter f: its
// name, an alias or its world contains it.
function matches(world: string, c: { Name: string; Aliases?: string[] }, f: string): boolean {
  return !f || [c.Name, world, ...(c.Aliases ?? [])].some((s) => s.toLowerCase().includes(f));
}

export function pickerRows(worlds: WorldInfo[], open: Record<string, unknown>, filter: string): PickRow[] {
  const f = filter.trim().toLowerCase();
  const rows: PickRow[] = [];
  for (const w of worlds) {
    const chars: PickRow[] = (w.Characters ?? [])
      .filter((c) => !open[`${w.ID}/${c.ID}`] && matches(w.ID, c, f))
      .map((c) => ({ kind: "char", world: w.ID, key: `${w.ID}/${c.ID}`, name: c.Name }));
    if (chars.length || w.ID.toLowerCase().includes(f)) rows.push({ kind: "world", world: w.ID }, ...chars, { kind: "add_char", world: w.ID });
  }
  return rows;
}

export function selKey(r: PickRow): string {
  return r.kind === "char" ? r.key : r.kind === "add_char" ? "+" + r.world : "";
}

export const pickable = (rows: PickRow[]) => rows.map(selKey).filter((k) => k);

export function fixPick(rows: PickRow[], sel: string): string {
  const keys = pickable(rows);
  if (keys.includes(sel)) return sel;
  return keys.find((k) => k.includes("/")) ?? keys[0] ?? "";
}

export function movePick(rows: PickRow[], sel: string, delta: number): string {
  const keys = pickable(rows);
  const i = keys.indexOf(sel);
  return i < 0 ? sel : keys[Math.min(Math.max(0, i + delta), keys.length - 1)];
}

export function pagePick(rows: PickRow[], sel: string, dir: number, page: number): string {
  const cur = rows.findIndex((r) => selKey(r) === sel);
  if (cur < 0) return sel;
  for (let i = Math.min(Math.max(0, cur + dir * page), rows.length - 1); i !== cur; i -= dir) {
    const k = selKey(rows[i]);
    if (k) return k;
  }
  return movePick(rows, sel, dir);
}
```

(Doc comments on each export, in the file's voice, as the TUI's.)

- [ ] **Step 4: Run** — `cd web/ui && npm test && npm run check` → PASS.

- [ ] **Step 5: Commit** — `web/ui: the picker's rows and highlight, as the TUI's`

---

### Task 4: The store holds the picker; `+ Connection`; ways to open it

**Files:**
- Modify: `web/ui/src/store.ts`, `web/ui/src/sidebar.ts`, `web/ui/src/ui/Sidebar.tsx`, `web/ui/src/ui/Input.tsx`, `web/ui/src/main.tsx`, `web/ui/src/kiln.css`
- Test: `web/ui/src/store.test.ts`, `web/ui/src/ui/App.test.tsx`

**Interfaces:**
- Consumes: `pickerRows`, `fixPick` (Task 3).
- Produces, in `store.ts`:

```ts
export interface Picker {
  filter: string;
  sel: string;     // the highlighted row's selection key (picker.ts)
  adding?: string; // the world the add-character form is for
  name?: string;   // what's typed there
  error?: string;  // why the name won't do (Go's check, or the save's problem)
}
// in State:
picker?: Picker;
answer?: { n: number; Op: string; Key?: string; Problems?: { Field: string; Msg: string }[] }; // the last Saved or Problems, numbered
```

and the Store methods:

```ts
openPicker(): void;   // filter "", highlight fixed (fixPick on the current rows)
closePicker(): void;  // picker undefined
setPicker(p: Partial<Picker>): void; // merge, then fixPick the highlight against the rows for the (new) filter
```

`Start` drops `picker` (fresh doesn't keep it). A `Saved` or `Problems` event sets `answer` with `n` one more than before.

`sidebar.ts`'s `sidebarRows` adds `{kind: "gap"}` (when any rows) then `{kind: "add"}`, as the TUI's.

- [ ] **Step 1: Write the failing tests**

`store.test.ts`:

```ts
test("the picker: opened with its highlight on the first character; Start drops it", () => {
  const s = new Store();
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }] }] }]);
  s.openPicker();
  assert.deepEqual(s.state.picker, { filter: "", sel: "fm/kit" });
  s.setPicker({ filter: "zzz" });
  assert.equal(s.state.picker?.sel, "", "nothing matches");
  s.apply([{ type: "Start" }]);
  assert.equal(s.state.picker, undefined);
});

test("Saved and Problems are the latest answer, numbered", () => {
  const s = new Store();
  s.apply([{ type: "Saved", Op: "add_character", Key: "fm/Ash" }]);
  assert.deepEqual(s.state.answer, { n: 1, Op: "add_character", Key: "fm/Ash", Problems: undefined });
  s.apply([{ type: "Problems", Op: "add_character", Problems: [{ Field: "name", Msg: "no" }] }]);
  assert.equal(s.state.answer?.n, 2);
  assert.equal(s.state.answer?.Problems?.[0].Msg, "no");
});
```

`App.test.tsx`:

```ts
test("the sidebar ends with + Connection, after a gap, as the TUI's", () => {
  const html = page({ phase: "running" }, three);
  assert.ok(html.includes(str.sidebarOpenConnection()), html);
  assert.match(html, new RegExp(`<button[^>]*class="[^"]*${roleClass("sidebar.add")}[^"]*"[^>]*>${str.sidebarOpenConnection()}`));
});

test("with the picker open, the sidebar is the picker: worlds, characters to open, + Character", () => {
  const s = new Store();
  s.set({ phase: "running" });
  s.apply([...three, { type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }, { ID: "mira", Name: "Mira" }] }] }]);
  s.openPicker();
  const html = render(<App store={s} />);
  const nav = html.slice(html.indexOf("<nav"), html.indexOf("</nav>"));
  assert.match(nav, new RegExp(`role="listbox"`));
  assert.ok(nav.includes("Mira") && !nav.includes(">Kit<"), "Kit's open: not offered");
  assert.ok(nav.includes(str.pickerAddCharacter()));
  assert.match(nav, /aria-selected="true"[^>]*>[^<]*Mira/);
  assert.ok(nav.includes(roleClass("picker")), "the picker's area role");
});
```

- [ ] **Step 2: Run** — `cd web/ui && npm test` → FAIL.

- [ ] **Step 3: Implement**

- `store.ts`: the `Picker` type, `picker`/`answer` in `State`; `step` handles `Saved` and `Problems`:

```ts
      case "Saved":
      case "Problems":
        return { ...s, answer: { n: (s.answer?.n ?? 0) + 1, Op: e.Op, Key: e.Key, Problems: e.Problems } };
```

  and the three methods (rows from `pickerRows(this.state.worlds, this.state.chars, filter)`).
- `sidebar.ts`: the `gap` and `add` rows; doc comment no longer says "less + Connection".
- `Sidebar.tsx`: a `gap` row is an empty `<li aria-hidden="true">`; an `add` row is `<li><button class={roleClass("sidebar.add")} onClick={() => store.openPicker()}>{str.sidebarOpenConnection()}</button></li>`. `Sidebar`, `BottomPane` and `Input` take the `store` as a prop beside `state` (`App` has it), for the picker's methods.
  With `state.picker` set, the `<nav>` gets `roleClass("picker")` and holds `<ul role="listbox" id="kiln-picker" aria-label={str.webPicker()}>`, one `<li role="option" id={"pick-" + key} aria-selected>` per highlightable row (`world` rows are `<li role="presentation">` headers in `picker.world`/`picker.world.selected`). A character row is ` Name` (one space in, as the TUI's), in `picker.selected` when highlighted; `+ Character` is indented one space, `picker.add` (or `picker.selected`). A click on a row does what Enter does (Task 5's `choose`); the highlighted row scrolls into view (`scrollIntoView({block: "nearest"})` in an effect).
- New string `web.picker = "Open a connection"` (the list's name for screen readers).
- `Input.tsx`'s submit: `if (!c && !t.value) { store.openPicker(); return; }` (replacing the step-4 comment).
- `main.tsx`: the effect handler opens the picker for `Do` `/open`; a document `keydown` listener: Ctrl+O (no other modifier) toggles the picker (`preventDefault`, so macOS's "open line" doesn't happen); Enter with focus on `body` while a world's overview shows opens it.
- `kiln.css`: rules for the listbox rows (one line each, `white-space: pre`, ellipsis like `.name`).

- [ ] **Step 4: Run** — `cd web/ui && npm test && npm run check` → PASS; `go generate ./internal/str && go test ./internal/str` → PASS.

- [ ] **Step 5: Commit** — `web/ui: + Connection, Ctrl+O, /open and Enter with nothing open show the picker` with `Strings: web.picker (new)`.

---

### Task 5: The filter in the bottom pane

**Files:**
- Create: `web/ui/src/ui/PickerForm.tsx`
- Modify: `web/ui/src/ui/BottomPane.tsx`, `web/ui/src/kiln.css`
- Test: `web/ui/src/ui/App.test.tsx`, `web/ui/src/picker.test.ts`

**Interfaces:**
- Consumes: the store's picker methods (Task 4), `pagePick`/`movePick` (Task 3), the `pick` action (Task 2).
- Produces: `choose(store, sel)` in `picker.ts`: a character's key → `act({Do: "pick", Key})` and `closePicker()`; `+world` → `setPicker({adding: world, name: "", error: undefined})`.

The filter is the TUI's one-field form, one row: `Filter: [input] · Enter to connect · Esc to close` (label `form.label`, hint `form.hint`). The input is a combobox: `role="combobox"`, `aria-controls="kiln-picker"`, `aria-expanded="true"`, `aria-activedescendant={"pick-" + sel}`, `aria-autocomplete="list"`, `autocomplete="off"`. Keys: ↑/↓ `movePick`, PgUp/PgDn `pagePick` by the sidebar's visible rows less one (`Math.max(1, Math.floor(nav.clientHeight / lineHeight) - 1)`), Enter `choose`, Esc `closePicker`. Typing `setPicker({filter})`. It mounts focused. The bottom pane, while the picker's open, is in `form` with `rule.form` above (both even on a world's overview, which otherwise has no bottom pane, as the TUI's).

- [ ] **Step 1: Write the failing tests**

`picker.test.ts`:

```ts
test("choosing a character picks it and closes the picker; + Character opens the name form", () => {
  const sent: object[] = [];
  const s = new Store();
  s.set({ act: (o) => sent.push(o) });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "kit", Name: "Kit" }] }] }]);
  s.openPicker();
  choose(s, "+fm");
  assert.equal(s.state.picker?.adding, "fm");
  choose(s, "fm/kit");
  assert.deepEqual(sent, [{ Do: "pick", Key: "fm/kit" }]);
  assert.equal(s.state.picker, undefined);
});
```

`App.test.tsx`:

```ts
test("the picker's filter: the TUI's one row, a combobox for the list, in the form's area", () => {
  const s = new Store();
  s.set({ phase: "running" });
  s.apply([...three, { type: "Worlds", Worlds: [{ ID: "fm", Characters: [{ ID: "mira", Name: "Mira" }] }] }]);
  s.openPicker();
  const html = bottom(render(<App store={s} />));
  assert.ok(html.includes(roleClass("form")) && !html.includes("<textarea"), "the filter takes the input's place");
  assert.match(html, new RegExp(`<label[^>]*>${str.pickerFilter()}: </label>`));
  assert.match(html, /role="combobox"[^>]*aria-activedescendant="pick-fm\/mira"|aria-activedescendant="pick-fm\/mira"[^>]*role="combobox"/);
  assert.ok(html.includes(str.pickerHint()));
});
```

The keys get a smoke check (Task 8); a node test can't press keys in a real layout.

Review Focus 4 (typing in the filter never reaches Typing): PickerForm never calls `typing`; the smoke checks `kilnPage.sent` holds no `input` after typing a filter (Task 8).

- [ ] **Step 2: Run** — FAIL.
- [ ] **Step 3: Implement** `choose` in `picker.ts` (it takes the `Store`), `PickerForm.tsx` (the filter form now; Task 6 adds the name form to the same file), and in `BottomPane.tsx`:

```tsx
  if (state.picker) return <div class={`bottom form ${roleClass("form")}`}><PickerForm store={store} state={state} /></div>;
```

  with `kiln.css`: `.bottom.form { border-top-color: var(--role-rule-form-fg); }` and the one-row form (`.pick-row { display: flex; white-space: pre; }`, the input `flex: 1; min-width: 1ch`, styled as the password field: no border, transparent, `font: inherit`, `color: inherit`).
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** — `web/ui: the picker's filter, its keys, and choosing a row`

---

### Task 6: Adding a character

**Files:**
- Modify: `web/ui/src/ui/PickerForm.tsx`
- Test: `web/ui/src/ui/App.test.tsx`, `web/ui/src/picker.test.ts`

**Interfaces:**
- Consumes: `add_character` (`{Do, World, Name}` → `Saved{Op, Key}` or `Problems`), `kiln.check("name", v)`, `pick`, `state.answer` (Task 4).
- Produces: `PickerForm` while `picker.adding` is set: `Name: [input] · Enter to connect · Esc to cancel` (`picker.name`, `picker.add_char_hint`). As you type, `kiln.check("name", value)`'s message (when not "") replaces the hint, in `form.error`, and the input is described by it (`aria-describedby`, `aria-invalid`). Enter sends `add_character`; Esc goes back to the filter (`setPicker({adding: undefined})`), focus with it.
- `answered(store, answer)` in `picker.ts`: an `add_character` answer while adding — `Saved` → `act({Do: "pick", Key})`, `closePicker()`; `Problems` → `setPicker({error: first problem's Msg})`. PickerForm calls it in an effect on `answer.n`.

- [ ] **Step 1: Write the failing tests**

`picker.test.ts`:

```ts
test("a saved character is picked; a refused name says why", () => {
  const sent: object[] = [];
  const s = new Store();
  s.set({ act: (o) => sent.push(o) });
  s.openPicker();
  s.setPicker({ adding: "fm", name: "Ash" });
  answered(s, { n: 1, Op: "add_character", Problems: [{ Field: "name", Msg: "taken" }] });
  assert.equal(s.state.picker?.error, "taken");
  answered(s, { n: 2, Op: "add_character", Key: "fm/Ash" });
  assert.deepEqual(sent, [{ Do: "pick", Key: "fm/Ash" }]);
  assert.equal(s.state.picker, undefined);
});

test("an answer to something else leaves the picker be", () => {
  const s = new Store();
  s.openPicker();
  s.setPicker({ adding: "fm" });
  answered(s, { n: 1, Op: "save_world", Key: "" });
  assert.equal(s.state.picker?.adding, "fm");
});
```

`App.test.tsx`:

```ts
test("adding a character: the TUI's one-row name form; a problem replaces the hint", () => {
  const s = new Store();
  s.set({ phase: "running" });
  s.apply(three);
  s.openPicker();
  s.setPicker({ adding: "fm", name: "A B", error: "no spaces" });
  const html = bottom(render(<App store={s} />));
  assert.match(html, new RegExp(`<label[^>]*>${str.pickerName()}: </label>`));
  assert.match(html, /aria-invalid="true"/);
  assert.ok(html.includes("no spaces") && html.includes(roleClass("form.error")) && !html.includes(str.pickerAddCharHint()));
});
```

- [ ] **Step 2: Run** — FAIL.
- [ ] **Step 3: Implement.** The name's live check runs on input: `setPicker({name: v, error: kiln.check?.("name", v) || undefined})`. Enter with an error showing still sends (Go's save is the check that counts, as spec decision 2 says: a too-early Enter is harmless).
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** — `web/ui: adding a character from the picker, as the TUI's`

---

### Task 7: The scrollback dims behind the picker

**Files:**
- Modify: `web/ui/src/ui/Scrollback.tsx`, `web/ui/src/kiln.css`, `web/ui/src/theme.ts` (only if `scrollback.inactive` needs a variable)
- Test: `web/ui/src/ui/App.test.tsx`

**Interfaces:**
- Produces: while `state.picker` is set, `.scroll` gets `dim` and `roleClass("scrollback.inactive")`; CSS strips the lines' own looks so they wear only the inactive role, as the TUI's `theme.Paint(theme.ScrollbackInactive, ansi.Strip(r))`. The pill isn't dimmed (it's outside `.scroll`, and the TUI paints it after). Links stay links (readable, not inert).

```css
/* Behind the picker or a form the scrollback is a backdrop, as the TUI's:
   every line in scrollback.inactive alone, its server colors and tags
   dropped. */
.dim .line, .dim .line * { color: inherit !important; background: none !important; font-weight: inherit !important; font-style: inherit !important; text-decoration: inherit !important; opacity: 1 !important; }
```

(`theme.ts` draws faint as `opacity:.6` on the role's element, and `scrollback.inactive` is `faint` in the built-in theme: the scroller fades as a whole, and the reset's `opacity: 1` keeps a line that's faint itself from fading twice. A dimmed line's color is the scroller's, which the smoke checks.)

- [ ] **Step 1: Write the failing test**

```ts
test("behind the picker, the scrollback is dimmed in scrollback.inactive; the overview too", () => {
  const s = new Store();
  s.set({ phase: "running" });
  s.apply(three);
  s.openPicker();
  const html = render(<App store={s} />);
  assert.match(html, new RegExp(`class="scroll dim ${roleClass("scrollback.inactive")}`));
  s.closePicker();
  assert.doesNotMatch(render(<App store={s} />), /class="scroll dim/);
});
```

(The overview is inside `.scroll`, so the same class dims it.)

- [ ] **Step 2: Run** — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS. The smoke (Task 8) checks a line's computed color equals the inactive role's while the picker's open.
- [ ] **Step 5: Commit** — `web/ui: the scrollback dims behind the picker, as the TUI's`

---

### Task 8: End to end, and the docs

**Files:**
- Create: `web/smoke/picker_test.go`
- Modify: `docs/web-dev.md`

- [ ] **Step 1: Write the smoke test** (it should pass on arrival if Tasks 1–7 are right; run each check against a mutation, below)

```go
// TestPicker opens connections the way a person does: Ctrl+O, a filter,
// Enter; and adds a character from the picker.
func TestPicker(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Worlds")`, "the worlds")
	input := `document.querySelector(".bottom textarea")`
	filter := `document.querySelector('.bottom input[role="combobox"]')`

	// Enter on an empty input with nothing open shows the picker, focused.
	waitFor(t, ev, `document.activeElement === `+input, "the input focused")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.activeElement === `+filter, "the filter focused")
	// Esc puts focus back.
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `!`+filter+` && document.activeElement === `+input, "focus back in the input")

	// Ctrl+O, a filter, Enter: Second opens, connects and shows, and the
	// input has focus.
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "rawKeyDown", "key": "o", "code": "KeyO", "windowsVirtualKeyCode": 79, "modifiers": 2})
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "o", "code": "KeyO", "windowsVirtualKeyCode": 79, "modifiers": 2})
	waitFor(t, ev, `document.activeElement === `+filter, "Ctrl+O's filter focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "sec"})
	waitFor(t, ev, `document.querySelectorAll('nav [role="option"]').length === 2`, "Second and + Character left")
	if got := ev(`kilnPage.sent.some(a => a.Do === "input")`); got != "false" {
		t.Error("typing a filter went to the chat input")
	}
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Char" && e.Key === "fake/second" && e.State === 2)`, "Second connected")
	waitFor(t, ev, `document.querySelector("header").textContent.includes("Second") && document.activeElement === document.querySelector('.bottom input[type="password"]')`, "Second showing, its password asked")

	// Dimmed behind the picker: a line wears scrollback.inactive's color.
	ev(`document.querySelector("nav .r-sidebar-add, nav button:last-child").click()`) // + Connection
	waitFor(t, ev, `document.querySelector(".scroll.dim")`, "the scrollback dimmed")
	if got := ev(`(() => { const l = document.querySelector(".scroll .line"), s = document.querySelector(".scroll"); return getComputedStyle(l).color === getComputedStyle(s).color; })()`); got != "true" {
		t.Error("a dimmed line isn't in scrollback.inactive's color")
	}

	// + Character: a name with a space is refused as you type; a good one
	// is saved, opened and connected.
	ev(`document.querySelector('nav [role="option"][id="pick-+fake"]').click()`)
	waitFor(t, ev, `document.activeElement === document.querySelector(".bottom input")`, "the name field focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "New Comer"})
	waitFor(t, ev, `document.querySelector('.bottom input[aria-invalid="true"]')`, "a space refused")
	ev(`document.querySelector(".bottom input").select()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "Newcomer"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Char" && e.Name === "Newcomer" && e.State === 2)`, "Newcomer added and connected")
	waitFor(t, ev, `!document.querySelector(".scroll.dim")`, "the picker closed")
}
```

(Selectors are the plan's; match them to what Tasks 4–6 built, and ledger any rename.)

- [ ] **Step 2: Run** — `cd web && KILN_SMOKE=1 go test -count=1 -run TestPicker ./smoke` → PASS. Mutations (each must make it FAIL, then be undone by a reverse edit): Input's Enter not opening the picker; PickerForm not focusing on mount; the `.dim` rule removed; `pick` not switching.

- [ ] **Step 3: Docs.** `docs/web-dev.md`: the page now draws Kiln (not "five areas, empty"); backup and restore are `/backup` and `/restore` (drop "aren't on the page yet"); opening a connection is `+ Connection`, Ctrl+O, or Enter with nothing open; `kilnPage.act` stays for tests.

- [ ] **Step 4: The whole suite** — `go vet ./... && go test ./...`; `(cd web && go vet ./... && go test ./... && GOOS=js GOARCH=wasm go vet ./...)`; `(cd web/ui && npm run check && npm test)`; `web/build.sh`; `(cd web && KILN_SMOKE=1 go test -count=1 ./smoke)` → all PASS.

- [ ] **Step 5: Commit** — `web: the picker end to end; web-dev.md catches up`
