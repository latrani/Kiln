# Web UI step 6: screenshot goldens, the plumbing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The TUI's nine golden scenarios also drive the real web page in headless Chrome. Each is screenshotted light and dark and compared with a PNG golden, rewritten by `-update`. No PNG goldens are committed yet: the look gets its own pass first, and its goldens are taken at the end of that pass.

**Architecture:**
- **Shared scenario data.** `internal/kilntest/golden.go` holds the data the TUI's goldens use: the world, the clock, the passwords, the lines and the log scene. Both front ends build from it. The TUI's text goldens don't move, which proves it's the same data.
- **The page in golden mode.** The page gets a second, test-only entry, `src/golden.tsx`. Instead of booting Go's wasm, it talks to Go over a WebSocket: actions go out, batches come back. Only `node build.mjs <dir> golden` builds it, so the shipped page never has it.
- **The Go side.** A test in `web/smoke` runs the real `webapi.Loop` natively, over an `app.App` built from the shared data, with fake connections and a fixed clock. It serves the golden page and the bridge, drives Chrome as a person would, takes the screenshots and compares them.

**Tech Stack:** Go (internal/kilntest, internal/webapi, web/smoke with coder/websocket and the Chrome DevTools Protocol), Preact + TypeScript (web/ui), node:test, `image/png`.

**Spec:** `docs/superpowers/specs/2026-10-12-web-ui-design.md`. This plan covers step 6, "Screenshot goldens", less the goldens themselves. The relevant parts:
- **Testing → Screenshots:** "the TUI's golden scenarios (main, input, picker, editor, editor error, password, log, log filter, scrolled) are built once and drive both front ends. The TUI checks its text goldens; headless Chrome renders the page and checks a screenshot per scenario, light and dark. `-update` only when a change to the look is intended."
- **Done when:** "The TUI's golden screens are unchanged. The web has screenshot goldens of its own, made from the same scenarios."

## For you to check

1. **No PNGs committed yet** (your call: plumbing now, look later). Until a scenario has a golden, its test is skipped, and the skip says how to make the golden: `-update`.
2. **The goldens run with the smoke tests** (`KILN_SMOKE=1`, on your Mac, with Chrome), not in CI: GitHub's runners have no Chrome set up for this, and a screenshot from Linux wouldn't match one from macOS anyway. The pure parts (the image compare, the bridge) run everywhere.
3. **The compare is strict.** A pixel counts as different when any channel is off by more than 16 of 255. One different pixel fails the test, since a changed letter is only a few hundred pixels. A Chrome update that changes antialiasing will fail every golden; `-update` then, after a look at the diffs.
4. **A failed golden leaves two files** in `web/smoke/testdata/golden/failed/` (git-ignored): `<name>-<scheme>.png`, what the page drew, and `<name>-<scheme>.diff.png`, the golden with each differing pixel in magenta.
5. **The window is 960×600 CSS pixels at 1×,** which is about the TUI's 80×24 in JetBrains Mono at the page's size. The caret is hidden, since it blinks. At 1× the PNGs stay small (tens of KB); 2× would show more detail at four times the size.
6. **Go's side runs natively in the test, not as wasm.** It's the same `webapi.Loop` and `app.App` the wasm runs. Only the transport is different: a WebSocket in place of `kiln.act` and the batch callback. Go answers asynchronously here, which the wasm never does, so a scenario waits for each answer before its next step.
7. **`webapi.Loop` gains `Call(f)`,** which runs f on the loop between turns. The TUI's goldens set some state directly: Rook's unread count and priority in `main`, the "not sent" status in `input`. `Call` is how the web scenarios set the same state without racing the loop.

## Global Constraints

- **The TUI's text goldens never move.** `go test ./internal/ui` passes without `-update` after every task.
- **The shipped page is unchanged** apart from the move of `main.tsx`'s body into `page.tsx`. `golden.tsx` is only in the golden build.
- **Every string a person reads comes from the catalog.** This plan adds none: test failure messages are for developers, and `golden.tsx` shows nothing of its own.
- **Colors come from theme roles.** The plan adds no colors to the page. The diff image's magenta is a test artifact, not the page.
- **No personal names** (Kit, Rook, Ash, fm only).
- **Pre-1.0:** no shims.

## Review Focus

1. **Determinism:** the same scenario drawn twice gives the same pixels: fonts loaded, no caret, a fixed clock, presence pinned. → Task 5's `TestGoldenStable`.
2. **The golden page never ships:** `web/build.sh`'s output has no `bridge`. → Task 3's build test.
3. **A scenario that doesn't reach its state fails loudly,** never a screenshot of the wrong screen. Each scenario waits for what it's after before shooting. → Task 6, in each scenario.
4. **The bridge's order:** actions the page took before the first batch go after it, as with the wasm (the typing queue). → Task 3's bridge test.
5. **`-update` with no golden directory** makes it; a failed dir that's left over from the last run never affects a pass. → Task 4's tests.

---

## File Structure

- `internal/kilntest/golden.go` (new): the shared scenario data. Test: `golden_test.go` there (it's well formed). `internal/ui/golden_test.go`, `browse_test.go`, `model_test.go`: use it.
- `internal/webapi/loop.go`: `Call`. Test: `loop_test.go`.
- `web/ui/src/page.tsx` (new): what `main.tsx` did, less `boot`. `web/ui/src/main.tsx`: `page.tsx` plus `boot`. `web/ui/src/bridge.ts` (new): the WebSocket stand-in for the wasm. `web/ui/src/golden.tsx` (new): `page.tsx` plus `bridge`. `web/ui/build.mjs`: the `golden` build. Tests: `bridge.test.ts`, `build.test.ts`.
- `web/smoke/shot.go` (new, not a test file, so its tests run in CI): `compare`, `diffImage`. Test: `shot_test.go`.
- `web/smoke/golden_test.go` (new): the harness and the nine scenarios.
- `.gitignore`: `web/smoke/testdata/golden/failed/`.
- `docs/web-dev.md`: running and updating the goldens.

---

### Task 1: The scenarios' data, shared

**Files:**
- Create: `internal/kilntest/golden.go`, `internal/kilntest/golden_test.go`
- Modify: `internal/ui/golden_test.go`, `internal/ui/browse_test.go`, `internal/ui/model_test.go`

**Interfaces:**
- Produces (package `kilntest`):

```go
const GoldenWorld string           // fm, with local echo: the TUI's goldenWorld
var GoldenStart time.Time          // the clock when a scenario starts: 2026-09-24 21:14 local
var GoldenPasswords map[string]string // fm/kit: hunter2, fm/rook: pw
var LogDay time.Time               // the log scene's first line: 2026-09-24 21:00 local
var LogScene []string              // the log scene: the TUI's scene1
var MainLines []string             // main's two lines from Rook
func ScrolledLine(i int) string    // scrolled's line i: "line i https://kiln.test/i"
const InputText = "this line runs past twenty bytes"
const MainTyped = ":waves."
const PasswordTyped = "s3cret"
const FindTerm = "Mira"
```

- [ ] **Step 1: Write the test** (`internal/kilntest/golden_test.go`): the world parses, and its characters are Kit (autoconnect) and Rook.

```go
package kilntest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/latrani/Kiln/internal/config"
)

func TestGoldenWorldLoads(t *testing.T) {
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(GoldenWorld), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, w := range cfg.Worlds {
		for _, c := range w.Characters {
			got = append(got, c.ID)
		}
	}
	if len(got) != 2 || got[0] != "kit" || got[1] != "rook" {
		t.Errorf("characters = %v", got)
	}
}
```

(If `config.Config`'s worlds aren't a slice of `{Characters}`, read them the way `app.AllChars` does; the check stays the same.)

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/kilntest`
Expected: FAIL: `GoldenWorld` undefined.

- [ ] **Step 3: Implement** (`internal/kilntest/golden.go`)

```go
package kilntest

import (
	"fmt"
	"time"
)

// The golden scenarios' data, which both front ends build from: the
// TUI's text goldens (internal/ui) and the web's screenshots
// (web/smoke). Change it and both change.

// GoldenWorld is fm with local echo on, so sent lines show.
const GoldenWorld = `host = "muck.test"
port = 8888
tls = true
use = ["fuzzball"]
login = "connect {name} {password}"
max_line_bytes = 20
local_echo = true

[[characters]]
id = "kit"
name = "Kit"
autoconnect = true

[[characters]]
id = "rook"
name = "Rook"
`

// GoldenStart is the clock when a scenario starts; each then moves it a
// minute on, once Kit's connected.
var GoldenStart = time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local)

// GoldenPasswords are the saved passwords: Kit logs in by itself.
var GoldenPasswords = map[string]string{"fm/kit": "hunter2", "fm/rook": "pw"}

// LogDay is when the log scene starts, a line a minute.
var LogDay = time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)

// LogScene is Kit's log in the log scenarios.
var LogScene = []string{
	"Rook says, \"Evening!\"",
	"Sable waves a paw.",
	"Mira pages: you around?",
	"> :grins.",
	"Kit grins.",
	"Rook says, \"The lighthouse is dark.\"",
	"Rook yawns.",
}

// MainLines are what Kit hears in main.
var MainLines = []string{"Rook says, \"Evening, Kit.\"", "See https://kiln.test/map for the way"}

// ScrolledLine is scrolled's line i of 40, each with a link.
func ScrolledLine(i int) string { return fmt.Sprintf("line %d https://kiln.test/%d", i, i) }

// What a person types in the scenarios.
const (
	InputText     = "this line runs past twenty bytes"
	MainTyped     = ":waves."
	PasswordTyped = "s3cret"
	FindTerm      = "Mira" // the page the Filter chip shows
)
```

- [ ] **Step 4: The TUI reads it.** In `internal/ui`:
  - `golden_test.go`:
    - `goldenWorld` becomes `kilntest.GoldenWorld`; delete the `strings.Replace`.
    - `TestGoldenMain`'s two `h.show` lines become `h.show(kilntest.MainLines[0])` and `h.show(kilntest.MainLines[1])`; `h.typeText(":waves.")` becomes `h.typeText(kilntest.MainTyped)`.
    - `TestGoldenPicker` shows `kilntest.MainLines[0]`. It's the same text.
    - `TestGoldenInput` types `kilntest.InputText`; `TestGoldenPassword` types `kilntest.PasswordTyped`.
    - `TestGoldenLog` types `kilntest.FindTerm`.
    - `TestGoldenScrolled` shows `kilntest.ScrolledLine(i)`. Its hover offset `len("line 19 h")` stays.
  - `browse_test.go`: `day24 = kilntest.LogDay`, `scene1 = kilntest.LogScene`.
  - `model_test.go`: `newHarness`'s `pw` map is `maps.Clone(kilntest.GoldenPasswords)`, and `h.now = kilntest.GoldenStart`.
  - Each file imports `kilntest` already or gains it.

- [ ] **Step 5: Run**

Run: `go test ./internal/kilntest ./internal/ui`
Expected: PASS, the goldens unchanged (no `-update`).

- [ ] **Step 6: Commit**

```bash
git add internal/kilntest internal/ui
git commit -m "kilntest: the golden scenarios' data, for both front ends"
```

---

### Task 2: `Loop.Call`

**Files:**
- Modify: `internal/webapi/loop.go`
- Test: `internal/webapi/loop_test.go`

**Interfaces:**
- Produces: `func (l *Loop) Call(f func(a *app.App))`. It runs f on the loop, as a turn of its own, and the page gets what changed.

- [ ] **Step 1: Write the failing test** (append to `loop_test.go`)

```go
func TestCallRunsOnTheLoop(t *testing.T) {
	h := start(t)
	h.l.Call(func(a *app.App) { a.SetStatus(true, "x") })
	b := h.next()
	if s := event(b, "Status"); s == nil || s["Text"] != "x" {
		t.Errorf("batch after Call = %v", types(b))
	}
}
```

(If the status event's type or field differ, read them from `TestSayPutsAStatusOnTheLoop`, which checks the same thing for `Say`.)

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/webapi -run TestCallRunsOnTheLoop`
Expected: FAIL: `h.l.Call undefined`.

- [ ] **Step 3: Implement** (`loop.go`)
  - Next to `Say`:

```go
// Call runs f on the loop, between turns, as a turn of its own: the page
// gets what it changed. Tests use it to set up what no action reaches.
func (l *Loop) Call(f func(a *app.App)) { l.post(call(f)) }
```

  - Next to `say`: `type call func(a *app.App)`.
  - In `step`: `case call: m(l.a)`.

- [ ] **Step 4: Run**

Run: `go test ./internal/webapi`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/webapi
git commit -m "webapi: Call runs a function on the loop, for tests"
```

---

### Task 3: The golden page: `page.tsx`, `bridge.ts`, `golden.tsx`, and its build

**Files:**
- Create: `web/ui/src/page.tsx`, `web/ui/src/bridge.ts`, `web/ui/src/golden.tsx`, `web/ui/src/bridge.test.ts`
- Modify: `web/ui/src/main.tsx`, `web/ui/build.mjs`, `web/ui/src/build.test.ts`

**Interfaces:**
- Produces:
  - `page.tsx`: `export function mount(store: Store): void`, which renders the App and adds the page's key listener and `watch`; `export function effect(store: Store): (e: Event) => void`. This is `main.tsx`'s body, moved.
  - `bridge.ts`: `export interface Socket { send(s: string): void; onmessage: ((m: { data: string }) => void) | null; onopen: (() => void) | null }` and `export function bridge(store: Store, ws: Socket): void`. It is what `boot` does once the wasm runs: `act` through the typing queue, batches into the store, and `kilnPage.act` for tests.
  - `golden.tsx`: `mount` plus `bridge(store, new WebSocket(<?bridge=>))`, and a style that hides the caret.
  - `node build.mjs <dir> golden` builds `golden.tsx` as `app.js` in `<dir>`.

- [ ] **Step 1: Write the failing tests**

`bridge.test.ts`:

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { bridge } from "./bridge.ts";
import { Store } from "./store.ts";

const fake = () => {
  const sent: any[] = [];
  const ws = { send: (s: string) => sent.push(JSON.parse(s)), onmessage: null as any, onopen: null as any };
  return { ws, sent };
};

test("the bridge runs the page as the wasm does: running once open, batches into the store", () => {
  const s = new Store();
  const { ws } = fake();
  bridge(s, ws);
  ws.onopen();
  assert.equal(s.state.phase, "running");
  ws.onmessage({ data: JSON.stringify([{ type: "Char", Key: "fm/kit", World: "fm", ID: "kit", Name: "Kit", State: 2 }, { type: "Active", Key: "fm/kit" }]) });
  assert.equal(s.state.active, "fm/kit");
});

test("what the page did before the first batch goes after it, in order", () => {
  const s = new Store();
  const { ws, sent } = fake();
  bridge(s, ws);
  ws.onopen();
  s.state.act!({ Do: "here" });
  assert.equal(sent.length, 0, "held until Go's first batch");
  ws.onmessage({ data: "[]" });
  assert.deepEqual(sent.at(-1), { Do: "here" });
});
```

`build.test.ts` (append):

```ts
test("the golden build has the WebSocket bridge; the page's build doesn't", (t) => {
  const page = mkdtempSync(join(tmpdir(), "kiln-ui-"));
  const golden = mkdtempSync(join(tmpdir(), "kiln-ui-"));
  t.after(() => { rmSync(page, { recursive: true, force: true }); rmSync(golden, { recursive: true, force: true }); });
  execFileSync(process.execPath, ["build.mjs", page], { cwd: ui });
  execFileSync(process.execPath, ["build.mjs", golden, "golden"], { cwd: ui });
  assert.ok(!readFileSync(join(page, "app.js"), "utf8").includes("WebSocket"), "the page talks to Go only through the wasm");
  assert.ok(readFileSync(join(golden, "app.js"), "utf8").includes("WebSocket"));
});
```

- [ ] **Step 2: Run to see them fail**

Run: `cd web/ui && npm test`
Expected: FAIL: `./bridge.ts` can't be resolved.

- [ ] **Step 3: Implement**
  - **`page.tsx`:**
    - Move `main.tsx`'s `effect` here as `effect(store)`, returning the function. It closes over the store, which `main.tsx` had as a module variable.
    - Move the `render`, the keydown listener and the `watch` subscription here, as `mount(store)`.
  - **`main.tsx`** becomes:

```tsx
// The page's entry: the store, the app drawn from it, and Kiln booting.
import "./kiln.css";
import { boot } from "./boot.ts";
import { effect, mount } from "./page.tsx";
import { Store } from "./store.ts";

const store: Store = new Store((e) => effect(store)(e));
mount(store);
boot(store).catch((e) => {
  console.error(e);
  store.set({ phase: "stopped" });
});
```

  - **`bridge.ts`:**

```ts
// The golden page's stand-in for Go's wasm: Go runs natively in the
// test, across a WebSocket. Actions go out through the typing queue, as
// boot's do; batches come in to the store. Only the golden build has it
// (golden.tsx); the page boots the wasm (boot.ts).
import type { Event, Store } from "./store.ts";

export interface Socket {
  send(s: string): void;
  onmessage: ((m: { data: string }) => void) | null;
  onopen: (() => void) | null;
}

export function bridge(store: Store, ws: Socket): void {
  const typing = store.state.typing!;
  const act = (o: object) => typing.act(o);
  store.set({ act, saving: true });
  (globalThis as any).kilnPage = { act };
  let running = false;
  ws.onopen = () => store.set({ phase: "running", progress: undefined });
  ws.onmessage = (m) => {
    const batch: Event[] = JSON.parse(m.data);
    store.apply(batch);
    if (!running) {
      running = true;
      const s = store.state;
      typing.ready((o) => ws.send(JSON.stringify(o)), s.chars[s.active] ? s.active : "");
      store.set({});
    }
  };
}
```

  - **`golden.tsx`:**

```tsx
// The golden page (web/smoke's screenshot goldens): the page, with Go
// across a WebSocket (bridge.ts) in place of the wasm. Not shipped:
// only build.mjs's golden build makes it.
import "./kiln.css";
import { bridge } from "./bridge.ts";
import { effect, mount } from "./page.tsx";
import { Store } from "./store.ts";

// The caret blinks, so screenshots hide it.
document.head.append(Object.assign(document.createElement("style"), { textContent: "* { caret-color: transparent !important; }" }));
const store: Store = new Store((e) => effect(store)(e));
mount(store);
bridge(store, new WebSocket(new URLSearchParams(location.search).get("bridge")!));
```

  - **`build.mjs`:** `const golden = process.argv[3] === "golden";`, and `entryPoints: [golden ? "src/golden.tsx" : "src/main.tsx"]`. Update its comment: "…; with golden, the golden page (golden.tsx) instead, for web/smoke's screenshots."

- [ ] **Step 4: Run**

Run: `cd web/ui && npm run check && npm test && cd .. && ./build.sh >/dev/null && KILN_SMOKE=1 go test -count=1 ./smoke`
Expected: PASS, the smoke tests too: the page works as before.

- [ ] **Step 5: Commit**

```bash
git add web/ui
git commit -m "web/ui: a golden build, Go across a WebSocket in place of the wasm"
```

---

### Task 4: Comparing screenshots (`shot.go`)

**Files:**
- Create: `web/smoke/shot.go`, `web/smoke/shot_test.go`
- Modify: `.gitignore`

**Interfaces:**
- Produces:

```go
// differs reports how many pixels of got and want differ: a channel off
// by more than 16 of 255, or sizes that don't match (every pixel).
func differs(got, want image.Image) int
// diffImage is want, dimmed, with each pixel that differs in magenta.
func diffImage(got, want image.Image) image.Image
// checkShot compares png with the golden at path: it writes it with
// update; skips when there's none; else, if they differ, writes
// failed/<name>.png and failed/<name>.diff.png next to it and fails.
func checkShot(t tb, path string, png []byte, update bool)
```

`shot.go` isn't a `_test.go` file, so its code is part of the package that `shot_test.go` tests in CI (`go test ./...`; the browser tests skip there).

- [ ] **Step 1: Write the failing tests** (`shot_test.go`)

```go
package smoke

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func solid(w, h int, c color.Color) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.Set(x, y, c)
		}
	}
	return m
}

func TestDiffersCountsPixelsPastTheTolerance(t *testing.T) {
	a := solid(4, 4, color.RGBA{100, 100, 100, 255})
	b := solid(4, 4, color.RGBA{100, 100, 100, 255})
	b.Set(1, 1, color.RGBA{116, 100, 100, 255}) // 16: within
	b.Set(2, 2, color.RGBA{100, 117, 100, 255}) // 17: past
	if n := differs(a, b); n != 1 {
		t.Errorf("differs = %d, want 1", n)
	}
	if n := differs(a, solid(4, 5, color.White)); n != 4*5 {
		t.Errorf("sizes differ: %d, want every pixel", n)
	}
}

func TestDiffImageMarksWhatDiffers(t *testing.T) {
	a := solid(2, 1, color.Black)
	b := solid(2, 1, color.Black)
	b.Set(1, 0, color.White)
	d := diffImage(b, a)
	if r, g, bl, _ := d.At(1, 0).RGBA(); r>>8 != 255 || g>>8 != 0 || bl>>8 != 255 {
		t.Errorf("the differing pixel isn't magenta: %v", d.At(1, 0))
	}
	if d.At(0, 0) == d.At(1, 0) {
		t.Error("the same pixel is marked too")
	}
}

func encode(t *testing.T, m image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// rec is a T that records instead of failing.
type rec struct {
	failed, skipped bool
}

func (r *rec) Helper()                      {}
func (r *rec) Errorf(string, ...any)        { r.failed = true }
func (r *rec) Fatalf(string, ...any)        { r.failed = true }
func (r *rec) Skipf(string, ...any)         { r.skipped = true }

func TestCheckShotUpdatesThenPassesThenFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "golden", "main-dark.png") // no golden dir yet
	shot := encode(t, solid(3, 3, color.Black))
	failed := filepath.Join(dir, "golden", "failed")

	r := &rec{}
	checkShot(r, path, shot, false)
	if !r.skipped || r.failed {
		t.Errorf("no golden: %+v, want skipped", *r)
	}
	checkShot(r, path, shot, true)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("-update didn't write the golden: %v", err)
	}

	// A different shot fails, leaving what it drew and the diff.
	r = &rec{}
	checkShot(r, path, encode(t, solid(3, 3, color.White)), false)
	if !r.failed {
		t.Error("a shot that differs passed")
	}
	for _, f := range []string{"main-dark.png", "main-dark.diff.png"} {
		if _, err := os.Stat(filepath.Join(failed, f)); err != nil {
			t.Errorf("no %s: %v", f, err)
		}
	}

	// The same shot passes, and clears the last run's failure.
	r = &rec{}
	checkShot(r, path, shot, false)
	if r.failed || r.skipped {
		t.Errorf("the same shot: %+v, want a pass", *r)
	}
	if m, _ := filepath.Glob(filepath.Join(failed, "main-dark*")); len(m) > 0 {
		t.Errorf("a pass left %v", m)
	}
}
```

`checkShot` takes `tb`, `interface{ Helper(); Errorf(string, ...any); Fatalf(string, ...any); Skipf(string, ...any) }`, which `*testing.T` satisfies, so `shot.go` doesn't import `testing`.

- [ ] **Step 2: Run to see them fail**

Run: `cd web && go test ./smoke -run 'Differs|DiffImage|CheckShot'`
Expected: FAIL: undefined `differs`.

- [ ] **Step 3: Implement** (`shot.go`)

```go
package smoke

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// tolerance is how far a channel may be off (of 255) and still match:
// enough for antialiasing jitter, not for a different color.
const tolerance = 16

// differs reports how many pixels of got and want differ; every pixel
// when their sizes don't match.
func differs(got, want image.Image) int {
	gb, wb := got.Bounds(), want.Bounds()
	if gb.Size() != wb.Size() {
		return max(gb.Dx()*gb.Dy(), wb.Dx()*wb.Dy())
	}
	n := 0
	for y := 0; y < gb.Dy(); y++ {
		for x := 0; x < gb.Dx(); x++ {
			if !near(got.At(gb.Min.X+x, gb.Min.Y+y), want.At(wb.Min.X+x, wb.Min.Y+y)) {
				n++
			}
		}
	}
	return n
}

func near(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	off := func(x, y uint32) bool { d := int(x>>8) - int(y>>8); return d > tolerance || d < -tolerance }
	return !off(ar, br) && !off(ag, bg) && !off(ab, bb) && !off(aa, ba)
}

// diffImage is want at a third of its brightness, each pixel that
// differs from got in magenta.
func diffImage(got, want image.Image) image.Image {
	wb := want.Bounds()
	d := image.NewRGBA(image.Rect(0, 0, wb.Dx(), wb.Dy()))
	for y := 0; y < wb.Dy(); y++ {
		for x := 0; x < wb.Dx(); x++ {
			w := want.At(wb.Min.X+x, wb.Min.Y+y)
			if !image.Pt(x, y).In(got.Bounds().Sub(got.Bounds().Min)) || !near(got.At(got.Bounds().Min.X+x, got.Bounds().Min.Y+y), w) {
				d.Set(x, y, color.RGBA{255, 0, 255, 255})
				continue
			}
			r, g, b, _ := w.RGBA()
			d.Set(x, y, color.RGBA{uint8(r >> 8 / 3), uint8(g >> 8 / 3), uint8(b >> 8 / 3), 255})
		}
	}
	return d
}
```

`checkShot`:
- **With update:** `os.MkdirAll(filepath.Dir(path))` and write the PNG.
- **No golden:** `Skipf("no golden %s yet: run with -update to make it", path)`.
- **Otherwise:** decode both. If they differ, write `failed/<base>.png` (what the page drew) and `failed/<base>.diff.png` (`diffImage`), then fail: `"%s: %d pixels differ; see %s"`.
- **Matching:** remove any `failed/<base>*.png` left from an earlier run, so the folder only ever holds this run's failures.

`.gitignore`: add `/web/smoke/testdata/golden/failed/`.

- [ ] **Step 4: Run**

Run: `cd web && go vet ./smoke && go test ./smoke`
Expected: PASS (the browser tests skip without `KILN_SMOKE=1`).

- [ ] **Step 5: Commit**

```bash
git add web/smoke/shot.go web/smoke/shot_test.go .gitignore
git commit -m "web/smoke: compare a screenshot with its golden, and show what differs"
```

---

### Task 5: The golden harness

**Files:**
- Create: `web/smoke/golden_test.go`

**Interfaces:**
- Consumes: Task 1's `kilntest` data, Task 2's `Call`, Task 3's golden build, Task 4's `checkShot`. Also smoke's `openTab`, `evaluator`, `devtools`, `waitFor`, `pressKey`, `jsString`.
- Produces:

```go
var update = flag.Bool("update", false, "rewrite the screenshot goldens in testdata/golden")

// scene is one scenario's run: Go's side and the page's.
type scene struct {
	t     *testing.T
	a     *app.App   // read only on the loop (Call)
	l     *webapi.Loop
	ws    *websocket.Conn // the tab
	ev    func(string) string
	conns func(k string) *kilntest.Conn // the fake connection Dial gave k
	logs  string                        // the log root
	clock *goldenClock
	pw    map[string]string
}
// goldenScene starts a scenario: Go over the shared data (with
// passwords pw; nil for GoldenPasswords), the golden page in Chrome at
// 960×600, Kit connected and the clock a minute on, as the TUI's
// goldenHarness. Before Kit connects, setup runs (writing a log).
func goldenScene(t *testing.T, pw map[string]string, setup func(s *scene)) *scene
// shoot screenshots the page, light and dark, and checks each against
// testdata/golden/<name>-<light|dark>.png.
func (s *scene) shoot(name string)
// shot is one screenshot, in scheme ("light" or "dark").
func (s *scene) shot(scheme string) []byte
```

- [ ] **Step 1: Write the test that proves the plumbing** (`TestGoldenStable`): a scenario drawn twice gives the same pixels, light and dark.

```go
// TestGoldenStable draws main's start twice in each scheme and expects
// the same pixels: fonts loaded, no caret, a fixed clock. Without it a
// golden could fail for nothing.
func TestGoldenStable(t *testing.T) {
	s := goldenScene(t, nil, nil)
	for _, scheme := range []string{"light", "dark"} {
		a, b := s.shot(scheme), s.shot(scheme)
		ga, _ := png.Decode(bytes.NewReader(a))
		gb, _ := png.Decode(bytes.NewReader(b))
		if n := differs(ga, gb); n > 0 {
			t.Errorf("%s: %d pixels differ between two shots of the same screen", scheme, n)
		}
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `cd web && KILN_SMOKE=1 go test -count=1 -run TestGoldenStable ./smoke`
Expected: FAIL: undefined `goldenScene`.

- [ ] **Step 3: Implement** (`golden_test.go`)
  - **The page:** build it once per test binary (a `sync.Once`) into a temp dir under `os.MkdirTemp`, removed in `TestMain`. If `TestMain` already exists in the package, add to it. Run `node build.mjs <dir> golden` in `web/ui`, as `buildPage` does but with no wasm, then `cp web/ui/fonts` as build.mjs does (it already copies them).
  - **The app:** as `webapi`'s `newApp`, but from `kilntest`:
    - `config.EnsureDefaults(dir)`, then `worlds/fm.toml` = `GoldenWorld`.
    - `Dial` records a `kilntest.NewConn()` per key, under a mutex.
    - `NewLog` is `kilntest.NopLog{}`.
    - `Password` reads `pw`.
    - `Now` reads the clock.
    - `LogRoot` is `<dir>/logs`.
    - `a.Reload()`.
  - **The clock:** `type goldenClock struct{ mu sync.Mutex; now time.Time }`, with `Now()` and `Advance(d)`.
  - **The server:** an `httptest.Server` with a `ServeMux`:
    - `/kiln/`: `http.StripPrefix("/kiln/", http.FileServer(http.Dir(page)))`.
    - `/bridge`: `websocket.Accept`, then `l := webapi.New(a, func(b []byte) { c.Write(ctx, websocket.MessageText, b) })`, `go l.Run(ctx, webapi.Options{Autoconnect: true, ConfigDir: dir})`, and a reader loop: `for { _, b, err := c.Read(ctx); if err != nil { return }; l.Act(b) }`.
    - Hand the Loop back to the scene through a channel, since the page connects after the tab opens.
  - **Chrome:** `openTab(t, chrome, srv.URL+"/kiln/?bridge="+url.QueryEscape("ws"+strings.TrimPrefix(srv.URL, "http")+"/bridge"))`, after the same `KILN_SMOKE` and Chrome checks as `launch`. Factor those checks out of `launch` into `chromePath(t) string` and use it in both.
  - **The window:** `devtools(t, ws, "Emulation.setDeviceMetricsOverride", {"width": 960, "height": 600, "deviceScaleFactor": 1, "mobile": false})`.
  - **Start, as the TUI's `goldenHarness`:**
    - `setup` runs before the page opens, so a log is on disk when Kit connects.
    - Wait for `document.querySelector(".scroll")` and Kit `State === 2`. With a password, also wait for the login line in `conns("fm/kit").Sent()`.
    - `clock.Advance(time.Minute)`.
    - Pin presence: `kilnPage.act({Do: "here"})`. The headless tab's focus would otherwise decide it.
  - **`shot(scheme)`:**
    - `Emulation.setEmulatedMedia` with `{"features": [{"name": "prefers-color-scheme", "value": scheme}]}`.
    - `ev("document.fonts.ready.then(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))))")`, so the next frame is drawn.
    - `Page.captureScreenshot` `{"format": "png"}`, decoding its `data` from base64.
  - **`shoot(name)`:** for each scheme, `checkShot(t, filepath.Join("testdata", "golden", name+"-"+scheme+".png"), s.shot(scheme), *update)`.
  - **Rulings:** `devtools` returns raw JSON, so unmarshal `{"data": string}` for the screenshot. If the bridge's batches arrive before the page's socket handler is set, the page misses the first one: `bridge` sets its handlers in the same tick as `new WebSocket`, before any message can come, so that can't happen; say so in a comment.

- [ ] **Step 4: Run**

Run: `cd web && KILN_SMOKE=1 go test -count=1 -run TestGoldenStable ./smoke`
Expected: PASS. A failure is a finding: find what moves between shots (the clock, a caret, a font, presence) and pin it in the harness, not in the compare.

- [ ] **Step 5: Commit**

```bash
git add web/smoke/golden_test.go web/smoke/smoke_test.go
git commit -m "web/smoke: the golden harness: Go natively, the golden page in Chrome, shots light and dark"
```

---

### Task 6: The nine scenarios, and the docs

**Files:**
- Modify: `web/smoke/golden_test.go`, `docs/web-dev.md`

Each scenario reaches the TUI's state the way a person would on the page, and **waits for that state before it shoots** (Review Focus 3). Go's side is fed through `conns` and `Call`, as the TUI's harness feeds its own. Keys and clicks use smoke's helpers.

**Interfaces:**
- Consumes: Task 5's `goldenScene`, `shoot`, and the scene's fields.
- Produces: `TestGoldenMain`, `TestGoldenPicker`, `TestGoldenEditor` (editor and editor-error), `TestGoldenLog`, `TestGoldenInput`, `TestGoldenScrolled`, `TestGoldenPassword`, `TestGoldenLogFilter`. Their names and goldens match the TUI's.

- [ ] **Step 1: Write the scenarios.** Each of them, with no goldens yet, ends in skips. That's the plan's call, not a pass. Run each once with `-update` into a scratch copy to see what it draws (Step 2), then delete the PNGs.

```go
// show has the server send Kit line, and waits for it on the page.
func (s *scene) show(line string) {
	s.t.Helper()
	s.conns("fm/kit").Feed(line)
	waitFor(s.t, s.ev, `[...document.querySelectorAll("main .line")].some(l => l.textContent.includes(`+jsString(line)+`))`, line)
}

// typeIn types text into the focused field, as keys.
func (s *scene) typeIn(text string) {
	s.t.Helper()
	devtools(s.t, s.ws, "Input.insertText", map[string]any{"text": text})
}

func TestGoldenMain(t *testing.T) {
	s := goldenScene(t, nil, nil)
	s.ev(`kilnPage.act({Do: "open", Key: "fm/rook"})`)
	waitFor(t, s.ev, `document.querySelector("header .where").textContent.endsWith("Rook")`, "Rook showing")
	for _, l := range kilntest.MainLines {
		s.conns("fm/kit").Feed(l)
	}
	waitFor(t, s.ev, `document.activeElement === document.querySelector(".bottom textarea")`, "the input")
	s.typeIn(kilntest.MainTyped)
	pressKey(t, s.ws, "Enter", 13)
	waitFor(t, s.ev, `[...document.querySelectorAll("main .line")].some(l => l.textContent.includes(`+jsString(kilntest.MainTyped)+`))`, "the echo")
	s.l.Call(func(a *app.App) { c := a.Char("fm/rook"); c.Unread, c.Priority = 3, true })
	waitFor(t, s.ev, `document.querySelector(".side").textContent.includes("3")`, "Rook's unread")
	s.shoot("main")
}

func TestGoldenPicker(t *testing.T) {
	s := goldenScene(t, nil, nil)
	s.show(kilntest.MainLines[0])
	ctrlO(t, s.ws)
	waitFor(t, s.ev, `document.activeElement === document.querySelector('.bottom input[role="combobox"]')`, "the picker")
	s.shoot("picker")
}

func TestGoldenEditor(t *testing.T) {
	s := goldenScene(t, nil, nil)
	waitFor(t, s.ev, `document.activeElement === document.querySelector(".bottom textarea")`, "the input")
	s.typeIn("/edit world")
	pressKey(t, s.ws, "Enter", 13)
	port := `[...document.querySelectorAll(".bottom label")].find(l => l.textContent.startsWith(` + jsString(str.EditorPort()) + `))`
	waitFor(t, s.ev, port, "the world's editor")
	s.shoot("editor")
	// As the TUI's focusOn(portLabel) and x: an x after the port.
	s.ev(`(f => { f.focus(); f.setSelectionRange(f.value.length, f.value.length); })(document.getElementById(` + port + `.htmlFor))`)
	s.typeIn("x")
	waitFor(t, s.ev, `document.querySelector(".bottom [role=alert], .bottom .error")`, "the port's error")
	s.shoot("editor-error")
}

func TestGoldenLog(t *testing.T) {
	s := goldenScene(t, nil, func(s *scene) {
		kilntest.WriteLog(t, logstore.Layout{Root: s.logs, World: "fm", Char: "kit", CharName: "Kit"}, kilntest.LogDay, kilntest.LogScene...)
	})
	ctrlL(t, s.ws)
	waitFor(t, s.ev, `document.querySelectorAll(".lrow").length >= 7`, "log mode, the scene")
	// The TUI's m, ↑, m: the last two lines.
	rows := `[...document.querySelectorAll(".lrow")]`
	click(t, s.ws, s.ev, rows+`.at(-1)`)
	shiftClick(t, s.ws, s.ev, rows+`.at(-2)`)
	waitFor(t, s.ev, `[...document.querySelectorAll(".lrow .gutter")].filter(g => g.textContent === `+jsString(glyph.Selected)+`).length === 2`, "the range")
	// The Filter chip on, as the TUI's: Only on page, set directly.
	s.l.Call(func(a *app.App) {
		a.Char("fm/kit").Log.PressOnly(scene.Item{Name: "page"}) // log mode's, open since Ctrl+L
	})
	waitFor(t, s.ev, `document.querySelectorAll(".lrow").length < 7`, "Only on page")
	ev := s.ev
	ev(`document.querySelector(".log").focus()`)
	devtools(t, s.ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "/", "text": "/", "code": "Slash", "windowsVirtualKeyCode": 191})
	waitFor(t, ev, `document.activeElement === document.querySelector("#kiln-logprompt")`, "find")
	s.typeIn(kilntest.FindTerm)
	pressKey(t, s.ws, "Enter", 13)
	waitFor(t, ev, `document.querySelector("mark")`, "found")
	s.shoot("log")
}

func TestGoldenInput(t *testing.T) {
	s := goldenScene(t, nil, nil)
	in := `document.querySelector(".bottom textarea")`
	waitFor(t, s.ev, `document.activeElement === `+in, "the input")
	s.typeIn(kilntest.InputText)
	s.ev(in + `.setSelectionRange(0, 4)`) // the first four, as the TUI's drag
	s.l.Call(func(a *app.App) { a.SetStatus(true, str.StatusNotSent(errors.New("x"))) })
	waitFor(t, s.ev, `document.querySelector(".status").textContent.includes(`+jsString(str.StatusNotSent(errors.New("x")))+`)`, "the status")
	s.shoot("input")
}

func TestGoldenScrolled(t *testing.T) {
	s := goldenScene(t, nil, nil)
	for i := 0; i < 40; i++ {
		s.clock.Advance(2 * time.Second)
		s.show(kilntest.ScrolledLine(i))
	}
	pressKey(t, s.ws, "PageUp", 33)
	link := `[...document.querySelectorAll("main a")].find(a => a.textContent === "https://kiln.test/19")`
	waitFor(t, s.ev, link+` && `+link+`.getBoundingClientRect().bottom <= document.querySelector("main .scroll").getBoundingClientRect().bottom`, "line 19 in view")
	r := s.ev(`JSON.stringify((r => ({x: r.left + 2, y: r.top + r.height / 2}))(` + link + `.getBoundingClientRect()))`)
	var p struct{ X, Y float64 }
	json.Unmarshal([]byte(mustUnquote(t, r)), &p)
	devtools(t, s.ws, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": p.X, "y": p.Y})
	s.shoot("scrolled")
}

func TestGoldenPassword(t *testing.T) {
	pw := maps.Clone(kilntest.GoldenPasswords)
	delete(pw, "fm/kit")
	s := goldenScene(t, pw, nil)
	field := `document.querySelector('.bottom input[type="password"]')`
	waitFor(t, s.ev, `document.activeElement === `+field, "the password form")
	s.typeIn(kilntest.PasswordTyped)
	s.shoot("password")
}

func TestGoldenLogFilter(t *testing.T) {
	s := goldenScene(t, nil, func(s *scene) {
		kilntest.WriteLog(t, logstore.Layout{Root: s.logs, World: "fm", Char: "kit", CharName: "Kit"}, kilntest.LogDay, kilntest.LogScene...)
	})
	ctrlL(t, s.ws)
	waitFor(t, s.ev, `document.activeElement === document.querySelector(".log")`, "log mode")
	letter := func(k string) {
		devtools(t, s.ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": k, "text": k})
		devtools(t, s.ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": k})
	}
	letter("f")
	sel := `document.querySelector(".filter-panel .fsel")`
	waitFor(t, s.ev, `document.activeElement === `+sel, "the panel")
	// As the TUI's: past Untagged, hide page/in, then Only on self.
	pressKey(t, s.ws, "ArrowDown", 40)
	pressKey(t, s.ws, "ArrowDown", 40)
	letter("h")
	pressKey(t, s.ws, "ArrowDown", 40)
	letter("o")
	waitFor(t, s.ev, `document.querySelectorAll('.filter-panel [aria-pressed="true"]').length === 2`, "Hide and Only pressed")
	s.shoot("log-filter")
}
```

These are rulings the executor makes against the code, keeping each scenario's state the TUI's:
- **`TestGoldenLog`'s Only:**
  - `Log.PressOnly` is what the TUI's `kitFilter().PressOnly` does, with the log's items (`internal/app/panel.go`).
  - `Call`'s turn sends the Log and Filter events it changed, since the feed diffs every turn. If it doesn't, that's a finding about `Call`: fix it there, not in the scenario.
- **`TestGoldenEditor`'s x:** the TUI's `focusOn(portLabel)` and `x` add an x after the port, so the page's field ends with an x after the port too. Use the field's id from its label, and the error's own selector from `EditorForm.tsx`.
- **`TestGoldenMain`:** the TUI feeds Kit's lines while Rook shows; feed Kit through `conns("fm/kit")` as above, and wait for Kit's unread in the sidebar, not for the lines in `main`.
- **The selectors** (`.status`, `.side`, `header .where`, `main a`, `main .scroll`) are checked against the existing smoke tests and components before use.

- [ ] **Step 2: Run them, and look at what they draw**

Run: `cd web && KILN_SMOKE=1 go test -count=1 -run 'TestGolden' ./smoke -v 2>&1 | grep -E '^(---|ok|FAIL)|SKIP'`
Expected: `TestGoldenStable` PASS; every other `TestGolden…` SKIP ("no golden … yet").

Then, to see the shots: `cp -R smoke/testdata /tmp/kiln-goldens-check`, run once with `-args -update`, and read each PNG (the Read tool shows images). Compare each with its TUI golden (`internal/ui/testdata/golden/<name>.txt`): the same scenario, the same state (who's showing, what's selected, what's typed). Then delete `web/smoke/testdata/golden/*.png`. **Don't commit any PNG** (For you to check 1). Send Indi the 18 shots with SendUserFile when reporting, since they're the look pass's starting point.

- [ ] **Step 3: The docs** (`docs/web-dev.md`): a "Screenshot goldens" section after the smoke test's:

````markdown
## Screenshot goldens

`web/smoke`'s `TestGolden…` tests draw the TUI's golden scenarios on
the page, light and dark, and compare each with
`web/smoke/testdata/golden/<scenario>-<light|dark>.png`. They run with
the smoke tests:

```sh
cd web && KILN_SMOKE=1 go test ./smoke -run TestGolden
```

A shot that differs leaves what it drew and a diff (the golden, dimmed,
with each differing pixel in magenta) in `testdata/golden/failed/`.
After a change to the look you meant, rewrite the goldens and look
through them before committing:

```sh
cd web && KILN_SMOKE=1 go test ./smoke -run TestGolden -args -update
```

A scenario with no golden yet is skipped. Go runs natively in these
tests, over the same data as the TUI's goldens
(`internal/kilntest/golden.go`); the page is a test-only build
(`src/golden.tsx`) that talks to it over a WebSocket.
````

- [ ] **Step 4: The whole suite**

Run: `go vet ./... && go test ./... && (cd web && go vet ./... && go test ./... && GOOS=js GOARCH=wasm go vet ./...) && (cd web/ui && npm run check && npm test) && web/build.sh >/dev/null && (cd web && KILN_SMOKE=1 go test -count=1 ./smoke)`
Expected: PASS. The goldens skip; `TestGoldenStable` passes; the TUI's goldens are unchanged.

- [ ] **Step 5: Commit**

```bash
git add web/smoke/golden_test.go docs/web-dev.md
git commit -m "web/smoke: the nine golden scenarios on the page; web-dev.md says how to run and update them"
```
