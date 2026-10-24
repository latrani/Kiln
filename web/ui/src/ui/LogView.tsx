import { useEffect, useLayoutEffect, useRef } from "preact/hooks";
import { tagsFor } from "../look.ts";
import { filterKey, panelAct } from "../filter.ts";
import { panelOnlyKeys } from "./FilterPanel.tsx";
import { cursorOf, dragEnd, dragStart, dragTo, keepsPlace, logAct, logClick, logKey, logRows, openAt, paintStart, paintTo, placeOf, scrolled, segments, syncCursor, topFor, wantsOlder } from "../log.ts";
import type { LogState, Paint, Place } from "../log.ts";
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
// read aloud: it's history (spec). A click moves the cursor; while
// selecting it grows the range or takes a line out, as the TUI's (see
// logClick). A mouse drag picks the lines it crosses, scrolling at the
// edges (dragStart), and with shift takes them out of the range or puts
// them back (paintStart); a finger's still scrolls. While selecting,
// a cursor scrolled out of view leaves an arrow by the time of the line
// nearest it. Its letters work while it has focus (spec). Near the top it asks for older history, keeping your
// place; find and go-to-date bring their line into view.
export function LogView({ state, store }: { state: State; store: Store }) {
  const k = state.active;
  const g = state.logs[k] as LogState;
  const v = state.logView[k] ?? { goN: 0 };
  const lines = state.logLines[k]?.lines ?? [];
  const tags = tagsFor(state.theme, k);
  const el = useRef<HTMLDivElement>(null);
  const act = state.act ?? (() => {});
  const cursor = cursorOf(g, v, lines);

  const place = useRef<{ k: string; first?: number; hidden: string; at?: Place }>({ k, hidden: "" });
  const rowsOf = (e: HTMLElement) => [...e.querySelectorAll<HTMLElement>(".lrow")].map((r) => ({ seq: Number(r.dataset.seq), top: r.offsetTop, height: r.offsetHeight }));

  // Shown (switched to, or back from Ctrl+L): focus where it was left, the
  // panel's highlight or the log.
  useEffect(() => {
    if (store.state.logView[k]?.focus === "panel") document.querySelector<HTMLElement>(".filter-panel .fsel")?.focus();
    else el.current?.focus();
  }, [k]);
  // Showing a character's log (switched to, or shown again by Ctrl+L): a
  // find or go-to-date that landed while it was away; else where you were,
  // as the TUI keeps each browse; else the newest line, as the TUI's
  // cursor starts there (openAt).
  useLayoutEffect(() => {
    const e = el.current;
    if (!e) return;
    const to = openAt(store.logPlaces.get(k), store.logSeen.get(k), v.goN, v.at);
    store.logSeen.set(k, v.goN);
    let top: number | undefined;
    if (to.to === "line") e.querySelector(`[data-seq="${to.seq}"]`)?.scrollIntoView({ block: "center" });
    else {
      top = to.to === "place" ? topFor(to.place, rowsOf(e)) : undefined;
      e.scrollTop = top ?? e.scrollHeight;
    }
    place.current = { ...place.current, k, at: to.to === "place" && top !== undefined ? to.place : undefined };
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
  // Find and go-to-date bring their line into view, once per landing,
  // for the character showing (one made while it was away shows when it
  // comes back, above).
  const landed = useRef({ k, goN: v.goN });
  useLayoutEffect(() => {
    const was = landed.current;
    landed.current = { k, goN: v.goN };
    store.logSeen.set(k, v.goN);
    if (was.k !== k || was.goN === v.goN || v.at === undefined) return;
    el.current?.querySelector(`[data-seq="${v.at}"]`)?.scrollIntoView({ block: "center" });
  });
  // The status line says the cursor line's tags: Go's to say, as it
  // knows which style each takes. Asked again when the line changes
  // (rules reloaded).
  const cursorLine = lines.find((l) => l.Seq === cursor);
  useEffect(() => {
    if (cursor !== undefined) act({ Do: "tags", Key: k, Seq: cursor });
  }, [k, cursor, cursorLine]);
  // While selecting, the cursor shown is the range's end (syncCursor).
  useLayoutEffect(() => syncCursor(store));
  // A key moved the cursor: it comes into view, as the TUI's.
  useLayoutEffect(() => {
    if (!v.follow) return;
    el.current?.querySelector(`[data-seq="${cursor}"]`)?.scrollIntoView({ block: "nearest" });
    store.setLogView(k, { follow: false });
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
    const sc = scrolled(!!g.Marking, rowsOf(e), e.scrollTop, e.clientHeight, cursor);
    if (sc.to !== undefined) store.setLogView(k, { cursor: sc.to, away: undefined });
    else if (sc.away?.seq !== v.away?.seq || sc.away?.dir !== v.away?.dir) store.setLogView(k, { away: sc.away });
    older(e);
  };
  // A press becomes a drag once the pointer leaves its line or moves a
  // few pixels; above or below the log it scrolls, faster the further
  // out. Letting go ends it, and the click that follows does nothing.
  const drag = useRef<{ id: number; from: number; x: number; y: number; at?: number; started: boolean; scroll: number; raf: number; ended: boolean;
    shift: boolean; paint?: { way: Paint; done: Set<number> } }>();
  const seqAt = (x: number, y: number): number | undefined => {
    const r = el.current!.getBoundingClientRect();
    return seqOf(document.elementFromPoint(Math.min(Math.max(x, r.left + 1), r.right - 1), Math.min(Math.max(y, r.top + 1), r.bottom - 1)));
  };
  const follow = (x: number, y: number) => {
    const d = drag.current!;
    const seq = seqAt(x, y);
    if (seq !== undefined && seq !== d.at) {
      d.at = seq;
      if (d.paint) paintTo(store, seq, d.paint.way, d.paint.done);
      else dragTo(store, seq);
    }
  };
  const autoscroll = () => {
    const d = drag.current, e = el.current;
    if (!d || !e || !d.scroll) return;
    e.scrollTop += d.scroll;
    follow(d.x, d.y);
    d.raf = requestAnimationFrame(autoscroll);
  };
  const onPointerDown = (e: PointerEvent) => {
    if (e.pointerType !== "mouse" || e.button !== 0) return;
    const from = seqOf(e.target as Node);
    if (from === undefined) return;
    drag.current = { id: e.pointerId, from, x: e.clientX, y: e.clientY, started: false, scroll: 0, raf: 0, ended: false, shift: e.shiftKey };
  };
  const onPointerMove = (e: PointerEvent) => {
    const d = drag.current;
    if (!d || d.ended || e.pointerId !== d.id) return;
    if (!d.started) {
      if (Math.hypot(e.clientX - d.x, e.clientY - d.y) < 4 && seqAt(e.clientX, e.clientY) === d.from) return;
      const way = d.shift ? paintStart(store, d.from) : undefined;
      if (d.shift ? !way : !dragStart(store, d.from)) {
        drag.current = undefined;
        return;
      }
      if (way) {
        d.paint = { way, done: new Set() };
        paintTo(store, d.from, way, d.paint.done);
      }
      d.started = true;
      d.at = d.from;
      el.current!.setPointerCapture(d.id);
    }
    d.x = e.clientX;
    d.y = e.clientY;
    follow(d.x, d.y);
    const r = el.current!.getBoundingClientRect();
    const out = d.y < r.top ? d.y - r.top : d.y > r.bottom ? d.y - r.bottom : 0;
    const was = d.scroll;
    d.scroll = Math.sign(out) * Math.min(40, Math.ceil(Math.abs(out) / 2));
    if (d.scroll && !was) d.raf = requestAnimationFrame(autoscroll);
  };
  const onPointerUp = (e: PointerEvent) => {
    const d = drag.current;
    if (!d || e.pointerId !== d.id) return;
    cancelAnimationFrame(d.raf);
    d.scroll = 0;
    if (!d.started) {
      drag.current = undefined;
      return;
    }
    d.ended = true; // until the click that follows
    if (!d.paint) dragEnd(store);
  };
  const onClick = (e: MouseEvent) => {
    if (drag.current?.ended) {
      drag.current = undefined;
      return;
    }
    const seq = seqOf(e.target as Node);
    if (seq === undefined) return;
    const inRange = (e.target as Element).closest(".lrow")?.querySelector(".gutter")?.textContent !== " ";
    logClick(store, seq, { shift: e.shiftKey, gutter: !!(e.target as Element).closest(".gutter"), inRange });
  };
  const onKeyDown = (e: KeyboardEvent) => {
    const does = logKey(e);
    if (does) {
      e.preventDefault();
      logAct(store, does);
      return;
    }
    const p = filterKey(e); // the filter panel's own keys reach it from here too
    if (p && panelOnlyKeys.has(p)) {
      e.preventDefault();
      panelAct(store, p);
    }
  };

  return (
    <main class="body">
      <div class="log" ref={el} tabindex={0} role="region" aria-label={str.webLog()} onScroll={onScroll} onClick={onClick} onKeyDown={onKeyDown}
        onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp} onPointerCancel={onPointerUp}
        onFocusIn={() => { if (state.logView[k]?.focus) store.setLogView(k, { focus: undefined }); }}>
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
              <span class={roleClass(l.Seq === cursor ? "log.cursor" : "log.time")}>{l.Clock ?? ""}</span>
              {g.Marking && v.away?.seq === l.Seq
                ? <span class={roleClass("log.cursor.away")}>{v.away.dir === "above" ? str.glyph.cursorAbove : str.glyph.cursorBelow}</span>
                : " "}
              {gutter} {text}
            </div>
          );
        })}
      </div>
    </main>
  );
}
