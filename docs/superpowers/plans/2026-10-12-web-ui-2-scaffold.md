# Web UI, step 2: the page scaffold — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The real page replaces the development page: a Preact + TypeScript app built with esbuild, drawing the TUI's five areas in the bundled JetBrains Mono, styled from the `Theme` event, with one store fed by Go's batches, and the states around the main screen (another tab, loading, Kiln stopped, not saving). The areas are empty frames until step 3 fills them; the smoke test still drives Kiln end to end through the new page.

**Architecture:**
- **`web/ui/`** holds the page: `package.json` (pinned: preact, esbuild, typescript, preact-render-to-string; `npm ci`, no global installs), `tsconfig.json` (strict, `noEmit`, `allowImportingTsExtensions`, JSX for Preact), `build.mjs` (esbuild: `src/main.tsx` → `<out>/app.js` + `app.css`, the fonts, `index.html` from a template), `test.mjs` (bundles `src/**/*.test.ts(x)` with esbuild and runs them under `node --test`).
- **`src/types.ts`** mirrors the JSON events; **`src/store.ts`** applies a batch as one update (one notification per batch), dropping everything on `Start`; **`src/theme.ts`** turns a `Theme` event into a stylesheet (one class per role, the terminal's sixteen as `--ansi-N`, light/dark by `prefers-color-scheme` or forced by the `appearance` setting) and folds a span's style (the page side of step 1's Ruling 1); **`src/boot.ts`** is the old `kiln.js` boot (tab lock, memfs, IndexedDB, preset, wasm) with download progress and "Kiln stopped".
- **Components** (`src/ui/*.tsx`): `App` (the grid), `TopBar`, `Sidebar`, `Scrollback`, `BottomPane`, `StatusLine`. One per TUI part, as the spec says; step 3 fills them.
- **Fonts:** the official JetBrains Mono v2.304 webfonts (Regular, Bold, Italic, BoldItalic) and `OFL.txt`, committed under `web/ui/fonts/`. Fontsource's npm package was considered and rejected: its subsets leave out box drawing and geometric shapes (`─ ► ● ◄ ▌ ░ ≡`), which would fall back to another font at another width.
- **Gone:** `web/static/kiln.js`, `web/static/index.html` (now built), `web/static/strings.json`, `catalog.WebJSON` and its test and generator step. `memfs.mjs`, `persist.mjs`, `tablock.mjs` and their tests stay in `web/static/` (served nowhere now, bundled into `app.js` from there).
- **Smoke test** builds the page with `build.mjs` into its temp dir, opens it with `?record` (only then does the page keep every event in `kilnPage.events`), and also checks that each glyph is one cell wide in the bundled font.

**Tech Stack:** Preact 10.29.8, TypeScript 6.0.3, esbuild 0.28.2, preact-render-to-string 6.7.0; Node (CI: lts); JetBrains Mono 2.304.

**Spec:** `docs/superpowers/specs/2026-10-12-web-ui-design.md` — "Layout", "Font", "Terminal colors", "States around the main screen", "The page's code", "Order of work" step 2. Step 1's plan (`2026-10-12-web-ui-1-api-data.md`) Ruling 1 for the fold.

**Rulings (for review):**
1. **Versions are at least two weeks old:** Preact 10.29.8, not 11.0.x (11.0.0 is 12 days old, 11.0.1 four); TypeScript 6.0.3, not 7 (the native rewrite; nothing here needs it).
2. **Fonts are committed**, not fetched at build time: four WOFF2 files (~90KB each) and the OFL, so a build needs no network beyond `npm ci`.
3. **Component tests render to a string** (preact-render-to-string), not a DOM library: step 2's components are static frames; step 3 can add a DOM if interaction tests need one.
4. **A stray-text lint for the page** (a node test over `src/ui/*.tsx`): JSX text and `aria-label`/`title`/`placeholder` values must come from `str.*`. I recommended this when we chose the TS module; the spec didn't carry it.
5. **`kilnPage.events` only with `?record`** in the URL: the dev page kept every event forever; the real page shouldn't.

## Global Constraints

- Every person-facing string comes from the catalog: new entries go in `internal/str/locales/en.toml` under `[web]`, then `go generate ./internal/str`, then `str.webX()` in TS. Commits that change `en.toml` end with a `Strings:` trailer listing the keys.
- TS tests build expected text from `str.gen.ts`, never a copy of catalog text.
- Colors come only from the `Theme` event and the terminal palette below; no color literals in components or `kiln.css` except the palette table in `theme.ts`.
- Sizes in `ch`/`rem`/`lh`; the sidebar is `16ch`; nothing resizable.
- TUI unchanged; golden screens NOT updated.
- Before each commit: `go vet ./... && go test ./...`; `(cd web && go vet ./... && go test ./... && GOOS=js GOARCH=wasm go vet ./...)`; `(cd web/ui && npm run check && npm test)`; `node --test web/static/*.test.mjs`; `web/build.sh`. No `-race`.
- End each commit with the session's attribution lines.

## Review Focus

1. **A batch that throws** (a malformed event, a bug in `apply`): Go calls `onBatch` from inside the wasm; an exception there must never reach Go. The store must catch per batch, log, and keep going. → Task 3's `TestBadEventDoesntStopTheBatch`.
2. **`Start` mid-session** (the page asks for `all`): every per-character map is dropped, including lines and log state, so nothing stale survives. → Task 3's `Start` test.
3. **Lines epochs:** a `Lines` event whose `Epoch` differs from what the store holds for that key, with an `append`/`prepend`/`replace` op, must not graft onto the old numbering; only `reset` starts a new epoch, and anything else for an unknown epoch is ignored (logged). → Task 3's epoch test.
4. **A wasm download with no `Content-Length`** (a proxy, gzip): progress shows megabytes so far, never `NaN` or a division by zero. → Task 4's `readWithProgress` test.
5. **Forced appearance:** `appearance = "light"` with a dark OS (and the reverse) must use the forced palette, not the media query. → Task 2's appearance test.

---

### Task 1: the `web/ui` toolchain

**Files:**
- Create: `web/ui/package.json`, `web/ui/package-lock.json` (from `npm install`), `web/ui/tsconfig.json`, `web/ui/build.mjs`, `web/ui/test.mjs`, `web/ui/index.html` (template), `web/ui/src/main.tsx` (placeholder for now), `web/ui/src/kiln.css` (font faces only for now), `web/ui/fonts/JetBrainsMono-{Regular,Bold,Italic,BoldItalic}.woff2`, `web/ui/fonts/OFL.txt`
- Modify: `.gitignore`, `web/build.sh`, `.github/workflows/web.yml`, `docs/web-dev.md`, `internal/str/locales/en.toml` (`web.needs_js`), `web/ui/src/str.gen.ts` (generated)

**Interfaces:**
- Produces: `node web/ui/build.mjs <outdir>` writes `<outdir>/{index.html,app.js,app.css,fonts/*}`; `npm run check` (tsc), `npm test` (all `src/**/*.test.ts(x)`), `npm run build` (= `node build.mjs ../static`).

- [ ] **Step 1: Fonts.** Download the official release and take the four webfonts and the license:

```bash
cd /private/tmp && curl -sLO https://github.com/JetBrains/JetBrainsMono/releases/download/v2.304/JetBrainsMono-2.304.zip
unzip -o -q JetBrainsMono-2.304.zip -d jbm
mkdir -p ~/Projects/KilnClient/web/ui/fonts
cp jbm/fonts/webfonts/JetBrainsMono-{Regular,Bold,Italic,BoldItalic}.woff2 ~/Projects/KilnClient/web/ui/fonts/
cp jbm/OFL.txt ~/Projects/KilnClient/web/ui/fonts/OFL.txt
```

(Use the scratchpad directory, not `/tmp`, and check the zip's layout if the paths differ.)

- [ ] **Step 2: package.json, tsconfig, catalog entry.**

```json
{
  "private": true,
  "type": "module",
  "scripts": {
    "check": "tsc",
    "test": "node test.mjs",
    "build": "node build.mjs ../static"
  },
  "dependencies": { "preact": "10.29.8" },
  "devDependencies": {
    "esbuild": "0.28.2",
    "preact-render-to-string": "6.7.0",
    "typescript": "6.0.3"
  }
}
```

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "noEmit": true,
    "allowImportingTsExtensions": true,
    "allowJs": true,
    "checkJs": false,
    "jsx": "react-jsx",
    "jsxImportSource": "preact",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "skipLibCheck": true
  },
  "include": ["src"]
}
```

Run `(cd web/ui && npm install)` to make the lockfile. Add to `en.toml` under `[web]`: `needs_js = "Kiln needs JavaScript."`; run `go generate ./internal/str`.

- [ ] **Step 3: Write the failing check** — `web/ui/src/build.test.ts`:

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import * as str from "./str.gen.ts";

test("build writes the page, its styles and its fonts", () => {
  const out = mkdtempSync(join(tmpdir(), "kiln-ui-"));
  execFileSync("node", ["build.mjs", out], { cwd: new URL("..", import.meta.url).pathname });
  for (const f of ["index.html", "app.js", "app.css", "fonts/JetBrainsMono-Regular.woff2", "fonts/OFL.txt"]) {
    assert.ok(existsSync(join(out, f)), f);
  }
  const html = readFileSync(join(out, "index.html"), "utf8");
  assert.ok(html.includes(str.webNeedsJs()));
  assert.ok(!html.includes("{{"), "a template slot left unfilled");
});
```

Run `node --test web/ui/src/build.test.ts` → FAIL (no `build.mjs`).

- [ ] **Step 4: build.mjs, test.mjs, template, placeholder entry.**

`web/ui/build.mjs`:

```js
// Builds the page into an output directory (default ../static): app.js
// and app.css from src/main.tsx, the fonts and their license, and
// index.html from its template with the catalog's strings filled in.
import * as esbuild from "esbuild";
import { copyFile, mkdir, readFile, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import * as str from "./src/str.gen.ts";

const here = new URL(".", import.meta.url).pathname;
const out = resolve(here, process.argv[2] ?? "../static");
await mkdir(join(out, "fonts"), { recursive: true });
await esbuild.build({
  absWorkingDir: here,
  entryPoints: ["src/main.tsx"],
  bundle: true,
  format: "esm",
  outfile: join(out, "app.js"),
  jsx: "automatic",
  jsxImportSource: "preact",
  loader: { ".woff2": "file" },
  assetNames: "fonts/[name]",
  minify: true,
  sourcemap: true,
  target: ["es2022"],
  logLevel: "warning",
});
await copyFile(join(here, "fonts", "OFL.txt"), join(out, "fonts", "OFL.txt"));
const slots = { needsJs: str.webNeedsJs() };
const html = (await readFile(join(here, "index.html"), "utf8")).replace(/\{\{(\w+)\}\}/g, (_, k) => slots[k]);
await writeFile(join(out, "index.html"), html);
```

(Node runs `build.mjs`'s `.ts` import by stripping types; `str.gen.ts` has only erasable syntax.)

`web/ui/test.mjs`:

```js
// Runs every src/**/*.test.ts(x): esbuild bundles each (JSX and all)
// into a scratch directory, then node --test runs them.
import * as esbuild from "esbuild";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const here = new URL(".", import.meta.url).pathname;
const tests = readdirSync(join(here, "src"), { recursive: true })
  .filter((f) => /\.test\.tsx?$/.test(f))
  .map((f) => join("src", f));
const out = mkdtempSync(join(tmpdir(), "kiln-ui-tests-"));
await esbuild.build({
  absWorkingDir: here, entryPoints: tests, bundle: true, platform: "node", format: "esm",
  outdir: out, outExtension: { ".js": ".mjs" }, jsx: "automatic", jsxImportSource: "preact",
  packages: "external", loader: { ".woff2": "empty", ".css": "empty" }, logLevel: "warning",
});
const files = readdirSync(out, { recursive: true }).filter((f) => f.endsWith(".mjs")).map((f) => join(out, f));
const r = spawnSync("node", ["--test", ...files], { stdio: "inherit", cwd: here });
process.exit(r.status ?? 1);
```

(`packages: "external"` keeps `preact` imports resolving from `web/ui/node_modules`; if Node can't find them from the scratch dir, put `out` inside `web/ui/.test-out/` and gitignore it — note it as a Ruling. `build.test.ts` uses `import.meta.url` to find `web/ui`; after bundling, compute the dir from `process.cwd()` instead, since `test.mjs` runs with `cwd: here`.)

`web/ui/index.html`:

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kiln</title>
<link rel="icon" href="data:,">
<link rel="stylesheet" href="app.css">
<style id="kiln-theme"></style>
</head>
<body>
<div id="kiln"></div>
<noscript>{{needsJs}}</noscript>
<script type="module" src="app.js"></script>
</body>
</html>
```

(`Kiln` in `<title>` is the product's name, not a translatable string; step 3 sets the title from the catalog with the unread count.)

`web/ui/src/kiln.css` (fonts for now):

```css
@font-face { font-family: "JetBrains Mono"; font-weight: 400; font-style: normal; font-display: block; src: url("../fonts/JetBrainsMono-Regular.woff2") format("woff2"); }
@font-face { font-family: "JetBrains Mono"; font-weight: 700; font-style: normal; font-display: block; src: url("../fonts/JetBrainsMono-Bold.woff2") format("woff2"); }
@font-face { font-family: "JetBrains Mono"; font-weight: 400; font-style: italic; font-display: block; src: url("../fonts/JetBrainsMono-Italic.woff2") format("woff2"); }
@font-face { font-family: "JetBrains Mono"; font-weight: 700; font-style: italic; font-display: block; src: url("../fonts/JetBrainsMono-BoldItalic.woff2") format("woff2"); }
```

(`font-display: block`: widths are measured in `ch`, so a fallback font flashing first would jump the layout.)

`web/ui/src/main.tsx`: `import "./kiln.css";` and nothing else yet.

- [ ] **Step 5: Run** `(cd web/ui && npm run check && npm test)` → PASS (build test and the existing `str.test.ts`).

- [ ] **Step 6: Wire it in.**
  - `.gitignore`: `/web/ui/node_modules/`, `/web/ui/.test-out/`, `/web/static/index.html`, `/web/static/app.js*`, `/web/static/app.css*`, `/web/static/fonts/`. `git rm --cached web/static/index.html` happens in Task 7, when the dev page goes.
  - `web/build.sh` doesn't change yet: building into `web/static` would overwrite the dev page's `index.html`. It changes in Task 7, when the dev page goes.
  - `.github/workflows/web.yml`: in place of the "Page strings" step, `- name: Page` with `working-directory: web/ui` and `run: npm ci && npm run check && npm test`.
  - `docs/web-dev.md`: "The page lives in `web/ui` (Preact, TypeScript, esbuild). `cd web/ui && npm ci` once; `web/build.sh` builds it with the wasm into `web/static`. `npm test` runs its tests, `npm run check` type-checks."

- [ ] **Step 7: Commit** `web/ui: the page's toolchain: Preact, TypeScript, esbuild, JetBrains Mono` with `Strings: web.needs_js (new)`.

### Task 2: the theme as a stylesheet, and the page's fold

**Files:**
- Create: `web/ui/src/types.ts` (the event shapes this and later tasks use), `web/ui/src/theme.ts`, `web/ui/src/theme.test.ts`, `internal/webapi/fold_test.go`, `web/ui/src/testdata/fold.json` (written by the Go test)

**Interfaces:**
- Produces (`types.ts`, matching step 1's JSON exactly):

```ts
export type Color = string; // "" | "#rrggbb" | "ansi:N"
export interface Look { FG?: Color; BG?: Color; Bold?: boolean; Faint?: boolean; Italic?: boolean; Underline?: boolean; Reverse?: boolean }
export interface TagLook extends Look { Match?: boolean }
export interface Looks { Roles: Record<string, Look>; Tags: Record<string, TagLook>; Chars?: Record<string, Record<string, TagLook>> }
export interface ThemeEvent { type: "Theme"; Appearance: "auto" | "light" | "dark"; Dark: Looks; Light: Looks }
export interface Span extends Look { Text: string; Tags?: string[]; Link?: string }
```

- Produces (`theme.ts`):

```ts
export const macchiato: Palette; export const latte: Palette; // the spec's table
export interface Palette { fg: string; bg: string; ansi: string[] } // 16
export function color(c: Color | undefined, p: Palette): string | undefined; // "ansi:N" → p.ansi[N], hex as is, "" → undefined
export function lookCSS(l: Look, p: Palette): string; // declarations, reverse swapped
export function roleClass(role: string): string; // "sidebar.active" → "r-sidebar-active"
export function themeCSS(t: ThemeEvent): string; // the whole stylesheet
export function fold(s: Span, tags: Record<string, TagLook>): Look; // server style, then each tag in order
```

- [ ] **Step 1: The shared fold fixture (Go side first).** `internal/webapi/fold_test.go` builds cases from the same theme and lines as `TestSpansDrawWhatTheTUIPaints`, folds them with the test's `fold` (already checked against the TUI there), and compares with `../../web/ui/src/testdata/fold.json`, rewriting it under `-update`:

```go
var updateFold = flag.Bool("update", false, "rewrite web/ui/src/testdata/fold.json")

// TestFoldFixture writes what the page's fold must give for each span,
// so the TypeScript fold is checked against the same answers the Go fold
// (and so the TUI) gives.
func TestFoldFixture(t *testing.T) {
	th, hl, looks := foldTheme(t) // factor the theme setup out of TestSpansDrawWhatTheTUIPaints
	_ = th
	type fcase struct {
		Spans []Span
		Tags  map[string]theme.TagLook
		Want  []ansi.SpanStyle
	}
	var cases []fcase
	for _, c := range spanCases() { // factor the case table out too
		sp := spansOf(c.text, hl, c.tags)
		var want []ansi.SpanStyle
		for _, s := range sp {
			want = append(want, fold(s, looks))
		}
		cases = append(cases, fcase{sp, looks, want})
	}
	got, _ := json.MarshalIndent(cases, "", " ")
	path := "../../web/ui/src/testdata/fold.json"
	if *updateFold {
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, append(got, '\n'), 0o644)
	}
	was, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(was, append(got, '\n')) {
		t.Errorf("%s is stale: go test ./internal/webapi -run TestFoldFixture -update", path)
	}
}
```

Run without `-update` → FAIL (no file); run with `-update` → writes it; run again → PASS.

- [ ] **Step 2: Write the failing TS tests** (`theme.test.ts`):

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import fixture from "./testdata/fold.json" with { type: "json" };
import { color, fold, lookCSS, macchiato, latte, roleClass, themeCSS } from "./theme.ts";
import type { ThemeEvent } from "./types.ts";

test("the fold matches Go's, case by case", () => {
  for (const c of fixture as any[]) {
    c.Spans.forEach((s: any, i: number) => {
      const want = c.Want[i];
      const got = fold(s, c.Tags);
      for (const k of ["FG", "BG", "Bold", "Faint", "Italic", "Underline", "Reverse"] as const) {
        assert.equal(got[k] || undefined, want[k] || undefined, `${JSON.stringify(s)} ${k}`);
      }
    });
  }
});

test("colors resolve against the palette", () => {
  assert.equal(color("ansi:1", macchiato), macchiato.ansi[1]);
  assert.equal(color("#123456", macchiato), "#123456");
  assert.equal(color("", macchiato), undefined);
  assert.equal(macchiato.ansi.length, 16);
  assert.equal(latte.ansi.length, 16);
});

test("reverse swaps, defaulting to the terminal's colors", () => {
  const css = lookCSS({ FG: "ansi:2", Reverse: true }, macchiato);
  assert.match(css, new RegExp(`color:${macchiato.bg}`));
  assert.match(css, new RegExp(`background:${macchiato.ansi[2]}`));
});

const ev = (appearance: ThemeEvent["Appearance"]): ThemeEvent => ({
  type: "Theme", Appearance: appearance,
  Dark: { Roles: { "sidebar.active": { FG: "#111111", Bold: true } }, Tags: {} },
  Light: { Roles: { "sidebar.active": { FG: "#eeeeee" } }, Tags: {} },
});

test("auto follows the system; a forced appearance doesn't", () => {
  const auto = themeCSS(ev("auto"));
  assert.match(auto, /@media \(prefers-color-scheme: light\)/);
  assert.ok(auto.includes(`.${roleClass("sidebar.active")}{`));
  assert.ok(auto.includes("#111111") && auto.includes("#eeeeee"));
  const light = themeCSS(ev("light"));
  assert.ok(!light.includes("@media") && light.includes("#eeeeee") && !light.includes("#111111"));
  const dark = themeCSS(ev("dark"));
  assert.ok(!dark.includes("@media") && dark.includes("#111111") && !dark.includes("#eeeeee"));
});

test("the sixteen and the default colors are variables", () => {
  const css = themeCSS(ev("dark"));
  assert.ok(css.includes(`--ansi-1:${macchiato.ansi[1]}`));
  assert.ok(css.includes(`--term-fg:${macchiato.fg}`) && css.includes(`--term-bg:${macchiato.bg}`));
});
```

Run `(cd web/ui && npm test)` → FAIL (no `theme.ts`).

- [ ] **Step 3: Implement `theme.ts`.**

```ts
import type { Color, Look, Span, TagLook, ThemeEvent, Looks } from "./types.ts";

export interface Palette { fg: string; bg: string; ansi: string[] }

// The terminal's colors on the web: Catppuccin Macchiato (dark) and Latte
// (light), from Catppuccin's own terminal ports. Normal and bright differ
// only in black and white.
export const macchiato: Palette = {
  fg: "#cad3f5", bg: "#24273a",
  ansi: ["#494d64", "#ed8796", "#a6da95", "#eed49f", "#8aadf4", "#f5bde6", "#8bd5ca", "#b8c0e0",
         "#5b6078", "#ed8796", "#a6da95", "#eed49f", "#8aadf4", "#f5bde6", "#8bd5ca", "#a5adcb"],
};
export const latte: Palette = {
  fg: "#4c4f69", bg: "#eff1f5",
  ansi: ["#bcc0cc", "#d20f39", "#40a02b", "#df8e1d", "#1e66f5", "#ea76cb", "#179299", "#5c5f77",
         "#acb0be", "#d20f39", "#40a02b", "#df8e1d", "#1e66f5", "#ea76cb", "#179299", "#6c6f85"],
};

export function color(c: Color | undefined, p: Palette): string | undefined {
  if (!c) return undefined;
  const m = /^ansi:(\d+)$/.exec(c);
  return m ? p.ansi[Number(m[1])] : c;
}

// lookCSS is l as CSS declarations. Reverse swaps foreground and
// background, each falling back to the terminal's own.
export function lookCSS(l: Look, p: Palette): string {
  let fg = color(l.FG, p), bg = color(l.BG, p);
  if (l.Reverse) [fg, bg] = [bg ?? p.bg, fg ?? p.fg];
  const d: string[] = [];
  if (fg) d.push(`color:${fg}`);
  if (bg) d.push(`background:${bg}`);
  if (l.Bold) d.push("font-weight:700");
  if (l.Faint) d.push("opacity:.6");
  if (l.Italic) d.push("font-style:italic");
  if (l.Underline) d.push("text-decoration:underline");
  return d.join(";");
}

export const roleClass = (role: string): string => "r-" + role.replaceAll(".", "-").replaceAll("_", "-");

function sheet(l: Looks, p: Palette): string {
  const vars = [`--term-fg:${p.fg}`, `--term-bg:${p.bg}`, ...p.ansi.map((c, i) => `--ansi-${i}:${c}`)];
  let css = `:root{${vars.join(";")}}body{color:var(--term-fg);background:var(--term-bg)}`;
  for (const [role, look] of Object.entries(l.Roles)) css += `.${roleClass(role)}{${lookCSS(look, p)}}`;
  return css;
}

// themeCSS is t as a stylesheet: dark unless the setting or (with auto)
// the system says light.
export function themeCSS(t: ThemeEvent): string {
  if (t.Appearance === "light") return sheet(t.Light, latte);
  if (t.Appearance === "dark") return sheet(t.Dark, macchiato);
  return sheet(t.Dark, macchiato) + `@media (prefers-color-scheme: light){${sheet(t.Light, latte)}}`;
}

// fold is a span's look: the server's style, then each named tag's style
// in order; a set color replaces, flags add up (theme.Style.Over).
export function fold(s: Span, tags: Record<string, TagLook>): Look {
  const out: Look = { FG: s.FG, BG: s.BG, Bold: s.Bold, Faint: s.Faint, Italic: s.Italic, Underline: s.Underline, Reverse: s.Reverse };
  for (const n of s.Tags ?? []) {
    const l = tags[n];
    if (!l) continue;
    if (l.FG) out.FG = l.FG;
    if (l.BG) out.BG = l.BG;
    for (const k of ["Bold", "Faint", "Italic", "Underline", "Reverse"] as const) out[k] = out[k] || l[k];
  }
  return out;
}
```

(The faint opacity is the one styling choice here: terminals dim; `opacity:.6` is the web's nearest. The `.6` is a number, not a color.)

- [ ] **Step 4: Run** `(cd web/ui && npm run check && npm test)` and `go test ./internal/webapi` → PASS.

- [ ] **Step 5: Commit** `web/ui: the theme as a stylesheet, and a fold checked against Go's`.

### Task 3: the store

**Files:** Create `web/ui/src/store.ts`, `web/ui/src/store.test.ts`; extend `web/ui/src/types.ts`

**Interfaces:**
- Produces:

```ts
export type Phase = "locked" | "loading" | "running" | "stopped";
export interface LineData { Seq: number; Kind: "server"|"echo"|"sys"|"day"|"history_end"; Day: string; DayLabel?: string; Clock?: string; Out: boolean; Plain: string; Spans: Span[] | null; Attention: boolean; Quiet: boolean }
export interface Lines { epoch: number; lines: LineData[] } // in Seq order
export interface State {
  phase: Phase; progress?: { got: number; total?: number }; saving: boolean;
  theme?: ThemeEvent; order: string[]; worlds: any[]; active: string;
  chars: Record<string, any>; inputs: Record<string, any>;
  lines: Record<string, Lines>; logs: Record<string, any>; logLines: Record<string, Lines>; filters: Record<string, any>;
  status?: { Text: string; Err: boolean; Gen: number }; presence?: number;
}
export class Store {
  state: State;
  constructor(onEffect?: (e: any) => void);
  apply(batch: any[]): void;              // one notify per batch; never throws
  set(patch: Partial<State>): void;       // page-side state (phase, progress, saving); notifies
  subscribe(fn: () => void): () => void;
}
```

- [ ] **Step 1: Write the failing tests** (`store.test.ts`):

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { Store } from "./store.ts";

const line = (Seq: number, Plain = `l${Seq}`) => ({ Seq, Kind: "server", Day: "", Out: false, Plain, Spans: [{ Text: Plain }], Attention: false, Quiet: false });

test("a batch is one update", () => {
  const s = new Store();
  let n = 0;
  s.subscribe(() => n++);
  s.apply([{ type: "Order", Keys: ["fm/kit"] }, { type: "Char", Key: "fm/kit", Name: "Kit" }, { type: "Active", Key: "fm/kit" }]);
  assert.equal(n, 1);
  assert.deepEqual(s.state.order, ["fm/kit"]);
  assert.equal(s.state.chars["fm/kit"].Name, "Kit");
  assert.equal(s.state.active, "fm/kit");
});

test("Start drops everything the page held", () => {
  const s = new Store();
  s.apply([{ type: "Char", Key: "fm/kit" }, { type: "Lines", Key: "fm/kit", Epoch: 1, Op: "reset", Lines: [line(0)] },
           { type: "Log", Key: "fm/kit", Open: true }, { type: "Status", Text: "x", Err: false, Gen: 1 }]);
  s.apply([{ type: "Start" }, { type: "Order", Keys: [] }]);
  assert.deepEqual(s.state.chars, {});
  assert.deepEqual(s.state.lines, {});
  assert.deepEqual(s.state.logs, {});
  assert.equal(s.state.status, undefined);
});

test("lines append, prepend, replace and reset within an epoch", () => {
  const s = new Store();
  const L = (Op: string, Epoch: number, seqs: number[]) => ({ type: "Lines", Key: "k", Epoch, Op, Lines: seqs.map((q) => line(q)) });
  s.apply([L("reset", 1, [0, 1])]);
  s.apply([L("append", 1, [2])]);
  s.apply([L("prepend", 1, [-2, -1])]);
  assert.deepEqual(s.state.lines.k.lines.map((l) => l.Seq), [-2, -1, 0, 1, 2]);
  s.apply([{ type: "Lines", Key: "k", Epoch: 1, Op: "replace", Lines: [line(-2, "a"), line(-1, "b"), line(0, "c"), line(1, "d"), line(2, "e")] }]);
  assert.deepEqual(s.state.lines.k.lines.map((l) => l.Plain), ["a", "b", "c", "d", "e"]);
  s.apply([L("append", 9, [3])]); // another epoch, not a reset: ignored
  assert.deepEqual(s.state.lines.k.lines.map((l) => l.Seq), [-2, -1, 0, 1, 2]);
  s.apply([L("reset", 2, [7])]);
  assert.deepEqual(s.state.lines.k, { epoch: 2, lines: [line(7)] });
});

test("Closed drops a character's things", () => {
  const s = new Store();
  s.apply([{ type: "Char", Key: "k" }, { type: "Lines", Key: "k", Epoch: 1, Op: "reset", Lines: [line(0)] }, { type: "Input", Key: "k", Text: "hi" }]);
  s.apply([{ type: "Closed", Key: "k" }]);
  assert.equal(s.state.chars.k, undefined);
  assert.equal(s.state.lines.k, undefined);
  assert.equal(s.state.inputs.k, undefined);
});

test("a bad event doesn't stop the batch or reach the caller", () => {
  const s = new Store();
  assert.doesNotThrow(() => s.apply([{ type: "Lines", Key: "k" } /* no Lines array */, { type: "Active", Key: "k" }]));
  assert.equal(s.state.active, "k");
});

test("effects go to the handler, in order", () => {
  const got: string[] = [];
  const s = new Store((e) => got.push(e.effect));
  s.apply([{ type: "Effect", effect: "Copy", Text: "x" }, { type: "Effect", effect: "Notify", Title: "Kit", Body: "hi" }]);
  assert.deepEqual(got, ["Copy", "Notify"]);
});

test("the theme is kept", () => {
  const s = new Store();
  s.apply([{ type: "Theme", Appearance: "auto", Dark: { Roles: {}, Tags: {} }, Light: { Roles: {}, Tags: {} } }]);
  assert.equal(s.state.theme?.Appearance, "auto");
});
```

Run → FAIL.

- [ ] **Step 2: Implement `store.ts`.** `apply` copies `state` shallowly once, then for each event (inside its own `try`/`catch` that `console.error`s and continues) updates the copy: `Start` → fresh state keeping `phase`, `progress`, `saving`; `Theme`; `Worlds`; `Order` (`Keys ?? []`); `Char`/`Input`/`Log`/`Filter` by `Key` into their maps; `Closed` deletes the key from `chars`, `inputs`, `lines`, `logs`, `logLines`, `filters`; `Active`; `Status`; `Presence`; `Confirm`/`SaveQuestion` stored as `confirm`/`saveQuestion` (add to `State`); `Lines`/`LogLines` through one `merge(prev: Lines | undefined, e): Lines | undefined`:

```ts
function merge(prev: Lines | undefined, e: { Epoch: number; Op: string; Lines: LineData[] | null }): Lines | undefined {
  const ls = e.Lines ?? [];
  if (e.Op === "reset") return { epoch: e.Epoch, lines: ls };
  if (!prev || prev.epoch !== e.Epoch) { console.error("lines for an epoch the page doesn't hold", e); return prev; }
  switch (e.Op) {
    case "append": return { epoch: prev.epoch, lines: [...prev.lines, ...ls] };
    case "prepend": return { epoch: prev.epoch, lines: [...ls, ...prev.lines] };
    case "replace": return { epoch: prev.epoch, lines: ls };
  }
  return prev;
}
```

`Effect` events go to `onEffect` after the state update, in order. Then `this.state = next` and every subscriber is called once (each in its own `try`). `set(patch)` merges and notifies.

- [ ] **Step 3: Run** `(cd web/ui && npm run check && npm test)` → PASS. **Commit** `web/ui: one store, a batch at a time`.

### Task 4: boot

**Files:** Create `web/ui/src/boot.ts`, `web/ui/src/boot.test.ts`; modify `internal/str/locales/en.toml` (`web.loading`, `web.loading_size`, `web.stopped`, `web.reload`), `web/ui/src/str.gen.ts` (generated)

**Interfaces:**
- Produces:

```ts
export async function readWithProgress(res: Response, onProgress: (got: number, total?: number) => void): Promise<ArrayBuffer>;
export function loadingText(got: number, total?: number): string; // the status line's words
export async function boot(store: Store): Promise<void>; // the old kiln.js, driving store.set({phase, progress, saving})
```

- [ ] **Step 1: Catalog.** Under `[web]`: `loading = "loading Kiln… {got} MB"`, `loading_size = "loading Kiln… {got}/{total} MB"`, `stopped = "Kiln stopped."`, `reload = "Reload"`. `go generate ./internal/str`.

- [ ] **Step 2: Write the failing tests** (`boot.test.ts`):

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { loadingText, readWithProgress } from "./boot.ts";
import * as str from "./str.gen.ts";

const res = (chunks: number[], length?: number) => new Response(new ReadableStream({
  start(c) { for (const n of chunks) c.enqueue(new Uint8Array(n)); c.close(); },
}), { headers: length === undefined ? {} : { "content-length": String(length) } });

test("progress counts every byte against the length", async () => {
  const seen: [number, number | undefined][] = [];
  const buf = await readWithProgress(res([3, 4], 7), (g, t) => seen.push([g, t]));
  assert.equal(buf.byteLength, 7);
  assert.deepEqual(seen.at(-1), [7, 7]);
});

test("no length: progress without a total", async () => {
  const seen: [number, number | undefined][] = [];
  await readWithProgress(res([5]), (g, t) => seen.push([g, t]));
  assert.deepEqual(seen.at(-1), [5, undefined]);
});

test("the words, in megabytes to one place", () => {
  assert.equal(loadingText(2_100_000, 6_400_000), str.webLoadingSize("2.1", "6.4"));
  assert.equal(loadingText(2_100_000), str.webLoading("2.1"));
  assert.ok(!loadingText(0, 0).includes("NaN"));
});
```

(Check the generated parameter order for `webLoadingSize` in `str.gen.ts` and match it.) Run → FAIL.

- [ ] **Step 3: Implement `boot.ts`.** `readWithProgress` reads `res.body` with a reader, summing `value.byteLength`, calling `onProgress(got, total)` after each chunk (`total` from `content-length` when it parses to a positive number, else `undefined`), then concatenates into one `ArrayBuffer`. `loadingText` formats `(n / 1e6).toFixed(1)`. `boot(store)` is `web/static/kiln.js` from the tab lock to `go.run`, ported to TS with these changes:
  - imports `createFS`, `createPersist`, `idbStore`, `createTabLock` from `../../static/*.mjs` (declare their types loosely in a `src/static.d.ts`: `declare module "*.mjs";` is enough with `allowJs`);
  - the lock screen: `store.set({ phase: "locked" })`, and the `Use here` button (rendered by `Scrollback`, Task 5) calls a `takeOver()` the store exposes — put `onUseHere?: () => void` in `State` and set it here;
  - `saving` goes to `store.set({ saving })`;
  - `kilnDownload` stays as a function, used by the effect handler for `SaveFile` (`main.tsx` wires `new Store(effect)`); `kilnPickFile` is dropped (nothing calls it since part 2; `/restore` comes back in step 3 with the page's own file picker);
  - the wasm is fetched with `readWithProgress`, `store.set({ phase: "loading", progress })` per chunk;
  - `kilnReady` starts Kiln with `onBatch = (json) => { try { store.apply(JSON.parse(json)) } catch (e) { console.error(e) } }` and sets `phase: "running"`;
  - after `await go.run(instance)` returns (Go exited: `/quit` or a start-up error), `store.set({ phase: "stopped" })`;
  - `kilnPage = { act }`, plus `events` (every event pushed in `onBatch`) only when `new URLSearchParams(location.search).has("record")`;
  - `saveDrafts` on leaving: without a page input yet, `act({ Do: "save_drafts" })` only (step 3 sends pending input first).

- [ ] **Step 4: Run** `(cd web/ui && npm run check && npm test)` → PASS. **Commit** `web/ui: boot, with download progress and Kiln stopping` with `Strings: web.loading, web.loading_size, web.stopped, web.reload (new)`.

### Task 5: the five areas

**Files:** Create `web/ui/src/ui/{App,TopBar,Sidebar,Scrollback,BottomPane,StatusLine}.tsx`, `web/ui/src/ui/use.ts` (the store hook), `web/ui/src/ui/App.test.tsx`; modify `web/ui/src/kiln.css`, `web/ui/src/main.tsx`, `internal/str/locales/en.toml` (`web.characters`), `web/ui/src/str.gen.ts`

**Interfaces:**
- Consumes: `Store`, `State` (Task 3), `themeCSS`, `roleClass` (Task 2), `loadingText` (Task 4).
- Produces: `App({ store }: { store: Store })`; `useStore(store)` returns the current `State` and re-renders on change.

- [ ] **Step 1: Write the failing tests** (`App.test.tsx`, rendering with `preact-render-to-string`):

```tsx
import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { Store } from "../store.ts";
import { loadingText } from "../boot.ts";
import * as str from "../str.gen.ts";

const page = (patch: object = {}, batch: any[] = []) => {
  const s = new Store();
  s.set(patch as any);
  s.apply(batch);
  return render(<App store={s} />);
};

test("five areas, with landmarks", () => {
  const html = page({ phase: "running" });
  for (const want of ["<header", "<nav", "<main", 'role="log"', 'class="bottom', 'role="status"']) assert.ok(html.includes(want), want);
  assert.ok(html.includes(`aria-label="${str.webCharacters()}"`));
});

test("loading shows progress in the status line", () => {
  assert.ok(page({ phase: "loading", progress: { got: 2_100_000, total: 6_400_000 } }).includes(loadingText(2_100_000, 6_400_000)));
});

test("another tab: the message and Use here, in the scrollback", () => {
  const html = page({ phase: "locked" });
  assert.ok(html.includes(str.webOtherTab()) && html.includes(`>${str.webUseHere()}</button>`));
});

test("stopped: an alert and Reload", () => {
  const html = page({ phase: "stopped" });
  assert.ok(html.includes('role="alert"') && html.includes(str.webStopped()) && html.includes(`>${str.webReload()}</button>`));
});

test("a status error is an alert; otherwise not saving stays", () => {
  assert.ok(page({ phase: "running" }, [{ type: "Status", Text: "boom", Err: true, Gen: 1 }]).match(/role="alert"[^>]*>[^<]*boom/));
  assert.ok(page({ phase: "running", saving: false }).includes(str.webNotSaving()));
});

test("the theme goes into the page's style", () => {
  const html = page({ phase: "running" }, [{ type: "Theme", Appearance: "dark", Dark: { Roles: { status: { FG: "#123456" } }, Tags: {} }, Light: { Roles: {}, Tags: {} } }]);
  assert.ok(html.includes("#123456"));
});
```

(`"boom"` and `"#123456"` are test data, not catalog text.) Run → FAIL.

- [ ] **Step 2: Implement.**
  - `use.ts`: `useStore(store)` with `useState` + `useEffect(() => store.subscribe(() => set(store.state)), [store])`.
  - `App.tsx`: `<style>{theme ? themeCSS(theme) : ""}</style>` then a grid `div.kiln` with `<Sidebar>`, `<div class="divider r-divider">`, `<TopBar>`, `<Scrollback>`, `<BottomPane>`, `<StatusLine>`. (The `<style>` inside the app replaces `#kiln-theme` in `index.html`; drop that tag from the template.)
  - `TopBar`: `<header class="top r-status">` (empty for now).
  - `Sidebar`: `<nav class="side r-sidebar" aria-label={str.webCharacters()}>` (empty list for now). Catalog: `[web] characters = "Characters"`.
  - `Scrollback`: `<main class="body" role="log">`; with `phase === "locked"`, a `<p>` with `str.webOtherTab()` and `<button onClick={state.onUseHere}>{str.webUseHere()}</button>`.
  - `BottomPane`: `<div class="bottom r-input">` (empty).
  - `StatusLine`: `<div class="status r-status">` containing exactly one of: stopped → `<span role="alert" class="r-status-error">{str.webStopped()}</span> <button onClick={() => location.reload()}>{str.webReload()}</button>`; loading → `<span role="status">{loadingText(...)}</span>`; a status with `Err` → `<span role="alert" class="r-status-error">`; a status → `<span role="status">`; `saving === false` → `<span role="status" class="r-status-error">{str.webNotSaving()}</span>`; else an empty `<span role="status">` (so the live region exists before anything is said).
  - `kiln.css` layout:

```css
html, body { margin: 0; height: 100%; }
body { font: 1rem/1.25 "JetBrains Mono", monospace; font-variant-ligatures: none; }
.kiln { display: grid; height: 100dvh; grid-template-columns: 16ch 1px 1fr;
  grid-template-rows: 1lh 1fr auto 1lh;
  grid-template-areas: "side div top" "side div body" "side div bottom" "side div status"; }
.side { grid-area: side; overflow-y: auto; }
.divider { grid-area: div; background: currentColor; }
.top { grid-area: top; white-space: pre; overflow: hidden; border-bottom: 1px solid; }
.body { grid-area: body; overflow-y: auto; min-height: 0; }
.bottom { grid-area: bottom; max-height: 50dvh; overflow-y: auto; border-top: 1px solid; }
.status { grid-area: status; white-space: pre; overflow: hidden; border-top: 1px solid; }
button { font: inherit; color: inherit; background: none; border: 0; padding: 0 1ch; cursor: pointer; }
```

    The rules' colors: in `themeCSS`, also emit `--role-<name>-fg` variables for `rule`, `rule.input`, `rule.form`, `rule.status` and `divider` (each role's `FG` resolved, falling back to `--term-fg`), and in `kiln.css` use `border-bottom-color: var(--role-rule-fg)` on `.top`, `border-top-color: var(--role-rule-input-fg)` on `.bottom`, `border-top-color: var(--role-rule-status-fg)` on `.status`, and `background: var(--role-divider-fg)` on `.divider` (replacing `currentColor` above). Add a `theme.test.ts` case: `themeCSS` output includes `--role-rule-status-fg:`.
  - `main.tsx`: `import "./kiln.css"`; make the `Store` with the effect handler (`SaveFile` → `kilnDownload`; others ignored for now), `render(<App store={store} />, document.getElementById("kiln")!)`, then `boot(store)`.

- [ ] **Step 3: Run** `(cd web/ui && npm run check && npm test)` → PASS. **Commit** `web/ui: the five areas and the states around them` with `Strings: web.characters (new)`.

### Task 6: no stray text in the page

**Files:** Create `web/ui/src/lint.test.ts`

- [ ] **Step 1: Write the test** — scan every `src/ui/*.tsx`: fail on JSX text containing a letter (`/>\s*[^<>{}\s][^<>{}]*</` with a letter in the match) and on `aria-label=`, `title=`, `placeholder=`, `alt=` given a quoted literal (`/\b(aria-label|title|placeholder|alt)="/`). A line ending in `// str:ok` is exempt. Message: `"<file>:<line>: text for people goes in the catalog (internal/str/locales/en.toml) and comes from str.*"`.
- [ ] **Step 2: Prove it bites:** add `<p>hello</p>` to `TopBar.tsx`, run `npm test` → FAIL naming TopBar; remove it.
- [ ] **Step 3: Run** → PASS. **Commit** `web/ui: a lint for text that skipped the catalog`.

### Task 7: the real page replaces the development page

**Files:** Delete `web/static/kiln.js`, `web/static/index.html` (tracked), `web/static/strings.json`; modify `internal/str/catalog/generate.go` (drop `WebJSON`), `internal/str/catalog/generate_test.go` (drop `TestWebJSON`), `internal/str/gen/main.go`, `internal/str/str_test.go`, `web/build.sh`, `web/smoke/smoke_test.go`, `docs/web-dev.md`

- [ ] **Step 1: Smoke first.** In `smoke_test.go`, `buildPage` stops copying `web/static`; it runs `node build.mjs <dir>` in `web/ui` (skipping with a message if `web/ui/node_modules` is missing: "run npm ci in web/ui"), then builds the wasm and copies `wasm_exec.js` as now. `openTab` opens `page + "?record"`. Add, after the echo:

```go
// Every glyph Kiln draws is one cell wide in the bundled font.
waitFor(t, ev, `document.fonts.status === "loaded"`, "the font")
ev(`(() => { const c = document.createElement("canvas").getContext("2d"); c.font = '16px "JetBrains Mono"';
  const w = c.measureText("M").width;
  const bad = ["›","*","─","▼","►","•","×","▌","░","●","◄","+","≡"].filter(g => Math.abs(c.measureText(g).width - w) > 0.01);
  window.kilnGlyphs = bad.join(""); })()`)
waitFor(t, ev, `window.kilnGlyphs === ""`, "every glyph one cell wide")
```

(Build the JS glyph list in Go from `glyph.All` and splice it into the expression with `json.Marshal`, so it can't drift from `internal/glyph`.) Also check `document.fonts.check('16px "JetBrains Mono"')`. Run `KILN_SMOKE=1 go test ./smoke` → FAIL (page not built that way yet / dev page has no fonts).

- [ ] **Step 2: Delete the dev page** (`git rm web/static/kiln.js web/static/index.html web/static/strings.json`), `WebJSON`, `TestWebJSON`, the generator's JSON step and the staleness check for it. `web/build.sh`: after the wasm, `cd ui && { [ -d node_modules ] || npm ci; } && npm run check && node build.mjs ../static`.

- [ ] **Step 3: Docs.** `docs/web-dev.md`: the page is `web/ui` (what each file is for, in a short list), `?record` for the smoke test, and that `web/static` now holds only the boot modules and build output.

- [ ] **Step 4: Run everything:** the Global Constraints list, plus `(cd web && KILN_SMOKE=1 go test ./smoke -v)` → PASS. Open the page by hand (`docs/web-dev.md`'s dev command) and look: five areas, the font, the theme. Screenshot it for the PR (headless Chrome via the smoke test's DevTools helper, or by hand).

- [ ] **Step 5: Commit** `web: the real page replaces the development page`.

### Task 8: wrap up

- [ ] Fresh whole-branch review with this plan's Review Focus; fix Critical/Important with a failing test first; PR listing the new strings (`web.needs_js`, `web.loading`, `web.loading_size`, `web.stopped`, `web.reload`, `web.characters`) and why each exists, the Rulings, and the screenshot.
