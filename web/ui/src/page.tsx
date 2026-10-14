// The page around Kiln, whatever runs Go: the app drawn from the store,
// what Go asks of the page (effect), and the page's own keys and
// watches. main.tsx boots the wasm under it; golden.tsx, for web/smoke's
// screenshots, a WebSocket to Go in the test (bridge.ts).
import { render } from "preact";
import { download, pickFile, toBase64 } from "./boot.ts";
import { paneSize } from "./layout.ts";
import { toggleLog } from "./log.ts";
import { editorKey, logToggleKey, pickerKey, splitKey } from "./picker.ts";
import { watch } from "./presence.ts";
import type { Event, Store } from "./store.ts";
import * as str from "./str.gen.ts";
import { App } from "./ui/App.tsx";

// effect does what Go asks of the page: a file to download (an export,
// a backup), for /restore a backup to pick, for /open the picker, for
// /edit an editor, for /log log mode, and Copy to the clipboard, saying
// so when the browser refuses (spec).
export const effect = (store: Store) => (e: Event): void => {
  if (e.effect === "SaveFile") {
    download(String(e.Name).split("/").pop()!, Uint8Array.from(atob(e.Data ?? ""), (c) => c.charCodeAt(0)));
  } else if (e.effect === "Do" && e.Cmd === "/open") {
    store.openPicker();
  } else if (e.effect === "Do" && e.Cmd === "/edit" && e.Key) {
    // As the TUI's editCommand: the active character's editor, or with
    // "world" its world's, closing with the picker.
    const [world, char] = splitKey(String(e.Key));
    store.openEditor(e.Args?.[0] === "world" ? { kind: "world", world } : { kind: "char", world, char }, true);
  } else if (e.effect === "Do" && e.Cmd === "/restore") {
    pickFile(".zip,application/zip", (globalThis as any).kiln.maxBackup, (bytes) => store.state.act?.({ Do: "restore", Data: toBase64(bytes) }));
  } else if (e.effect === "Do" && e.Cmd === "/log") {
    toggleLog(store);
  } else if (e.effect === "Copy") {
    // The browser may refuse (no clipboard here, or no user gesture by
    // the time Go answers): say so, in place of Go's "copied".
    const refused = () => store.state.act?.({ Do: "say", Text: str.webCopyRefused(), Err: true });
    const clip = navigator.clipboard;
    if (!clip) refused();
    else {
      clip.writeText(String(e.Text ?? "")).catch((err) => {
        console.error("copy", err);
        refused();
      });
    }
  }
};

// mount draws the app from store, and adds the page's keys and watches.
export function mount(store: Store): void {
  render(<App store={store} />, document.getElementById("kiln")!);
  // Ctrl+L for log mode, and the picker's and editor's keys from anywhere
  // (picker.ts): Ctrl+T and Alt+T, Ctrl+O, before the browser or macOS's
  // "open line" takes it, and Enter on an overview.
  addEventListener("keydown", (e) => {
    if (logToggleKey(e)) {
      e.preventDefault();
      toggleLog(store);
      return;
    }
    const t = e.target as Element | null;
    const ed = editorKey({ key: e.key, code: e.code, ctrlKey: e.ctrlKey, altKey: e.altKey, metaKey: e.metaKey, shiftKey: e.shiftKey, onBody: !t || t === document.body }, store.state);
    if (ed) {
      e.preventDefault();
      if (ed === "hide") store.hideEditor();
      else if (ed !== "eat") store.openEditor(ed.open, ed.closeAll);
      return;
    }
    const does = pickerKey({ key: e.key, ctrlKey: e.ctrlKey, altKey: e.altKey, metaKey: e.metaKey, shiftKey: e.shiftKey, onBody: !t || t === document.body }, store.state);
    if (!does) return;
    e.preventDefault();
    if (does === "close") store.closePicker();
    else if (does === "open") store.openPicker();
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
}
