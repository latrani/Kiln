import { test } from "node:test";
import assert from "node:assert/strict";
import { titleFor } from "./title.ts";
import { Store } from "./store.ts";
import * as str from "./str.gen.ts";

test("the tab's title counts unread lines", () => {
  const s = new Store();
  assert.equal(titleFor(s.state), str.webTitle());
  s.apply([
    { type: "Char", Key: "fm/kit", Unread: 0 },
    { type: "Char", Key: "fm/rook", Unread: 3 },
    { type: "Char", Key: "pf/ash", Unread: 1 },
  ]);
  assert.equal(titleFor(s.state), str.webTitleUnread(4));
});
