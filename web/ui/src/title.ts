import type { State } from "./store.ts";
import * as str from "./str.gen.ts";

// titleFor is the browser tab's title: Kiln, with how many lines have
// come that you haven't read, across every character.
export function titleFor(s: State): string {
  const n = Object.values(s.chars).reduce((sum, c) => sum + (c.Unread ?? 0), 0);
  return n > 0 ? str.webTitleUnread(n) : str.webTitle();
}
