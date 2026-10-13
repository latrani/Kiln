// Telling Kiln you're here (app.Here, Scrolled, Focus), and the keys
// that switch characters.

// throttle is fn run at most once every ms: a call runs it unless it ran
// less than ms ago, when the call is dropped.
export function throttle(fn: () => void, ms: number, now: () => number = Date.now): () => void {
  let last = -Infinity;
  return () => {
    const t = now();
    if (t - last < ms) return;
    last = t;
    fn();
  };
}

interface Keyish {
  key: string;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
  metaKey: boolean;
}

// switchAction is the action a key asks for if it's one of the switching
// keys: Ctrl+↑/↓ steps through the sidebar's worlds and characters,
// Alt+↓/↑ goes to the next or previous character with unread lines (Tab
// in the TUI; on the web Tab moves focus). Kiln's core decides where.
export function switchAction(e: Keyish): object | undefined {
  if (e.shiftKey || e.metaKey || e.ctrlKey === e.altKey) return undefined;
  const dir = e.key === "ArrowDown" ? 1 : e.key === "ArrowUp" ? -1 : 0;
  if (!dir) return undefined;
  return e.ctrlKey ? { Do: "step", N: dir } : { Do: "next_unread", N: dir };
}

// watch tells Kiln what the page sees of you, and the pane's size: a key
// or click is you here (at most once a second); the window gaining or
// losing focus is focus; the switching keys switch; and the scrollback's
// size in characters goes to the MUCK, settled for 200ms first.
export function watch(act: (o: object) => void, pane: HTMLElement, size: (el: HTMLElement) => { cols: number; rows: number }): void {
  const here = throttle(() => act({ Do: "here" }), 1000);
  addEventListener("keydown", (e) => {
    here();
    const a = switchAction(e);
    if (a) {
      e.preventDefault();
      act(a);
    }
  });
  addEventListener("pointerdown", here);
  act({ Do: "focus", In: document.hasFocus() }); // a browser can say now; a terminal waits for a change
  addEventListener("focus", () => act({ Do: "focus", In: true }));
  addEventListener("blur", () => act({ Do: "focus", In: false }));
  let last = "", timer: ReturnType<typeof setTimeout> | undefined;
  const report = () => {
    const { cols, rows } = size(pane);
    const now = `${cols}x${rows}`;
    if (cols > 0 && rows > 0 && now !== last) {
      last = now;
      act({ Do: "resize", Cols: cols, Rows: rows });
    }
  };
  report();
  new ResizeObserver(() => {
    clearTimeout(timer);
    timer = setTimeout(report, 200);
  }).observe(pane);
}
