import { test } from "node:test";
import assert from "node:assert/strict";
import { announcements } from "./announce.ts";
import { Store } from "./store.ts";
import type { Event } from "./store.ts";
import * as str from "./str.gen.ts";

const line = (Seq: number, Plain: string, Priority = true) => ({ Seq, Kind: "server", Day: "", Out: false, Plain, Spans: [{ Text: Plain }], Priority, Quiet: false });
const lines = (Key: string, Op: string, ls: object[], Epoch = 1): Event => ({ type: "Lines", Key, Epoch, Op, Lines: ls });

// after is the announcements batch makes, from a page with Kit showing
// and Rook in the background, each with a line.
function after(batch: Event[]): string[] {
  const s = new Store();
  s.apply([
    { type: "Char", Key: "fm/kit", World: "fm", Name: "Kit" },
    { type: "Char", Key: "fm/rook", World: "fm", Name: "Rook" },
    { type: "Active", Key: "fm/kit" },
    lines("fm/kit", "reset", [line(1, "old kit")]),
    lines("fm/rook", "reset", [line(1, "old rook")]),
  ]);
  const prev = s.state;
  s.apply(batch);
  return announcements(prev, s.state);
}

test("a priority line, from a character not showing, is read aloud", () => {
  assert.deepEqual(after([lines("fm/rook", "append", [line(2, "Ash pages you")])]), [str.webAnnounce("Rook", "Ash pages you")]);
});

test("only new priority lines, in the background, once", () => {
  assert.deepEqual(after([lines("fm/rook", "append", [line(2, "chatter", false)])]), [], "no priority");
  assert.deepEqual(after([lines("fm/kit", "append", [line(2, "Ash pages you")])]), [], "the character showing");
  assert.deepEqual(after([lines("fm/rook", "replace", [line(1, "old rook")])]), [], "lines made again");
  assert.deepEqual(after([lines("fm/rook", "prepend", [line(0, "older")])]), [], "older history");
  assert.deepEqual(after([lines("fm/rook", "reset", [line(1, "old rook"), line(2, "again")], 2)]), [], "lines loaded again");
  assert.deepEqual(after([lines("fm/rook", "append", [line(2, "Ash pages you")]), { type: "Closed", Key: "fm/rook" }]), [], "closed");
});
