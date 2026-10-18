// The editor's forms, as the TUI's (internal/ui/editor.go): which fields
// a world or character has, their values to and from Go's settings, and
// what's wrong with them. Values are the form's: text as typed, a
// toggle's on, a choice's option ("" for inheriting).
import * as str from "./str.gen.ts";

// Target is what an editor adds or edits.
export type Target = { kind: "world"; world: string } | { kind: "char"; world: string; char: string };

// targetKey names a target as the picker's rows do: "=world",
// "world/char". One editor (or draft) per key.
export const targetKey = (t: Target): string => (t.kind === "world" ? "=" + t.world : `${t.world}/${t.char}`);

// targetOf is the target Go's Settings (or a settings action) is for.
export function targetOf(e: { World: string; Char: string }): Target {
  return e.Char ? { kind: "char", world: e.World, char: e.Char } : { kind: "world", world: e.World };
}

// Inherited is what a world or character gets for settings it leaves
// unset (config.Inherited).
export interface Inherited { Login: string; MaxLineBytes: number; NewlineMode: string; Autoconnect: boolean; Reconnect: boolean; Notify: string; LocalEcho: boolean }

// SettingsEvent is Go's Settings (internal/webapi/settings.go).
export interface SettingsEvent { World: string; Char: string; Name: string; WorldSettings?: any; CharSettings?: any; Inherited: Inherited }

// Field is one of a form's fields, keyed by its config key (what Go's
// problems and check() name). A fixed one is shown, not edited.
export type Field =
  | { kind: "text"; key: string; label: string; hint?: string; extra?: boolean; fixed?: boolean }
  | { kind: "toggle"; key: string; label: string; extra?: boolean; fixed?: boolean }
  | { kind: "choice"; key: string; label: string; options: { value: string; shown: string }[]; extra?: boolean; fixed?: boolean };

export type Values = Record<string, string | boolean>;

// Problem is one of Go's Problems: a field ("" for none) and why.
export interface Problem { Field: string; Msg: string }

// Editor is an open (or hidden, or parked) editor.
export interface Editor {
  target: Target;
  title: string;
  fields: Field[];
  values: Values;
  saved: string; // the values as read, to tell unsaved edits
  more: boolean; // More settings open
  armed: boolean; // Delete pressed once: pressed again, it deletes
  problems: Problem[]; // why Go refused the last save or delete
  live: Record<string, string>; // what check() says of a field as typed
  refused: number; // how many refusals, so the form can move focus on each
  pending?: string; // the action sent (its Do), until Go answers
  closeAll: boolean; // opened from the main view: closing it closes the picker
}

const onOff = (b: boolean) => (b ? "on" : "off"); //str:ok config values

// shown is how a choice's option reads: on and off in words, config
// values (batch, pin, …) as they're written in the file.
const shown = (v: string) => (v === "on" ? str.editorOn() : v === "off" ? str.editorOff() : v);

// inheritChoice is a choice between inheriting (showing what that
// gives) and the given options.
function inheritChoice(key: string, label: string, inherits: string, options: string[], extra = false): Field {
  return { kind: "choice", key, label, extra, options: [{ value: "", shown: str.editorDefault(shown(inherits)) }, ...options.map((o) => ({ value: o, shown: shown(o) }))] };
}

// worldFields is a world's form. Its host, port, TLS and cert trust are
// fixed: on the web they're the relay's allowlist's, which the page
// can't change.
function worldFields(inh: Inherited): Field[] {
  return [
    { kind: "text", key: "host", label: str.editorHost(), fixed: true },
    { kind: "text", key: "port", label: str.editorPort(), fixed: true },
    { kind: "toggle", key: "tls", label: str.editorTls(), fixed: true },
    { ...inheritChoice("tls_trust", str.editorCertTrust(), "pin", ["pin", "ca"], true), fixed: true },
    { kind: "text", key: "use", label: str.editorPacks(), hint: str.editorNone(), extra: true },
    { kind: "text", key: "login", label: str.editorLogin(), hint: str.editorDefaultHint(inh.Login || str.editorNone()), extra: true },
    { kind: "text", key: "max_line_bytes", label: str.editorMaxBytes(), hint: str.editorDefaultHint(inh.MaxLineBytes), extra: true },
    inheritChoice("newline_mode", str.editorNewlines(), inh.NewlineMode, ["batch", "flatten"], true),
    ...charChoices(inh, true),
  ];
}

function charChoices(inh: Inherited, extra: boolean): Field[] {
  return [
    inheritChoice("autoconnect", str.editorAutoconnect(), onOff(inh.Autoconnect), ["on", "off"], extra),
    inheritChoice("reconnect", str.editorReconnect(), onOff(inh.Reconnect), ["on", "off"], extra),
    inheritChoice("notify", str.editorNotify(), inh.Notify, ["all", "first", "priority", "none"], extra),
    inheritChoice("local_echo", str.editorLocalEcho(), onOff(inh.LocalEcho), ["on", "off"], extra),
  ];
}

const optBool = (b: boolean | null | undefined) => (b == null ? "" : onOff(b));
const boolOf = (v: string | boolean) => (v === "on" ? true : v === "off" ? false : null);
const text = (v: Values, k: string) => String(v[k] ?? "");
// list splits a comma- or space-separated field, as app.List.
const list = (s: string) => s.split(/[, ]/).filter((x) => x);
// num reads a number as the TUI's Atoi: anything else is 0, which the
// save refuses.
const num = (s: string) => (/^\d+$/.test(s) ? Number(s) : 0);
const orNull = (s: string) => (s === "" ? null : s);

function valuesOf(e: SettingsEvent): Values {
  if (e.CharSettings) {
    const s = e.CharSettings;
    return { aliases: (s.Aliases ?? []).join(", "), autoconnect: optBool(s.Autoconnect), reconnect: optBool(s.Reconnect),
      notify: s.Notify ?? "", local_echo: optBool(s.LocalEcho) };
  }
  const s = e.WorldSettings;
  const v: Values = { host: s.Host ?? "", port: s.Port > 0 ? String(s.Port) : "", tls: !!s.TLS, tls_trust: s.TLSTrust ?? "",
    use: (s.Use ?? []).join(", "), login: s.Login ?? "", max_line_bytes: s.MaxLineBytes == null ? "" : String(s.MaxLineBytes),
    newline_mode: s.NewlineMode ?? "", autoconnect: optBool(s.Autoconnect), reconnect: optBool(s.Reconnect),
    notify: s.Notify ?? "", local_echo: optBool(s.LocalEcho) };
  return v;
}

// newEditor is the editor for what Go's Settings e is for, filled in.
export function newEditor(e: SettingsEvent, closeAll: boolean): Editor {
  const target = targetOf(e);
  const fields = target.kind === "char" ? [{ kind: "text", key: "aliases", label: str.editorAliases(), hint: str.editorNone() } as Field, ...charChoices(e.Inherited, false)]
    : worldFields(e.Inherited);
  const title = str.editorEditing(target.kind === "char" ? `${e.World}/${e.Name}` : e.World);
  const values = valuesOf(e);
  return { target, title, fields, values, saved: JSON.stringify(values), more: false, armed: false, problems: [], live: {}, refused: 0, closeAll };
}

// edit sets field key to value: check (Go's, for text) says what's
// wrong with it as typed, the field's refusal from the last save goes,
// and Delete has to be pressed twice again.
export function edit(ed: Editor, key: string, value: string | boolean, check?: (field: string, value: string) => string): Editor {
  const live = { ...ed.live };
  const msg = typeof value === "string" && check ? check(key, value) : "";
  if (msg) live[key] = msg;
  else delete live[key];
  return { ...ed, values: { ...ed.values, [key]: value }, live, problems: ed.problems.filter((p) => p.Field !== key), armed: false };
}

// unsaved reports whether the values differ from what was read.
export const unsaved = (ed: Editor): boolean => JSON.stringify(ed.values) !== ed.saved;

// saveAction is the action that saves ed: save_world or
// save_character, with the settings as Go reads them (unset is null).
export function saveAction(ed: Editor): { Do: string; [k: string]: unknown } {
  const v = ed.values;
  const t = ed.target;
  if (t.kind === "char") {
    return { Do: "save_character", World: t.world, Char: t.char, CharSettings: { Aliases: list(text(v, "aliases")),
      Autoconnect: boolOf(v.autoconnect), Reconnect: boolOf(v.reconnect), Notify: orNull(text(v, "notify")), LocalEcho: boolOf(v.local_echo) } };
  }
  const s = { Host: text(v, "host"), Port: num(text(v, "port")), TLS: v.tls === true, TLSTrust: text(v, "tls_trust"), Use: list(text(v, "use")),
    Login: orNull(text(v, "login")), MaxLineBytes: text(v, "max_line_bytes") === "" ? null : num(text(v, "max_line_bytes")),
    NewlineMode: orNull(text(v, "newline_mode")), Autoconnect: boolOf(v.autoconnect), Reconnect: boolOf(v.reconnect),
    Notify: orNull(text(v, "notify")), LocalEcho: boolOf(v.local_echo) };
  return { Do: "save_world", World: t.world, WorldSettings: s };
}

// deleteAction is the action that deletes what ed edits.
export function deleteAction(ed: Editor): { Do: string; [k: string]: unknown } | undefined {
  const t = ed.target;
  if (t.kind === "char") return { Do: "delete_character", World: t.world, Char: t.char };
  if (t.kind === "world") return { Do: "delete_world", World: t.world };
  return undefined;
}

const has = (ed: Editor, key: string) => ed.fields.some((f) => f.key === key);

// errorOf is what's wrong with field key: as typed, or else as Go
// refused it.
export const errorOf = (ed: Editor, key: string): string | undefined => ed.live[key] ?? ed.problems.find((p) => p.Field === key)?.Msg;

// formError is what Go refused about no one field of the form.
export const formError = (ed: Editor): string | undefined => ed.problems.find((p) => !has(ed, p.Field))?.Msg;

// firstError is the first field, in the form's order, with something
// wrong.
export const firstError = (ed: Editor): string | undefined => ed.fields.find((f) => errorOf(ed, f.key) !== undefined)?.key;

// refuse is ed after Go refused its save or delete: every problem
// shown, More settings open if one is inside it.
export function refuse(ed: Editor, problems: Problem[]): Editor {
  const more = ed.more || problems.some((p) => ed.fields.find((f) => f.key === p.Field)?.extra);
  return { ...ed, problems, more, armed: false, pending: undefined, refused: ed.refused + 1 };
}
