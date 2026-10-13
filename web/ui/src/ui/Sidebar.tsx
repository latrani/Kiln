import type { State } from "../store.ts";
import * as str from "../str.gen.ts";
import { roleClass } from "../theme.ts";

// Sidebar is the worlds and characters (step 3); in log mode, the filter
// panel; with the picker open, the picker.
export function Sidebar(_: { state: State }) {
  return <nav class={`side ${roleClass("sidebar")}`} aria-label={str.webCharacters()} />;
}
