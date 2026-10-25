import { useLayoutEffect, useRef } from "preact/hooks";
import { composing } from "../input.ts";
import type { State, Store } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// PasswordForm is the TUI's password prompt as a real form, so password
// managers fill it and offer to save it: a username out of sight (not
// hidden, which they skip), the character's name @ its world's id (so
// they tell worlds apart), the password, and a submit button out of
// sight, which Enter presses (some save only a login a button sent).
// Esc skips. The password goes to Go once, as the password action, and
// is never held or drafted here; it stays in its field until the form
// goes, so password managers see what was used. With the host's signIn
// the form then really submits, into a hidden frame (see Host.signIn).
export function PasswordForm({ state, store }: { state: State; store: Store }) {
  const ref = useRef<HTMLInputElement>(null);
  const key = state.active;
  const c = state.chars[key];
  // In the draw that shows it, as the input's (Input.tsx), so iOS keeps
  // the keyboard up from the field before.
  useLayoutEffect(() => ref.current?.focus(), [key]);
  const signIn = store.host.signIn;
  const onSubmit = (e: Event) => {
    if (!signIn) e.preventDefault();
    state.typing?.password(key, ref.current!.value);
  };
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter" && composing(e)) {
      e.preventDefault(); // the IME's Enter, not a login
    } else if (e.key === "Escape") {
      e.preventDefault();
      state.typing?.act({ Do: "cancel" });
    }
  };
  return (
    <form class="password" onSubmit={onSubmit} {...(signIn ? { action: signIn, method: "post", target: "kiln-signin" } : {})}>
      <input type="text" name="username" autocomplete="username" value={`${c.Name}@${c.World}`} readOnly class="sr-only" tabindex={-1} aria-hidden="true" />
      <label for="kiln-password" class={roleClass("input.hint")}>{str.viewPasswordLabel(c.Name)}</label>
      <input id="kiln-password" ref={ref} type="password" name="password" autocomplete="current-password" aria-describedby="kiln-password-keys" onKeyDown={onKeyDown} disabled={state.phase === "stopped"} />
      <span id="kiln-password-keys" class={roleClass("input.hint")}>{str.viewPasswordKeys()}</span>
      <button type="submit" class="sr-only" tabindex={-1}>{str.webLogIn()}</button>
      {signIn && <iframe name="kiln-signin" class="sr-only" tabindex={-1} aria-hidden="true" />}
    </form>
  );
}
