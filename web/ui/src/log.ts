// Log mode's rules, as the TUI's (internal/ui/browse.go): the rows it
// shows, find's status and marks, and its keys. Lines are addressed by
// their number (Seq); one Go names that the page doesn't hold (not yet
// loaded) is nothing.
import type { LineData, Store } from "./store.ts";
import * as str from "./str.gen.ts";

// LogState is Go's Log event (feed.Log).
export interface LogState {
  Key: string; Open: boolean; Find: string; Status: string; StatusErr: boolean;
  Searching: boolean; Loading: boolean; HistDone: boolean;
  Start?: number | null; End?: number | null; Selected: number;
  Excluded?: number[] | null; Hidden?: number[] | null; Matches?: number[] | null;
  ExportFormat: string;
}

// LogRow is one row of log mode: a day's rule, or a line with its marks.
export type LogRow =
  | { kind: "day"; day: string; label: string }
  | { kind: "line"; line: LineData; selected: boolean; excluded: boolean; match: boolean };

// logRows is what log mode shows of lines, oldest first: the lines the
// filter shows, a rule before each day's first, each marked in the range
// (start to end by place; a start alone is just itself), excluded, or a
// find match.
export function logRows(lines: LineData[], g: LogState): LogRow[] {
  const hidden = new Set(g.Hidden ?? []), excluded = new Set(g.Excluded ?? []), matches = new Set(g.Matches ?? []);
  const at = (seq: number | null | undefined) => (seq == null ? -1 : lines.findIndex((l) => l.Seq === seq));
  const s = at(g.Start), e = g.End == null ? s : at(g.End);
  const rows: LogRow[] = [];
  let day = "";
  lines.forEach((l, i) => {
    if (hidden.has(l.Seq)) return;
    if (l.Day !== day) {
      day = l.Day;
      rows.push({ kind: "day", day, label: l.DayLabel ?? day });
    }
    const selected = s >= 0 && e >= 0 && i >= Math.min(s, e) && i <= Math.max(s, e);
    rows.push({ kind: "line", line: l, selected, excluded: selected && excluded.has(l.Seq), match: !!g.Find && matches.has(l.Seq) });
  });
  return rows;
}

// findStatus is find's place for the top bar, as the TUI's FindStatus:
// the term and which match at is, counted from the newest, of how many
// (+ while older history may hold more); while it reads older days, how
// far back it's got.
export function findStatus(g: LogState, at: number | undefined, lines: LineData[]): string {
  if (!g.Find) return "";
  if (g.Searching) return str.browseSearching(g.Find, lines[0]?.Day ?? "");
  const ms = g.Matches ?? [];
  const k = at === undefined ? -1 : ms.indexOf(at);
  const i = k < 0 ? 0 : ms.length - k;
  return g.HistDone ? str.browseFindStatus(g.Find, i, ms.length) : str.browseFindStatusMore(g.Find, i, ms.length);
}

// segments splits text at every match of term, literally and ignoring
// case, as app.FindRE.
export function segments(text: string, term: string): { text: string; match: boolean }[] {
  if (!term) return [{ text, match: false }];
  const re = new RegExp(term.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "giu");
  const out: { text: string; match: boolean }[] = [];
  let last = 0;
  for (const m of text.matchAll(re)) {
    if (m.index! > last) out.push({ text: text.slice(last, m.index), match: false });
    out.push({ text: m[0], match: true });
    last = m.index! + m[0].length;
  }
  if (last < text.length || out.length === 0) out.push({ text: text.slice(last), match: false });
  return out;
}

// LogKey is what a key does in log mode.
export type LogKey = "find" | "older" | "newer" | "date" | "export" | "copy" | "tags" | "back";

const letters: Record<string, LogKey> = { "/": "find", n: "older", N: "newer", g: "date", e: "export", c: "copy", t: "tags", Escape: "back" };

// logKey is a key in log mode, as the TUI's browseKeys less its cursor
// keys (spec). Ctrl+A and Cmd+A stay the browser's select-all.
export function logKey(e: { key: string; ctrlKey: boolean; altKey: boolean; metaKey: boolean }): LogKey | undefined {
  if (e.ctrlKey || e.altKey || e.metaKey) return undefined;
  return letters[e.key];
}

// exportFormats are export's format keys, as the TUI's exportFormatKeys.
export const exportFormats: Record<string, string> = { p: "plain", a: "ansi", h: "html" }; //str:ok config values

// logAct does what a log-mode key asks, as the TUI's browse.key: a
// prompt for find, a date or export's format; n and N step find from
// where it last landed (else the line last clicked, else the newest); c
// copies; t says the tags of that same line; Esc stops a search, else
// closes log mode.
export function logAct(store: Store, does: LogKey): void {
  const s = store.state, k = s.active, g = s.logs[k] as LogState, v = s.logView[k] ?? { goN: 0 };
  const lines = s.logLines[k]?.lines ?? [];
  const here = v.at ?? v.clicked ?? lines.at(-1)?.Seq ?? -1;
  const act = s.act ?? (() => {});
  switch (does) {
    case "find": store.setLogView(k, { prompt: { kind: "find", value: g.Find } }); break;
    case "date": store.setLogView(k, { prompt: { kind: "date", value: "" } }); break;
    case "export": store.setLogView(k, { prompt: { kind: "format" } }); break;
    case "older": act({ Do: "find_older", Key: k, Seq: here }); break;
    case "newer": act({ Do: "find_newer", Key: k, Seq: here }); break;
    case "copy": act({ Do: "copy", Key: k }); break;
    case "tags": if (here >= 0) act({ Do: "tags", Key: k, Seq: here }); break;
    case "back":
      if (g.Searching) act({ Do: "stop_search", Key: k });
      else act({ Do: "log_close", Key: k });
      break;
  }
}

// toggleLog is Ctrl+L, as the TUI's. (For now it only opens log mode;
// hiding and showing it again come with Ctrl+L itself.)
export function toggleLog(store: Store): void {
  store.state.act?.({ Do: "log", Key: store.state.active });
}
