import { useEffect, useState } from "preact/hooks";
import type { State, Store } from "../store.ts";

// useStore is the store's state, drawing again whenever it changes.
export function useStore(store: Store): State {
  const [state, set] = useState(store.state);
  useEffect(() => store.subscribe(() => set(store.state)), [store]);
  return state;
}
