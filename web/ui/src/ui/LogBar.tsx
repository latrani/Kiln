import { useEffect, useRef } from "preact/hooks";
import { composing } from "../input.ts";
import { cursorOf, filterPromptEnter, formatKey } from "../log.ts";
import type { LogState } from "../log.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// LogBar is log mode's bar in the bottom pane, as the TUI's action bar:
// its keys (the filter panel's while it's open), or a prompt. A text
// prompt (find, a date, export's file name, a text to filter by) is a
// label and a field: Enter sends, Esc closes it and goes back to the log
// (the filter's, to the panel). Export's format prompt takes p, a or h
// (Enter for a default).
export function LogBar({ state, store }: { state: State; store: Store }) {
  const k = state.active;
  const g = state.logs[k] as LogState;
  const v = state.logView[k] ?? { goN: 0 };
  const p = v.prompt;
  const ref = useRef<HTMLInputElement>(null);
  const formatRef = useRef<HTMLDivElement>(null);
  // The caret starts at the end, so go-to's day and time are there to
  // edit, as the TUI's.
  useEffect(() => {
    (p?.kind === "format" ? formatRef.current : ref.current)?.focus();
    ref.current?.setSelectionRange(ref.current.value.length, ref.current.value.length);
  }, [p?.kind]);
  const lines = state.logLines[k]?.lines ?? [];
  const here = cursorOf(g, v, lines) ?? -1;
  const act = state.act ?? (() => {});
  const back = () => {
    if (p?.kind === "filter") {
      store.setLogView(k, { prompt: undefined, panel: { sel: v.panel?.sel ?? "+", follow: true } });
      return;
    }
    store.setLogView(k, { prompt: undefined });
    document.querySelector<HTMLElement>(".log")?.focus();
  };
  if (!p) {
    return <div class={`logbar ${roleClass("log.bar")}`}><span class={roleClass("log.bar.hints")}>{v.panel ? str.filterHints() : g.Marking ? str.browseHintsSelecting() : g.Start != null ? str.browseHintsSelected() : str.browseHintsNone()}</span></div>;
  }
  if (p.kind === "format") {
    const onKeyDown = (e: KeyboardEvent) => {
      const f = formatKey(e, g.ExportFormat);
      if (f === "back") {
        e.preventDefault();
        back();
      } else if (f) {
        e.preventDefault();
        back(); // the log keeps the keys until Go's file name comes (or why not)
        act({ Do: "export_name", Key: k, Format: f });
      }
    };
    return (
      <div ref={formatRef} class={`logbar ${roleClass("log.bar")}`} tabindex={0} role="group" aria-labelledby="kiln-format" onKeyDown={onKeyDown}>
        <span id="kiln-format">{g.ExportFormat ? str.browseFormatPromptDefault(g.ExportFormat) : str.browseFormatPrompt()}</span>
      </div>
    );
  }
  const label = p.kind === "filter" ? str.filterPrompt() : p.kind === "find" ? str.browseFindPrompt() : p.kind === "date" ? str.browseDatePrompt() : str.browseSavePrompt();
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      back();
    } else if (e.key === "Enter" && !composing(e)) {
      e.preventDefault();
      if (p.kind === "filter") {
        filterPromptEnter(store);
        return;
      }
      const value = p.value.trim();
      if (p.kind === "find") act({ Do: "find", Key: k, Term: value, Seq: here });
      else if (p.kind === "date") act({ Do: "goto", Key: k, When: value });
      else act({ Do: "export", Key: k, Format: p.format, Name: value });
      back();
    }
  };
  return (
    <div class={`logbar prompt ${roleClass("log.bar")}`}>
      <label for="kiln-logprompt">{label}</label><input id="kiln-logprompt" ref={ref} value={p.value} autocomplete="off" spellcheck={false}
        onInput={(e) => store.setLogView(k, { prompt: { ...p, value: (e.currentTarget as HTMLInputElement).value } })} onKeyDown={onKeyDown} />
    </div>
  );
}
