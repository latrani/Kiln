import { useEffect, useRef, useState } from "preact/hooks";
import { deleteAction, errorOf, firstError, formError, saveAction, unsaved } from "../editor.ts";
import type { Editor, Field } from "../editor.ts";
import { composing } from "../input.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// EditorForm is an editor over the picker, as the TUI's: its title (and
// whether it's unsaved), its fields with labels lined up, More settings
// (a world's extras), Save, Delete, and the keys. Each TUI field is a
// real control (spec, Controls). Enter in a field saves (spec: a
// browser's forms); Tab moves; Esc cancels. Errors sit by their field,
// and after a refusal focus goes to the first. Focus is drawn in
// form.focus, as the TUI draws its focused button.
export function EditorForm({ state, store }: { state: State; store: Store }) {
  const ed = state.picker!.edit!;
  const root = useRef<HTMLFormElement>(null);
  const [focus, setFocus] = useState("");
  useEffect(() => root.current?.querySelector<HTMLElement>("input, button")?.focus(), []);
  // After a refusal, focus goes to the first field with an error; one
  // already focused is focused anew, so a screen reader reads it and its
  // error again rather than nothing.
  useEffect(() => {
    const k = firstError(ed);
    const el = ed.refused && k ? document.getElementById("ed-" + k) : null;
    if (el === document.activeElement) el?.blur();
    el?.focus();
  }, [ed.refused]);
  const off = state.phase === "stopped";
  const save = () => {
    if (!ed.pending) store.sendEditor(saveAction(ed));
  };
  const del = deleteAction(ed);
  const what = ed.target.kind === "char" ? `${ed.target.world}/${ed.target.char}` : ed.target.kind === "world" ? ed.target.world : "";
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.defaultPrevented) return;
    const t = e.target as HTMLElement;
    if (e.key === "Escape") {
      e.preventDefault();
      store.closeEditor();
    } else if (e.key === "Enter" && (t.tagName === "INPUT" || t === document.body) && !composing(e) && !e.shiftKey && !e.altKey && !e.ctrlKey && !e.metaKey) {
      e.preventDefault();
      save();
    }
  };
  useBodyKeys(root, onKeyDown);
  const main = ed.fields.filter((f) => !f.extra);
  const extra = ed.fields.filter((f) => f.extra);
  const top = formError(ed);
  // The keys, or Delete's question; one live region, so the question is
  // read out when it comes.
  const hint = (
    <>
      <span class={roleClass("form.hint")}>{" " + str.separator()}</span>
      <span aria-live="polite" class={roleClass(ed.armed ? "form.error" : "form.hint")}>{ed.armed ? str.webDeleteAgain(what) : str.webFormHint(str.editorHint())}</span>
    </>
  );
  return (
    <form ref={root} class="editor" aria-labelledby="ed-title" onKeyDown={onKeyDown}
      onSubmit={(e) => { e.preventDefault(); save(); }}
      onFocusIn={(e) => setFocus((e.target as HTMLElement).dataset.f ?? "")} onFocusOut={() => setFocus("")}>
      <div id="ed-title" class={roleClass("form.title")}>{ed.title}{unsaved(ed) ? str.separator() + str.editorUnsaved() : ""}</div>
      {top && <div role="alert" class={roleClass("form.error")}>{top}</div>}
      <div class="fields">{main.map((f) => <Row key={f.key} f={f} ed={ed} store={store} focus={focus} off={off} />)}</div>
      {extra.length > 0 && (
        <details open={ed.more} onToggle={(e) => store.setEditor({ more: (e.currentTarget as HTMLDetailsElement).open })}>
          {/* It folds as the filter panel's tags do: Enter and Space
              toggle it, ←/→ shut and open it, and of the mouse's clicks
              only one on its glyph does (a key's click has no detail). */}
          <summary data-f="more" class={focus === "more" ? roleClass("form.focus") : undefined}
            onClick={(e) => { if (e.detail && !(e.target as Element).closest(".dglyph")) e.preventDefault(); }}
            onKeyDown={(e) => {
              if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
              e.preventDefault();
              store.setEditor({ more: e.key === "ArrowRight" });
            }}>
            <span aria-hidden="true" class="dglyph">{ed.more ? str.glyph.open : str.glyph.collapsed}</span>{" " + str.editorExtra()}
          </summary>
          <div class="fields">{extra.map((f) => <Row key={f.key} f={f} ed={ed} store={store} focus={focus} off={off} />)}</div>
        </details>
      )}
      {/* The buttons a row each, as the TUI's; the keys after the last. */}
      <div class="buttons">
        <span class="row">
          <button type="submit" data-f="save" class={roleClass(focus === "save" ? "form.focus" : "form.button")} disabled={off}>{str.editorSave()}</button>
          {!del && hint}
        </span>
        {del && (
          <span class="row">
            <button type="button" data-f="delete" class={roleClass(focus === "delete" ? "form.focus" : "form.button.secondary")} disabled={off}
              // Pressed once it asks; again, it deletes. A double-click's
              // second click is the same press, and moving off stops asking.
              onClick={(e) => {
                if (!ed.armed) store.setEditor({ armed: true });
                else if (e.detail < 2) store.sendEditor(del);
              }}
              onBlur={() => ed.armed && store.setEditor({ armed: false })}>
              {ed.target.kind === "char" ? str.editorDeleteCharacter() : str.editorDeleteWorld()}
            </button>
            {hint}
          </span>
        )}
      </div>
    </form>
  );
}

// useBodyKeys keeps the keys with the editor, as the TUI's editor always
// has them: a key landing on no control (after a click on the
// scrollback or the title) goes to the form's own keys (Enter saves, Esc
// cancels), and anything else to its first field.
function useBodyKeys(root: { current: HTMLFormElement | null }, onKeyDown: (e: KeyboardEvent) => void): void {
  const handle = useRef(onKeyDown);
  handle.current = onKeyDown;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as Element | null;
      if (!root.current || (t && t !== document.body && t.closest("a, button, input, textarea, select, summary, [contenteditable]"))) return;
      if (e.key === "Enter" || e.key === "Escape") handle.current(e);
      else root.current.querySelector<HTMLElement>("input:not([disabled])")?.focus();
    };
    addEventListener("keydown", onKey, true);
    return () => removeEventListener("keydown", onKey, true);
  }, []);
}

// Row is one field: its label, its control, and what's wrong with it.
function Row({ f, ed, store, focus, off }: { f: Field; ed: Editor; store: Store; focus: string; off: boolean }) {
  const id = "ed-" + f.key;
  const err = errorOf(ed, f.key);
  const desc = err ? id + "-err" : undefined;
  const v = ed.values[f.key];
  const lit = focus === f.key ? roleClass("form.focus") : "";
  let control;
  if (f.kind === "text") {
    control = (
      <input id={id} data-f={f.key} type="text" value={String(v ?? "")} placeholder={f.hint} autocomplete="off" spellcheck={false} disabled={off}
        aria-invalid={err ? "true" : undefined} aria-describedby={desc}
        onInput={(e) => store.editField(f.key, (e.currentTarget as HTMLInputElement).value)} />
    );
  } else if (f.kind === "toggle") {
    // The TUI's [x]: the checkbox is read aloud, the box drawn; a click on
    // the box toggles it (it's in the checkbox's label).
    const box = v === true ? "[x]" : "[ ]"; // str:ok a drawn box, as form.go's
    control = (
      <label class="box">
        <input type="checkbox" id={id} data-f={f.key} class="sr-only" checked={v === true} disabled={off}
          aria-invalid={err ? "true" : undefined} aria-describedby={desc}
          onChange={(e) => store.editField(f.key, (e.currentTarget as HTMLInputElement).checked)} />
        <span aria-hidden="true" class={lit || undefined}>{box}</span>
      </label>
    );
  } else {
    // The TUI's ◄ v ►: a radio group showing only the chosen option (the
    // arrows choose, as a radio group's do), ◄ ► while focused.
    control = (
      <span role="radiogroup" class={lit ? "choice " + lit : "choice"} aria-labelledby={id + "-label"} aria-invalid={err ? "true" : undefined} aria-describedby={desc}>
        {f.options.map((o, i) => (
          <label key={o.value}>
            <input type="radio" id={v === o.value ? id : undefined} data-f={f.key} class="sr-only" name={id} value={o.value}
              aria-invalid={err ? "true" : undefined} aria-describedby={desc}
              checked={v === o.value} disabled={off} onChange={() => store.editField(f.key, o.value)} />
            <span onClick={(e) => {
              // Only the chosen option shows: a click on it takes the next,
              // as Space does in the TUI, and the group keeps the focus.
              e.preventDefault();
              const next = f.options[(i + 1) % f.options.length].value;
              store.editField(f.key, next);
              (e.currentTarget as HTMLElement).closest("[role=radiogroup]")?.querySelector<HTMLInputElement>(`input[value="${next}"]`)?.focus();
            }}>{o.shown}</span>
          </label>
        ))}
      </span>
    );
  }
  return (
    <>
      {f.kind === "choice"
        ? <span id={id + "-label"} class={roleClass("form.label")}>{f.label + ": "}</span>
        : <label for={id} class={roleClass("form.label")}>{f.label + ": "}</label>}
      <span class="control">
        {control}
        {err && <span id={id + "-err"} class={roleClass("form.error")}>{" " + err}</span>}
      </span>
    </>
  );
}
