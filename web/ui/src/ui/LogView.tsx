import { useEffect, useLayoutEffect, useRef } from "preact/hooks";
import { tagsFor } from "../look.ts";
import { keepsPlace, logAct, logClick, logKey, logRows, placeOf, segments, topFor, wantsOlder } from "../log.ts";
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

  const place = useRef<{ k: string; first?: number; hidden: string; at?: Place }>({ k, hidden: "" });
  const rowsOf = (e: HTMLElement) => [...e.querySelectorAll<HTMLElement>(".lrow")].map((r) => ({ seq: Number(r.dataset.seq), top: r.offsetTop, height: r.offsetHeight }));

  useEffect(() => el.current?.focus(), [k]);
  // Showing a character's log (switched to, or shown again by Ctrl+L):
  // where you were, as the TUI keeps each browse; else the newest line,
  // as the TUI's cursor starts there.
  useLayoutEffect(() => {
    const e = el.current;
    if (!e) return;
    const p = store.logPlaces.get(k);
    const top = p && topFor(p, rowsOf(e));
    e.scrollTop = top ?? e.scrollHeight;
    place.current = { ...place.current, k, at: top !== undefined ? p : undefined };
  }, [k]);
  // Hidden by Ctrl+L (or another view taking the pane): keep where it is
  // now, even if the last scroll's event hasn't come yet. Not once log
  // mode has closed (Esc): it opens at the newest line next time. (A
  // switch keeps the place onScroll saved: by then the rows are the next
  // character's.)
  const shownK = useRef(k);
  shownK.current = k;
  useLayoutEffect(() => {
    const e = el.current; // the ref is gone by the time the cleanup runs
    return () => {
      const k = shownK.current;
      const at = e && placeOf(rowsOf(e), e.scrollTop);
      if (at && store.state.logs[k]?.Open) store.logPlaces.set(k, at);
    };
  }, []);
  // Older history arriving above, or the filter hiding lines, keeps your
  // place, as the scrollback's.
  useLayoutEffect(() => {
    const e = el.current, p = place.current, now = { first: lines[0]?.Seq, hidden: (g.Hidden ?? []).join(",") };
    if (e && p.k === k && p.at && keepsPlace(p, now)) {
      const top = topFor(p.at, rowsOf(e));
      if (top !== undefined) e.scrollTop = top;
    }
    place.current = { k, ...now, at: p.k === k ? p.at : undefined };
  });
  // Find and go-to-date bring their line into view, once per landing:
  // a new one for the character showing, not one from before it showed
  // (its place, above, wins then).
  const landed = useRef({ k, goN: v.goN });
  useLayoutEffect(() => {
    const was = landed.current;
    landed.current = { k, goN: v.goN };
    if (was.k !== k || was.goN === v.goN || v.at === undefined) return;
    el.current?.querySelector(`[data-seq="${v.at}"]`)?.scrollIntoView({ block: "center" });
  });

  // older asks for older history near the top, or when what's loaded
  // doesn't fill the view (so there's nothing to scroll): once per Log
  // from Go, so it doesn't ask again before Go says it's loading.
  const askedFor = useRef<LogState | undefined>(undefined);
  const older = (e: HTMLElement) => {
    if (askedFor.current !== g && wantsOlder(g, e)) {
      askedFor.current = g;
      act({ Do: "log_older", Key: k });
    }
  };
  useLayoutEffect(() => {
    if (el.current) older(el.current);
  });
  const onScroll = () => {
    const e = el.current!;
    place.current.at = placeOf(rowsOf(e), e.scrollTop);
    if (place.current.at) store.logPlaces.set(k, place.current.at);
    older(e);
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
