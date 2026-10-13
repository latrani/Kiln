import { test } from "node:test";
import assert from "node:assert/strict";
import { sidebarRows } from "./sidebar.ts";

test("characters under their worlds, in order", () => {
  const chars = { "fm/kit": { World: "fm" }, "fm/rook": { World: "fm" }, "pf/ash": { World: "pf" } };
  assert.deepEqual(sidebarRows(["fm/kit", "fm/rook", "pf/ash"], chars), [
    { kind: "world", world: "fm" },
    { kind: "char", key: "fm/kit", world: "fm" },
    { kind: "char", key: "fm/rook", world: "fm" },
    { kind: "world", world: "pf" },
    { kind: "char", key: "pf/ash", world: "pf" },
  ]);
});

test("a key the page has no character for yet is skipped", () => {
  assert.deepEqual(sidebarRows(["fm/kit"], {}), []);
});
