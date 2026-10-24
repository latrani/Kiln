// Builds the page into an output directory (default ../static): app.js
// and app.css from src/main.tsx, sw.js from src/sw.ts, the icons and
// manifest.json, the fonts and their license, and
// index.html from its template with the catalog's strings filled in.
// With golden after the directory, the golden page (src/golden.tsx)
// instead, for web/smoke's screenshots: never shipped. With desktop, the
// desktop app's page (src/desktop.tsx), for desktop/frontend/dist: no
// service worker, icons, manifest or source map, which a Wails window
// doesn't use. With watch after the directory (docs/web-dev.md) or after
// desktop (the app's dev build), it keeps building app.js, app.css and
// sw.js as src changes, unminified, with source maps. The desktop app
// reloads its window itself and ends the build by ending its stdin;
// the page reloads when esbuild says it rebuilt (devPort).
import * as esbuild from "esbuild";
import { copyFile, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { join, resolve } from "node:path";
import * as str from "./src/str.gen.ts";
import { terminalDark, terminalLight } from "./src/terminal.gen.ts";
import { fill } from "./src/template.ts";

const here = fileURLToPath(new URL(".", import.meta.url));
const out = resolve(here, process.argv[2] ?? "../static");
const mode = process.argv[3]; // golden, desktop, watch or the page
const desktop = mode === "desktop";
const watch = mode === "watch" || (desktop && process.argv[4] === "watch");
// Where the watching page build tells the page it rebuilt: esbuild's
// server's /esbuild events, which only that build's page listens to.
const devPort = 8081;
const reload = `new EventSource("http://localhost:${devPort}/esbuild").addEventListener("change", () => location.reload());`;
await mkdir(join(out, "fonts"), { recursive: true });
const page = {
  absWorkingDir: here,
  entryPoints: [mode === "golden" ? "src/golden.tsx" : desktop ? "src/desktop.tsx" : "src/main.tsx"],
  bundle: true,
  format: "esm",
  outfile: join(out, "app.js"),
  jsx: "automatic",
  jsxImportSource: "preact",
  loader: { ".woff2": "file" },
  assetNames: "fonts/[name]",
  minify: !watch,
  sourcemap: !desktop || watch, // a production webview has no devtools to read one
  target: ["es2022"],
  logLevel: watch ? "info" : "warning",
  ...(watch && !desktop ? { banner: { js: reload } } : {}),
};
const ctx = watch ? await esbuild.context(page) : undefined;
if (ctx) await ctx.rebuild();
else await esbuild.build(page);
let swCtx;
await copyFile(join(here, "fonts", "OFL.txt"), join(out, "fonts", "OFL.txt"));
// The app embeds all of desktop/frontend/dist, so an earlier build's maps
// would ride along.
if (desktop && !watch) for (const f of ["app.js.map", "app.css.map"]) await rm(join(out, f), { force: true });
if (!desktop) {
  // The service worker (offline launches and pushes), at the page's
  // level so its scope is the page's.
  const sw = {
    absWorkingDir: here,
    entryPoints: ["src/sw.ts"],
    bundle: true,
    format: "iife",
    outfile: join(out, "sw.js"),
    minify: !watch,
    target: ["es2022"],
    logLevel: "warning",
  };
  swCtx = watch ? await esbuild.context(sw) : undefined;
  if (swCtx) await swCtx.rebuild();
  else await esbuild.build(sw);
  // The icons (rendered from icons/kiln.svg by icons/render.sh) and the
  // manifest that makes Kiln installable as a Home Screen app.
  for (const f of ["icon-192.png", "icon-512.png", "icon-maskable-192.png", "icon-maskable-512.png", "apple-touch-icon.png"]) await copyFile(join(here, "icons", f), join(out, f));
  await copyFile(join(here, "icons", "kiln.svg"), join(out, "icon.svg"));
  const manifest = {
    name: str.webAppName(),
    short_name: str.webAppName(),
    start_url: "./",
    scope: "./",
    display: "standalone",
    background_color: terminalDark.bg,
    theme_color: terminalDark.bg,
    icons: [
      { src: "icon.svg", type: "image/svg+xml", sizes: "any", purpose: "any" },
      { src: "icon-192.png", type: "image/png", sizes: "192x192", purpose: "any" },
      { src: "icon-512.png", type: "image/png", sizes: "512x512", purpose: "any" },
      // Android crops to its own shape: the art fitted inside the safe zone.
      { src: "icon-maskable-192.png", type: "image/png", sizes: "192x192", purpose: "maskable" },
      { src: "icon-maskable-512.png", type: "image/png", sizes: "512x512", purpose: "maskable" },
    ],
  };
  await writeFile(join(out, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
}
let html = await readFile(join(here, "index.html"), "utf8");
// The desktop page alone, without links to the manifest and icons it hasn't.
if (desktop) html = html.replace(/^<link rel="(manifest|icon|apple-touch-icon)".*\n/gm, "");
await writeFile(join(out, "index.html"), fill(html, {
  title: str.webTitle(),
  needsJs: str.webNeedsJs(),
  themeDark: terminalDark.bg,
  themeLight: terminalLight.bg,
}));
// The page is all written: from here on, each change rewrites it. The
// desktop app's build runs until its stdin ends (the app has quit); the
// page's until it's stopped, telling the page each time it rebuilt.
if (ctx) {
  await ctx.watch();
  if (swCtx) await swCtx.watch();
  if (desktop) {
    process.stdin.on("end", () => ctx.dispose());
    process.stdin.resume();
  } else {
    // The page is the dev relay's (docs/web-dev.md), another origin.
    await ctx.serve({ host: "localhost", port: devPort, cors: { origin: "http://localhost:8080" } });
  }
}
