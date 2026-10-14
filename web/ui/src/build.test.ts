import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import * as str from "./str.gen.ts";

// The tests run from web/ui (test.mjs sets the directory).
const ui = process.cwd();

test("build writes the page, its styles and its fonts", (t) => {
  const out = mkdtempSync(join(tmpdir(), "kiln-ui-"));
  t.after(() => rmSync(out, { recursive: true, force: true }));
  execFileSync(process.execPath, ["build.mjs", out], { cwd: ui });
  for (const f of ["index.html", "app.js", "app.css", "fonts/JetBrainsMono-Regular.woff2", "fonts/OFL.txt"]) {
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
