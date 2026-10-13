// The page's entry: the store, the app drawn from it, and Kiln booting.
import "./kiln.css";
import { render } from "preact";
import { boot, download, pickFile, toBase64 } from "./boot.ts";
import { paneSize } from "./layout.ts";
import { pickerKey } from "./picker.ts";
import { watch } from "./presence.ts";
import { Store } from "./store.ts";
import type { Event } from "./store.ts";
import { App } from "./ui/App.tsx";

// effect does what Go asks of the page: a file to download (an export,
// a backup), for /restore a backup to pick, and for /open the picker.
// /log and /edit come with log mode and the editor (steps 4b and 5).
function effect(e: Event): void {
  if (e.effect === "SaveFile") {
    download(String(e.Name).split("/").pop()!, Uint8Array.from(atob(e.Data ?? ""), (c) => c.charCodeAt(0)));
  } else if (e.effect === "Do" && e.Cmd === "/open") {
    store.openPicker();
  } else if (e.effect === "Do" && e.Cmd === "/restore") {
    pickFile(".zip,application/zip", (globalThis as any).kiln.maxBackup, (bytes) => store.state.act?.({ Do: "restore", Data: toBase64(bytes) }));
  }
}

const store = new Store(effect);
render(<App store={store} />, document.getElementById("kiln")!);
// The picker's keys from anywhere (picker.ts): Ctrl+O, before the
// browser or macOS's "open line" takes it, and Enter on an overview.
addEventListener("keydown", (e) => {
  const t = e.target as Element | null;
  const does = pickerKey({ key: e.key, ctrlKey: e.ctrlKey, altKey: e.altKey, metaKey: e.metaKey, shiftKey: e.shiftKey, onBody: !t || t === document.body }, store.state.active);
  if (!does || store.state.phase !== "running") return;
  e.preventDefault();
  if (does === "toggle" && store.state.picker) store.closePicker();
  else store.openPicker();
}, true);
// Once Kiln runs, tell it what the page sees of you and the pane's size.
const stop = store.subscribe(() => {
  const { phase, act } = store.state;
  if (phase !== "running" || !act) return;
  stop();
  // Measured inside the scroller: its text box, less any scrollbar.
  const main = document.querySelector("main")!;
  watch(act, main, () => paneSize(main.querySelector<HTMLElement>(".scroll") ?? main));
});
boot(store).catch((e) => {
  console.error(e);
  store.set({ phase: "stopped" });
});
