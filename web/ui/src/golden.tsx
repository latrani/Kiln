// The golden page (web/smoke's screenshot goldens): the page, with Go
// across a WebSocket (bridge.ts) in place of the wasm. Not shipped:
// only build.mjs's golden build makes it. The handlers are set in the
// same tick as the socket is made, so no batch can come before them.
import "./kiln.css";
import { bridge } from "./bridge.ts";
import { effect, mount } from "./page.tsx";
import { Store } from "./store.ts";

// The caret blinks, so screenshots hide it.
document.head.append(Object.assign(document.createElement("style"), { textContent: "* { caret-color: transparent !important; }" }));
const store: Store = new Store((e) => effect(store)(e));
mount(store);
bridge(store, new WebSocket(new URLSearchParams(location.search).get("bridge")!));
