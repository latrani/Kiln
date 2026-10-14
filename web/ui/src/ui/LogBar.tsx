import { useEffect, useRef } from "preact/hooks";
import { composing } from "../input.ts";
import { exportFormats } from "../log.ts";
import type { LogState } from "../log.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// LogBar is log mode's bar in the bottom pane, as the TUI's action bar:
// its keys, or a prompt. A text prompt (find, a date, export's file name)
// is a label and a field: Enter sends, Esc closes it and goes back to the
// log. Export's format prompt takes p, a or h (Enter for a default).
export function LogBar({ state, store }: { state: State; store: Store }) {
  const k = state.active;
  const g = state.logs[k] as LogState;
  const v = state.logView[k] ?? { goN: 0 };
  const p = v.prompt;
  const ref = useRef<HTMLInputElement>(null);
  const formatRef = useRef<HTMLDivElement>(null);
  useEffect(() => (p?.kind === "format" ? formatRef.current : ref.current)?.focus(), [p?.kind]);
  const lines = state.logLines[k]?.lines ?? [];
  const here = v.at ?? v.clicked ?? lines.at(-1)?.Seq ?? -1;
  const act = state.act ?? (() => {});
  const back = () => {
    store.setLogView(k, { prompt: undefined });
    document.querySelector<HTMLElement>(".log")?.focus();
  };
  if (!p) {
    return <div class={`logbar ${roleClass("log.bar")}`}><span class={roleClass("log.bar.hints")}>{str.webLogHints()}</span></div>;
  }
  if (p.kind === "format") {
    const onKeyDown = (e: KeyboardEvent) => {
      const f = e.key === "Enter" && g.ExportFormat ? g.ExportFormat : exportFormats[e.key];
      if (e.key === "Escape") {
        e.preventDefault();
        back();
      } else if (f) {
        e.preventDefault();
        store.setLogView(k, { prompt: undefined });
        act({ Do: "export_name", Key: k, Format: f });
      }
    };
    return (
      <div ref={formatRef} class={`logbar ${roleClass("log.bar")}`} tabindex={0} onKeyDown={onKeyDown}>
        {g.ExportFormat ? str.browseFormatPromptDefault(g.ExportFormat) : str.browseFormatPrompt()}
      </div>
    );
  }
  const label = p.kind === "find" ? str.browseFindPrompt() : p.kind === "date" ? str.browseDatePrompt() : str.browseSavePrompt();
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      back();
    } else if (e.key === "Enter" && !composing(e)) {
      e.preventDefault();
      const value = p.value.trim();
      if (p.kind === "find") act({ Do: "find", Key: k, Term: value, Seq: here });
      else if (p.kind === "date") act({ Do: "goto_date", Key: k, Day: value });
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
