import { useEffect, useLayoutEffect, useRef } from "preact/hooks";
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

// useKeys keeps the keys with a picker form's field, as the TUI's picker
// always has them: a key landing on no control (after a click on the
// scrollback or a world's name) goes back to the field, the form's own
// keys handled as if pressed there.
function useKeys(ref: { current: HTMLInputElement | null }, onKeyDown: (e: KeyboardEvent) => void): void {
  const handle = useRef(onKeyDown);
  handle.current = onKeyDown;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as Element | null;
      if (!ref.current || (t && t !== document.body && t.closest("a, button, input, textarea, select, summary, [contenteditable]"))) return;
      ref.current.focus();
      handle.current(e);
    };
    addEventListener("keydown", onKey, true);
    return () => removeEventListener("keydown", onKey, true);
  }, []);
}

// PickerForm is the picker's part of the bottom pane, a TUI one-field
// form: the filter, or while adding a character the keys for its name,
// which is typed in the sidebar (NameField). The filter is a combobox
// for the picker's list in the
// sidebar (↑/↓ and PgUp/PgDn move the highlight, Enter chooses, Esc
// closes). It takes focus when it opens; closing gives the input its
// place back, and the input takes focus as it comes.
export function PickerForm({ state, store }: { state: State; store: Store }) {
  if (state.picker!.adding !== undefined) {
    return (
      <div class="pick-row">
        <span id="kiln-name-hint" class={roleClass("form.hint")}>{str.pickerAddCharHint()}</span>
      </div>
    );
  }
  return <FilterForm state={state} store={store} />;
}

function FilterForm({ state, store }: { state: State; store: Store }) {
  const p = state.picker!;
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => ref.current?.focus(), []);
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.defaultPrevented) return;
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
  useKeys(ref, onKeyDown);
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

// NameField asks for a new character's name, as the TUI's: in the
// sidebar, in place of its world's "+ Character" and in its colors, why
// a name won't do under it in picker.error. Enter adds it to the world
// and connects it; Esc goes back to the filter. As you type, Go's check
// says what won't do; Enter still tries (Go's save has the last word,
// and says why in the same place).
export function NameField({ state, store }: { state: State; store: Store }) {
  const p = state.picker!;
  const ref = useRef<HTMLInputElement>(null);
  const seen = useRef(state.answer?.n ?? 0);
  // Focus as it mounts, in the tap's own turn: iOS opens the keyboard
  // for a focus only while it's handling a tap, and an effect comes
  // later.
  useLayoutEffect(() => ref.current?.focus(), []);
  useEffect(() => {
    const a = state.answer;
    if (a && a.n > seen.current) {
      seen.current = a.n;
      answered(store, a);
    }
  }, [state.answer]);
  // A problem comes into view with the name, as the TUI's.
  useLayoutEffect(() => {
    if (p.error) ref.current?.closest("li")?.scrollIntoView?.({ block: "nearest" });
  }, [p.error]);
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.defaultPrevented || e.shiftKey || e.altKey || e.ctrlKey || e.metaKey) return;
    if (e.key === "Enter" && !composing(e)) {
      e.preventDefault();
      state.act?.({ Do: "add_character", World: p.adding, Name: p.name ?? "" });
    } else if (e.key === "Escape") {
      e.preventDefault();
      store.setPicker({ adding: undefined, name: undefined, error: undefined });
    }
  };
  useKeys(ref, onKeyDown);
  const onInput = (e: Event) => {
    const name = (e.currentTarget as HTMLInputElement).value;
    const check = (globalThis as any).kiln?.check;
    store.setPicker({ name, error: (check && check("name", name)) || undefined });
  };
  return (
    <>
      <span class={"name-field " + roleClass("picker.add")}>
        <input id="kiln-name" ref={ref} value={p.name ?? ""} aria-label={str.pickerName()} placeholder={str.pickerName()} autocomplete="off" spellcheck={false}
          disabled={state.phase === "stopped"} aria-invalid={p.error ? "true" : undefined} aria-describedby="kiln-name-note kiln-name-hint"
          onKeyDown={onKeyDown} onInput={onInput} />
      </span>
      <span id="kiln-name-note" aria-live="polite" class={"name-note " + roleClass("picker.error")}>{p.error ?? ""}</span>
    </>
  );
}
