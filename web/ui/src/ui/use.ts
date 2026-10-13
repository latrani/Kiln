import { useEffect, useState } from "preact/hooks";
import type { State, Store } from "../store.ts";

// useStore is the store's state, drawing again whenever it changes. The
// subscription starts after the first paint, so it catches up then:
// boot can change the state before that (another tab has Kiln).
export function useStore(store: Store): State {
  const [state, set] = useState(store.state);
  useEffect(() => {
    const done = store.subscribe(() => set(store.state));
    set(store.state);
    return done;
  }, [store]);
  return state;
}
