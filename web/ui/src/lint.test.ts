import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

// Text people read comes from the catalog (internal/str/locales/en.toml)
// through str.*, as in Go. This flags JSX text with a letter in it, and
// labels given a quoted literal. A line ending in "// str:ok" is exempt.
const dir = join(process.cwd(), "src", "ui");
const jsxText = />\s*([^<>{}]*\p{L}[^<>{}]*)</u;
const label = /\b(aria-label|title|placeholder|alt)="/;

test("no stray text in the page", () => {
  const found: string[] = [];
  for (const f of readdirSync(dir).filter((f) => f.endsWith(".tsx") && !f.endsWith(".test.tsx"))) {
    readFileSync(join(dir, f), "utf8").split("\n").forEach((line, i) => {
      if (line.trimEnd().endsWith("// str:ok") || line.trimStart().startsWith("//")) return;
      if (jsxText.test(line) || label.test(line)) found.push(`${f}:${i + 1}: ${line.trim()}`);
    });
  }
  assert.deepEqual(found, [], "text for people goes in the catalog (internal/str/locales/en.toml) and comes from str.*");
});
