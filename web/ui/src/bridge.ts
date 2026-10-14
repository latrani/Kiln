// The golden page's stand-in for Go's wasm: Go runs natively in the
// test, across a WebSocket. Actions go out through the typing queue, as
// boot's do; batches come in to the store. Only the golden build has it
// (golden.tsx); the page boots the wasm (boot.ts).
import type { Event, Store } from "./store.ts";

// bridge runs the page over ws as boot runs it over the wasm: running
// once ws opens, each message a batch, and what was done before the
// first batch sent after it, in order (typing.ts). check is Go's field
// check, which the editor calls as you type and needs at once, so it
// can't go over ws (golden.tsx asks the test's server, synchronously).
// kilnPage.act is the tests' handle, as boot's.
export function bridge(store: Store, ws: WebSocket, check: (field: string, value: string) => string): void {
  (globalThis as any).kiln = { check };
  const typing = store.state.typing!;
  const act = (o: object) => typing.act(o);
  store.set({ act, saving: true });
  (globalThis as any).kilnPage = { act };
  let running = false;
  ws.onopen = () => store.set({ phase: "running", progress: undefined });
  ws.onmessage = (m) => {
    const batch: Event[] = JSON.parse(m.data);
    store.apply(batch);
    if (!running) {
      running = true;
      const s = store.state;
      typing.ready((o) => ws.send(JSON.stringify(o)), s.chars[s.active] ? s.active : "");
      store.set({}); // draw the typing where it went
    }
  };
}
