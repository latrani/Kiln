import { test } from "node:test";
import assert from "node:assert/strict";
import { chipsWidth, sidebarCSS } from "./layout.ts";
import * as str from "./str.gen.ts";

test("the sidebar is the TUI's width rule", () => {
  const len = (s: string) => [...s].length;
  assert.equal(chipsWidth(), 2 + Math.max(len(str.sidebarOpenConnection()), 1 + len(str.pickerAddCharacter())), "no + World on the web");
  assert.equal(sidebarCSS(), `min(22ch, max(${chipsWidth()}ch, round(down, 20vw, 1ch)))`);
});
