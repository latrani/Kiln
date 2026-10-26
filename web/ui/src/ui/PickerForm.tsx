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

// useKeys keeps the keys with a picker field while on, as the TUI's
// picker always has them: a key landing on no control (after a click on
// the scrollback or a world's name) goes back to the field, the field's
// own keys handled as if pressed there.
function useKeys(ref: { current: HTMLInputElement | null }, onKeyDown: (e: KeyboardEvent) => void, on = true): void {
  const handle = useRef(onKeyDown);
  handle.current = onKeyDown;
  const live = useRef(on);
  live.current = on;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as Element | null;
      if (!live.current || !ref.current || (t && t !== document.body && t.closest("a, button, input, textarea, select, summary, [contenteditable]"))) return;
      ref.current.focus();
      handle.current(e);
    };
    addEventListener("keydown", onKey, true);
    return () => removeEventListener("keydown", onKey, true);
  }, []);
}

// PickerForm is the picker's part of the bottom pane, as the TUI's: the
// keys, while the filter (FilterField) or a new character's name
// (NameField) is typed in the sidebar.
export function PickerForm({ state }: { state: State }) {
  const adding = state.picker!.adding !== undefined;
  return (
    <div class="pick-row">
      <span id={adding ? "kiln-name-hint" : "kiln-filter-hint"} class={roleClass("form.hint")}>{adding ? str.pickerAddCharHint() : str.pickerHint()}</span>
    </div>
  );
}

// FilterField is the picker's filter, under the sidebar's title, as the
// TUI's: a combobox for the picker's list (↑/↓ and PgUp/PgDn move the
// highlight, Enter chooses, Esc closes), saying what it is while empty.
// It has the keys, and focus, unless a character's name is being added
// or an editor is open over the picker; closing gives the input its
// place back, and the input takes focus as it comes.
export function FilterField({ state, store }: { state: State; store: Store }) {
  const p = state.picker!;
  const on = p.adding === undefined && !p.edit;
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => { if (on) ref.current?.focus(); }, [on]);
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.defaultPrevented || !on) return;
    const rows = pickerRows(state.worlds, state.chars, p.filter, store.host.worlds === true);
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
  useKeys(ref, onKeyDown, on);
  return (
    <div class={"name-field filter-field " + roleClass("picker.filter")}>
      <input id="kiln-filter" ref={ref} value={p.filter} role="combobox" aria-controls="kiln-picker" aria-expanded="true"
        aria-autocomplete="list" aria-activedescendant={on && p.sel ? "pick-" + p.sel : undefined} autocomplete="off" spellcheck={false}
        aria-label={str.pickerFilter()} placeholder={str.pickerFilter()} aria-describedby="kiln-filter-hint"
        readOnly={!on} tabIndex={on ? undefined : -1} disabled={state.phase === "stopped"} onKeyDown={onKeyDown}
        onInput={(e) => store.setPicker({ filter: (e.currentTarget as HTMLInputElement).value })} />
    </div>
  );
}

// NameField asks for a new character's name, as the TUI's: in the
// sidebar, in place of its world's "+ Character" and in its colors, why
// a name won't do under it in picker.error. Enter adds it to the world
// and connects it; Esc goes back to the filter. As you type, Go's check
// says what won't do; Enter still tries (Go's save has the last word,
// and says why in the same place).
// nameTyped takes name as typed in the add form, then asks Go's check
// of it; the answer shows if the form is still open on that name.
export function nameTyped(store: Store, name: string): void {
  store.setPicker({ name });
  store.host.check("name", name).then((msg) => {
    const p = store.state.picker;
    if (p?.adding && p.name === name) store.setPicker({ error: msg || undefined });
  }, (err) => console.error("check", err));
}

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
    nameTyped(store, name);
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
