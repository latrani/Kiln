import { test } from "node:test";
import assert from "node:assert/strict";
import { fit, watchViewport } from "./viewport.ts";

test("with the keyboard up, the grid takes what shows and follows the pan", () => {
  assert.deepEqual(fit({ height: 420.4, offsetTop: 280.6, scale: 1 }, 800), { height: 420, top: 281 });
});

test("nothing covering the page: left at 100dvh, even with a stale pan", () => {
  assert.equal(fit({ height: 800, offsetTop: 0, scale: 1 }, 800), undefined);
  assert.equal(fit({ height: 799.5, offsetTop: 120, scale: 1 }, 800), undefined, "iOS 26 can leave offsetTop set");
});

test("pinch-zoomed: the layout stays put", () => {
  assert.equal(fit({ height: 400, offsetTop: 100, scale: 2 }, 800), undefined);
});

// fakes: a visual viewport to move, and a root that keeps its properties.
function fakes(layout: number) {
  const vv = Object.assign(new EventTarget(), { height: layout, offsetTop: 0, scale: 1 });
  const props = new Map<string, string>();
  const root = { clientHeight: layout, style: { setProperty: (k: string, v: string) => props.set(k, v), removeProperty: (k: string) => props.delete(k) } };
  return { vv, props, root: root as unknown as HTMLElement };
}

test("the keyboard coming and going sets and clears the grid's fit, until stopped", () => {
  const { vv, props, root } = fakes(800);
  const stop = watchViewport(vv, root);
  assert.equal(props.size, 0, "nothing covering at the start");
  Object.assign(vv, { height: 450 });
  vv.dispatchEvent(new Event("resize"));
  assert.equal(props.get("--vvh"), "450px");
  assert.equal(props.get("--vvbottom"), "0px", "no room for the home indicator under the keyboard");
  Object.assign(vv, { offsetTop: 300 });
  vv.dispatchEvent(new Event("scroll"));
  assert.equal(props.get("--vvtop"), "300px", "the pan follows on scroll");
  Object.assign(vv, { height: 800 });
  vv.dispatchEvent(new Event("resize"));
  assert.equal(props.size, 0, "keyboard gone: cleared");
  stop();
  Object.assign(vv, { height: 450 });
  vv.dispatchEvent(new Event("resize"));
  assert.equal(props.size, 0, "stopped");
});
