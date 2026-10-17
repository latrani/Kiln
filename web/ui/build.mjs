// Builds the page into an output directory (default ../static): app.js
// and app.css from src/main.tsx, sw.js from src/sw.ts, the fonts and their license, and
// index.html from its template with the catalog's strings filled in.
// With golden after the directory, the golden page (src/golden.tsx)
// instead, for web/smoke's screenshots: never shipped.
import * as esbuild from "esbuild";
import { copyFile, mkdir, readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { join, resolve } from "node:path";
import * as str from "./src/str.gen.ts";
import { fill } from "./src/template.ts";

const here = fileURLToPath(new URL(".", import.meta.url));
const out = resolve(here, process.argv[2] ?? "../static");
const golden = process.argv[3] === "golden";
await mkdir(join(out, "fonts"), { recursive: true });
await esbuild.build({
  absWorkingDir: here,
  entryPoints: [golden ? "src/golden.tsx" : "src/main.tsx"],
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
// The service worker (push only), at the page's level so its scope is
// the page's.
await esbuild.build({
  absWorkingDir: here,
  entryPoints: ["src/sw.ts"],
  bundle: true,
  format: "iife",
  outfile: join(out, "sw.js"),
  minify: true,
  target: ["es2022"],
  logLevel: "warning",
});
await copyFile(join(here, "fonts", "OFL.txt"), join(out, "fonts", "OFL.txt"));
await writeFile(join(out, "index.html"), fill(await readFile(join(here, "index.html"), "utf8"), { needsJs: str.webNeedsJs() }));
