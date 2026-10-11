# Web API, step 1b: "Enter again" keyed to the text; settings errors name their field — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The over-limit confirmation remembers the text it refused instead of being reset by every keystroke. Settings are checked when saved, every bad field at once, and each error says which field it's about.

**Architecture:**
- **Confirmation.** `App.confirm` becomes the refused text (`""`: none). `Submit` sends an over-limit line when the text equals it. Switching and closing still clear it, and so does Esc (`Unconfirm`, now called only from Esc). Keystrokes and pastes no longer call `Unconfirm`. `OverLimit(text string, ch config.Character) bool` is exported for front ends that mark the overflow live.
- **Field errors.** `config.FieldError{Field, Msg}` (aliased as `app.FieldError`). `config.CheckWorld(dir, s)` and `config.CheckCharacter(s)` return every problem with a setting joined with `errors.Join`; `WriteWorld` and `WriteCharacter` call them before anything else. `config.AddWorld` and `config.AddCharacter` return `FieldError`s for the id, host, port and name. `app.AddWorld` checks the settings and the id together before writing anything. `app.Problems(err)` flattens a joined error into its parts, in order.
- **The TUI shows the first problem**, where it shows one now. Its port field is parsed with `strconv.Atoi`; the range check is the core's. `app.ParsePort` and the `editor.port_range` string go.

**Tech Stack:** Go; `internal/config`, `internal/app`, `internal/ui`, `internal/str`.

**Spec:** `docs/superpowers/specs/2026-10-11-web-api-design.md`, "For you to check" items 1, 3 and 4, and "What changes in the core": "The over-limit confirmation" and "Settings errors".

## Global Constraints

- Golden screens are NOT updated (never `-update`).
- Existing tests change only how they reach state, except the over-limit tests that relied on an edit clearing the confirmation (spec, "Testing").
- Field names are config keys (`host`, `port`, `use`, `max_line_bytes`, `newline_mode`, `notify`, `aliases`), plus `id` for a new world's id and `name` for a new character's name. They're vocabulary, never translated: mark the literals `//str:ok`.
- Messages come from `internal/str`. Reuse the `config.*` entries that exist; this step adds none and removes `editor.port_range`. Its commit ends with `Strings: editor.port_range (removed)`.
- Run `go generate ./internal/str` after editing `en.toml`, then `go vet ./... && go test ./...` and the web module's tests (`cd web && go test ./... && GOOS=js GOARCH=wasm go test -exec "$PWD/testdata/memfs_exec.sh" ./...`) before each commit. No `-race` (`TestBrowseFindIsFastOnLargeHistories` flakes under it on `main`).
- End each commit message with the session's attribution lines.

## What the old code did as a side effect (keep all of it)

Confirmation:
- `Submit`: an over-limit line (by `overLimit`, with flatten joining lines) when not confirmed sets the confirmation and `StatusOverLimit(max_line_bytes)` and sends nothing; when confirmed it clears the confirmation and sends.
- `Open`, `Close` and `Switch` clear it (`app.go:269`, `295`, `342`).
- The TUI cleared it on every editing key that changed something (`model.go:771`), on paste into the input (`543`), and on Esc (`746`). Esc stays.
- Commands, the password prompt, and a line under the limit don't touch it except that sending clears it.

Settings:
- `saveNewWorld`: `ParsePort` (its message is `editor.port_range`, the same text as `config.port_range`) ran before anything else; then `AddWorld` checked the id's characters, the host, the port and whether the world exists, in that order; then `WriteWorld`, whose failure deletes the half-made world. The first failure went to `f.reject` and the editor stayed open.
- `saveWorld`/`saveCharSettings`/`saveCharacter`: the first error to `reject`, editor stays open.
- `WriteWorld`/`WriteCharacter` load the edited file before replacing it (`editWorld`), and a file that doesn't load isn't written. That stays as the backstop.

## Review Focus

1. **One message in the TUI.** A joined error's `Error()` has newlines; every `reject = …` for a save must show only the first problem, or a form grows a row. Task 2's `TestEditorShowsTheFirstProblem` pins it.
2. **Esc still drops the confirmation**, and typing then deleting a character doesn't. Task 1's `TestOverLimitConfirmIsKeyedToTheText` (core) and `TestEscDropsTheOverLimitConfirm` (TUI) pin them.
3. **A new world isn't half-made by a bad setting.** `app.AddWorld` must check before `config.AddWorld` creates the file; the existing `TestAddWorldTakesBackAHalfMadeWorld` still covers a setting only the load check catches (`tls_trust`).
4. **An unchanged bad value.** `WriteWorld` checks the whole `WorldSettings`, so a world whose file already has, say, `notify = "bogus"` can't be saved until that's fixed, even if the edit is elsewhere. That matches the load check, which already refused such a file. The reviewer should confirm no valid file is refused: every check must accept whatever `loadWorldData` accepts.
5. **`errors.As` through a join.** `Problems` must flatten nested joins (`errors.Join(errors.Join(a, b), c)`) in order. Task 2's `TestProblemsFlattens` pins it.

---

### Task 1: the confirmation remembers the refused text

**Files:**
- Modify: `internal/app/app.go`, `internal/app/submit.go`, `internal/app/submit_test.go`
- Modify: `internal/ui/model.go`, `internal/ui/model_test.go`

**Interfaces:**
- Produces: `App.confirm string`; `App.Unconfirm()` (Esc only); `App.Confirming() bool`; `func OverLimit(text string, ch config.Character) bool`.

- [ ] **Step 1: Core tests**

In `internal/app/submit_test.go`, replace `TestOverLimitAsksFirst` with:

```go
func TestOverLimitConfirmIsKeyedToTheText(t *testing.T) {
	a, c, _ := submitApp(t, strings.Replace(fmWorld, "tls = true\n", "tls = true\nmax_line_bytes = 5\n", 1))
	a.SetInput("too long")
	if res, _ := a.Submit(); res.Sent || !a.Confirming() || a.Status().Text != str.StatusOverLimit(5) {
		t.Fatalf("first Enter: %+v confirming %v status %q", res, a.Confirming(), a.Status().Text)
	}
	a.SetInput("too long!") // an edit
	if res, _ := a.Submit(); res.Sent {
		t.Error("a changed line should ask again")
	}
	a.SetInput("too long") // typed and taken back: what was refused
	a.SetInput("too long!")
	if res, _ := a.Submit(); !res.Sent || !slices.Equal(c.Sent(), []string{"too long!"}) {
		t.Errorf("Enter on the refused text: %+v sent %q", res, c.Sent())
	}
	a.SetInput("still too long")
	a.Submit()
	a.Unconfirm() // Esc
	if res, _ := a.Submit(); res.Sent {
		t.Error("Esc should drop the confirmation")
	}
}

func TestOverLimitIsExported(t *testing.T) {
	ch := config.Character{MaxLineBytes: 5, NewlineMode: "batch"}
	if OverLimit("12345", ch) || !OverLimit("123456", ch) || OverLimit("123\n456", ch) {
		t.Error("batch: each line on its own")
	}
	ch.NewlineMode = "flatten"
	if !OverLimit("123\n45", ch) {
		t.Error("flatten: joined with a space")
	}
}
```

(Add `"github.com/latrani/Kiln/internal/config"` to the test file's imports if it isn't there.)

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app -run 'OverLimit'`
Expected: `OverLimit` undefined (build failure).

- [ ] **Step 3: Implement**

In `internal/app/app.go`, the field becomes:

```go
	confirm      string                 // an over-limit line Submit refused; Enter on the same text sends it
```

and the three `a.confirm = false` assignments in `Open`, `Close` and `Switch` (lines ~269, ~295, ~342, written as `a.active, a.confirm = k, false` / `"", false`) become `a.active, a.confirm = k, ""` / `"", ""`.

In `internal/app/submit.go`:

```go
// Unconfirm drops the confirmation, as Esc does: the next Submit of an
// over-limit line asks again.
func (a *App) Unconfirm() { a.confirm = "" }

// Confirming reports whether an over-limit line is waiting for Enter
// again.
func (a *App) Confirming() bool { return a.confirm != "" }
```

In `Submit`, replace

```go
	flatten := c.Ch.NewlineMode == "flatten"
	if overLimit(text, c.Ch.MaxLineBytes, flatten) && !a.confirm {
		a.confirm = true
		a.SetStatus(true, str.StatusOverLimit(c.Ch.MaxLineBytes))
		return SubmitResult{}, nil
	}
	a.confirm = false
```

with

```go
	flatten := c.Ch.NewlineMode == "flatten"
	if OverLimit(text, c.Ch) && text != a.confirm {
		a.confirm = text
		a.SetStatus(true, str.StatusOverLimit(c.Ch.MaxLineBytes))
		return SubmitResult{}, nil
	}
	a.confirm = ""
```

and add, next to `overLimit`:

```go
// OverLimit reports whether text is too long to send as ch: a line over
// max_line_bytes, or with newline_mode "flatten" the lines joined.
func OverLimit(text string, ch config.Character) bool {
	return overLimit(text, ch.MaxLineBytes, ch.NewlineMode == "flatten")
}
```

In `internal/ui/model.go`, delete the `m.a.Unconfirm()` after `m.input().InsertText(msg.Content)` (paste, ~543) and the one at the end of the input's key handling (~771). Keep the one under `case "esc":` (~746).

- [ ] **Step 4: TUI test**

Add to `internal/ui/model_test.go`, after `TestOverLimitNeedsConfirm`:

```go
func TestEscDropsTheOverLimitConfirm(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld}) // max_line_bytes = 20
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("this line is definitely too long")
	h.enter()
	h.typeText("x")
	h.key("backspace") // typed and taken back
	if !h.m.a.Confirming() {
		t.Fatal("an edit that changes nothing in the end should keep the confirmation")
	}
	h.key("esc")
	h.enter()
	if n := len(h.conn("fm/kit").Sent()); n != 1 {
		t.Errorf("after Esc, Enter sent; %d lines", n)
	}
}
```

- [ ] **Step 5: Run everything** (Global Constraints). Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app internal/ui
git commit -m "app: Enter again sends the over-limit line it refused, unless it changed

Edits no longer clear the confirmation; Esc, switching and closing do.
OverLimit is exported for front ends that mark the overflow."
```

---

### Task 2: settings are checked on save, and errors name their field

**Files:**
- Create: `internal/config/check.go`, `internal/config/check_test.go`
- Modify: `internal/config/edit.go` (`WriteWorld`, `WriteCharacter`), `internal/config/write.go` (`AddWorld`, `AddCharacter`)
- Modify: `internal/app/edit.go`, `internal/app/edit_test.go`
- Modify: `internal/ui/editor.go`, `internal/ui/picker.go`, `internal/ui/editor_test.go`
- Modify: `internal/str/locales/en.toml` (remove `editor.port_range`), regenerate

**Interfaces:**
- Produces (in `config`): `type FieldError struct{ Field, Msg string }` with `Error() string`; `CheckWorld(dir string, s WorldSettings) error`; `CheckCharacter(s CharacterSettings) error`; `CheckNewWorld(dir, id string) error`.
- Produces (in `app`): `type FieldError = config.FieldError`; `func Problems(err error) []error`.
- Removed: `app.ParsePort`, `str.EditorPortRange`.

- [ ] **Step 1: Config tests** — `internal/config/check_test.go`:

```go
package config

import (
	"errors"
	"slices"
	"testing"
)

// fields lists the fields err's FieldErrors name, in order.
func fields(err error) []string {
	var out []string
	var walk func(error)
	walk = func(e error) {
		if j, ok := e.(interface{ Unwrap() []error }); ok {
			for _, x := range j.Unwrap() {
				walk(x)
			}
			return
		}
		var fe *FieldError
		if errors.As(e, &fe) {
			out = append(out, fe.Field)
		}
	}
	if err != nil {
		walk(err)
	}
	return out
}

func TestCheckWorldNamesEveryBadField(t *testing.T) {
	dir := editDir(t, uglyWorld)
	zero, bogus, sideways := 0, "bogus", "sideways"
	s := WorldSettings{Host: "a b", Port: 0, Use: []string{"fuzzball", "no-such", "bad id"},
		MaxLineBytes: &zero, NewlineMode: &sideways, Notify: &bogus}
	want := []string{"port", "host", "use", "use", "max_line_bytes", "newline_mode", "notify"}
	if got := fields(CheckWorld(dir, s)); !slices.Equal(got, want) {
		t.Errorf("fields %q, want %q", got, want)
	}
	good, _, _ := ReadWorld(dir, "fm")
	if err := CheckWorld(dir, good); err != nil {
		t.Errorf("a world as read is refused: %v", err)
	}
}

func TestCheckCharacter(t *testing.T) {
	bogus := "bogus"
	if got := fields(CheckCharacter(CharacterSettings{Aliases: []string{"Kitty", "#K"}, Notify: &bogus})); !slices.Equal(got, []string{"aliases", "notify"}) {
		t.Errorf("fields %q", got)
	}
	if err := CheckCharacter(CharacterSettings{Aliases: []string{"Kitty"}}); err != nil {
		t.Error(err)
	}
}

func TestCheckNewWorld(t *testing.T) {
	dir := editDir(t, uglyWorld)
	for _, id := range []string{"", "a b", "FM"} {
		if got := fields(CheckNewWorld(dir, id)); !slices.Equal(got, []string{"id"}) {
			t.Errorf("%q: fields %q", id, got)
		}
	}
	if err := CheckNewWorld(dir, "new"); err != nil {
		t.Error(err)
	}
}

func TestWritesCheckFirst(t *testing.T) {
	dir := editDir(t, uglyWorld)
	s, _, _ := ReadWorld(dir, "fm")
	s.Host = "a b"
	if got := fields(WriteWorld(dir, "fm", s)); !slices.Equal(got, []string{"host"}) {
		t.Errorf("WriteWorld fields %q", got)
	}
	c, _, _ := ReadCharacter(dir, "fm", "Kit")
	c.Aliases = []string{"#K"}
	if got := fields(WriteCharacter(dir, "fm", "Kit", c)); !slices.Equal(got, []string{"aliases"}) {
		t.Errorf("WriteCharacter fields %q", got)
	}
	if readWorld(t, dir) != uglyWorld {
		t.Error("a refused save changed the file")
	}
	if got := fields(AddCharacter(dir, "fm", "me")); !slices.Equal(got, []string{"name"}) {
		t.Errorf("AddCharacter fields %q", got)
	}
	if got := fields(AddWorld(dir, "fm", "h", 1, false)); !slices.Equal(got, []string{"id"}) {
		t.Errorf("AddWorld fields %q", got)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/config -run 'Check|WritesCheckFirst'`
Expected: build failure (`FieldError`, `CheckWorld` undefined).

- [ ] **Step 3: Implement the checks** — `internal/config/check.go`:

```go
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/str"
)

// FieldError is a setting that can't be saved. Field is its config key
// ("id" for a new world's id, "name" for a new character's name), so a
// form can put Msg by the field.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

func fieldErr(field, msg string) error { return &FieldError{Field: field, Msg: msg} }

// checkHost and checkPort are a world's server; see CheckWorld.
func checkHost(host string) error {
	if host == "" || strings.ContainsFunc(host, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return fieldErr("host", str.ConfigBadHost()) //str:ok
	}
	return nil
}

func checkPort(port int) error {
	if port < 1 || port > 65535 {
		return fieldErr("port", str.ConfigPortRange()) //str:ok
	}
	return nil
}

func checkNotify(n *string) error {
	if n == nil {
		return nil
	}
	if _, err := notify.ParseLevel(*n); err != nil {
		return fieldErr("notify", err.Error()) //str:ok
	}
	return nil
}

// CheckWorld reports every problem with s as a world's settings, each a
// *FieldError, joined: the port, the host, each pack in use (a pack id,
// and one in dir/packs), max_line_bytes, newline_mode and notify. nil if
// there are none.
func CheckWorld(dir string, s WorldSettings) error {
	errs := []error{checkPort(s.Port), checkHost(s.Host)}
	for _, p := range s.Use {
		switch {
		case !idRE.MatchString(p):
			errs = append(errs, fieldErr("use", str.ConfigBadPackId(p))) //str:ok
		default:
			if _, err := os.Stat(filepath.Join(dir, "packs", p+".toml")); err != nil {
				errs = append(errs, fieldErr("use", str.ConfigUnknownPack(p))) //str:ok
			}
		}
	}
	if s.MaxLineBytes != nil && *s.MaxLineBytes <= 0 {
		errs = append(errs, fieldErr("max_line_bytes", str.ConfigBadMaxLineBytes())) //str:ok
	}
	if s.NewlineMode != nil && *s.NewlineMode != "batch" && *s.NewlineMode != "flatten" { //str:ok
		errs = append(errs, fieldErr("newline_mode", str.ConfigBadNewlineMode())) //str:ok
	}
	errs = append(errs, checkNotify(s.Notify))
	return errors.Join(errs...)
}

// CheckCharacter reports every problem with s as a character's settings:
// each alias, and notify.
func CheckCharacter(s CharacterSettings) error {
	var errs []error
	for _, a := range s.Aliases {
		if err := NameChars(a); err != nil {
			errs = append(errs, fieldErr("aliases", err.Error())) //str:ok
			break
		}
	}
	errs = append(errs, checkNotify(s.Notify))
	return errors.Join(errs...)
}

// CheckNewWorld reports whether id can be a new world's id in dir: an id
// that isn't taken, ignoring case.
func CheckNewWorld(dir, id string) error {
	if !idRE.MatchString(id) {
		return fieldErr("id", str.ConfigWorldIdChars()) //str:ok
	}
	existing, err := filepath.Glob(filepath.Join(dir, "worlds", "*.toml"))
	if err != nil {
		return err
	}
	for _, p := range existing {
		if strings.EqualFold(strings.TrimSuffix(filepath.Base(p), ".toml"), id) {
			return fieldErr("id", str.ConfigWorldExists(id)) //str:ok
		}
	}
	return nil
}
```

Check the generated names before relying on them: `grep -n "func ConfigBadPackId\|func ConfigUnknownPack\|func ConfigBadMaxLineBytes\|func ConfigBadNewlineMode" internal/str/*_gen.go`. If `bad_pack_id`/`unknown_pack` are worded for a file being loaded (they take only the id, so they aren't), use them as they are. Check that `idRE` is the pattern pack ids are loaded with; if packs use a different one, use that (`grep -n "BadPackId" internal/config/*.go`).

In `internal/config/write.go`, `AddWorld`'s checks become:

```go
func AddWorld(dir, id, host string, port int, tls bool) error {
	if err := errors.Join(CheckNewWorld(dir, id), checkHost(host), checkPort(port)); err != nil {
		return err
	}
```

deleting the old id, host, port and exists checks (and the `existing` glob, now in `CheckNewWorld`). In `AddCharacter`, wrap the name errors:

```go
	if err := CheckName(name); err != nil {
		return "", fieldErr("name", err.Error()) //str:ok
	}
```

and `return "", errors.New(str.ConfigCharacterIdTaken(world, cid))` becomes `return "", fieldErr("name", str.ConfigCharacterIdTaken(world, cid)) //str:ok`.

In `internal/config/edit.go`, `WriteWorld` starts its body with

```go
	if err := CheckWorld(dir, s); err != nil {
		return err
	}
```

and `WriteCharacter` with

```go
	if err := CheckCharacter(s); err != nil {
		return err
	}
```

- [ ] **Step 4: Run the config tests**

Run: `go test ./internal/config`
Expected: PASS, including the existing `TestAddWorldErrors` and `TestWriteWorldRejectsBadResult`.

- [ ] **Step 5: App tests** — in `internal/app/edit_test.go`, delete `TestParsePort` and add:

```go
func TestAddWorldChecksEverythingFirst(t *testing.T) {
	a, dir := reloadApp(t)
	err := a.AddWorld("fm", config.WorldSettings{Host: "", Port: 0})
	var got []string
	for _, e := range Problems(err) {
		var fe *FieldError
		if errors.As(e, &fe) {
			got = append(got, fe.Field)
		}
	}
	if !slices.Equal(got, []string{"port", "host", "id"}) {
		t.Errorf("fields %q, from %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "worlds", "fm.toml")); err != nil {
		t.Fatal(err) // reloadApp's fm is still there
	}
}

func TestProblemsFlattens(t *testing.T) {
	a, b, c := errors.New("a"), errors.New("b"), errors.New("c")
	if got := Problems(errors.Join(errors.Join(a, b), nil, c)); !slices.Equal(got, []error{a, b, c}) {
		t.Errorf("got %v", got)
	}
	if got := Problems(a); !slices.Equal(got, []error{a}) {
		t.Errorf("one: %v", got)
	}
	if Problems(nil) != nil {
		t.Error("nil should have no problems")
	}
}
```

(`reloadApp` must give a config with a world `fm`; check it does — `grep -n "func reloadApp" -A15 internal/app/*_test.go` — and use its world id if it differs.)

- [ ] **Step 6: Run them to see them fail**

Run: `go test ./internal/app -run 'AddWorldChecks|Problems'`
Expected: build failure (`Problems`, `FieldError` undefined).

- [ ] **Step 7: Implement in the core** — in `internal/app/edit.go`:

Delete `ParsePort` (and `strconv` from the imports if now unused). Add:

```go
// FieldError is a setting that can't be saved, naming its field; see
// config.FieldError. The save calls return them joined with errors.Join.
type FieldError = config.FieldError

// Problems is err's errors in order, a joined error flattened: what a
// form marks. nil for a nil err.
func Problems(err error) []error {
	if err == nil {
		return nil
	}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		var out []error
		for _, e := range j.Unwrap() {
			out = append(out, Problems(e)...)
		}
		return out
	}
	return []error{err}
}
```

`AddWorld` becomes:

```go
// AddWorld writes a new world with settings s. Every problem with s and
// id is reported before anything is written; if its settings still can't
// be written, the half-made world is taken back. Nothing reloads: the
// front end does, as after every edit.
func (a *App) AddWorld(id string, s config.WorldSettings) error {
	if err := errors.Join(config.CheckWorld(a.d.ConfigDir, s), config.CheckNewWorld(a.d.ConfigDir, id)); err != nil {
		return err
	}
	if err := config.AddWorld(a.d.ConfigDir, id, s.Host, s.Port, s.TLS); err != nil {
		return err
	}
	if err := config.WriteWorld(a.d.ConfigDir, id, s); err != nil {
		config.DeleteWorld(a.d.ConfigDir, id)
		return err
	}
	return nil
}
```

Update the doc comments of `SaveWorld`, `SaveCharacter` and `AddCharacter` to say they return `FieldError`s (joined) for bad settings.

- [ ] **Step 8: TUI test** — add to `internal/ui/editor_test.go` a test that opens the new-world editor, fills it with an existing world's id, a host and an empty port, saves, and checks the form shows exactly `str.ConfigPortRange()` (the first problem) and not `str.ConfigWorldExists(…)`. Model it on the existing new-world test in `editor_test.go` or `picker_test.go` (`grep -n "ConfigWorldExists\|saveNewWorld\|newWorld" internal/ui/*_test.go`), using its helpers to open the editor and type into fields, and name it `TestEditorShowsTheFirstProblem`.

- [ ] **Step 9: The TUI shows the first problem; its port is parsed, not checked**

In `internal/ui/editor.go`, add:

```go
// firstProblem is what a form shows of err: its first problem, since
// the save calls report every one.
func firstProblem(err error) string { return app.Problems(err)[0].Error() }
```

`worldSettings` no longer fails:

```go
// worldSettings reads a world form. An empty or unreadable port is 0,
// which the save refuses.
func worldSettings(f *form) config.WorldSettings {
	port, _ := strconv.Atoi(f.value(f.field(portLabel)))
	s := config.WorldSettings{
```

with its final `return s, nil` → `return s`. In `saveNewWorld`:

```go
	f := m.picker.edit.form
	s := worldSettings(f)
	id := f.value(f.field(worldLabel))
	if err := m.a.AddWorld(id, s); err != nil {
		f.reject = firstProblem(err)
		return
	}
```

In `saveWorld`:

```go
	e := m.picker.edit
	if err := m.a.SaveWorld(e.world, worldSettings(e.form)); err != nil {
		e.form.reject = firstProblem(err)
		return
	}
```

In `saveCharSettings`, `f.reject = err.Error()` → `f.reject = firstProblem(err)`. In `internal/ui/picker.go`'s `saveCharacter`, `e.form.reject = err.Error()` → `e.form.reject = firstProblem(err)`.

- [ ] **Step 10: Remove the string**

Delete `port_range = "port must be 1-65535"` from the `[editor]` table of `internal/str/locales/en.toml` (not the `[config]` one), then `go generate ./internal/str`. `grep -rn "EditorPortRange" internal web` must find nothing.

- [ ] **Step 11: Run everything** (Global Constraints). Expected: PASS, golden screens untouched. `go test ./internal/str` passes.

- [ ] **Step 12: Commit**

```bash
git add internal/config internal/app internal/ui internal/str
git commit -m "config: settings are checked on save, every problem at once, each naming its field

FieldError carries the config key; the save calls join them. The TUI
shows the first, as before; its port range check is the core's.

Strings: editor.port_range (removed)"
```

---

### Task 3: review and PR

- [ ] **Step 1: Fresh review** of the whole branch against `main` with this plan's Review Focus and side-effect list. Fix Critical and Important findings with a failing test first; log minors.
- [ ] **Step 2: Open the PR** "Web API step 1b: Enter again keyed to the text; settings errors name their field". The body links the spec, notes the one TUI behavior change (an edit you take back keeps the confirmation; Esc still drops it), says golden screens are unchanged, and lists `Strings: editor.port_range (removed)` with why (its check moved to the core, which uses `config.port_range`, the same text). Then stop.
