import { test } from "node:test";
import assert from "node:assert/strict";
import { sidebarRows } from "./sidebar.ts";

test("characters under their worlds, in order, each world's last ending its tree; then a gap and + Connection", () => {
  const chars = { "fm/kit": { World: "fm" }, "fm/rook": { World: "fm" }, "pf/ash": { World: "pf" } };
  assert.deepEqual(sidebarRows(["fm/kit", "fm/rook", "pf/ash"], chars), [
    { kind: "world", world: "fm" },
    { kind: "char", key: "fm/kit", world: "fm", last: false },
    { kind: "char", key: "fm/rook", world: "fm", last: true },
    { kind: "world", world: "pf" },
    { kind: "char", key: "pf/ash", world: "pf", last: true },
    { kind: "gap" },
    { kind: "add" },
  ]);
});

test("a key the page has no character for yet is skipped; with nothing open, only + Connection", () => {
  assert.deepEqual(sidebarRows(["fm/kit"], {}), [{ kind: "add" }]);
});
