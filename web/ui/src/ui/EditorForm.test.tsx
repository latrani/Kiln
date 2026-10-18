import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { App } from "./App.tsx";
import { Store } from "../store.ts";
import { roleClass } from "../theme.ts";
import * as str from "../str.gen.ts";

const inh = { Login: "", MaxLineBytes: 4096, NewlineMode: "batch", Autoconnect: false, Reconnect: true, Notify: "first", LocalEcho: true };
const fm = { type: "Settings", World: "fm", Char: "", Name: "", Inherited: inh,
  WorldSettings: { Host: "muck.test", Port: 8888, TLS: true, TLSTrust: "", Use: [], Login: null, MaxLineBytes: null, NewlineMode: null, Autoconnect: null, Reconnect: null, Notify: null, LocalEcho: null } };
const open = (patch?: (s: Store) => void) => {
  const s = new Store();
  s.set({ phase: "running", act: () => {} });
  s.apply([{ type: "Worlds", Worlds: [{ ID: "fm", Characters: [] }] }]);
  s.openEditor({ kind: "world", world: "fm" }, true);
  s.apply([fm]);
  patch?.(s);
  return render(<App store={s} />);
};

test("the editor is a form named by its title, in the bottom pane, with the web's hint", () => {
  const html = open();
  assert.match(html, /<div class="bottom form[^"]*">\s*<form[^>]*aria-labelledby="ed-title"/);
  assert.ok(html.includes(`id="ed-title"`) && html.includes(str.editorEditing("fm")));
  assert.ok(html.includes(str.webFormHint(str.editorHint())));
});

test("fields are real controls with labels: text, a checkbox, radios; no select", () => {
  const html = open();
  assert.match(html, /<label[^>]*for="ed-host"[^>]*>/);
  assert.match(html, /<input[^>]*id="ed-host"[^>]*value="muck.test"/);
  assert.match(html, /<input[^>]*type="checkbox"[^>]*id="ed-tls"[^>]*checked/);
  assert.match(html, /role="radiogroup"[^>]*aria-labelledby="ed-reconnect-label"/);
  assert.match(html, /<input[^>]*type="radio"[^>]*name="ed-reconnect"/);
  assert.ok(!html.includes("<select"));
});

test("the relay's host, port, TLS and cert trust are read-only and dim", () => {
  const html = open();
  assert.match(html, /<input(?=[^>]*id="ed-host")(?=[^>]*readonly)(?=[^>]*value="muck.test")[^>]*>/);
  assert.match(html, /<input(?=[^>]*id="ed-port")(?=[^>]*readonly)[^>]*>/);
  assert.match(html, /<input(?=[^>]*id="ed-tls")(?=[^>]*disabled)(?=[^>]*checked)[^>]*>/);
  assert.match(html, new RegExp(`<span aria-hidden="true" class="${roleClass("form.hint")}">\\[x\\]</span>`), "its box dim");
  assert.match(html, /<input(?=[^>]*type="radio")(?=[^>]*name="ed-tls_trust")(?=[^>]*disabled)[^>]*>/, "cert trust, the relay's too");
  assert.match(html, new RegExp(`role="radiogroup" class="choice ${roleClass("form.hint")}" aria-labelledby="ed-tls_trust-label"`), "dim");
});

test("More settings is a details element, closed, with the TUI's >", () => {
  const html = open();
  assert.match(html, new RegExp(`<details(?![^>]*\\sopen)[^>]*>\\s*<summary[^>]*><span aria-hidden="true" class="dglyph">${str.glyph.collapsed}</span>`));
  assert.ok(html.includes(str.editorExtra()));
});

test("unsaved shows in the title", () => {
  const html = open((s) => s.editField("host", "x"));
  assert.ok(html.includes(str.editorEditing("fm") + str.separator() + str.editorUnsaved()));
});

test("an error sits beside its field, tied to it; one about no field, at the top", () => {
  const html = open((s) => {
    s.sendEditor({ Do: "save_world" });
    s.apply([{ type: "Problems", Op: "save_world", World: "fm", Char: "", Problems: [{ Field: "port", Msg: "bad port" }, { Field: "", Msg: "disk full" }] }]);
  });
  assert.match(html, /<input[^>]*id="ed-port"[^>]*aria-invalid="true"[^>]*aria-describedby="ed-port-err"/);
  assert.match(html, new RegExp(`id="ed-port-err"[^>]*class="${roleClass("form.error")}"[^>]*>[^<]*bad port`));
  assert.match(html, /role="alert"[^>]*>[^<]*disk full/);
});

test("Save is the primary button; Delete, once pressed, asks again", () => {
  const html = open((s) => s.setEditor({ armed: true }));
  assert.match(html, new RegExp(`<button[^>]*type="submit"[^>]*class="${roleClass("form.button")}"[^>]*>${str.editorSave()}`));
  assert.match(html, new RegExp(`<button[^>]*type="button"[^>]*class="${roleClass("form.button.secondary")}"[^>]*>${str.editorDeleteWorld()}`));
  assert.ok(html.includes(str.webDeleteAgain("fm")));
  assert.ok(!html.includes(str.editorForgetPassword()), "the web never stores a password");
});

test("for screen readers: the summary's glyph is hidden; a choice's id and error are on its chosen radio", () => {
  const html = open((s) => {
    s.editField("reconnect", "off");
    s.sendEditor({ Do: "save_world" });
    s.apply([{ type: "Problems", Op: "save_world", World: "fm", Char: "", Problems: [{ Field: "reconnect", Msg: "no" }] }]);
  });
  assert.match(html, /<details[^>]*\sopen[^>]*>\s*<summary[^>]*><span aria-hidden="true" class="dglyph">▼<\/span>/, "opened for the refused choice inside it");
  assert.match(html, /<input type="radio" id="ed-reconnect"[^>]*value="off"[^>]*checked/, "focusing the field lands on what's chosen");
  assert.match(html, /<input type="radio"[^>]*value="on"[^>]*aria-invalid="true"[^>]*aria-describedby="ed-reconnect-err"/, "each radio carries the error");
});
