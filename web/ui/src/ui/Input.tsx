import { useEffect, useRef, useState } from "preact/hooks";
import { keyAction, strayKey } from "../input.ts";
import { historyShows } from "../store.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// Session states (session.State).
const connecting = 1, connected = 2, failed = 3;

// hint is what the empty input says, as the TUI's prompt: how to choose
// a connection with nothing open, or why there's nothing to send to yet.
// Nothing while Kiln loads: there's nothing to choose from yet.
function hint(c: any, running: boolean): string | undefined {
  if (!running) return undefined;
  if (!c) return str.viewNothingOpen();
  if (c.State === connected) return undefined;
  if (c.CertChanged) return str.viewCertChanged();
  if (c.State === connecting) return str.viewConnecting();
  if (c.State === failed) return str.viewConnectFailed();
  return str.viewDisconnected();
}

// over is where the text runs past the line limit (Go's kiln.over), in
// the browser's string offsets; none before Kiln runs.
function over(text: string, c: any): [number, number][] {
  const k = (globalThis as any).kiln;
  if (!c || !k?.over || !text) return [];
  return k.over(text, c.MaxLineBytes ?? 0, c.NewlineMode ?? "");
}

// Overflow draws text with what's over the limit in input.over_limit,
// behind the textarea, whose own text is then transparent: a textarea
// can't color part of its text.
function Overflow({ text, ranges }: { text: string; ranges: [number, number][] }) {
  const parts = [];
  let at = 0;
  for (const [a, b] of ranges) {
    parts.push(text.slice(at, a), <span class={roleClass("input.over_limit")}>{text.slice(a, b)}</span>);
    at = b;
  }
  parts.push(text.slice(at), "\n"); // a trailing newline still takes a row
  return <div class="overflow" aria-hidden="true">{parts}</div>;
}

// caretRows says whether the caret is on t's first screen row:
// the text up to it, laid out as t lays it out, shows where its row is.
function caretRows(t: HTMLTextAreaElement): { first: boolean } {
  const m = document.createElement("div");
  const cs = getComputedStyle(t);
  m.style.cssText = `position:absolute;visibility:hidden;top:0;left:0;width:${t.clientWidth}px;font:${cs.font};letter-spacing:${cs.letterSpacing};white-space:pre-wrap;overflow-wrap:anywhere;tab-size:${cs.tabSize};padding:0;border:0`;
  const mark = document.createElement("span");
  mark.textContent = "\u200b";
  m.append(t.value.slice(0, t.selectionStart), mark, t.value.slice(t.selectionStart) + "\u200b");
  t.parentElement!.append(m);
  const lh = parseFloat(cs.lineHeight) || mark.offsetHeight;
  const top = mark.offsetTop;
  m.remove();
  return { first: top < lh / 2 };
}

// Input is the TUI's input: what you type for the character showing (or,
// with nothing open, commands), sent with Enter; Shift+Enter is a new
// line; ↑ from the first row opens the history panel, whose query it
// then is (keyAction has its keys); Esc cancels and, as the pill does,
// goes back to the end of the scrollback; PgUp/PgDn scroll the
// scrollback. The hint is its placeholder. Focus
// comes here on load, after a switch, and with a letter typed outside
// any control.
export function Input({ state, store }: { state: State; store: Store }) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const [, redraw] = useState(0);
  const c = state.chars[state.active];
  const key = c ? state.active : "";
  const go = state.inputs[key];
  const typing = state.typing;
  let value: string = go?.Text ?? "";
  if (typing) {
    if (typing.takes(key, go?.Ack ?? 0)) typing.adopt(key, value);
    else value = typing.text(key) ?? value;
  }
  const ranges = over(value, c);
  const said = value ? undefined : hint(c, state.phase === "running"); // shown instead of the ›, as the TUI does
  const flat = c?.NewlineMode === "flatten"; // sent as one line: one ›
  const panel = historyShows(state);

  useEffect(() => ref.current?.focus(), [state.active]);
  useEffect(() => {
    // What's typed with focus outside any control goes to the input:
    // letters, accents and IMEs too (strayKey), never a shortcut. That
    // rests on page.tsx's capture listener being added first (in mount,
    // before this effect runs), so its shortcuts are defaultPrevented here.
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as Element | null;
      if (!strayKey(e)) return;
      if (t && t !== document.body && t.closest("a, button, input, textarea, select, summary, [contenteditable], nav")) return;
      ref.current?.focus();
    };
    addEventListener("keydown", onKey, true);
    return () => removeEventListener("keydown", onKey, true);
  }, []);

  const onKeyDown = (e: KeyboardEvent) => {
    const t = e.currentTarget as HTMLTextAreaElement;
    const rows = e.key === "ArrowUp" && !panel ? caretRows(t) : {};
    const does = keyAction(e, { start: t.selectionStart, end: t.selectionEnd, value: t.value, ...rows }, panel);
    if (!does) return;
    e.preventDefault();
    switch (does) {
      case "submit":
        if (!c && !t.value && !panel) {
          store.openPicker(); // nothing to send to: choose a connection, as the TUI's Enter
          return;
        }
        typing?.submit(key, t.value);
        redraw((n) => n + 1);
        return;
      case "older":
        typing?.send(key, { Do: does, Text: t.value });
        return;
      case "history_up":
      case "history_down":
        typing?.act({ Do: "history_move", N: does === "history_up" ? -1 : 1 });
        return;
      case "history_close":
        typing?.act({ Do: "history_close" });
        return;
      case "cancel":
        typing?.act({ Do: "cancel" });
        document.querySelector<HTMLElement>("main .pill")?.click(); // back to live, from a pause or a scroll
        return;
      case "page_up":
      case "page_down": {
        const l = panel && document.querySelector<HTMLElement>(".history .list");
        if (l) { // a page of the panel, as the TUI's; a page down never closes it
          const n = Math.max(1, Math.floor(l.clientHeight / parseFloat(getComputedStyle(l).lineHeight)) - 1);
          const last = (state.history?.Rows?.length ?? 0) - 1, sel = state.history?.Sel ?? last;
          const by = does === "page_up" ? -n : Math.min(n, last - sel);
          if (by) typing?.act({ Do: "history_move", N: by });
          return;
        }
        const s = document.querySelector<HTMLElement>("main .scroll");
        if (s) s.scrollBy({ top: (does === "page_up" ? -1 : 1) * (s.clientHeight - parseFloat(getComputedStyle(s).lineHeight)) });
      }
    }
  };
  const onInput = (e: Event) => {
    typing?.edit(key, (e.currentTarget as HTMLTextAreaElement).value);
    if (panel) typing?.flush(); // it's the panel's query: filter as it's typed
    redraw((n) => n + 1);
  };

  return (
    <div class="input-row">
      <div class={"field" + (said ? "" : " echo") + (ranges.length ? " marked" : "")}>
        <div class="mirror" aria-hidden="true">
          {value.split("\n").map((l, i) => <div data-mark={!said && (i === 0 || !flat) ? str.glyph.echo : undefined}>{l}</div>)}
        </div>
        {ranges.length > 0 && <Overflow text={value} ranges={ranges} />}
        <textarea ref={ref} rows={1} value={value} placeholder={said}
          aria-label={c ? str.webInput(c.Name) : str.webInputIdle()} disabled={state.phase === "stopped"}
          spellcheck={false} autocomplete="off" onInput={onInput} onKeyDown={onKeyDown} />
      </div>
    </div>
  );
}
