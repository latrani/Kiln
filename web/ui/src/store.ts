// The page's one store: Go's batches applied a batch at a time, plus the
// page's own state (how far it's got starting Kiln, whether it's saving).
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
}

// Answer is the latest Saved or Problems, numbered, so a form can tell
// a new one from the one it's seen.
export interface Answer {
  n: number;
  Op: string;
  Key?: string;
  Problems?: { Field: string; Msg: string }[];
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
  status?: { Text: string; Err: boolean; Gen: number };
  presence?: number;
  confirm?: boolean;
  saveQuestion?: any;
  picker?: Picker;
  answer?: Answer;
}

// fresh is a state holding nothing of Go's: only the page's own.
const fresh = ({ phase, progress, saving, onUseHere, act, typing }: Pick<State, "phase" | "progress" | "saving" | "onUseHere" | "act" | "typing">): State => ({
  phase, progress, saving, onUseHere, act, typing,
  worlds: [], order: [], active: "", chars: {}, inputs: {}, lines: {}, logs: {}, logLines: {}, filters: {},
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

export class Store {
  state: State = fresh({ phase: "loading", saving: true, typing: new Typing() });
  private subs = new Set<() => void>();

  constructor(private onEffect?: (e: Event) => void) {}

  // apply takes one batch from Go: every event, then one notification.
  // It never throws: Go calls it from inside the wasm.
  apply(batch: Event[]): void {
    let s = this.state;
    const effects: Event[] = [];
    for (const e of batch) {
      try {
        s = this.step(s, e, effects);
      } catch (err) {
        console.error("event not applied", e, err);
      }
    }
    if (s.picker) {
      // What's offered may have changed (worlds, a character opening or
      // closing): the highlight stays on a row there is, as the TUI's
      // fixPick after a reload. Opened before any worlds came, it was on
      // + World for want of anything else: it starts on the first
      // character once there are some.
      const was = this.state.worlds.length ? s.picker.sel : "";
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
      case "Log": return { ...s, logs: { ...s.logs, [k]: e } };
      case "Filter": return { ...s, filters: { ...s.filters, [k]: e } };
      case "Closed":
        return { ...s, chars: without(s.chars, k), inputs: without(s.inputs, k), lines: without(s.lines, k),
          logs: without(s.logs, k), logLines: without(s.logLines, k), filters: without(s.filters, k) };
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
      case "Effect": effects.push(e); return s;
    }
    return s;
  }

  // openPicker shows the picker, its filter empty and its highlight on
  // the first character.
  openPicker(): void {
    this.state = { ...this.state, picker: { filter: "", sel: "" } };
    this.setPicker({});
  }

  // closePicker goes back to the open characters.
  closePicker(): void {
    this.set({ picker: undefined });
  }

  // setPicker changes the open picker, keeping its highlight on a row the
  // filter offers.
  setPicker(patch: Partial<Picker>): void {
    if (!this.state.picker) return;
    const p = { ...this.state.picker, ...patch };
    p.sel = fixPick(pickerRows(this.state.worlds, this.state.chars, p.filter), p.sel);
    this.set({ picker: p });
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
