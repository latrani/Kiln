import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { Store } from "../store.ts";
import type { Event, State } from "../store.ts";
import { loadingText } from "../boot.ts";
import * as str from "../str.gen.ts";

const page = (patch: Partial<State> = {}, batch: Event[] = []) => {
  const s = new Store();
  s.set(patch);
  s.apply(batch);
  return render(<App store={s} />);
};

test("five areas, with landmarks", () => {
  const html = page({ phase: "running" });
  for (const want of ["<header", "<nav", "<main", 'role="log"', 'class="bottom', 'role="status"']) assert.ok(html.includes(want), want);
  assert.ok(html.includes(`aria-label="${str.webCharacters()}"`));
});

test("loading shows progress in the status line", () => {
  assert.ok(page({ phase: "loading", progress: { got: 2_100_000, total: 6_400_000 } }).includes(loadingText(2_100_000, 6_400_000)));
});

test("another tab: the message and Use here, in the scrollback", () => {
  const html = page({ phase: "locked" });
  assert.ok(html.includes(str.webOtherTab()));
  assert.ok(html.includes(`>${str.webUseHere()}</button>`));
});

test("stopped: an alert and Reload", () => {
  const html = page({ phase: "stopped" });
  assert.ok(html.includes('role="alert"') && html.includes(str.webStopped()));
  assert.ok(html.includes(`>${str.webReload()}</button>`));
});

test("a status error is an alert; otherwise not saving stays", () => {
  assert.match(page({ phase: "running" }, [{ type: "Status", Text: "boom", Err: true, Gen: 1 }]), /role="alert"[^>]*>boom</);
  assert.ok(page({ phase: "running", saving: false }).includes(str.webNotSaving()));
});

test("the theme goes into the page's style", () => {
  const html = page({ phase: "running" }, [{ type: "Theme", Appearance: "dark", Dark: { Roles: { status: { FG: "#123456" } }, Tags: {} }, Light: { Roles: {}, Tags: {} } }]);
  assert.ok(html.includes("#123456"));
});
