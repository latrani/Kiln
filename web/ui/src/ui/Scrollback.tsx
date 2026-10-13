import { useLayoutEffect, useRef, useState } from "preact/hooks";
import { tagsFor } from "../look.ts";
import { below, pause } from "../pager.ts";
import { throttle } from "../presence.ts";
import type { LineData, State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";
import { Line } from "./Line.tsx";
import { Overview } from "./Overview.tsx";

// View is the page's memory of one character's scrollback, kept across
// switches as the TUI keeps each character's: whether it's live (at the
// bottom), where it was (its distance from the end), the last line seen
// (the pager's mark) and how many lines came below the view.
interface View {
  live: boolean;
  fromEnd: number;
  seen?: { epoch: number; seq: number };
  unseen: number;
}

// Shown is what the last draw showed.
interface Shown {
  key: string;
  epoch?: number;
  first?: number;
  last?: number;
}

function atEnd(e: HTMLElement): boolean {
  return e.scrollHeight - e.scrollTop - e.clientHeight < 2;
}

// Scrollback is the active character's lines, read aloud as they come:
// the page's main landmark, with the log inside it (a role on <main>
// itself would hide the landmark). It sticks to the bottom while live,
// and stops as the TUI's pager does when more has come than fits since
// you last looked (sent, switched away, or scrolled to the end); the
// pill then says how many lines are below. Near the top it asks for
// older history, keeping your place when it comes. With a world active,
// it's the world's overview; with nothing configured, a hint. While
// another tab has Kiln, it says so here instead.
export function Scrollback({ state }: { state: State }) {
  const el = useRef<HTMLDivElement>(null);
  const views = useRef(new Map<string, View>());
  const shown = useRef<Shown>({ key: "" });
  const ours = useRef<number | undefined>(undefined); // the scrollTop the page set
  const asked = useRef(false);
  const act = useRef(state.act);
  act.current = state.act;
  const scrolled = useRef(throttle(() => act.current?.({ Do: "scrolled" }), 1000));
  const [pill, setPill] = useState<number | undefined>(undefined); // unseen lines; undefined while live

  const key = state.active;
  const ls = state.lines[key];
  const ch = state.chars[key];
  const lines: LineData[] = ls?.lines ?? [];
  const first = lines[0]?.Seq, last = lines.at(-1)?.Seq;

  const view = (k: string): View => {
    let v = views.current.get(k);
    if (!v) views.current.set(k, (v = { live: true, fromEnd: 0, unseen: 0 }));
    return v;
  };
  const scrollTo = (e: HTMLElement, top: number) => {
    e.scrollTop = top;
    ours.current = e.scrollTop;
  };
  const toEnd = (e: HTMLElement, v: View) => {
    scrollTo(e, e.scrollHeight);
    v.live = true;
    v.unseen = 0;
  };
  const markSeen = (v: View) => {
    if (ls && last !== undefined) v.seen = { epoch: ls.epoch, seq: last };
  };
  // older asks for a page of older history when the view nears the top.
  const older = (e: HTMLElement) => {
    if (key && ch?.More && !ch.Loading && !asked.current && e.scrollTop < e.clientHeight) {
      asked.current = true;
      act.current?.({ Do: "page", Key: key });
    }
  };

  useLayoutEffect(() => {
    const e = el.current;
    const prev = shown.current;
    shown.current = { key, epoch: ls?.epoch, first, last };
    if (ch?.Loading || prev.key !== key || prev.first !== first) asked.current = false;
    if (!e) return;
    const v = view(key);
    if (prev.key !== key) {
      // Switching away counts as seeing what was showing, if it was live.
      const pv = prev.key ? views.current.get(prev.key) : undefined;
      if (pv?.live && prev.last !== undefined) pv.seen = { epoch: prev.epoch!, seq: prev.last };
      if (!v.seen) markSeen(v); // history isn't news
      if (v.live) toEnd(e, v);
      else scrollTo(e, e.scrollHeight - v.fromEnd);
    } else if (ls && prev.epoch !== ls.epoch) {
      markSeen(v); // a character's lines loaded again aren't news
      if (v.live) toEnd(e, v);
      else scrollTo(e, e.scrollHeight - v.fromEnd);
    } else if (first !== undefined && prev.first !== undefined && first < prev.first) {
      scrollTo(e, e.scrollHeight - v.fromEnd); // older lines came above: stay put
    } else if (last !== undefined && prev.last !== undefined && last > prev.last) {
      if (v.live) {
        const rows = [...e.querySelectorAll<HTMLElement>(":scope > .sb")];
        const tops = rows.map((r) => r.offsetTop), heights = rows.map((r) => r.offsetHeight);
        const seen = v.seen?.epoch === ls?.epoch ? v.seen!.seq : -Infinity;
        const top = pause(tops, heights, lines.findIndex((l) => l.Seq > seen), e.clientHeight);
        if (top === undefined) toEnd(e, v);
        else {
          scrollTo(e, top);
          v.live = false;
          v.unseen = below(tops, top, e.clientHeight);
        }
      } else v.unseen += lines.filter((l) => l.Seq > prev.last!).length;
    } else if (v.live) toEnd(e, v);
    v.fromEnd = e.scrollHeight - e.scrollTop;
    setPill(v.live ? undefined : v.unseen);
    older(e);
  });

  const onScroll = () => {
    const e = el.current!;
    const v = view(key);
    const mine = ours.current !== undefined && Math.abs(e.scrollTop - ours.current) < 1;
    ours.current = undefined;
    v.live = atEnd(e);
    v.fromEnd = e.scrollHeight - e.scrollTop;
    if (v.live) {
      v.unseen = 0;
      if (!mine) markSeen(v); // you scrolled to the end: you've seen it
    }
    setPill(v.live ? undefined : v.unseen);
    older(e);
  };
  const onPill = () => {
    const e = el.current!;
    const v = view(key);
    toEnd(e, v);
    markSeen(v);
    v.fromEnd = e.scrollHeight - e.scrollTop;
    setPill(undefined);
  };

  const tags = tagsFor(state.theme, key);
  const prompt = state.inputs[key]?.Prompt;
  return (
    <main class="body">
      <div class="log" role="log" aria-live="polite" ref={el} onScroll={onScroll} onWheel={scrolled.current} onTouchMove={scrolled.current} key={`${key}:${ls?.epoch}`}>
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
        {lines.map((l) => <Line key={`${ls!.epoch}:${l.Seq}`} line={l} tags={tags} class="sb" />)}
        {prompt?.length > 0 && <Line line={{ Seq: 0, Kind: "server", Day: "", Out: false, Plain: "", Spans: prompt, Attention: false, Quiet: false }} tags={tags} />}
      </div>
      {pill !== undefined && (
        <button class={"pill " + roleClass("scrollback.pill")} onClick={onPill}>{pill > 0 ? str.viewPillNew(pill) : str.viewPillMore()}</button>
      )}
    </main>
  );
}
