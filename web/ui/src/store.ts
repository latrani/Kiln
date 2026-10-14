// The page's one store: Go's batches applied a batch at a time, plus the
// page's own state (how far it's got starting Kiln, whether it's saving).
import { edit, newEditor, refuse, targetKey, targetOf } from "./editor.ts";
import type { Editor, SettingsEvent, Target } from "./editor.ts";
import { fixPick, pickerRows } from "./picker.ts";
import { Typing } from "./typing.ts";
import type { Span, ThemeEvent } from "./types.ts";

// Phase is how far the page has got: another tab has Kiln, the wasm is
// loading, Kiln is running, or it stopped.
export type Phase = "locked" | "loading" | "running" | "stopped";

// LineData is a scrollback or log-mode line (webapi.Line).
export interface LineData {
  Seq: number;
  Kind: "server" | "echo" | "sys" | "day" | "history_end";
  Day: string;
  DayLabel?: string;
  Clock?: string;
  Out: boolean;
  Plain: string;
  Spans: Span[] | null;
  Priority: boolean;
  Quiet: boolean;
}

// Lines are a character's lines in one numbering (epoch), in order.
export interface Lines {
  epoch: number;
  lines: LineData[];
}

// Event is one of Go's events; the store reads the fields it needs.
// eslint-style any: the shapes are Go's (internal/feed, internal/webapi).
export type Event = { type: string; [k: string]: any };

// Picker is the open picker (picker.ts): its filter and highlighted row,
// and while adding a character, the world, the name typed and why it
// won't do.
export interface Picker {
  filter: string;
  sel: string;
  adding?: string;
  name?: string;
  error?: string;
  edit?: Editor; // an editor open over the picker (editor.ts)
}

// Answer is the latest Saved or Problems, numbered, so a form can tell
// a new one from the one it's seen.
export interface Answer {
  n: number;
  Op: string;
  Key?: string;
  Problems?: { Field: string; Msg: string }[];
}

// Prompt is log mode's prompt in the bottom pane, as the TUI's: find,
// go to date, export's format (a key), export's file name.
export type Prompt = { kind: "find" | "date" | "filename"; value: string; format?: string } | { kind: "format" };

// LogView is the page's side of a character's log mode: hidden by Ctrl+L
// (kept as it was), its prompt, the line find or go-to-date last landed
// on (at; goN counts landings, so each moves the view once), and the
// line last clicked.
export interface LogView {
  hidden?: boolean;
  prompt?: Prompt;
  at?: number;
  goN: number;
  clicked?: number;
}

export interface State {
  phase: Phase;
  progress?: { got: number; total?: number };
  saving: boolean;
  onUseHere?: () => void; // takes Kiln over from another tab
  act?: (o: object) => void; // asks Kiln for something (webapi's actions), after what's typed
  typing?: Typing; // what's typed, sent ahead of every other action
  theme?: ThemeEvent;
  worlds: any[];
  order: string[];
  active: string;
  chars: Record<string, any>;
  inputs: Record<string, any>;
  lines: Record<string, Lines>;
  logs: Record<string, any>;
  logLines: Record<string, Lines>;
  filters: Record<string, any>;
  logView: Record<string, LogView>; // by character
  status?: { Text: string; Err: boolean; Gen: number };
  presence?: number;
  confirm?: boolean;
  saveQuestion?: any;
  picker?: Picker;
  answer?: Answer;
  drafts: Record<string, Editor>; // hidden editors' edits, by targetKey, for the next time
  parked: Record<string, Editor>; // editors opened from the main view, by the item they were left on
  opening?: { key: string; closeAll: boolean }; // an editor waiting for Go's settings
}

// fresh is a state holding nothing of Go's: only the page's own.
const fresh = ({ phase, progress, saving, onUseHere, act, typing }: Pick<State, "phase" | "progress" | "saving" | "onUseHere" | "act" | "typing">): State => ({
  phase, progress, saving, onUseHere, act, typing,
  worlds: [], order: [], active: "", chars: {}, inputs: {}, lines: {}, logs: {}, logLines: {}, filters: {}, logView: {}, drafts: {}, parked: {},
});

// merge is prev with a Lines or LogLines event applied. Only a reset
// starts a numbering; anything else for one the page doesn't hold would
// graft onto the wrong lines, so it's dropped.
function merge(prev: Lines | undefined, e: Event): Lines | undefined {
  const ls: LineData[] = e.Lines ?? [];
  if (e.Op === "reset") return { epoch: e.Epoch, lines: ls };
  if (!prev || prev.epoch !== e.Epoch) {
    console.error("lines for a numbering the page doesn't hold", e);
    return prev;
  }
  switch (e.Op) {
    case "append": return { epoch: prev.epoch, lines: [...prev.lines, ...ls] };
    case "prepend": return { epoch: prev.epoch, lines: [...ls, ...prev.lines] };
    case "replace": return { epoch: prev.epoch, lines: ls };
  }
  console.error("unknown lines op", e);
  return prev;
}

const without = <T>(m: Record<string, T>, k: string): Record<string, T> => {
  const out = { ...m };
  delete out[k];
  return out;
};

const pickerOf = (s: State): Picker => s.picker ?? { filter: "", sel: fixPick(pickerRows(s.worlds, s.chars, ""), "") };

// stash keeps ed's edits for the next time its target's editor opens.
const stash = (s: State, ed: Editor): State => ({ ...s, drafts: { ...s.drafts, [targetKey(ed.target)]: ed } });

// show opens ed over the picker (opening it), the editor open there
// before kept as a draft, as the TUI's setEditor.
function show(s: State, ed: Editor): State {
  const old = s.picker?.edit;
  if (old && targetKey(old.target) !== targetKey(ed.target)) s = stash(s, old);
  return { ...s, picker: { ...pickerOf(s), adding: undefined, edit: ed } };
}

// shut closes the open editor: back to the picker, or with closeAll out
// of it, as the TUI's closeEditor.
function shut(s: State): State {
  if (!s.picker?.edit) return s;
  return s.picker.edit.closeAll ? { ...s, picker: undefined } : { ...s, picker: { ...s.picker, edit: undefined } };
}

// exists reports whether k (a character's key or "=world") is still
// something to show.
const exists = (s: State, k: string): boolean =>
  k.startsWith("=") ? Object.values(s.chars).some((c: any) => "=" + c.World === k) : !!s.chars[k];

// settle is s after Go's answer to the editor's save or delete, as the
// TUI's save and delete calls: refused, it shows why; saved, it closes (a
// new world's to the picker, on its + Character), and a deleted world's
// drafts go with it.
function settle(s: State, a: Answer): State {
  const ed = s.picker!.edit!;
  if (a.Problems) return { ...s, picker: { ...s.picker!, edit: refuse(ed, a.Problems) } };
  if (a.Op === "delete_world" && ed.target.kind === "world") {
    const w = ed.target.world;
    s = { ...s, drafts: Object.fromEntries(Object.entries(s.drafts).filter(([, d]) => d.target.kind === "new_world" || d.target.world !== w)) };
  }
  if (a.Op === "add_world") return { ...s, picker: { ...s.picker!, edit: undefined, filter: "", sel: "+" + String(ed.values.id) } };
  return shut(s);
}

// follow moves editors with the active item, as the TUI's parkEditor
// and unparkEditor: one opened from the main view stays on the item it
// was opened on (parked) when you go elsewhere, or becomes a draft if
// that item went; a parked one on an item that went becomes a draft;
// and the one parked on the item now active comes back.
function follow(s: State, prev: string): State {
  const ed = s.picker?.edit;
  if (s.active !== prev && ed?.closeAll) {
    s = exists(s, prev) ? { ...s, parked: { ...s.parked, [prev]: ed } } : stash(s, ed);
    s = { ...s, picker: undefined };
  }
  for (const [k, p] of Object.entries(s.parked)) {
    if (!exists(s, k)) s = stash({ ...s, parked: without(s.parked, k) }, p);
  }
  const back = s.parked[s.active];
  if (back && s.active !== prev) {
    if (s.picker?.edit) s = stash(s, s.picker.edit);
    s = show({ ...s, picker: undefined, parked: without(s.parked, s.active) }, back);
  }
  return s;
}

export class Store {
  state: State = fresh({ phase: "loading", saving: true, typing: new Typing() });
  private subs = new Set<() => void>();

  constructor(private onEffect?: (e: Event) => void) {}

  // apply takes one batch from Go: every event, then one notification.
  // It never throws: Go calls it from inside the wasm.
  apply(batch: Event[]): void {
    let s = this.state;
    const prev = s.active;
    const seen = s.answer?.n ?? 0;
    const effects: Event[] = [];
    for (const e of batch) {
      try {
        s = this.step(s, e, effects);
      } catch (err) {
        console.error("event not applied", e, err);
      }
    }
    const ed = s.picker?.edit;
    if (ed?.pending && s.answer && s.answer.n > seen && s.answer.Op === ed.pending) s = settle(s, s.answer);
    s = follow(s, prev);
    if (s.picker) {
      // What's offered may have changed (worlds, a character opening or
      // closing): the highlight stays on a row there is, as the TUI's
      // fixPick after a reload. Opened before any worlds came, it was on
      // + World for want of anything else: it starts on the first
      // character once there are some (unless this batch moved it, as a
      // first world saved does to its + Character).
      const was = this.state.worlds.length || s.picker.sel !== "+" ? s.picker.sel : "";
      const sel = fixPick(pickerRows(s.worlds, s.chars, s.picker.filter), was);
      if (sel !== s.picker.sel) s = { ...s, picker: { ...s.picker, sel } };
    }
    this.state = s;
    for (const e of effects) {
      try {
        this.onEffect?.(e);
      } catch (err) {
        console.error("effect failed", e, err);
      }
    }
    this.notify();
  }

  private step(s: State, e: Event, effects: Event[]): State {
    const k: string = e.Key;
    switch (e.type) {
      case "Start": return fresh(s);
      case "Theme": return { ...s, theme: e as ThemeEvent };
      case "Worlds": return { ...s, worlds: e.Worlds ?? [] };
      case "Order": return { ...s, order: e.Keys ?? [] };
      case "Active": return { ...s, active: k };
      case "Char": return { ...s, chars: { ...s.chars, [k]: e } };
      case "Input":
        s.typing?.saw(e.Ack ?? 0);
        return { ...s, inputs: { ...s.inputs, [k]: e } };
      case "Log": {
        const logs = { ...s.logs, [k]: e };
        const logView = e.Open ? { ...s.logView, [k]: s.logView[k] ?? { goN: 0 } } : without(s.logView, k);
        return { ...s, logs, logView };
      }
      case "LogAt": {
        const v = s.logView[k] ?? { goN: 0 };
        return { ...s, logView: { ...s.logView, [k]: { ...v, at: e.Seq, goN: v.goN + 1 } } };
      }
      case "ExportName": {
        const v = s.logView[k] ?? { goN: 0 };
        return { ...s, logView: { ...s.logView, [k]: { ...v, prompt: { kind: "filename", value: e.Name, format: e.Format } } } };
      }
      case "Filter": return { ...s, filters: { ...s.filters, [k]: e } };
      case "Closed":
        return { ...s, chars: without(s.chars, k), inputs: without(s.inputs, k), lines: without(s.lines, k),
          logs: without(s.logs, k), logLines: without(s.logLines, k), filters: without(s.filters, k), logView: without(s.logView, k) };
      case "Lines": {
        const m = merge(s.lines[k], e);
        return m ? { ...s, lines: { ...s.lines, [k]: m } } : s;
      }
      case "LogLines": {
        const m = merge(s.logLines[k], e);
        return m ? { ...s, logLines: { ...s.logLines, [k]: m } } : s;
      }
      case "Status": return { ...s, status: { Text: e.Text, Err: e.Err, Gen: e.Gen } };
      case "Presence": return { ...s, presence: e.State };
      case "Confirm": return { ...s, confirm: e.On };
      case "SaveQuestion": return { ...s, saveQuestion: e };
      case "Saved":
      case "Problems":
        return { ...s, answer: { n: (s.answer?.n ?? 0) + 1, Op: e.Op, Key: e.Key, Problems: e.Problems } };
      case "Settings": {
        const o = s.opening;
        if (!o || o.key !== targetKey(targetOf(e as unknown as SettingsEvent))) return s;
        return show({ ...s, opening: undefined }, newEditor(e as unknown as SettingsEvent, o.closeAll));
      }
      case "Effect": effects.push(e); return s;
    }
    return s;
  }

  // openPicker shows the picker, its filter empty and its highlight on
  // the first character.
  openPicker(): void {
    // An editor up from the main view stays with what it was opened on,
    // as the TUI's + Connection parks it; one in the picker is kept as a
    // draft.
    let st = this.state;
    const ed = st.picker?.edit;
    if (ed) st = ed.closeAll && exists(st, st.active) ? { ...st, parked: { ...st.parked, [st.active]: ed } } : stash(st, ed);
    this.state = { ...st, picker: { filter: "", sel: "" } };
    this.setPicker({});
  }


  // closePicker goes back to the open characters.
  closePicker(): void {
    this.set({ picker: undefined, opening: undefined });
  }

  // setPicker changes the open picker, keeping its highlight on a row the
  // filter offers.
  setPicker(patch: Partial<Picker>): void {
    if (!this.state.picker) return;
    const p = { ...this.state.picker, ...patch };
    p.sel = fixPick(pickerRows(this.state.worlds, this.state.chars, p.filter), p.sel);
    this.set({ picker: p });
  }

  // openEditor opens target's editor over the picker (opening the picker
  // too, and with closeAll closing with it): its draft, or one left
  // parked, as it was; otherwise Go's settings are asked for, and it
  // opens when they come.
  openEditor(t: Target, closeAll: boolean): void {
    const k = targetKey(t);
    let s = this.state;
    let ed: Editor | undefined;
    for (const [at, p] of Object.entries(s.parked)) {
      if (targetKey(p.target) === k) {
        s = { ...s, parked: without(s.parked, at) };
        ed = p;
      }
    }
    ed ??= s.drafts[k];
    if (ed) {
      this.state = show({ ...s, drafts: without(s.drafts, k) }, { ...ed, armed: false, pending: undefined, closeAll });
      this.notify();
      return;
    }
    this.set({ opening: { key: k, closeAll } });
    const w = t.kind === "new_world" ? "" : t.world;
    this.state.act?.({ Do: "settings", World: w, Char: t.kind === "char" ? t.char : "" });
  }

  // closeEditor drops the editor's edits (Esc).
  closeEditor(): void {
    this.state = shut(this.state);
    this.notify();
  }

  // hideEditor closes the editor but keeps its edits for the next time
  // (Ctrl+T, the key that opened it).
  hideEditor(): void {
    const ed = this.state.picker?.edit;
    if (!ed) return;
    this.state = shut(stash(this.state, ed));
    this.notify();
  }

  // leaveEditor keeps the editor's edits as a draft and goes back to the
  // picker, which stays open: a row of it was chosen.
  leaveEditor(): void {
    const ed = this.state.picker?.edit;
    if (!ed) return;
    this.state = { ...stash(this.state, ed), picker: { ...this.state.picker!, edit: undefined } };
    this.notify();
  }

  // setEditor changes the open editor.
  setEditor(patch: Partial<Editor>): void {
    const p = this.state.picker;
    if (p?.edit) this.set({ picker: { ...p, edit: { ...p.edit, ...patch } } });
  }

  // editField sets one of the editor's fields, with Go's check of what's
  // typed.
  editField(key: string, value: string | boolean): void {
    const p = this.state.picker;
    if (!p?.edit) return;
    const check = (globalThis as any).kiln?.check as ((f: string, v: string) => string) | undefined;
    this.set({ picker: { ...p, edit: edit(p.edit, key, value, check) } });
  }

  // sendEditor sends the editor's save or delete; its answer settles it
  // (apply).
  sendEditor(action: { Do: string; [k: string]: unknown }): void {
    this.setEditor({ pending: action.Do });
    this.state.act?.(action);
  }

  // setLogView changes k's log view.
  setLogView(k: string, patch: Partial<LogView>): void {
    this.set({ logView: { ...this.state.logView, [k]: { ...(this.state.logView[k] ?? { goN: 0 }), ...patch } } });
  }

  // set changes the page's own state.
  set(patch: Partial<State>): void {
    this.state = { ...this.state, ...patch };
    this.notify();
  }

  subscribe(fn: () => void): () => void {
    this.subs.add(fn);
    return () => this.subs.delete(fn);
  }

  private notify(): void {
    for (const fn of this.subs) {
      try {
        fn();
      } catch (err) {
        console.error(err);
      }
    }
  }
}

// listing reports whether the sidebar lists the picker, as the TUI's:
// not under an editor opened from the main view (Ctrl+T, /edit), which
// leaves the characters showing so you can switch away from it.
export const listing = (s: State): boolean => !!s.picker && !s.picker.edit?.closeAll;

// logShows reports whether k's log mode is up: open in Go, not hidden.
export const logShows = (s: State, k: string): boolean => !!s.logs[k]?.Open && !s.logView[k]?.hidden;
