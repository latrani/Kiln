import { test } from "node:test";
import assert from "node:assert/strict";
import { resetter } from "./reset.ts";

// deferred is a promise the test settles.
function deferred() {
  let ok!: () => void, fail!: (e: Error) => void;
  const p = new Promise<void>((o, f) => { ok = o; fail = f; });
  return { p, ok, fail };
}

test("a reset wipes, forgets, then reloads", async () => {
  const did: string[] = [];
  const r = resetter({ wipe: async () => { did.push("wipe"); }, forget: () => did.push("forget"), reload: () => did.push("reload") });
  await r.reset();
  assert.deepEqual(did, ["wipe", "forget", "reload"]);
});

test("a handoff waits for a wipe already running", async () => {
  const w = deferred();
  let left = false;
  const r = resetter({ wipe: () => w.p, forget: () => {}, reload: () => {} });
  const resetting = r.reset();
  const leaving = r.leave().then(() => { left = true; });
  await new Promise((ok) => setTimeout(ok));
  assert.equal(left, false, "the handoff went ahead before the wipe finished");
  w.ok();
  await Promise.all([resetting, leaving]);
  assert.equal(left, true);
});

test("a reset after a handoff began does nothing: the other tab has Kiln", async () => {
  let wiped = 0, reloaded = 0;
  const r = resetter({ wipe: async () => { wiped++; }, forget: () => {}, reload: () => { reloaded++; } });
  await r.leave();
  await r.reset();
  assert.equal(wiped, 0);
  assert.equal(reloaded, 0);
});

test("a failed wipe rejects without reloading, and doesn't hold up a handoff", async () => {
  let reloaded = 0;
  const r = resetter({ wipe: () => Promise.reject(new Error("no")), forget: () => {}, reload: () => { reloaded++; } });
  await assert.rejects(r.reset());
  await r.leave();
  assert.equal(reloaded, 0);
});
