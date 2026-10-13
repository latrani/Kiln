import type { State } from "../store.ts";
import { roleClass } from "../theme.ts";

// TopBar is the row above the scrollback: where you are, and the Log
// button (step 3).
export function TopBar(_: { state: State }) {
  return <header class={`top ${roleClass("status")}`} />;
}
