import { test } from "node:test";
import assert from "node:assert/strict";
import { fill } from "./template.ts";

test("slots fill in, escaped", () => {
  assert.equal(fill("<p>{{a}}</p>", { a: "x < y & z" }), "<p>x &lt; y &amp; z</p>");
});

test("an unknown slot is an error, not undefined", () => {
  assert.throws(() => fill("{{nope}}", {}), /nope/);
});
