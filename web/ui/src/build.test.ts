import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import * as str from "./str.gen.ts";
import { terminalDark, terminalLight } from "./terminal.gen.ts";

// The tests run from web/ui (test.mjs sets the directory).
const ui = process.cwd();

test("build writes the page, its styles and its fonts", (t) => {
  const out = mkdtempSync(join(tmpdir(), "kiln-ui-"));
  t.after(() => rmSync(out, { recursive: true, force: true }));
  execFileSync(process.execPath, ["build.mjs", out], { cwd: ui });
  for (const f of ["index.html", "app.js", "app.css", "sw.js", "manifest.json", "icon.svg", "icon-192.png", "icon-512.png", "apple-touch-icon.png", "fonts/JetBrainsMono-Regular.woff2", "fonts/OFL.txt"]) {
    assert.ok(existsSync(join(out, f)), f);
  }
  const html = readFileSync(join(out, "index.html"), "utf8");
  assert.ok(html.includes(str.webNeedsJs()));
  assert.ok(!html.includes("{{"), "a template slot left unfilled");
});

test("build works from a folder with a space in its name", (t) => {
  const dir = mkdtempSync(join(tmpdir(), "kiln ui "));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  symlinkSync(ui, join(dir, "my ui"));
  const out = join(dir, "out");
  execFileSync(process.execPath, ["--preserve-symlinks", "--preserve-symlinks-main", join(dir, "my ui", "build.mjs"), out], { cwd: dir, stdio: "pipe" });
  assert.ok(existsSync(join(out, "app.js")));
});

test("the golden build has the WebSocket bridge; the page's build doesn't", (t) => {
  const page = mkdtempSync(join(tmpdir(), "kiln-ui-"));
  const golden = mkdtempSync(join(tmpdir(), "kiln-ui-"));
  t.after(() => { rmSync(page, { recursive: true, force: true }); rmSync(golden, { recursive: true, force: true }); });
  execFileSync(process.execPath, ["build.mjs", page], { cwd: ui });
  execFileSync(process.execPath, ["build.mjs", golden, "golden"], { cwd: ui });
  assert.ok(!readFileSync(join(page, "app.js"), "utf8").includes("WebSocket"), "the page talks to Go only through the wasm");
  assert.ok(readFileSync(join(golden, "app.js"), "utf8").includes("WebSocket"));
});

// built is the page built once into a temporary directory.
function built(t: { after(f: () => void): void }): string {
  const out = mkdtempSync(join(tmpdir(), "kiln-ui-"));
  t.after(() => rmSync(out, { recursive: true, force: true }));
  execFileSync(process.execPath, ["build.mjs", out], { cwd: ui });
  return out;
}

test("the manifest names Kiln and its icons", (t) => {
  const out = built(t);
  const m = JSON.parse(readFileSync(join(out, "manifest.json"), "utf8"));
  assert.equal(m.name, str.webAppName());
  assert.equal(m.short_name, str.webAppName());
  assert.equal(m.display, "standalone");
  assert.equal(m.start_url, "./");
  assert.equal(m.scope, "./");
  assert.equal(m.theme_color, terminalDark.bg);
  assert.equal(m.background_color, terminalDark.bg);
  assert.deepEqual(m.icons, [
    { src: "icon.svg", type: "image/svg+xml", sizes: "any", purpose: "any" },
    { src: "icon-192.png", type: "image/png", sizes: "192x192", purpose: "any" },
    { src: "icon-512.png", type: "image/png", sizes: "512x512", purpose: "any" },
  ]);
  for (const i of m.icons) assert.ok(existsSync(join(out, i.src)), i.src);
});

test("the PNGs are the sizes they say", (t) => {
  const out = built(t);
  for (const [f, n] of [["icon-192.png", 192], ["icon-512.png", 512], ["apple-touch-icon.png", 180]] as const) {
    const b = readFileSync(join(out, f));
    assert.equal(b.toString("latin1", 12, 16), "IHDR", f);
    assert.deepEqual([b.readUInt32BE(16), b.readUInt32BE(20)], [n, n], f);
  }
});

test("index.html links the manifest and icons, and takes its title from the catalog", (t) => {
  const html = readFileSync(join(built(t), "index.html"), "utf8");
  for (const want of [
    `<link rel="manifest" href="manifest.json">`,
    `<link rel="icon" href="icon.svg" type="image/svg+xml">`,
    `<link rel="apple-touch-icon" href="apple-touch-icon.png">`,
    `<meta name="theme-color" content="${terminalDark.bg}" media="(prefers-color-scheme: dark)">`,
    `<meta name="theme-color" content="${terminalLight.bg}" media="(prefers-color-scheme: light)">`,
    `<title>${str.webTitle()}</title>`,
  ]) {
    assert.ok(html.includes(want), want);
  }
  assert.ok(!html.includes("data:,"), "the empty favicon is gone");
});
