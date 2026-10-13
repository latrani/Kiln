import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { Store } from "../store.ts";
import type { Event, State } from "../store.ts";
import { loadingText } from "../boot.ts";
import { sidebarCSS } from "../layout.ts";
import * as str from "../str.gen.ts";

const page = (patch: Partial<State> = {}, batch: Event[] = []) => {
  const s = new Store();
  s.set(patch);
  s.apply(batch);
  return render(<App store={s} />);
};

test("five areas, with landmarks", () => {
  const html = page({ phase: "running" });
  for (const want of ["<header", "<nav", 'class="bottom', 'role="status"']) assert.ok(html.includes(want), want);
  assert.ok(html.includes(`aria-label="${str.webCharacters()}"`));
  assert.match(html, /<main(?![^>]*role=)[^>]*>\s*<div[^>]*role="log"/, "the scrollback is the main landmark, with the log inside it");
});

test("buttons look like the TUI's", () => {
  for (const patch of [{ phase: "locked", onUseHere: () => {} }, { phase: "stopped" }] as Partial<State>[]) {
    assert.match(page(patch), /<button[^>]*class="[^"]*r-form-button/, JSON.stringify(patch.phase));
  }
});

test("loading shows progress in the status line", () => {
  assert.ok(page({ phase: "loading", progress: { got: 2_100_000, total: 6_400_000 } }).includes(loadingText(2_100_000, 6_400_000)));
});

test("another tab: the message and Use here, in the scrollback", () => {
  const html = page({ phase: "locked" });
  assert.ok(html.includes(str.webOtherTab()));
  assert.ok(html.includes(`>${str.webUseHere()}</button>`));
});

test("stopped: Reload", () => {
  assert.ok(page({ phase: "stopped" }).includes(`>${str.webReload()}</button>`));
});

// liveRegions is each live region's role and text, in order.
const liveRegions = (html: string) => [...html.matchAll(/<(\w+)[^>]*role="(status|alert)"[^>]*>(.*?)<\/\1>/g)].map((m) => [m[2], m[3].replace(/<[^>]*>/g, "")]);

test("the status line keeps one polite and one urgent region, always", () => {
  for (const patch of [{ phase: "running" }, { phase: "loading", progress: { got: 1, total: 2 } }, { phase: "stopped" }] as Partial<State>[]) {
    assert.deepEqual(liveRegions(page(patch)).map((r) => r[0]), ["status", "alert"], JSON.stringify(patch));
  }
});

test("errors go in the urgent region, the rest in the polite one", () => {
  assert.deepEqual(liveRegions(page({ phase: "running" }, [{ type: "Status", Text: "boom", Err: true, Gen: 1 }])), [["status", ""], ["alert", "boom"]]);
  assert.deepEqual(liveRegions(page({ phase: "running" }, [{ type: "Status", Text: "hi", Err: false, Gen: 1 }])), [["status", "hi"], ["alert", ""]]);
  assert.deepEqual(liveRegions(page({ phase: "running", saving: false })), [["status", str.webNotSaving()], ["alert", ""]]);
  assert.deepEqual(liveRegions(page({ phase: "stopped" })), [["status", ""], ["alert", str.webStopped()]]);
});

test("download progress isn't read aloud chunk by chunk", () => {
  const html = page({ phase: "loading", progress: { got: 2_100_000, total: 6_400_000 } });
  assert.ok(html.includes(loadingText(2_100_000, 6_400_000)));
  assert.ok(!liveRegions(html).some((r) => r[1].includes(loadingText(2_100_000, 6_400_000))));
});

test("the theme goes into the page's style", () => {
  const html = page({ phase: "running" }, [{ type: "Theme", Appearance: "dark", Dark: { Roles: { status: { FG: "#123456" } }, Tags: {} }, Light: { Roles: {}, Tags: {} } }]);
  assert.ok(html.includes("#123456"));
});

test("loading, before any theme, is already in the terminal's colors", () => {
  assert.ok(page({ phase: "loading", progress: { got: 1 } }).includes("--term-bg:"));
});

test("the sidebar's width is the TUI's", () => {
  assert.ok(page({ phase: "running" }).includes(`--side:${sidebarCSS()}`));
});
