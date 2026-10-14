import { useEffect, useLayoutEffect, useRef } from "preact/hooks";
import { tagsFor } from "../look.ts";
import { logAct, logClick, logKey, logRows, placeOf, segments, topFor } from "../log.ts";
import type { LogState, Place } from "../log.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";
import { Line } from "./Line.tsx";

// seqOf is the line number of the log row holding node n, if any.
function seqOf(n: Node | null): number | undefined {
  const el = (n instanceof Element ? n : n?.parentElement)?.closest<HTMLElement>(".lrow");
  return el ? Number(el.dataset.seq) : undefined;
}

// LogView is log mode in the main area, as the TUI's: the history,
// oldest first, a rule before each day, each line's time and gutter (the
// range's ▌, an excluded line's ░), find's matches marked. Nothing is
// read aloud: it's history (spec). The mouse selects as the TUI's: a
// click takes a line, Shift+click extends the range (or inside it takes
// a line out or back), a click on the gutter takes a line out; dragging
// selects a run, as text selection does. Its letters work while it has
// focus (spec). Near the top it asks for older history, keeping your
// place; find and go-to-date bring their line into view.
export function LogView({ state, store }: { state: State; store: Store }) {
  const k = state.active;
  const g = state.logs[k] as LogState;
  const v = state.logView[k] ?? { goN: 0 };
  const lines = state.logLines[k]?.lines ?? [];
  const tags = tagsFor(state.theme, k);
  const el = useRef<HTMLDivElement>(null);
  const act = state.act ?? (() => {});

  const place = useRef<{ k: string; first?: number; at?: Place }>({ k });
  const rowsOf = (e: HTMLElement) => [...e.querySelectorAll<HTMLElement>(".lrow")].map((r) => ({ seq: Number(r.dataset.seq), top: r.offsetTop, height: r.offsetHeight }));

  useEffect(() => el.current?.focus(), [k]);
  // Opened at the newest line, as the TUI's cursor starts there.
  useLayoutEffect(() => {
    if (el.current) el.current.scrollTop = el.current.scrollHeight;
  }, [k]);
  // Older history arriving above keeps your place, as the scrollback's.
  useLayoutEffect(() => {
    const e = el.current, p = place.current, first = lines[0]?.Seq;
    if (e && p.k === k && p.at && p.first !== undefined && first !== p.first) {
      const top = topFor(p.at, rowsOf(e));
      if (top !== undefined) e.scrollTop = top;
    }
    place.current = { k, first, at: p.k === k ? p.at : undefined };
  });
  // Find and go-to-date bring their line into view, once per landing.
  useLayoutEffect(() => {
    if (v.at !== undefined) el.current?.querySelector(`[data-seq="${v.at}"]`)?.scrollIntoView({ block: "center" });
  }, [v.goN]);

  const onScroll = () => {
    const e = el.current!;
    place.current.at = placeOf(rowsOf(e), e.scrollTop);
    if (e.scrollTop < e.clientHeight && !g.Loading && !g.HistDone) act({ Do: "log_older", Key: k });
  };
  const onClick = (e: MouseEvent) => {
    if (window.getSelection()?.isCollapsed === false) return; // a drag: onMouseUp took it
    const seq = seqOf(e.target as Node);
    if (seq === undefined) return;
    const inRange = (e.target as Element).closest(".lrow")?.querySelector(".gutter")?.textContent !== " ";
    logClick(store, seq, { shift: e.shiftKey, gutter: !!(e.target as Element).closest(".gutter"), inRange });
  };
  // Shift+click is log mode's (extend, or take a line out), not the
  // browser's text selection growing, which would read as a drag.
  const onMouseDown = (e: MouseEvent) => {
    if (e.shiftKey) e.preventDefault();
  };
  // A drag selects the lines it covers, as a range.
  const onMouseUp = () => {
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed || !el.current?.contains(sel.anchorNode)) return;
    const a = seqOf(sel.anchorNode), b = seqOf(sel.focusNode);
    if (a !== undefined && b !== undefined) {
      if (g.Searching) act({ Do: "stop_search", Key: k });
      act({ Do: "range", Key: k, Start: a, End: b });
      store.setLogView(k, { clicked: b, at: undefined });
    }
  };
  const onKeyDown = (e: KeyboardEvent) => {
    const does = logKey(e);
    if (!does) return;
    e.preventDefault();
    logAct(store, does);
  };

  return (
    <main class="body">
      <div class="log" ref={el} tabindex={0} role="region" aria-label={str.webLog()} onScroll={onScroll} onMouseDown={onMouseDown} onClick={onClick} onMouseUp={onMouseUp} onKeyDown={onKeyDown}>
        {g.Loading && <div class={roleClass("log.loading")}>{str.browseLoading()}</div>}
        {!lines.length && g.HistDone && <div class={roleClass("log.loading")}>{str.browseNoLogs()}</div>}
        {logRows(lines, g).map((r) => {
          if (r.kind === "day") {
            const rule = str.glyph.rule + str.glyph.rule;
            return <div key={"d" + r.day} class={roleClass("log.day")}>{`${rule} ${r.label} ${rule}`}</div>;
          }
          const l = r.line;
          const gutter = r.excluded ? <span class={"gutter " + roleClass("log.excluded")}>{str.glyph.excluded}</span>
            : r.selected ? <span class={"gutter " + roleClass("log.selected")}>{str.glyph.selected}</span>
            : <span class="gutter"> </span>;
          const text = r.match
            ? <span class="ltext">{segments(l.Plain, g.Find).map((sg, i) => (sg.match ? <mark key={i} class={roleClass("log.find")}>{sg.text}</mark> : sg.text))}</span>
            : <span class="ltext"><Line line={l} tags={tags} /></span>;
          return (
            <div key={l.Seq} class="lrow" data-seq={l.Seq}>
              <span class={roleClass(l.Seq === v.at ? "log.cursor" : "log.time")}>{l.Clock ?? ""}</span> {gutter} {text}
            </div>
          );
        })}
      </div>
    </main>
  );
}
