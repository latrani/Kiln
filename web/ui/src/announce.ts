import type { State } from "./store.ts";
import * as str from "./str.gen.ts";

// announcements is what to read aloud going from prev to next: each line
// that wants you (Attention) newly come to a character that isn't
// showing. Only lines appended in the same numbering count: lines loaded
// again, made again, or older history aren't news, and a character that
// closed says nothing.
export function announcements(prev: State, next: State): string[] {
  const out: string[] = [];
  for (const [key, ls] of Object.entries(next.lines)) {
    const c = next.chars[key];
    const was = prev.lines[key];
    if (!c || key === next.active || !was || was.epoch !== ls.epoch || ls === was) continue;
    const last = was.lines.at(-1)?.Seq ?? -Infinity;
    for (const l of ls.lines) {
      if (l.Seq > last && l.Attention) out.push(str.webAnnounce(c.Name, l.Plain));
    }
  }
  return out;
}
