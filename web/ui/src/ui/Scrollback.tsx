import { useLayoutEffect, useRef, useState } from "preact/hooks";
import { tagsFor } from "../look.ts";
import { below, pause } from "../pager.ts";
import { throttle } from "../presence.ts";
import { sent, settle, switchedAway } from "../scroll.ts";
import type { View } from "../scroll.ts";
import type { LineData, State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";
import { Line } from "./Line.tsx";
import { Overview } from "./Overview.tsx";

function atEnd(e: HTMLElement): boolean {
  return e.scrollHeight - e.scrollTop - e.clientHeight < 2;
}

// rows are the scrollback's lines on the page, in order.
const rows = (e: HTMLElement) => [...e.querySelectorAll<HTMLElement>(".sb")];

// Scrollback is the active character's lines: the page's main landmark.
// Lines that come while it shows are read aloud (the log inside it; a
// role on <main> itself would hide the landmark); lines already there,
// older history, a world's overview and the hints aren't. It moves as the
// TUI's does (scroll.ts): live at the end, stopped by the pager when more
// has come than fits since you last looked (switched away, scrolled to
// the end), the pill then saying how many lines are below; sending a
// line goes to the end, as the TUI's Enter does; scrolled up,
// it keeps the line you're reading where it is, whatever comes above or
// below or however the pane changes size. Near the top it asks for older
// history. While another tab has Kiln, it says so here instead. Behind
// the picker it's a backdrop, dimmed (kiln.css's .dim), as the TUI's.
export function Scrollback({ state }: { state: State }) {
  const el = useRef<HTMLDivElement>(null);
  const views = useRef(new Map<string, View>());
  const shown = useRef({ key: "", epoch: undefined as number | undefined });
  const ours = useRef<number | undefined>(undefined); // the scrollTop the page set
  const left = useRef<number | undefined>(undefined); // the scrollTop as the last draw or scroll left it
  const askedFor = useRef<object | undefined>(undefined); // the Char event older history was last asked for under
  const sends = useRef(state.sent?.n); // the last send taken in
  const act = useRef(state.act);
  act.current = state.act;
  const scrolled = useRef(throttle(() => act.current?.({ Do: "scrolled" }), 1000));
  const [pill, setPill] = useState<number | undefined>(undefined); // unseen lines; undefined while live

  const key = state.active;
  const ls = state.lines[key];
  const ch = state.chars[key];
  const lines: LineData[] = ls?.lines ?? [];
  const mount = `${key}:${ls?.epoch}`;
  const keyNow = useRef(key);
  keyNow.current = key;

  // The lines there when this character's lines were first drawn are
  // history; only those after them go in the live region.
  const boundary = useRef({ mount: "", seq: -1 });
  if (boundary.current.mount !== mount) boundary.current = { mount, seq: lines.at(-1)?.Seq ?? -1 };
  const told = lines.filter((l) => l.Seq <= boundary.current.seq), live = lines.filter((l) => l.Seq > boundary.current.seq);

  const scrollTo = (e: HTMLElement, top: number) => {
    e.scrollTop = top;
    ours.current = e.scrollTop;
  };
  const toEnd = (e: HTMLElement, v: View) => {
    scrollTo(e, e.scrollHeight);
    v.live = true;
    v.unseen = 0;
  };
  // remember notes the line at the top of the view; restore puts it back.
  const remember = (e: HTMLElement, v: View) => {
    const r = rows(e).find((r) => r.offsetTop + r.offsetHeight > e.scrollTop);
    v.anchor = r ? { seq: Number(r.dataset.seq), off: r.offsetTop - e.scrollTop } : undefined;
  };
  const restore = (e: HTMLElement, v: View) => {
    const r = v.anchor && e.querySelector<HTMLElement>(`.sb[data-seq="${v.anchor.seq}"]`);
    if (r) scrollTo(e, r.offsetTop - v.anchor!.off);
  };
  // older asks for a page of older history when the view nears the top:
  // once until Go says anything new of the character (Loading, or a page
  // that came), so scrolling doesn't ask over and over, and nothing Go
  // sends can leave it waiting.
  const older = (e: HTMLElement) => {
    if (key && ch?.More && !ch.Loading && askedFor.current !== ch && e.scrollTop < e.clientHeight) {
      askedFor.current = ch;
      act.current?.({ Do: "page", Key: key });
    }
  };

  // took takes in a scroll: where the view is now, and whether it's live;
  // reaching the end yourself counts as seeing what's there. The page's
  // own scrolls already decided whether it's live: their events can come
  // after the pane changed size, when the end has moved, and mustn't
  // undo that (the resize and the draw that follow put it right).
  const took = (e: HTMLElement) => {
    const v = views.current.get(keyNow.current);
    const mine = ours.current !== undefined && Math.abs(e.scrollTop - ours.current) < 1;
    ours.current = undefined;
    left.current = e.scrollTop;
    if (!v) return;
    remember(e, v);
    if (mine) return;
    v.live = atEnd(e);
    if (v.live) {
      v.unseen = 0;
      v.seen = v.last;
    }
    setPill(v.live ? undefined : v.unseen);
  };

  useLayoutEffect(() => {
    const e = el.current;
    const prev = shown.current;
    shown.current = { key, epoch: ls?.epoch };
    for (const k of views.current.keys()) if (!state.chars[k]) views.current.delete(k); // closed
    // A line sent goes to the end, as the TUI's Enter: after a scroll you
    // made before it, and before a switch that came after it (which then
    // sees the rest), as the TUI's order. Those two never meet: a pending
    // scroll is only taken in when the character didn't change.
    const takeSent = () => {
      if (!state.sent || state.sent.n === sends.current) return;
      sends.current = state.sent.n;
      const sv = views.current.get(state.sent.key);
      if (sv) views.current.set(state.sent.key, sent(sv, state.sent.seen));
    };
    if (prev.key !== key) {
      takeSent();
      // Switching away counts as seeing what was showing, if it was live.
      const pv = views.current.get(prev.key);
      if (pv) views.current.set(prev.key, switchedAway(pv, state.lines[prev.key]?.lines ?? []));
    }
    if (!e) return;
    if (!ls) {
      setPill(undefined);
      return;
    }
    // A scroll whose event hasn't come yet is taken in first, or this draw
    // would undo it.
    if (prev.key === key && prev.epoch === ls.epoch && left.current !== undefined && Math.abs(e.scrollTop - left.current) >= 1) took(e);
    takeSent();
    const [v, move] = settle(views.current.get(key), ls.epoch, lines, prev.key !== key || prev.epoch !== ls.epoch);
    views.current.set(key, v);
    if (move.to === "anchor") restore(e, v);
    else {
      let top: number | undefined;
      if (move.pauseFrom !== undefined) {
        const rs = rows(e);
        top = pause(rs.map((r) => r.offsetTop), rs.map((r) => r.offsetHeight), move.pauseFrom, e.clientHeight);
        if (top !== undefined) {
          scrollTo(e, top);
          v.live = false;
          v.unseen = below(rs.map((r) => r.offsetTop), e.scrollTop, e.clientHeight);
        }
      }
      if (top === undefined) toEnd(e, v);
    }
    remember(e, v);
    left.current = e.scrollTop;
    setPill(v.live ? undefined : v.unseen);
    older(e);
  });

  // The pane changing size (the window, the input growing) rewraps the
  // lines: a live view stays at the end, another keeps its line.
  useLayoutEffect(() => {
    const e = el.current;
    if (!e) return;
    const ro = new ResizeObserver(() => {
      const v = views.current.get(keyNow.current);
      if (!v) return;
      if (v.live) toEnd(e, v);
      else restore(e, v);
      left.current = e.scrollTop;
    });
    ro.observe(e);
    return () => ro.disconnect();
  }, [mount]);

  const onScroll = () => {
    const e = el.current!;
    took(e);
    older(e);
  };
  const onPill = () => {
    const e = el.current!;
    const v = views.current.get(key);
    if (!v) return;
    toEnd(e, v);
    v.seen = v.last;
    remember(e, v);
    left.current = e.scrollTop;
    setPill(undefined);
  };

  const tags = tagsFor(state.theme, key);
  const prompt = state.inputs[key]?.Prompt;
  const sb = (l: LineData) => <Line key={`${ls!.epoch}:${l.Seq}`} line={l} tags={tags} class="sb" seq={l.Seq} />;
  return (
    <main class="body">
      <div class={state.picker ? `scroll dim ${roleClass("scrollback.inactive")}` : "scroll"} ref={el} onScroll={onScroll} onWheel={scrolled.current} onTouchMove={scrolled.current} key={mount}>
        {state.phase === "locked" && (
          <div class="lock">
            <p>{str.webOtherTab()}</p>
            <button class={roleClass("form.button")} onClick={state.onUseHere} disabled={!state.onUseHere}>{str.webUseHere()}</button>
          </div>
        )}
        {key.startsWith("=") && <Overview state={state} world={key.slice(1)} />}
        {!key && state.phase === "running" && state.worlds.every((w) => !w.Characters?.length) && (
          <div class={"line " + roleClass("scrollback.empty")}>{str.viewNoCharacters()}</div>
        )}
        {ch?.More && <div class={"line " + roleClass("scrollback.loading")}>{str.scrollbackLoading()}</div>}
        <div>{told.map(sb)}</div>
        <div role="log" aria-live="polite">
          {live.map(sb)}
          {prompt?.length > 0 && <Line line={{ Seq: 0, Kind: "server", Day: "", Out: false, Plain: "", Spans: prompt, Priority: false, Quiet: false }} tags={tags} />}
        </div>
      </div>
      {pill !== undefined && (
        <button class={"pill " + roleClass("scrollback.pill")} onClick={onPill}>{pill > 0 ? str.viewPillNew(pill) : str.viewPillMore()}</button>
      )}
    </main>
  );
}
