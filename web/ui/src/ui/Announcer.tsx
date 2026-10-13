import { useEffect, useState } from "preact/hooks";
import { announcements } from "../announce.ts";
import type { Store } from "../store.ts";

// Announcer reads aloud lines that want you from characters not showing
// (the showing one's are in the scrollback's log). Its region is always
// there, hidden from sight; each new message replaces its text node, so
// the same words come again are read again.
export function Announcer({ store }: { store: Store }) {
  const [said, setSaid] = useState({ text: "", n: 0 });
  useEffect(() => {
    let prev = store.state;
    return store.subscribe(() => {
      const next = store.state;
      const a = announcements(prev, next);
      prev = next;
      if (a.length) setSaid((s) => ({ text: a.join("\n"), n: s.n + 1 }));
    });
  }, [store]);
  return <div role="status" class="sr-only"><span key={said.n}>{said.text}</span></div>;
}
