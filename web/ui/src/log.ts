// Log mode's rules, as the TUI's (internal/ui/browse.go): the rows it
// shows, find's status and marks, and its keys. Lines are addressed by
// their number (Seq); one Go names that the page doesn't hold (not yet
// loaded) is nothing.
import { closePanel, togglePanel } from "./filter.ts";
import type { LineData, LogView, Store } from "./store.ts";
import * as str from "./str.gen.ts";

// LogState is Go's Log event (feed.Log).
export interface LogState {
  Key: string; Open: boolean; Find: string; LastFind?: string; Status: string; StatusErr: boolean;
  Searching: boolean; Loading: boolean; HistDone: boolean; Marking?: boolean;
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
// (start to end by place), excluded, or a find match.
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

// findStatus is find's place for the status line, as the TUI's FindStatus:
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

// cursorOf is the cursor's line, as the TUI's: the line clicked, or the
// one find, go-to-date or a key moved it to; with none yet, the newest
// line shown. One the filter now hides gives way to the nearest shown
// line after it, else before it, as the TUI's Refilter.
export function cursorOf(g: LogState, v: LogView | undefined, lines: LineData[]): number | undefined {
  const hidden = new Set(g.Hidden ?? []);
  const shown = lines.filter((l) => !hidden.has(l.Seq));
  if (v?.cursor === undefined || !lines.some((l) => l.Seq === v.cursor)) return shown.at(-1)?.Seq;
  return (shown.find((l) => l.Seq >= v.cursor!) ?? shown.at(-1))?.Seq;
}

// dragCursor is where the cursor goes when the view scrolls, as the
// TUI's wheel: it stays put until it would leave the view, then rides
// along at the top or bottom edge, on the first or last line wholly in
// view. undefined: it stays where it is.
// scrolled is what scrolling the view does to the cursor. While
// selecting it stays put, as the TUI's wheel, so looking around never
// changes the range; out of view, away says which way it went, by the
// first or last line wholly in view. Otherwise it's dragged along
// (dragCursor).
export function scrolled(selecting: boolean, rows: { seq: number; top: number; height: number }[], top: number, height: number, cursor: number | undefined): { to?: number; away?: { seq: number; dir: "above" | "below" } } {
  if (!selecting) {
    const to = dragCursor(rows, top, height, cursor);
    return to === undefined ? {} : { to };
  }
  const at = rows.find((r) => r.seq === cursor);
  const whole = rows.filter((r) => r.top >= top && r.top + r.height <= top + height);
  if (!at || !whole.length) return {};
  if (at.top < top) return { away: { seq: whole[0].seq, dir: "above" } };
  if (at.top + at.height > top + height) return { away: { seq: whole.at(-1)!.seq, dir: "below" } };
  return {};
}

export function dragCursor(rows: { seq: number; top: number; height: number }[], top: number, height: number, cursor: number | undefined): number | undefined {
  const at = rows.find((r) => r.seq === cursor);
  const whole = rows.filter((r) => r.top >= top && r.top + r.height <= top + height);
  if (!at || !whole.length) return undefined;
  if (at.top < top) return whole[0].seq;
  if (at.top + at.height > top + height) return whole.at(-1)!.seq;
  return undefined;
}

// LogKey is what a key does in log mode.
export type LogKey = "up" | "down" | "mark" | "exclude" | "find" | "older" | "newer" | "date" | "filter" | "export" | "copy" | "back";

const letters: Record<string, LogKey> = {
  ArrowUp: "up", k: "up", ArrowDown: "down", j: "down", s: "mark", " ": "exclude",
  "/": "find", n: "older", N: "newer", g: "date", f: "filter", e: "export", c: "copy", Escape: "back",
};

// logKey is a key in log mode, as the TUI's browseKeys: ↑/↓ and j/k move
// the cursor; PgUp, PgDn, Home and End stay the browser's scrolling, the
// cursor riding along (dragCursor) except while selecting; s selects and
// Space leaves a line out, as the TUI's. Ctrl+A and Cmd+A stay the
// browser's select-all.
export function logKey(e: { key: string; ctrlKey: boolean; altKey: boolean; metaKey: boolean }): LogKey | undefined {
  if (e.ctrlKey || e.altKey || e.metaKey) return undefined;
  return letters[e.key];
}

// exportFormats are export's format keys, as the TUI's exportFormatKeys.
export const exportFormats: Record<string, string> = { p: "plain", a: "ansi", h: "html" }; //str:ok config values

// formatKey is a key in export's format prompt: the format it picks (Enter
// takes the default, if there is one), or "back" for Esc. A key with a
// modifier is never a pick.
export function formatKey(e: { key: string; ctrlKey: boolean; altKey: boolean; metaKey: boolean }, def: string): string | undefined {
  if (e.ctrlKey || e.altKey || e.metaKey) return undefined;
  if (e.key === "Escape") return "back";
  return e.key === "Enter" ? def || undefined : exportFormats[e.key];
}

// wantsOlder reports whether log mode should ask for older history: the
// view near its top, or what's loaded too short to scroll at all, and
// none already coming or none left.
export function wantsOlder(g: { Loading: boolean; HistDone: boolean }, v: { scrollTop: number; scrollHeight: number; clientHeight: number }): boolean {
  return !g.Loading && !g.HistDone && (v.scrollTop < v.clientHeight || v.scrollHeight <= v.clientHeight);
}

// logAct does what a log-mode key asks, as the TUI's browse.key: ↑/↓
// move the cursor a shown line (up from the oldest asks for older
// history); a prompt for find, go-to (starting from the cursor's day and
// time) or export's format; n and N step
// find from the cursor; c copies; f opens or closes the filter panel; Esc stops a search, else closes the
// panel, else turns find off, else closes log mode.
export function logAct(store: Store, does: LogKey): void {
  const s = store.state, k = s.active, g = s.logs[k] as LogState, v = s.logView[k] ?? { goN: 0 };
  const lines = s.logLines[k]?.lines ?? [];
  const here = cursorOf(g, v, lines) ?? -1;
  const act = s.act ?? (() => {});
  switch (does) {
    case "up":
    case "down": {
      if (g.Searching) act({ Do: "stop_search", Key: k }); // you've moved on, as the TUI's
      const hidden = new Set(g.Hidden ?? []);
      const shown = lines.filter((l) => !hidden.has(l.Seq));
      const i = shown.findIndex((l) => l.Seq === here) + (does === "up" ? -1 : 1);
      if (i < 0 && !g.HistDone && !g.Loading) act({ Do: "log_older", Key: k });
      const to = shown[Math.min(Math.max(0, i), shown.length - 1)];
      if (to) store.setLogView(k, { cursor: to.Seq, follow: true, away: undefined });
      break;
    }
    case "find": store.setLogView(k, { prompt: { kind: "find", value: g.LastFind ?? "" } }); break; // the last term, so Enter brings it back
    case "date": { // starting from the cursor's day and time, to edit
      const l = lines.find((l) => l.Seq === here);
      store.setLogView(k, { prompt: { kind: "date", value: l?.Clock ? `${l.Day} ${l.Clock}` : "" } });
      break;
    }
    case "export":
      // Nothing selected: Go says to mark a range, as the TUI's e.
      if (!g.Selected) act({ Do: "export_name", Key: k, Format: "" });
      else store.setLogView(k, { prompt: { kind: "format" } });
      break;
    case "mark": // selecting from now, or locked, before Go answers: a move right after counts
      if (g.Searching) act({ Do: "stop_search", Key: k }); // as the TUI's keys
      act({ Do: "mark", Key: k, Seq: here });
      selecting(store, !g.Marking);
      break;
    case "exclude":
      if (g.Searching) act({ Do: "stop_search", Key: k });
      act({ Do: "exclude", Key: k, Seq: here });
      break;
    case "older": act({ Do: "find_older", Key: k, Seq: here }); break;
    case "newer": act({ Do: "find_newer", Key: k, Seq: here }); break;
    case "copy": act({ Do: "copy", Key: k }); break;
    case "filter": togglePanel(store); break;
    case "back": // one thing at a time: a search, the panel, selecting, find, then log mode
      if (g.Searching) act({ Do: "stop_search", Key: k });
      else if (v.panel) closePanel(store);
      else if (g.Marking) {
        act({ Do: "end_mark", Key: k });
        selecting(store, false);
      }
      else if (g.Find) act({ Do: "end_find", Key: k });
      else act({ Do: "log_close", Key: k });
      break;
  }
}

// filterPromptEnter is Enter in the filter text prompt, as the TUI's:
// the text is added, a new row becoming the highlight, and focus goes
// back to the panel.
export function filterPromptEnter(store: Store): void {
  const s = store.state, k = s.active, v = s.logView[k];
  if (v?.prompt?.kind !== "filter") return;
  const term = v.prompt.value.trim();
  // act first: Go answers inside it, so the new row is listed before the
  // panel draws, and its highlight isn't lost to fixSel's fallback.
  s.act?.({ Do: "add_text", Key: k, Term: term });
  store.setLogView(k, { prompt: undefined, panel: { sel: term ? "t:" + term : v.panel?.sel ?? "+", follow: true } });
}

// toggleLog is Ctrl+L, as the TUI's: log mode showing is hidden, kept
// as it was (a search reading older days stops); hidden, it shows again;
// not open, it opens for the active character. Shown again or opened,
// Go's told: it takes down the status, as the TUI's openBrowse. Nothing with no character
// showing, or with the picker or an editor up.
export function toggleLog(store: Store): void {
  const s = store.state, k = s.active;
  if (!s.chars[k] || s.picker) return; // the picker and editors keep the keys, as the TUI's
  if (s.logs[k]?.Open) {
    const hide = !s.logView[k]?.hidden;
    if (hide && s.logs[k].Searching) s.act?.({ Do: "stop_search", Key: k }); // as the TUI's hideBrowse
    store.setLogView(k, { hidden: hide });
    if (hide) return;
  }
  s.act?.({ Do: "log", Key: k });
}

// selecting says the page is selecting (or not) at once, ahead of Go's
// Log event saying so, and drops an arrow left from the last selection.
function selecting(store: Store, on: boolean): void {
  const k = store.state.active;
  store.set({ logs: { ...store.state.logs, [k]: { ...store.state.logs[k], Marking: on } } });
  store.setLogView(k, { away: undefined });
}

// syncCursor keeps the cursor the page shows on the range's end while
// selecting, as the TUI's: one never moved (the newest line, implicitly)
// is pinned, and one on a line the filter now hides moves to the
// nearest shown, taking the range's end along.
export function syncCursor(store: Store): void {
  const s = store.state, k = s.active, g = s.logs[k] as LogState | undefined, v = s.logView[k];
  if (!g?.Marking) return;
  const c = cursorOf(g, v, s.logLines[k]?.lines ?? []);
  if (c !== undefined && c !== v?.cursor) store.setLogView(k, { cursor: c });
}

// selectChip is a click on the Select chip: s, as the TUI's. It stops a
// search reading older days, and does nothing under a prompt, as a click
// on the log doesn't.
export function selectChip(store: Store): void {
  const s = store.state, k = s.active, g = s.logs[k] as LogState;
  if (g.Searching) s.act?.({ Do: "stop_search", Key: k });
  if (s.logView[k]?.prompt) return;
  logAct(store, "mark");
}

// logClick is a click on line seq, as the TUI's click: it stops a search
// reading older days, and moves the cursor there. While selecting it's a
// shift-click: inside the range it takes the line out (or back in),
// leaving the cursor, the range's end, where it is; outside it grows the
// range to the line. Shift and the gutter change nothing. With a prompt
// open it changes nothing, as the TUI's.
export function logClick(store: Store, seq: number, how: { shift?: boolean; gutter?: boolean; inRange?: boolean }): void {
  const s = store.state, k = s.active, g = s.logs[k] as LogState;
  const act = s.act ?? (() => {});
  if (g.Searching) act({ Do: "stop_search", Key: k });
  if (s.logView[k]?.prompt) return; // the selection can't change under a prompt
  if (g.Marking && how.inRange) {
    act({ Do: "exclude", Key: k, Seq: seq });
    return;
  }
  if (g.Marking) act({ Do: "grow", Key: k, Seq: seq });
  store.setLogView(k, { cursor: seq, away: undefined });
}

// Place is where the view is: the first row showing, and how far its top
// is from the view's (negative: above it).
export type Place = { seq: number; off: number };

// placeOf is the place of a view scrolled to scrollTop over rows (their
// tops and heights within it).
export function placeOf(rows: { seq: number; top: number; height: number }[], scrollTop: number): Place | undefined {
  const r = rows.find((r) => r.top + r.height > scrollTop);
  return r && { seq: r.seq, off: r.top - scrollTop };
}

// topFor is the scrollTop that puts place p back over rows, as the
// scrollback keeps your place when older lines arrive above (the browser's
// own anchoring isn't in Safari); undefined if its line is gone and
// nothing follows it. A hidden line gives way to the next one shown.
export function topFor(p: Place, rows: { seq: number; top: number }[]): number | undefined {
  const r = rows.find((r) => r.seq === p.seq);
  if (r) return r.top - p.off;
  return rows.find((r) => r.seq > p.seq)?.top; // hidden: the next line shown, at the top
}

// openAt is where a character's log opens when it shows (switched to,
// or shown again by Ctrl+L): a find or go-to-date that landed while it
// wasn't showing (goN past seen, the landings it showed), else where you
// were, else the newest line.
export function openAt(p: Place | undefined, seen: number | undefined, goN: number, at: number | undefined): { to: "line"; seq: number } | { to: "place"; place: Place } | { to: "newest" } {
  if (seen !== undefined && goN > seen && at !== undefined) return { to: "line", seq: at };
  return p ? { to: "place", place: p } : { to: "newest" };
}

// keepsPlace reports whether a draw changed what's above your place, so
// it must be put back: older lines arrived, or the filter changed what's
// hidden. Not on the first draw.
export function keepsPlace(was: { first?: number; hidden: string }, now: { first?: number; hidden: string }): boolean {
  return was.first !== undefined && (was.first !== now.first || was.hidden !== now.hidden);
}
