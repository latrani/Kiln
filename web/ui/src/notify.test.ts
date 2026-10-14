import { test } from "node:test";
import assert from "node:assert/strict";
import { ask, offer, show } from "./notify.ts";
import type { API, Shown } from "./notify.ts";
import { Store } from "./store.ts";
import * as str from "./str.gen.ts";

// fake is a Notification API answering with answer when asked; shown
// collects what it shows.
function fake(permission: NotificationPermission, answer: NotificationPermission = permission) {
  const shown: { title: string; body?: string; note: Shown; closed: boolean }[] = [];
  const api = class {
    static permission = permission;
    static async requestPermission() {
      api.permission = answer;
      return answer;
    }
    onclick: ((e: Event) => void) | null = null;
    constructor(title: string, opts?: { body?: string }) {
      const rec = { title, body: opts?.body, note: this as Shown, closed: false };
      this.close = () => { rec.closed = true; };
      shown.push(rec);
    }
    close() {}
  } satisfies API;
  return { api, shown };
}

const running = () => {
  const s = new Store();
  const sent: any[] = [];
  s.set({ phase: "running", act: (o) => sent.push(o), notifyOffer: true });
  return { s, sent };
};

test("the offer shows until notifications are allowed or it's been answered", () => {
  assert.equal(offer("default", false), true);
  assert.equal(offer("denied", false), true);
  assert.equal(offer("unsupported", false), true);
  assert.equal(offer("granted", false), false);
  assert.equal(offer("default", true), false);
});

test("asking: allowed says nothing; denied or no API says why, once", async () => {
  for (const [api, want] of [[fake("default", "granted").api, undefined], [fake("default", "denied").api, str.webNotifyDenied()], [fake("denied").api, str.webNotifyDenied()], [undefined, str.webNotifyUnsupported()]] as const) {
    const { s, sent } = running();
    await ask(s, api);
    assert.equal(s.state.notifyOffer, false);
    assert.deepEqual(sent, want ? [{ Do: "say", Text: want, Err: false }] : []);
  }
});

test("a notification shows the line, and clicking it focuses and switches", () => {
  const { s, sent } = running();
  const { api, shown } = fake("granted");
  let focused = 0;
  show(s, { type: "Effect", effect: "Notify", Title: "Kit", Body: "Mira pages", Key: "fm/kit" }, api, () => focused++);
  assert.equal(shown.length, 1);
  assert.equal(shown[0].title, "Kit");
  assert.equal(shown[0].body, "Mira pages");
  shown[0].note.onclick?.(new Event("click"));
  assert.equal(focused, 1);
  assert.deepEqual(sent, [{ Do: "switch", Key: "fm/kit" }]);
  assert.equal(shown[0].closed, true);
});

test("without permission nothing shows", () => {
  const { s } = running();
  for (const p of ["default", "denied"] as const) {
    const { api, shown } = fake(p);
    show(s, { type: "Effect", effect: "Notify", Title: "Kit", Body: "hi", Key: "fm/kit" }, api);
    assert.equal(shown.length, 0);
  }
  show(s, { type: "Effect", effect: "Notify", Title: "Kit", Body: "hi" }, undefined);
});
