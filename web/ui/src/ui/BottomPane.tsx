import type { State } from "../store.ts";
import { roleClass } from "../theme.ts";

// BottomPane is under the scrollback: the input, or the picker's filter,
// a form, the password form, or log mode's hints (steps 3–5).
export function BottomPane(_: { state: State }) {
  return <div class={`bottom ${roleClass("input")}`} />;
}
