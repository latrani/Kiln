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
  useEffect(() => {
    const k = firstError(ed);
    if (ed.refused && k) document.getElementById("ed-" + k)?.focus();
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
    } else if (e.key === "Enter" && t.tagName === "INPUT" && !composing(e) && !e.shiftKey && !e.altKey && !e.ctrlKey && !e.metaKey) {
      e.preventDefault();
      save();
    }
  };
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
          <summary data-f="more" class={focus === "more" ? roleClass("form.focus") : undefined}>
            {(ed.more ? str.glyph.open : str.glyph.collapsed) + " " + str.editorExtra()}
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
              onClick={() => (ed.armed ? store.sendEditor(del) : store.setEditor({ armed: true }))}>
              {ed.target.kind === "char" ? str.editorDeleteCharacter() : str.editorDeleteWorld()}
            </button>
            {hint}
          </span>
        )}
      </div>
    </form>
  );
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
    // The TUI's [x]: the checkbox is read aloud, the box drawn.
    const box = v === true ? "[x]" : "[ ]"; // str:ok a drawn box, as form.go's
    control = (
      <span class="box">
        <input type="checkbox" id={id} data-f={f.key} class="sr-only" checked={v === true} disabled={off}
          aria-invalid={err ? "true" : undefined} aria-describedby={desc}
          onChange={(e) => store.editField(f.key, (e.currentTarget as HTMLInputElement).checked)} />
        <span aria-hidden="true" class={lit || undefined}>{box}</span>
      </span>
    );
  } else {
    // The TUI's ◄ v ►: a radio group showing only the chosen option (the
    // arrows choose, as a radio group's do), ◄ ► while focused.
    control = (
      <span role="radiogroup" class={lit ? "choice " + lit : "choice"} aria-labelledby={id + "-label"} aria-invalid={err ? "true" : undefined} aria-describedby={desc}>
        {f.options.map((o, i) => (
          <label key={o.value}>
            <input id={i === 0 ? id : undefined} data-f={f.key} type="radio" class="sr-only" name={id} value={o.value}
              checked={v === o.value} disabled={off} onChange={() => store.editField(f.key, o.value)} />
            <span>{o.shown}</span>
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
