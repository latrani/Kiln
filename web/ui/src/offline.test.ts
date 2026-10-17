import { test } from "node:test";
import assert from "node:assert/strict";
import { launchedFiles } from "./offline.ts";

test("a launch's files are the page and its own files, without queries, the relay or the worker", () => {
  const scope = "https://fm.example/kiln/";
  const names = [
    "https://fm.example/kiln/app.js",
    "https://fm.example/kiln/app.css",
    "https://fm.example/kiln/kiln.wasm",
    "https://fm.example/kiln/kiln.wasm",
    "https://fm.example/kiln/preset/manifest.json",
    "https://fm.example/kiln/preset/config/worlds/fm.toml",
    "https://fm.example/kiln/relay?push_key=1",
    "https://fm.example/kiln/sw.js",
    "https://fm.example/kiln/fonts/JetBrainsMono-Regular.woff2?v=1",
    "https://fm.example/elsewhere.js",
    "https://cdn.example/kiln/app.js",
  ];
  assert.deepEqual(launchedFiles(names, scope), [
    "https://fm.example/kiln/",
    "https://fm.example/kiln/app.js",
    "https://fm.example/kiln/app.css",
    "https://fm.example/kiln/kiln.wasm",
    "https://fm.example/kiln/preset/manifest.json",
    "https://fm.example/kiln/preset/config/worlds/fm.toml",
    "https://fm.example/kiln/fonts/JetBrainsMono-Regular.woff2",
  ]);
});
