import { useEffect, useRef } from "preact/hooks";
import type { State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// PasswordForm is the TUI's password prompt as a real form, so password
// managers fill it: a hidden username, the character's name @ its
// world's id (so they tell worlds apart), and the password. Enter logs
// in; Esc skips. The password goes to Go once, as the submit, and is
// never held or drafted here.
export function PasswordForm({ state }: { state: State }) {
  const ref = useRef<HTMLInputElement>(null);
  const key = state.active;
  const c = state.chars[key];
  useEffect(() => ref.current?.focus(), [key]);
  const onSubmit = (e: Event) => {
    e.preventDefault();
    const f = ref.current!;
    state.typing?.password(key, f.value);
    f.value = "";
  };
  // Enter submits: with the hidden username beside it, the browser won't
  // by itself (no submit button, two fields).
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter" && !e.isComposing) {
      e.preventDefault();
      (e.currentTarget as HTMLInputElement).form?.requestSubmit();
    } else if (e.key === "Escape") {
      e.preventDefault();
      state.typing?.act({ Do: "cancel" });
    }
  };
  return (
    <form class="password" onSubmit={onSubmit}>
      <input type="text" name="username" autocomplete="username" value={`${c.Name}@${c.World}`} hidden readOnly />
      <label for="kiln-password" class={roleClass("input.hint")}>{str.viewPasswordLabel(c.Name)}</label>
      <input id="kiln-password" ref={ref} type="password" name="password" autocomplete="current-password" onKeyDown={onKeyDown} disabled={state.phase === "stopped"} />
      <span class={roleClass("input.hint")} aria-hidden="true">{str.viewPasswordKeys()}</span>
    </form>
  );
}
