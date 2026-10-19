// The page's one store: Go's batches applied a batch at a time, plus the
// page's own state (how far it's got starting Kiln, whether it's saving).
import { checked, edit, newEditor, refuse, targetKey, targetOf } from "./editor.ts";
import type { Editor, SettingsEvent, Target } from "./editor.ts";
import { noHost } from "./host.ts";
import type { Host } from "./host.ts";
import type { Place } from "./log.ts";
import { fixPick, pickerRows } from "./picker.ts";
import * as str from "./str.gen.ts";
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
  Tags?: { Name: string }[] | null;
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
  target: string; // the editor it answers (targetKey of its World and Char)
  Key?: string;
  Problems?: { Field: string; Msg: string }[];
}

// Prompt is log mode's prompt in the bottom pane, as the TUI's: find,
// go to date, export's format (a key), export's file name, a text to
// filter by.
export type Prompt = { kind: "find" | "date" | "filename" | "filter"; value: string; format?: string } | { kind: "format" };

// LogView is the page's side of a character's log mode: hidden by Ctrl+L
// (kept as it was), its prompt, the line find or go-to-date last landed
// on (at; goN counts landings, so each moves the view once), the cursor
// (see cursorOf; follow: scroll it into view on the next draw), the
// filter panel's highlight (none yet: its first entry), and which side has
// focus (focus: the panel; none: the log, or nothing).
export interface LogView {
  hidden?: boolean;
  prompt?: Prompt;
  at?: number;
  goN: number;
  cursor?: number;
  follow?: boolean;
  tags?: { seq: number; text: string }; // Go's word on a line's tags (LineTags), for the status line while it's the cursor's
  panel?: { sel: string; follow?: boolean }; // the filter panel's highlight (filter.ts); the panel's always there in log mode
  focus?: "panel"; // the panel has focus: its keys and footer; else the log's
  away?: { seq: number; dir: "above" | "below" }; // while selecting, the cursor scrolled out of view: which way, by which line (log.ts scrolled)
}

// HistoryPanel is the History event: the active input's history panel
// (Key "" for the idle input's), its lines oldest first, and the
// selected one's index (-1 for none).
export interface HistoryPanel {
  Key: string;
  Open: boolean;
  Query: string;
  Rows: { Text: string; Clock: string }[] | null;
  Sel: number;
  Scope: "character" | "session"; // which lines the panels list, open or not
}

// historyShows reports whether the history panel is up in the input
// area: open for the input showing, nothing else having the area.
export function historyShows(s: State): boolean {
  const h = s.history;
  const key = s.chars[s.active] ? s.active : "";
  return !!h?.Open && h.Key === key && !s.picker && !s.active.startsWith("=") && !logShows(s, s.active) && !s.inputs[s.active]?.NeedPW;
}

export interface State {
  phase: Phase;
  progress?: { got: number; total?: number };
  saving: boolean;
  handingOff?: boolean; // Use here was clicked: Kiln is coming over from another tab
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
  sidebar?: { Collapsed: boolean; Unread: number; Priority: boolean }; // Go's Sidebar event: the collapsed preference and the badge
  narrow?: boolean; // the page is under 64 of its characters across: no sidebar column (App measures it)
  narrowOpen?: boolean; // the narrow sidebar is open; never saved
  narrowPicked?: number; // when a row was last picked from the narrow sidebar (ms), for swallowsClick
  confirm?: boolean;
  history?: HistoryPanel; // the active input's history panel, as Go holds it
  saveQuestion?: any;
  picker?: Picker;
  answer?: Answer;
  drafts: Record<string, Editor>; // hidden editors' edits, by targetKey, for the next time
  parked: Record<string, Editor>; // editors opened from the main view, by the item they were left on
  opening?: { key: string; closeAll: boolean }; // an editor waiting for Go's settings
  notifyOffer?: boolean; // the status line offers to allow notifications (notify.ts)
  subscribe?: () => void; // subscribe to the relay's pushes, once notifications are allowed (push.ts, boot.ts)
  sent?: { key: string; seen: number; n: number }; // the last line sent, and how many so far (the scrollback goes to the end)
}

// fresh is a state holding nothing of Go's: only the page's own (and the
// count of sends, which the scrollback goes by).
const fresh = ({ phase, progress, saving, act, typing, notifyOffer, subscribe, sent }: Pick<State, "phase" | "progress" | "saving" | "act" | "typing" | "notifyOffer" | "subscribe" | "sent">): State => ({
  phase, progress, saving, act, typing, notifyOffer, subscribe, sent,
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

const pickerOf = (s: State, addWorld: boolean): Picker => s.picker ?? { filter: "", sel: fixPick(pickerRows(s.worlds, s.chars, "", addWorld), "") };

// stash keeps ed's edits for the next time its target's editor opens.
const stash = (s: State, ed: Editor): State => ({ ...s, drafts: { ...s.drafts, [targetKey(ed.target)]: ed } });

// show opens ed over the picker (opening it), the editor open there
// before kept as a draft, as the TUI's setEditor.
function show(s: State, ed: Editor, addWorld: boolean): State {
  const old = s.picker?.edit;
  if (old && targetKey(old.target) !== targetKey(ed.target)) s = stash(s, old);
  return { ...s, picker: { ...pickerOf(s, addWorld), adding: undefined, edit: ed } };
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
// TUI's save and delete calls: refused, it shows why; saved, it closes,
// and a deleted world's drafts go with it.
function settle(s: State, a: Answer): State {
  const ed = s.picker!.edit!;
  if (a.Problems) return { ...s, picker: { ...s.picker!, edit: refuse(ed, a.Problems) } };
  if (a.Op === "delete_world" && ed.target.kind === "world") s = dropWorld(s, ed.target.world);
  s = shut(s);
  // A world added: its row is listed whatever the filter was, and its
  // + Character highlighted, as the TUI's saveNewWorld.
  if (a.Op === "add_world" && ed.target.kind === "new-world") s = { ...s, picker: { ...s.picker!, filter: "", sel: "+" + String(ed.values.id) } };
  return s;
}

// dropWorld is s without the drafts and parked editors of world w, which
// is gone.
function dropWorld(s: State, w: string): State {
  const keep = (m: Record<string, Editor>) => Object.fromEntries(Object.entries(m).filter(([, d]) => d.target.kind === "new-world" || d.target.world !== w));
  return { ...s, drafts: keep(s.drafts), parked: keep(s.parked) };
}

// settleAway settles an editor that isn't open (parked, or kept as a
// draft) when Go answers what it sent: saved, it goes, and a deleted
// world's other drafts and parked editors with it; refused, it keeps
// Go's reasons for when it comes back.
function settleAway(s: State, a: Answer): State {
  const mine = (ed: Editor) => ed.pending === a.Op && targetKey(ed.target) === a.target;
  const keep = (m: Record<string, Editor>) =>
    Object.fromEntries(Object.entries(m).flatMap(([k, ed]) => (!mine(ed) ? [[k, ed]] : a.Problems ? [[k, refuse(ed, a.Problems)]] : [])));
  s = { ...s, parked: keep(s.parked), drafts: keep(s.drafts) };
  return a.Op === "delete_world" && !a.Problems ? dropWorld(s, a.target.slice(1)) : s;
}

// follow moves editors with the active item, as the TUI's parkEditor
// and unparkEditor: one opened from the main view stays on the item it
// was opened on (parked) when you go elsewhere, or becomes a draft if
// that item went; a parked one on an item that went becomes a draft;
// and the one parked on the item now active comes back.
function follow(s: State, prev: string, addWorld: boolean): State {
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
    s = show({ ...s, picker: undefined, parked: without(s.parked, s.active) }, back, addWorld);
  }
  return s;
}

export class Store {
  state: State = fresh({ phase: "loading", saving: true, typing: new Typing() });
  // logPlaces is where each character's log view was (LogView), off the
  // state so scrolling doesn't draw; gone when its log mode closes.
  logPlaces = new Map<string, Place>();
  // logSeen is how many landings (find, go to date) each character's log
  // view last showed (LogView), so one made while it was away shows when
  // it comes back; gone with its place.
  logSeen = new Map<string, number>();
  private subs = new Set<() => void>();

  // host is what runs Go, for what the page asks of it (host.ts).
  constructor(private onEffect?: (e: Event) => void, public host: Host = noHost) {}

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
    // Go's answer to a settings action settles the editor it's for, open
    // or not; a settings read that failed stops the wait for it.
    const a = s.answer;
    if (a && a.n > seen) {
      const ed = s.picker?.edit;
      if (a.Op === "settings") {
        if (s.opening?.key === a.target) s = { ...s, opening: undefined };
      } else if (ed?.pending === a.Op && targetKey(ed.target) === a.target) s = settle(s, a);
      else s = settleAway(s, a);
    }
    s = follow(s, prev, this.host.worlds === true);
    if (s.picker) {
      // What's offered may have changed (worlds, a character opening or
      // closing): the highlight stays on a row there is, as the TUI's
      // fixPick after a reload. Opened before any worlds came, it was on
      // + World for want of anything else: it starts on the first
      // character once there are some (unless this batch moved it, as a
      // first world saved does to its + Character).
      const was = this.state.worlds.length || s.picker.sel !== "+" ? s.picker.sel : "";
      const sel = fixPick(pickerRows(s.worlds, s.chars, s.picker.filter, this.host.worlds === true), was);
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
      case "Start":
        this.logPlaces.clear();
        this.logSeen.clear();
        return fresh(s);
      case "Theme": return { ...s, theme: e as ThemeEvent };
      case "Worlds": return { ...s, worlds: e.Worlds ?? [] };
      case "Order": return { ...s, order: e.Keys ?? [] };
      case "Active": return { ...s, active: k };
      case "Char": return { ...s, chars: { ...s.chars, [k]: e } };
      case "Input":
        s.typing?.saw(e.Ack ?? 0);
        return { ...s, inputs: { ...s.inputs, [k]: e } };
      case "Log": {
        if (!e.Open) {
          this.logPlaces.delete(k);
          this.logSeen.delete(k);
        }
        const logs = { ...s.logs, [k]: e };
        const logView = e.Open ? { ...s.logView, [k]: s.logView[k] ?? { goN: 0 } } : without(s.logView, k);
        return { ...s, logs, logView };
      }
      case "LogAt": {
        const v = s.logView[k] ?? { goN: 0 };
        return { ...s, logView: { ...s.logView, [k]: { ...v, at: e.Seq, cursor: e.Seq, goN: v.goN + 1 } } };
      }
      case "Sent": return { ...s, sent: { key: k, seen: e.Seen, n: (s.sent?.n ?? 0) + 1 } };
      case "LineTags": {
        const v = s.logView[k] ?? { goN: 0 };
        return { ...s, logView: { ...s.logView, [k]: { ...v, tags: { seq: e.Seq, text: e.Text } } } };
      }
      case "ExportName": {
        const v = s.logView[k] ?? { goN: 0 };
        return { ...s, logView: { ...s.logView, [k]: { ...v, prompt: { kind: "filename", value: e.Name, format: e.Format } } } };
      }
      case "Filter": return { ...s, filters: { ...s.filters, [k]: e } };
      case "Closed":
        this.logPlaces.delete(k);
        this.logSeen.delete(k);
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
      case "Sidebar": return { ...s, sidebar: e as unknown as State["sidebar"] };
      case "Confirm": return { ...s, confirm: e.On };
      case "History": return { ...s, history: e as unknown as HistoryPanel };
      case "SaveQuestion": return { ...s, saveQuestion: e };
      case "Saved":
      case "Problems":
        return { ...s, answer: { n: (s.answer?.n ?? 0) + 1, Op: e.Op, target: targetKey(targetOf({ World: e.World ?? "", Char: e.Char ?? "" })), Key: e.Key, Problems: e.Problems } };
      case "Settings": {
        const o = s.opening;
        if (!o || o.key !== targetKey(targetOf(e as unknown as SettingsEvent))) return s;
        return show({ ...s, opening: undefined }, newEditor(e as unknown as SettingsEvent, o.closeAll, this.host.worlds === true), this.host.worlds === true);
      }
      case "Effect": effects.push(e); return s;
    }
    return s;
  }

  // openPicker shows the picker, its filter empty and its highlight on
  // the first character.
  openPicker(): void {
    if (logShows(this.state, this.state.active)) {
      this.state.act?.({ Do: "say", Text: str.pickerBlockedByLog(), Err: true });
      return;
    }
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
    this.set({ picker: undefined, opening: undefined, narrowOpen: false }); // picked or escaped: the narrow sidebar closes
  }

  // pickFromSidebar is a click on a sidebar row: Go switches to it, and
  // the narrow sidebar, picked from, closes.
  pickFromSidebar(key: string): void {
    this.state.act?.({ Do: "switch", Key: key });
    const was = this.state.narrowOpen;
    this.set({ narrowOpen: false, narrowPicked: was ? Date.now() : this.state.narrowPicked });
    // Picked: back to the input, as from ≡ closing it (an overview has none).
    if (was) setTimeout(() => globalThis.document?.querySelector<HTMLElement>(".bottom textarea")?.focus());
  }

  // setPicker changes the open picker, keeping its highlight on a row the
  // filter offers.
  setPicker(patch: Partial<Picker>): void {
    if (!this.state.picker) return;
    const p = { ...this.state.picker, ...patch };
    p.sel = fixPick(pickerRows(this.state.worlds, this.state.chars, p.filter, this.host.worlds === true), p.sel);
    this.set({ picker: p });
  }

  // openEditor opens target's editor over the picker (opening the picker
  // too, and with closeAll closing with it): its draft, or one left
  // parked, as it was; otherwise Go's settings are asked for, and it
  // opens when they come. One still waiting on Go's answer to its save or
  // delete keeps waiting: the answer settles it.
  openEditor(t: Target, closeAll: boolean): void {
    if (logShows(this.state, this.state.active)) {
      this.state.act?.({ Do: "say", Text: str.pickerBlockedByLog(), Err: true });
      return;
    }
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
      this.state = show({ ...s, drafts: without(s.drafts, k) }, { ...ed, armed: false, closeAll }, this.host.worlds === true);
      this.notify();
      return;
    }
    this.set({ opening: { key: k, closeAll } });
    this.state.act?.({ Do: "settings", World: t.kind === "new-world" ? "" : t.world, Char: t.kind === "char" ? t.char : "" });
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

  // editField sets one of the editor's fields, then asks Go's check of
  // what's typed; the answer marks it if the same editor is still open.
  editField(key: string, value: string | boolean): void {
    const p = this.state.picker;
    if (!p?.edit) return;
    const target = JSON.stringify(p.edit.target);
    this.set({ picker: { ...p, edit: edit(p.edit, key, value) } });
    if (typeof value !== "string") return;
    this.host.check(key, value).then((msg) => {
      const ed = this.state.picker?.edit;
      if (!ed || JSON.stringify(ed.target) !== target) return;
      const next = checked(ed, key, value, msg);
      if (next !== ed) this.setEditor(next);
    }, (err) => console.error("check", err));
  }

  // sendEditor sends the editor's save or delete; its answer settles it
  // (apply).
  sendEditor(action: { Do: string; [k: string]: unknown }): void {
    this.setEditor({ pending: action.Do });
    this.state.act?.(action);
  }

  // setLogView changes k's log view. A cursor move while selecting moves
  // the range's end, as the TUI's SetCursor.
  setLogView(k: string, patch: Partial<LogView>): void {
    const was = this.state.logView[k]?.cursor;
    this.set({ logView: { ...this.state.logView, [k]: { ...(this.state.logView[k] ?? { goN: 0 }), ...patch } } });
    if (patch.cursor !== undefined && patch.cursor !== was && (this.state.logs[k] as { Marking?: boolean } | undefined)?.Marking)
      this.state.act?.({ Do: "mark_to", Key: k, Seq: patch.cursor });
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
