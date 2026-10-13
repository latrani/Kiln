// The page's entry: the store, the app drawn from it, and Kiln booting.
import "./kiln.css";
import { render } from "preact";
import { boot, download } from "./boot.ts";
import { Store } from "./store.ts";
import type { Event } from "./store.ts";
import { App } from "./ui/App.tsx";

// effect does what Go asks of the page. More come in step 3.
function effect(e: Event): void {
  if (e.effect === "SaveFile") {
    download(String(e.Name).split("/").pop()!, Uint8Array.from(atob(e.Data ?? ""), (c) => c.charCodeAt(0)));
  }
}

const store = new Store(effect);
render(<App store={store} />, document.getElementById("kiln")!);
boot(store).catch((e) => {
  console.error(e);
  store.set({ phase: "stopped" });
});
