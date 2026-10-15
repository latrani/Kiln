// How the scrollback moves, as the TUI's (internal/ui/scrollback.go):
// the decisions, apart from the DOM that carries them out.
import type { LineData } from "./store.ts";

// View is the page's memory of one character's scrollback, kept while
// it's open: the numbering it's for, whether it's live (at the end), the
// last line seen (the pager's mark), the last line drawn, how many lines
// came below the view since it stopped, and the line at the view's top
// (and how far above the top its own top is), which keeps your place.
export interface View {
  epoch: number;
  live: boolean;
  seen: number;
  last: number;
  unseen: number;
  anchor?: { seq: number; off: number };
}

// Move is what a draw does to the view: go to the end, unless the pager
// stops it at line pauseFrom (an index) because more has come than fits;
// or keep the anchored line where it was.
export type Move = { to: "end"; pauseFrom?: number } | { to: "anchor" };

// settle is the view and its move after a draw of lines (numbering
// epoch). shown says the draw shows the character anew: switched to, or
// its lines loaded again. A view from another numbering starts over,
// live with its history seen. A live view goes to the end, and the pager
// may stop it at the first line not seen when it's shown or lines come.
// A view scrolled up keeps its place, counting the lines that came
// (quiet ones aren't news).
export function settle(v: View | undefined, epoch: number, lines: LineData[], shown: boolean): [View, Move] {
  const last = lines.at(-1)?.Seq ?? -1;
  if (!v || v.epoch !== epoch) return [{ epoch, live: true, seen: last, last, unseen: 0 }, { to: "end" }];
  const come = lines.filter((l) => l.Seq > v.last);
  const next = { ...v, last };
  if (!v.live) {
    next.unseen += come.filter((l) => !l.Quiet).length;
    return [next, { to: "anchor" }];
  }
  if (!shown && !come.length) return [next, { to: "end" }];
  const i = lines.findIndex((l) => l.Seq > v.seen);
  return [next, i < 0 ? { to: "end" } : { to: "end", pauseFrom: i }];
}

// sent is v after you send a line, as the TUI's ToBottomKeeping: live,
// at the end, with everything seen; except that a line echoed (the
// newest) stays unseen, so if what comes next fills the pane the pager
// stops with your line at the top.
export function sent(v: View, lines: LineData[], echoed: boolean): View {
  const last = lines.at(-1)?.Seq ?? -1;
  const seen = echoed ? (lines.at(-2)?.Seq ?? -1) : last;
  return { ...v, live: true, unseen: 0, seen, last, anchor: undefined };
}
