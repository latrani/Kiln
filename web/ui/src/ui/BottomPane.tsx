import type { State } from "../store.ts";
import { roleClass } from "../theme.ts";
import { Input } from "./Input.tsx";

// BottomPane is under the scrollback: the input, or the password form;
// later the picker's filter, a form, or log mode's hints (steps 4–5). A
// world's overview has none, as in the TUI: the overview has the pane.
export function BottomPane({ state }: { state: State }) {
  const overview = state.active.startsWith("=");
  return <div class={`bottom ${roleClass("input")}`}>{!overview && state.phase !== "locked" && <Input state={state} />}</div>;
}
