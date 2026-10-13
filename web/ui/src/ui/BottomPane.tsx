import type { State } from "../store.ts";
import { roleClass } from "../theme.ts";
import { Input } from "./Input.tsx";
import { PasswordForm } from "./PasswordForm.tsx";

// BottomPane is under the scrollback: the input, or the password form;
// later the picker's filter, a form, or log mode's hints (steps 4–5). A
// world's overview has none, as in the TUI: the overview has the pane.
export function BottomPane({ state }: { state: State }) {
  const overview = state.active.startsWith("=");
  const password = state.chars[state.active] && state.inputs[state.active]?.NeedPW;
  return (
    <div class={`bottom ${roleClass("input")}`}>
      {!overview && state.phase !== "locked" && (password ? <PasswordForm state={state} /> : <Input state={state} />)}
    </div>
  );
}
