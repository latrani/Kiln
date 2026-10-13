import { useEffect, useRef } from "preact/hooks";
import { composing } from "../input.ts";
import { choose, movePick, pagePick, pickerRows } from "../picker.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// pageRows is how many rows PgUp/PgDn move the picker's highlight: the
// sidebar's visible rows less one, as the TUI's.
function pageRows(): number {
  const nav = document.querySelector<HTMLElement>("nav.side");
  if (!nav) return 1;
  const lh = parseFloat(getComputedStyle(nav).lineHeight) || 16;
  return Math.max(1, Math.floor(nav.clientHeight / lh) - 1);
}

// PickerForm is the picker's part of the bottom pane, the TUI's
// one-field form: the filter, a combobox for the picker's list in the
// sidebar (↑/↓ and PgUp/PgDn move the highlight, Enter chooses, Esc
// closes). It takes focus when it opens; closing gives the input its
// place back, and the input takes focus as it comes.
export function PickerForm({ state, store }: { state: State; store: Store }) {
  const p = state.picker!;
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => ref.current?.focus(), []);
  const onKeyDown = (e: KeyboardEvent) => {
    const rows = pickerRows(state.worlds, state.chars, p.filter);
    const plain = !e.shiftKey && !e.altKey && !e.ctrlKey && !e.metaKey;
    let sel: string | undefined;
    switch (plain ? e.key : "") {
      case "ArrowUp": sel = movePick(rows, p.sel, -1); break;
      case "ArrowDown": sel = movePick(rows, p.sel, 1); break;
      case "PageUp": sel = pagePick(rows, p.sel, -1, pageRows()); break;
      case "PageDown": sel = pagePick(rows, p.sel, 1, pageRows()); break;
      case "Enter":
        if (composing(e)) return;
        e.preventDefault();
        choose(store, p.sel);
        return;
      case "Escape":
        e.preventDefault();
        store.closePicker();
        return;
      default:
        return;
    }
    e.preventDefault();
    store.setPicker({ sel });
  };
  return (
    <div class="pick-row">
      <label for="kiln-filter" class={roleClass("form.label")}>{str.pickerFilter()}: </label>
      <input id="kiln-filter" ref={ref} value={p.filter} role="combobox" aria-controls="kiln-picker" aria-expanded="true"
        aria-autocomplete="list" aria-activedescendant={p.sel ? "pick-" + p.sel : undefined} autocomplete="off" spellcheck={false}
        disabled={state.phase === "stopped"} onKeyDown={onKeyDown}
        onInput={(e) => store.setPicker({ filter: (e.currentTarget as HTMLInputElement).value })} />
      <span class={roleClass("form.hint")}>{str.separator()}{str.pickerHint()}</span>
    </div>
  );
}
