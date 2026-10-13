import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import * as str from "./str.gen.ts";

// The tests run from web/ui (test.mjs sets the directory).
test("build writes the page, its styles and its fonts", () => {
  const out = mkdtempSync(join(tmpdir(), "kiln-ui-"));
  execFileSync("node", ["build.mjs", out], { cwd: process.cwd() });
  for (const f of ["index.html", "app.js", "app.css", "fonts/JetBrainsMono-Regular.woff2", "fonts/OFL.txt"]) {
    assert.ok(existsSync(join(out, f)), f);
  }
  const html = readFileSync(join(out, "index.html"), "utf8");
  assert.ok(html.includes(str.webNeedsJs()));
  assert.ok(!html.includes("{{"), "a template slot left unfilled");
});
