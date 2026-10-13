// Runs every src/**/*.test.ts(x): esbuild bundles each (JSX and all)
// into .test-out, then node --test runs them from here.
import * as esbuild from "esbuild";
import { spawnSync } from "node:child_process";
import { readdirSync, rmSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

const here = fileURLToPath(new URL(".", import.meta.url));
const tests = readdirSync(join(here, "src"), { recursive: true })
  .filter((f) => /\.test\.tsx?$/.test(f))
  .map((f) => join("src", f));
const out = join(here, ".test-out");
rmSync(out, { recursive: true, force: true });
await esbuild.build({
  absWorkingDir: here, entryPoints: tests, bundle: true, platform: "node", format: "esm",
  outdir: out, outExtension: { ".js": ".mjs" }, jsx: "automatic", jsxImportSource: "preact",
  packages: "external", loader: { ".woff2": "empty", ".css": "empty" }, logLevel: "warning",
});
const files = readdirSync(out, { recursive: true }).filter((f) => f.endsWith(".mjs")).map((f) => join(out, f));
const r = spawnSync(process.execPath, ["--test", ...files], { stdio: "inherit", cwd: here });
process.exit(r.status ?? 1);
