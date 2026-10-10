# Headless core, step 5: the core owns world and character edits and config reload — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reloading the config, and adding, saving and deleting worlds and characters, forgetting a password, the delete warnings and what each form field accepts, live in `app`. The TUI keeps its forms, the picker, editor drafts and parking, and the theme.

**Architecture:**
- **Reload:** `app.Reload()` loads the config with `Deps.Load` and applies it, or says why not on the status line and changes nothing. The TUI's `reloadNow` calls it and then does the screen's part (closed characters, looks, repaint, the picker), as `applyConfig` does now.
- **Edits:** `app` methods write the config (`AddWorld`, `SaveWorld`, `SaveCharacter`, `AddCharacter`, `DeleteWorld`, `DeleteCharacter`, `ForgetPassword`) and return errors for the form. They don't reload: the front end reloads after a successful write and then says what happened, so its message lands after anything the reload said, as now.
- **Field rules:** what each text field accepts while typing (`AcceptWorldID`, `AcceptHost`, `AcceptPort`, `AcceptPacks`, `AcceptByteCount`, `AcceptAliases`), `ParsePort`, and `List` move to `app` as plain functions. The TUI's form fields point their `accept` at them.

**Tech Stack:** Go; `internal/app`, `internal/ui`, `config`, `str`.

**Spec:** `docs/superpowers/specs/2026-10-07-headless-core-design.md`, sections "Worlds and characters" and "Config".

## Global Constraints

- Nothing visible changes: golden screens are NOT updated (never `-update`).
- Every existing test passes. Tests may change how they reach state or set it up, never what they expect.
- `internal/app` doesn't depend on Bubble Tea or lipgloss and never paints.
- Strings come from `internal/str`; this step adds none (`EditorPortRange`, `EditorWorldIdChars`, `EditorHostSpaces`, `EditorPortNumber`, `EditorPacksList`, `EditorBytesNumber`, `EditorDeleteWorldWarning`, `EditorDeleteCharacterWarning`, `StatusConfigNotReloaded` move with their code).
- `ui.Deps` doesn't change: `ui.New` passes `Load` and `DeletePassword` on to `app.Deps`.
- Run `go vet ./... && go test ./...`, and the web module's tests (`cd web && go test ./... && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...`), before each commit.

## Rulings carried from the spec

- **Errors, not field errors.** The spec has the edit methods "return field errors by field". The TUI shows one message per form (`form.reject`), and the web form's shape is part 3's design, which Indi wants to be part of. So the methods return a plain `error` now. Per-field errors are additive: a typed `FieldError{Field, Err}` that callers find with `errors.As`, so the signatures and the TUI's `err.Error()` stay as they are. The work then is tagging the few field errors where they arise (`ParsePort`; `config.AddWorld`'s id and host checks; `config.AddCharacter`'s name check) once part 3 decides how fields are named.
- **Edit statuses stay with the front end.** "Saved X", "Deleted X", "Forgot the password for X" and "deleted, but the password was kept" are set after the reload, so they win over anything the reload said (a character's rules not compiling). That order belongs to whoever reloads, so the messages stay in the TUI. `DeleteCharacter` returns the password error separately for the same reason.
- **The world form's parsing stays in the TUI** (`worldSettings`: reading fields into `config.WorldSettings`), except `ParsePort`, which is the rule a web form needs too.

## What the old code did as a side effect (keep all of it)

- `reloadNow`: on a load error, the status says so and returns false; on success, `applyConfig` then `loadTheme` (a broken theme doesn't stop the config).
- `applyConfig`: snapshots the open characters' configs and the active item *before* applying; closes what the core closed (`closed(k, prev)`, then `activated(prev)`); for each previously open character in sidebar order: export settings for log mode (even orphans), skip orphans, show a rules error and skip, recompile the look, repaint only when `styleInputs` changed; finally `fixPick` if the picker is open.
- `saveNewWorld`: rejects the port range before writing; `AddWorld` then `WriteWorld`, deleting the half-made world if `WriteWorld` fails; on success closes the editor, clears the picker's filter, reloads (result ignored), selects the new world's add row, `fixPick`.
- `saveWorld` / `saveCharSettings`: on a write error the editor stays with the message; on success close the editor, reload, and only if the reload worked say "Saved".
- `saveCharacter`: on success closes the editor (`picker.edit = nil`), reloads, and only if that worked picks (opens and connects) the new character.
- `forgetPassword`: does nothing at all (editor stays) when passwords can't be forgotten; on an error the editor stays with it; on success says so, then closes the editor.
- `deleteEdited`, character: config delete; on success close the editor *before* closing the character (or closing would keep it as a draft), close the character if open, then delete its password, saying so if that fails. World: config delete; on success drop the TUI's drafts for that world. Both: on a config error, disarm the button and show the error in the editor; on success close the editor, remember the status, reload, then say "Deleted" only if the reload worked and the remembered status wasn't an error, otherwise put the remembered error back; `fixPick` if the picker is still open.
- `deleteWarning`: world → `EditorDeleteWorldWarning(world)`; character → `EditorDeleteCharacterWarning(world/char)`.

## Review Focus

1. **Deleting the active character from its `/edit` editor.** The editor closes before the screen's part of closing the character, so it isn't kept as a draft. A parked editor for it becomes a draft. The next character down becomes active. Existing `editor_test.go` delete tests cover most of it; the reviewer should trace the order against `main`.
2. **The status after a delete.** A password that couldn't be deleted stays on the status line after the reload, instead of "Deleted". Task 2's `TestDeleteCharacterClosesAndForgetsThePassword` pins the core's half; the TUI's half is unchanged code.
3. **A half-made world is taken back** when writing its settings fails after `config.AddWorld` succeeded. No test can easily make `WriteWorld` fail; the code moves verbatim.
4. **Reload with a broken config** changes nothing (characters, order, active item) and says why. Task 1's `TestReloadKeepsTheConfigOnAnError` pins it.
5. **Typing into a form field** is filtered exactly as before: the `Accept*` functions accept every prefix of a valid value (the empty string included), since they run on each keystroke. Task 2's `TestAcceptFilters` pins it.

---

### Task 1: `app.Reload`

**Files:**
- Create: `internal/app/reload_test.go`
- Modify: `internal/app/app.go` (`Deps.Load`, `Reload`), `internal/ui/model.go` (`New`, `reloadNow`, `applyConfig`)

**Interfaces:**
- Produces: `app.Deps.Load func(dir string) (*config.Config, error)`; `func (a *App) Reload() (ConfigResult, bool)`. `ui`'s `applyConfig(cfg)` becomes `applyWith(apply func() (app.ConfigResult, bool)) bool`.

- [ ] **Step 1: Write the tests** — `internal/app/reload_test.go`:

```go
package app

import (
	"testing"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

func reloadApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := configDir(t, map[string]string{"fm": fmWorld})
	a := New(Deps{ConfigDir: dir, Load: config.Load})
	a.ApplyConfig(load(t, dir))
	return a, dir
}

func TestReloadAppliesTheConfigAgain(t *testing.T) {
	a, dir := reloadApp(t)
	writeWorlds(t, dir, map[string]string{"zz": zzWorld})
	if _, ok := a.Reload(); !ok {
		t.Fatalf("Reload failed: %q", a.Status().Text)
	}
	if _, ok := a.Find("zz/ash"); !ok {
		t.Error("the new world's character isn't configured")
	}
}

func TestReloadKeepsTheConfigOnAnError(t *testing.T) {
	a, dir := reloadApp(t)
	openAll(t, a, "fm/kit")
	cfg := a.Config()
	writeWorlds(t, dir, map[string]string{"fm": "host = "})
	_, err := config.Load(dir)
	if _, ok := a.Reload(); ok {
		t.Fatal("Reload of a broken config succeeded")
	}
	if a.Config() != cfg || a.Char("fm/kit") == nil || a.Active() != "fm/kit" {
		t.Error("a broken config changed what's open")
	}
	if s := a.Status(); !s.Err || s.Text != str.StatusConfigNotReloaded(err) {
		t.Errorf("status = %+v", s)
	}
}
```

- [ ] **Step 2: Run them; they fail to compile**

Run: `go test ./internal/app/`
Expected: FAIL — `unknown field Load in struct literal`, `a.Reload undefined`.

- [ ] **Step 3: Implement** — in `internal/app/app.go`, add to `Deps` (after `ConfigDir`):

```go
	Load         func(dir string) (*config.Config, error)       // reads the config; Reload uses it
```

and after `ApplyConfig`:

```go
// Reload reads the config again and applies it. If it can't be read, it
// says why on the status line, changes nothing, and reports false.
func (a *App) Reload() (ConfigResult, bool) {
	cfg, err := a.d.Load(a.d.ConfigDir)
	if err != nil {
		a.SetStatus(true, str.StatusConfigNotReloaded(err))
		return ConfigResult{}, false
	}
	return a.ApplyConfig(cfg), true
}
```

(add the `str` import if `app.go` lacks it).

- [ ] **Step 4: The TUI goes through it** — in `internal/ui/model.go`:
  - `New`: pass `Load: d.Load` in `app.Deps`. Replace `m.applyConfig(cfg)` with `m.applyWith(func() (app.ConfigResult, bool) { return m.a.ApplyConfig(cfg), true })`.
  - `reloadNow` becomes:

    ```go
    // reloadNow loads the config and theme and applies them, reporting whether
    // the config could be.
    func (m *Model) reloadNow() bool {
    	if !m.applyWith(m.a.Reload) {
    		return false // Reload said why
    	}
    	m.loadTheme() // a broken theme doesn't stop the config; see themeErr
    	return true
    }
    ```
  - `applyConfig(cfg)` becomes `applyWith(apply func() (app.ConfigResult, bool)) bool`: same body, but `res, ok := apply()` replaces `res := m.a.ApplyConfig(cfg)`, followed by `if !ok { return false }`; `b.setExport(cfg)` becomes `b.setExport(m.a.Config())`; it ends `return true`. Update its doc comment: "applyWith hands a config to the core through apply, then updates the open characters' views …; false if apply changed nothing."

- [ ] **Step 5: Run everything** (see Global Constraints). Expected: PASS, golden screens untouched.

- [ ] **Step 6: Commit**

```bash
git add internal/app internal/ui
git commit -m "app: Reload reads and applies the config"
```

---

### Task 2: world and character edits in `app`

**Files:**
- Create: `internal/app/edit.go`, `internal/app/edit_test.go`
- Modify: `internal/app/app.go` (`Deps.DeletePassword`), `internal/ui/editor.go`, `internal/ui/picker.go` (`saveCharacter`), `internal/ui/model.go` (`New`)
- Modify (access paths only): any `ui` test hit by `grep -rnE 'onlyMatching|worldIDChar|portChar|numChar|packsChar|\blist\(' internal/ui/*_test.go`

**Interfaces:**
- Consumes: Task 1's `Reload` (through `ui.reloadNow`).
- Produces:
  - `app.Deps.DeletePassword func(store, world, char string) error` (nil: passwords can't be forgotten)
  - `func AcceptWorldID(s string) error`, `AcceptHost`, `AcceptPort`, `AcceptPacks`, `AcceptByteCount`, `AcceptAliases` — what a field takes as it's typed
  - `func ParsePort(s string) (int, error)`, `func List(s string) []string`
  - `func DeleteWarning(world, char string) string` (char "" for the world)
  - `func (a *App) AddWorld(id string, s config.WorldSettings) error`
  - `func (a *App) SaveWorld(world string, s config.WorldSettings) error`
  - `func (a *App) SaveCharacter(world, char string, s config.CharacterSettings) error`
  - `func (a *App) AddCharacter(world, name string) (id string, err error)`
  - `func (a *App) DeleteWorld(world string) error`
  - `func (a *App) DeleteCharacter(world, char string) (pwErr, err error)`
  - `func (a *App) CanForgetPasswords() bool`, `func (a *App) ForgetPassword(world, char string) error`

- [ ] **Step 1: Write the tests** — `internal/app/edit_test.go`:

```go
package app

import (
	"errors"
	"slices"
	"testing"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

// Each runs on every keystroke, so every prefix of a good value passes.
func TestAcceptFilters(t *testing.T) {
	for _, c := range []struct {
		name   string
		accept func(string) error
		good   []string
		bad    []string
	}{
		{"world id", AcceptWorldID, []string{"", "fm", "fm-2_b"}, []string{"f m", "fm!"}},
		{"host", AcceptHost, []string{"", "muck.test"}, []string{"a b", "é.test"}},
		{"port", AcceptPort, []string{"", "8", "65535", "99999"}, []string{"123456", "8a"}},
		{"packs", AcceptPacks, []string{"", "fuzzball", "fuzzball, x y"}, []string{"a;b"}},
		{"bytes", AcceptByteCount, []string{"", "4096"}, []string{"x", "1234567890"}},
		{"aliases", AcceptAliases, []string{"", "Kitty", "Kitty, K"}, []string{"#Kit", "K=t"}},
	} {
		for _, s := range c.good {
			if err := c.accept(s); err != nil {
				t.Errorf("%s: %q rejected: %v", c.name, s, err)
			}
		}
		for _, s := range c.bad {
			if c.accept(s) == nil {
				t.Errorf("%s: %q accepted", c.name, s)
			}
		}
	}
}

func TestParsePort(t *testing.T) {
	if p, err := ParsePort("8888"); p != 8888 || err != nil {
		t.Errorf("8888: %d, %v", p, err)
	}
	for _, s := range []string{"", "0", "65536"} {
		if _, err := ParsePort(s); err == nil || err.Error() != str.EditorPortRange() {
			t.Errorf("%q: %v", s, err)
		}
	}
}

func TestDeleteWarning(t *testing.T) {
	if w := DeleteWarning("fm", ""); w != str.EditorDeleteWorldWarning("fm") {
		t.Errorf("world: %q", w)
	}
	if w := DeleteWarning("fm", "kit"); w != str.EditorDeleteCharacterWarning("fm/kit") {
		t.Errorf("character: %q", w)
	}
}

func TestAddWorldWritesIt(t *testing.T) {
	a, _ := reloadApp(t)
	if err := a.AddWorld("zz", config.WorldSettings{Host: "zz.test", Port: 7777, TLS: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	if !slices.ContainsFunc(a.Config().Worlds, func(w config.World) bool { return w.ID == "zz" }) {
		t.Error("zz isn't in the config")
	}
	if err := a.AddWorld("z z", config.WorldSettings{Host: "zz.test", Port: 7777}); err == nil {
		t.Error("a bad world id was written")
	}
}

func TestDeleteCharacterClosesAndForgetsThePassword(t *testing.T) {
	a, _ := reloadApp(t)
	var forgot []string
	pwErr := error(nil)
	a.d.DeletePassword = func(store, world, char string) error {
		forgot = append(forgot, world+"/"+char)
		return pwErr
	}
	openAll(t, a, "fm/kit", "fm/rook")
	if perr, err := a.DeleteCharacter("fm", "kit"); perr != nil || err != nil {
		t.Fatalf("DeleteCharacter: %v, %v", perr, err)
	}
	if a.Char("fm/kit") != nil || !slices.Equal(forgot, []string{"fm/kit"}) {
		t.Errorf("open %v, forgot %q", a.Char("fm/kit") != nil, forgot)
	}
	pwErr = errors.New("locked")
	if perr, err := a.DeleteCharacter("fm", "rook"); perr != pwErr || err != nil {
		t.Errorf("a kept password: %v, %v", perr, err)
	}
	if _, err := a.DeleteCharacter("fm", "nobody"); err == nil {
		t.Error("deleting a missing character succeeded")
	}
}

func TestForgetPassword(t *testing.T) {
	a, _ := reloadApp(t)
	if a.CanForgetPasswords() {
		t.Error("can forget with no DeletePassword")
	}
	a.d.DeletePassword = func(string, string, string) error { return nil }
	if !a.CanForgetPasswords() || a.ForgetPassword("fm", "kit") != nil {
		t.Error("ForgetPassword failed")
	}
}
```

Before relying on the `bad` aliases, check `config.NameChars` (`internal/config/write.go:68`): it rejects a leading `!*#$` and any `=`, `&`, `|` or space inside a name.

- [ ] **Step 2: Run them; they fail to compile**

Run: `go test ./internal/app/`
Expected: FAIL — `undefined: AcceptWorldID`, etc.

- [ ] **Step 3: Write `internal/app/edit.go`** — moved from `ui/editor.go` and `ui/picker.go`:

```go
package app

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

var (
	worldIDChar = regexp.MustCompile(`^[A-Za-z0-9_-]*$`) //str:ok
	portChar    = regexp.MustCompile(`^[0-9]{0,5}$`)
	numChar     = regexp.MustCompile(`^[0-9]{0,9}$`)
	packsChar   = regexp.MustCompile(`^[A-Za-z0-9_, -]*$`) //str:ok
)

// onlyMatching accepts what re matches, and otherwise says why.
func onlyMatching(re *regexp.Regexp, why string) func(string) error {
	return func(s string) error {
		if !re.MatchString(s) {
			return errors.New(why)
		}
		return nil
	}
}

// What a form field takes as it's typed: each is checked on every
// keystroke, so every start of a good value passes.
var (
	AcceptWorldID   = onlyMatching(worldIDChar, str.EditorWorldIdChars())
	AcceptPort      = onlyMatching(portChar, str.EditorPortNumber())
	AcceptPacks     = onlyMatching(packsChar, str.EditorPacksList())
	AcceptByteCount = onlyMatching(numChar, str.EditorBytesNumber())
)

// AcceptHost takes a host name: no spaces or characters outside ASCII.
func AcceptHost(s string) error {
	if strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return errors.New(str.EditorHostSpaces())
	}
	return nil
}

// AcceptAliases takes a list of names a character also goes by.
func AcceptAliases(s string) error {
	for _, a := range List(s) {
		if err := config.NameChars(a); err != nil {
			return err
		}
	}
	return nil
}

// List splits a comma- or space-separated field into its items.
func List(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

// ParsePort reads a port number, 1 to 65535.
func ParsePort(s string) (int, error) {
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New(str.EditorPortRange())
	}
	return port, nil
}

// DeleteWarning says what deleting world (char "") or its character char
// will do.
func DeleteWarning(world, char string) string {
	if char == "" {
		return str.EditorDeleteWorldWarning(world)
	}
	return str.EditorDeleteCharacterWarning(world + "/" + char)
}

// AddWorld writes a new world with settings s. If its settings can't be
// written, the half-made world is taken back. Nothing reloads: the front
// end does, as after every edit.
func (a *App) AddWorld(id string, s config.WorldSettings) error {
	if err := config.AddWorld(a.d.ConfigDir, id, s.Host, s.Port, s.TLS); err != nil {
		return err
	}
	if err := config.WriteWorld(a.d.ConfigDir, id, s); err != nil {
		config.DeleteWorld(a.d.ConfigDir, id)
		return err
	}
	return nil
}

// SaveWorld writes world's settings.
func (a *App) SaveWorld(world string, s config.WorldSettings) error {
	return config.WriteWorld(a.d.ConfigDir, world, s)
}

// SaveCharacter writes the settings of world's character char.
func (a *App) SaveCharacter(world, char string, s config.CharacterSettings) error {
	return config.WriteCharacter(a.d.ConfigDir, world, char, s)
}

// AddCharacter writes a new character called name to world and returns
// its id.
func (a *App) AddCharacter(world, name string) (string, error) {
	return config.AddCharacter(a.d.ConfigDir, world, name)
}

// DeleteWorld deletes a world with no characters. Logs are never touched.
func (a *App) DeleteWorld(world string) error {
	return config.DeleteWorld(a.d.ConfigDir, world)
}

// DeleteCharacter deletes world's character char from the config, closing
// it first if it's open, then its saved password. Logs are never touched.
// err is the config's; pwErr is a password that couldn't be deleted, for
// the front end to say after it reloads.
func (a *App) DeleteCharacter(world, char string) (pwErr, err error) {
	if err := config.DeleteCharacter(a.d.ConfigDir, world, char); err != nil {
		return nil, err
	}
	a.Close(Key(world, char))
	if a.d.DeletePassword != nil {
		pwErr = a.d.DeletePassword(a.PasswordStore(), world, char)
	}
	return pwErr, nil
}

// CanForgetPasswords reports whether saved passwords can be deleted here.
func (a *App) CanForgetPasswords() bool { return a.d.DeletePassword != nil }

// ForgetPassword deletes world's character char's saved password.
func (a *App) ForgetPassword(world, char string) error {
	return a.d.DeletePassword(a.PasswordStore(), world, char)
}
```

In `app.go`'s `Deps`, add after `Password`:

```go
	DeletePassword func(store, world, char string) error // nil: passwords can't be forgotten
```

Run: `go test ./internal/app/` — Expected: PASS. Then run `go test ./internal/str` too: the moved regexps keep their `//str:ok` marks.

- [ ] **Step 4: The TUI calls the core** — `internal/ui/model.go` `New`: pass `DeletePassword: d.DeletePassword` in `app.Deps`. In `internal/ui/editor.go`:
  - Delete `worldIDChar`, `portChar`, `numChar`, `packsChar`, `onlyMatching`, `list`; drop imports that are no longer used (`regexp`, maybe `errors`, `strconv`).
  - `worldForm`: `world.accept = app.AcceptWorldID`, `host.accept = app.AcceptHost`, `port.accept = app.AcceptPort`, `packs.accept = app.AcceptPacks`, `maxBytes.accept = app.AcceptByteCount`.
  - `charForm`: `aliases.accept = app.AcceptAliases`; `saveCharSettings` uses `app.List`.
  - `worldSettings`: `port, err := app.ParsePort(f.value(f.field(portLabel))); if err != nil { return config.WorldSettings{}, err }`; `Use: app.List(…)`.
  - `deleteWarning(e)`: `char := ""; if e.kind == editChar { char = e.char }; return app.DeleteWarning(e.world, char)`.
  - `saveNewWorld`: replace the `config.AddWorld` / `config.WriteWorld` / rollback block with `if err := m.a.AddWorld(id, s); err != nil { f.reject = err.Error(); return }`. The rest stays.
  - `saveWorld`: `err = m.a.SaveWorld(e.world, s)`.
  - `saveCharSettings`: `m.a.SaveCharacter(e.world, e.char, s)`.
  - `forgetPassword`: `if !m.a.CanForgetPasswords() { return }`; `if err := m.a.ForgetPassword(e.world, e.char); err != nil { … }`. The status and `closeEditor` stay as they are.
  - `deleteEdited`, character branch:

    ```go
		what = e.world + "/" + e.char
		k := key(e.world, e.char)
		prev := m.a.Active()
		var perr error
		perr, err = m.a.DeleteCharacter(e.world, e.char)
		if err == nil {
			m.closeEditor() // before the screen's part of closing, which would keep it as a draft
			if m.chars[k] != nil {
				m.closed(k, prev)
				m.activated(prev)
			}
			if perr != nil {
				m.setStatus(true, str.EditorDeletedPasswordKept(what, perr))
			}
		}
    ```
    World branch: `err = m.a.DeleteWorld(e.world)`; the drafts loop stays. Everything after stays.
  - `internal/ui/picker.go` `saveCharacter`: `id, err := m.a.AddCharacter(e.world, e.form.value(0))`.

- [ ] **Step 5: Tests reach the core** — run the grep from **Files**; change access paths only (`list(` → `app.List(`, etc.).

- [ ] **Step 6: Run everything** (see Global Constraints, web module included). Expected: PASS, golden screens untouched (`git status internal/ui/testdata` clean).

- [ ] **Step 7: Commit**

```bash
git add internal/app internal/ui
git commit -m "app: world and character edits and the fields' rules live in the core"
```

Then: fresh-reviewer pass over the branch against `main` with this plan's Review Focus and side-effect list, fix Critical/Important with a failing test first, open the PR (no `Strings:` trailer: no catalog changes) and stop.
