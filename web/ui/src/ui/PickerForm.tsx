import { useEffect, useRef } from "preact/hooks";
import { composing } from "../input.ts";
import { answered, choose, movePick, pagePick, pickerRows } from "../picker.ts";
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

// PickerForm is the picker's part of the bottom pane, a TUI one-field
// form: the filter, or while adding a character its name. The filter is
// a combobox for the picker's list in the
// sidebar (↑/↓ and PgUp/PgDn move the highlight, Enter chooses, Esc
// closes). It takes focus when it opens; closing gives the input its
// place back, and the input takes focus as it comes.
export function PickerForm({ state, store }: { state: State; store: Store }) {
  return state.picker!.adding !== undefined ? <NameForm key={state.picker!.adding} state={state} store={store} /> : <FilterForm state={state} store={store} />;
}

function FilterForm({ state, store }: { state: State; store: Store }) {
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

// NameForm asks for a new character's name, as the TUI's: Enter adds it
// to the world and connects it; Esc goes back to the filter. As you type,
// Go's check says what won't do, in place of the hint; Enter still
// tries (Go's save has the last word, and says why in the same place).
function NameForm({ state, store }: { state: State; store: Store }) {
  const p = state.picker!;
  const ref = useRef<HTMLInputElement>(null);
  const seen = useRef(state.answer?.n ?? 0);
  useEffect(() => ref.current?.focus(), []);
  useEffect(() => {
    const a = state.answer;
    if (a && a.n > seen.current) {
      seen.current = a.n;
      answered(store, a);
    }
  }, [state.answer]);
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.shiftKey || e.altKey || e.ctrlKey || e.metaKey) return;
    if (e.key === "Enter" && !composing(e)) {
      e.preventDefault();
      state.act?.({ Do: "add_character", World: p.adding, Name: p.name ?? "" });
    } else if (e.key === "Escape") {
      e.preventDefault();
      store.setPicker({ adding: undefined, name: undefined, error: undefined });
    }
  };
  const onInput = (e: Event) => {
    const name = (e.currentTarget as HTMLInputElement).value;
    const check = (globalThis as any).kiln?.check;
    store.setPicker({ name, error: (check && check("name", name)) || undefined });
  };
  return (
    <div class="pick-row">
      <label for="kiln-name" class={roleClass("form.label")}>{str.pickerName()}: </label>
      <input id="kiln-name" ref={ref} value={p.name ?? ""} autocomplete="off" spellcheck={false} disabled={state.phase === "stopped"}
        aria-invalid={p.error ? "true" : undefined} aria-describedby="kiln-name-note" onKeyDown={onKeyDown} onInput={onInput} />
      <span class={roleClass("form.hint")}>{str.separator()}</span>
      <span id="kiln-name-note" aria-live="polite" class={roleClass(p.error ? "form.error" : "form.hint")}>{p.error ?? str.pickerAddCharHint()}</span>
    </div>
  );
}
