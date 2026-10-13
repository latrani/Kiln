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
