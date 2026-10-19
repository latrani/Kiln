// The golden page (web/smoke's screenshot goldens): the page, with Go
// across a WebSocket (bridge.ts) in place of the wasm. Not shipped:
// only build.mjs's golden build makes it. The handlers are set in the
// same tick as the socket is made, so no batch can come before them.
import "./kiln.css";
import { bridge } from "./bridge.ts";
import { effect, mount } from "./page.tsx";
import { noHost } from "./host.ts";
import type { Host } from "./host.ts";
import { Store } from "./store.ts";

// The caret blinks, so screenshots hide it.
document.head.append(Object.assign(document.createElement("style"), { textContent: "* { caret-color: transparent !important; }" }));
const ws = new URL(new URLSearchParams(location.search).get("bridge")!);
// What the wasm's kiln answers at once, from the test's server:
// synchronous, as the wasm's is (a synchronous request is fine in a
// test page).
const ask = (path: string, q: Record<string, string>): string => {
  const u = new URL(path, ws);
  u.protocol = ws.protocol === "wss:" ? "https:" : "http:";
  u.search = new URLSearchParams(q).toString();
  const x = new XMLHttpRequest();
  x.open("GET", u, false);
  x.send();
  return x.responseText;
};
// The host: Go's checks from the test's server; nothing to save or show.
const host: Host = {
  ...noHost,
  check: async (field, value) => ask("check", { field, value }),
  over: async (text, max, mode) => JSON.parse(ask("over", { text, max: String(max), mode })),
};
const store: Store = new Store((e) => effect(store)(e), host);
mount(store);
bridge(store, new WebSocket(ws));
