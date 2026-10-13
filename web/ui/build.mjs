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
