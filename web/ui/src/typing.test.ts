import { test, mock } from "node:test";
import assert from "node:assert/strict";
import { Typing } from "./typing.ts";
import { Store } from "./store.ts";

const setup = () => {
  mock.timers.enable({ apis: ["setTimeout"] });
  const sent: any[] = [];
  const t = new Typing(300);
  t.ready((o) => sent.push(o));
  return { t, sent };
};

test("edits within the wait send one input, the last text, numbered", (c) => {
  const { t, sent } = setup();
  c.after(() => mock.timers.reset());
  t.edit("k", "h");
  t.edit("k", "hi");
  assert.equal(sent.length, 0);
  mock.timers.tick(300);
  assert.equal(sent.length, 1);
  assert.equal(sent[0].Do, "input");
  assert.equal(sent[0].Key, "k");
  assert.equal(sent[0].Text, "hi");
  assert.equal(typeof sent[0].N, "number");
});

test("any other action sends what's typed first", (c) => {
  const { t, sent } = setup();
  c.after(() => mock.timers.reset());
  t.edit("kit", "half a pose");
  t.act({ Do: "switch", Key: "rook" });
  assert.deepEqual(sent.map((o) => o.Do), ["input", "switch"]);
  mock.timers.tick(1000);
  assert.equal(sent.length, 2, "nothing left to send");
});

test("send numbers an action after the pending input", (c) => {
  const { t, sent } = setup();
  c.after(() => mock.timers.reset());
  t.edit("k", "x");
  t.send("k", { Do: "submit", Text: "x" });
  assert.deepEqual(sent.map((o) => o.Do), ["input", "submit"]);
  assert.ok(sent[1].N > sent[0].N);
  assert.equal(sent[1].Key, "k");
});

test("Go's text is taken only once it has the page's newest action and nothing's typed since", (c) => {
  const { t, sent } = setup();
  c.after(() => mock.timers.reset());
  assert.equal(t.takes("k", 0), true);
  t.edit("k", "typed");
  assert.equal(t.takes("k", 0), false, "typing pending");
  assert.equal(t.text("k"), "typed");
  t.send("k", { Do: "older", Text: "typed" });
  const n = sent.at(-1).N;
  assert.equal(t.takes("k", n - 1), false, "a reply to the input, not the recall");
  assert.equal(t.takes("k", n), true);
});

test("submitting clears the page's text at once", (c) => {
  const { t } = setup();
  c.after(() => mock.timers.reset());
  t.edit("k", "hello");
  t.submit("k", "hello");
  assert.equal(t.text("k"), "");
});

test("before Kiln starts, typing waits, then goes", (c) => {
  mock.timers.enable({ apis: ["setTimeout"] });
  c.after(() => mock.timers.reset());
  const sent: any[] = [];
  const t = new Typing(300);
  t.edit("", "/help");
  mock.timers.tick(1000);
  t.submit("", "/help");
  assert.equal(t.text(""), "/help", "Enter before Kiln runs keeps the text");
  t.ready((o) => sent.push(o));
  assert.deepEqual(sent.map((o) => [o.Do, o.Text]), [["input", "/help"]]);
});

test("actions before Kiln runs wait their turn, after what was typed", (c) => {
  mock.timers.enable({ apis: ["setTimeout"] });
  c.after(() => mock.timers.reset());
  const sent: any[] = [];
  const t = new Typing(300);
  t.act({ Do: "resize", Cols: 80, Rows: 24 });
  t.edit("", "/help");
  t.act({ Do: "focus", In: true });
  t.ready((o) => sent.push(o));
  assert.deepEqual(sent.map((o) => o.Do), ["resize", "input", "focus"]);
});

test("leaving the tab saves drafts after what's typed", (c) => {
  const { t, sent } = setup();
  c.after(() => mock.timers.reset());
  t.edit("kit", "half a pose");
  t.act({ Do: "save_drafts" });
  assert.deepEqual(sent.map((o) => [o.Do, o.Text]), [["input", "half a pose"], ["save_drafts", undefined]]);
});

test("the page numbers past anything Go has taken, so its typing is never older", (c) => {
  const { t, sent } = setup();
  c.after(() => mock.timers.reset());
  t.saw(2001); // someone else's numbered actions
  t.edit("k", "draft");
  t.flush();
  assert.ok(sent[0].N > 2001);
  assert.equal(t.takes("k", 2001), false, "Go's text is older than the draft");
});

test("the store tells typing what Go has taken", () => {
  const s = new Store();
  s.apply([{ type: "Input", Key: "k", Text: "", Ack: 50 }]);
  s.state.typing!.edit("k", "x");
  s.state.typing!.ready(() => {});
  assert.equal(s.state.typing!.takes("k", 50), false);
});

test("typing while Kiln loaded goes to the character it opened with", (c) => {
  mock.timers.enable({ apis: ["setTimeout"] });
  c.after(() => mock.timers.reset());
  const sent: any[] = [];
  const t = new Typing(300);
  t.edit("", "hello all");
  t.ready((o) => sent.push(o), "fm/kit");
  assert.deepEqual(sent.map((o) => [o.Do, o.Key, o.Text]), [["input", "fm/kit", "hello all"]]);
  assert.equal(t.text("fm/kit"), "hello all");
  assert.equal(t.text(""), undefined);
});

test("with nothing opened, it stays the command input's", (c) => {
  mock.timers.enable({ apis: ["setTimeout"] });
  c.after(() => mock.timers.reset());
  const sent: any[] = [];
  const t = new Typing(300);
  t.edit("", "/help");
  t.ready((o) => sent.push(o), "");
  assert.deepEqual(sent.map((o) => [o.Key, o.Text]), [["", "/help"]]);
});

test("a password goes as the password action, kept secret, never as a submit", () => {
  const sent: [any, boolean | undefined][] = [];
  const t = new Typing(300);
  t.ready((o, secret) => sent.push([o, secret]));
  t.password("kit", "hunter2");
  assert.equal(sent.length, 1);
  assert.equal(sent[0][0].Do, "password");
  assert.equal(sent[0][0].Key, "kit");
  assert.equal(sent[0][1], true);
});
