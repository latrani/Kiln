import { useLayoutEffect, useRef, useState } from "preact/hooks";
import type { State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// firstLine is what a history row shows of a sent line: its first line,
// with … when there's more, as the TUI's.
export function firstLine(text: string): string {
  const i = text.indexOf("\n");
  return i < 0 ? text : text.slice(0, i) + "…";
}

// HistoryPanel is the TUI's history panel, above the input in the bottom
// pane: what was sent, oldest first, filtered by what's typed, a line a
// row cut with …, and when it was sent at the right. The selected line
// is bold, with the input's › and its time reversed, and stays in view.
// Scrolled up off the newest, the "more" badge shows at the bottom
// right; scrolled to the top, older days are asked for. A tap on a line
// takes it to the input to edit, as Enter does; the input keeps the
// focus. Where the TUI scrolls a row at a time the browser scrolls
// itself.
export function HistoryPanel({ state }: { state: State }) {
  const h = state.history!;
  const rows = h.Rows ?? [];
  const list = useRef<HTMLDivElement>(null);
  const [up, setUp] = useState(false);
  useLayoutEffect(() => {
    list.current?.querySelector<HTMLElement>("[aria-selected=true]")?.scrollIntoView({ block: "nearest" });
  }, [h.Sel, rows.length]);
  const onScroll = () => {
    const l = list.current!;
    setUp(l.scrollTop + l.clientHeight < l.scrollHeight - 1);
    if (l.scrollTop === 0) state.act?.({ Do: "history_more" });
  };
  return (
    <div class="history">
      <div class="list" ref={list} role="listbox" aria-label={str.viewHistoryButton()} onScroll={onScroll}>
        {rows.length === 0 && <div class={roleClass("input.hint")}>{h.Query ? str.viewHistoryNoMatch() : str.viewHistoryNone()}</div>}
        {rows.map((r, i) => {
          const sel = i === h.Sel;
          return (
            <div role="option" aria-selected={sel} class={`hrow ${roleClass(sel ? "input.history.selected" : "input.history")}`}
              data-mark={sel ? str.glyph.echo : undefined}
              onMouseDown={(e) => e.preventDefault()} onClick={() => state.act?.({ Do: "history_pick", N: i })}>
              <span class="htext">{firstLine(r.Text)}</span>
              <span class={roleClass(sel ? "input.history.cursor" : "input.history.time")}>{r.Clock}</span>
            </div>
          );
        })}
      </div>
      {up && <span class={`more ${roleClass("input.history.more")}`}>{" " + str.viewPillMore() + " "}</span>}
    </div>
  );
}
