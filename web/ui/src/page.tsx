// The page around Kiln, whatever runs Go: the app drawn from the store,
// what Go asks of the page (effect), and the page's own keys and
// watches. main.tsx boots the wasm under it; golden.tsx, for web/smoke's
// screenshots, a WebSocket to Go in the test (bridge.ts).
import { render } from "preact";
import { toBase64 } from "./host.ts";
import { paneSize } from "./layout.ts";
import { toggleLog } from "./log.ts";
import { choose, editorKey, firstMatch, logToggleKey, pickerKey, pickerRows, splitKey } from "./picker.ts";
import { watch } from "./presence.ts";
import { sidebarKey, swipeDir, swiped, toggleSidebar } from "./sidebar.ts";
import { logShows } from "./store.ts";
import type { Event, Store } from "./store.ts";
import * as str from "./str.gen.ts";
import { App } from "./ui/App.tsx";
import { watchViewport } from "./viewport.ts";

// effect does what Go asks of the page: a file to download (an export,
// a backup), for /restore a backup to pick, for /open the picker, for
// /edit an editor, for /log log mode, for /reset a wipe, Copy to the clipboard (saying so
// when the browser refuses), and Notify as a desktop notification, best
// effort (spec).
export const effect = (store: Store) => (e: Event): void => {
  if (e.effect === "SaveFile") {
    store.host.saveFile(String(e.Name).split("/").pop()!, Uint8Array.from(atob(e.Data ?? ""), (c) => c.charCodeAt(0)));
  } else if (e.effect === "Do" && e.Cmd === "/open") {
    // With letters, as the TUI's jump: their first match, chosen; none,
    // the picker stays up, filtered.
    const f = (e.Args ?? []).join("");
    store.openPicker(f);
    const p = store.state.picker;
    const sel = f && p ? firstMatch(pickerRows(store.state.worlds, store.state.chars, f, store.host.worlds === true)) : undefined;
    if (sel) choose(store, sel);
  } else if (e.effect === "Do" && e.Cmd === "/edit" && e.Key) {
    // As the TUI's editCommand: the active character's editor, or with
    // "world" its world's, closing with the picker.
    const [world, char] = splitKey(String(e.Key));
    store.openEditor(e.Args?.[0] === "world" ? { kind: "world", world } : { kind: "char", world, char }, true);
  } else if (e.effect === "Do" && e.Cmd === "/restore") {
    store.host.restore?.((bytes) => store.state.act?.({ Do: "restore", Data: toBase64(bytes) }));
  } else if (e.effect === "Do" && e.Cmd === "/reset") {
    // Go asked first (Enter again). A browser that won't clear what it
    // saved has still stopped saving: show that, as when saving fails,
    // and say to reload (after, so it's what the status line shows).
    store.host.reset?.().catch((err) => {
      console.error("reset", err);
      store.set({ saving: false });
      store.host.storageFailed?.();
      store.state.act?.({ Do: "say", Text: str.webResetFailed(), Err: true });
    });
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
  } else if (e.effect === "Notify") {
    store.host.notify(store, e);
  }
};

// mount draws the app from store, and adds the page's keys and watches.
export function mount(store: Store): void {
  render(<App store={store} />, document.getElementById("kiln")!);
  watchViewport(globalThis.visualViewport, document.documentElement);
  // Added here, before any component's effects, so it runs first: the
  // input's own listener (Input.tsx) leaves alone what it took.
  // Ctrl+L for log mode, and the picker's and editor's keys from anywhere
  // (picker.ts): Ctrl+T and Alt+T, Ctrl+O, before the browser or macOS's
  // "open line" takes it, and Enter on an overview.
  addEventListener("keydown", (e) => {
    if (logToggleKey(e)) {
      e.preventDefault();
      toggleLog(store);
      return;
    }
    if (sidebarKey(e)) { // Ctrl+B, as the TUI's
      e.preventDefault();
      toggleSidebar(store);
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
  // A flick opens or closes the narrow sidebar (sidebar.ts's swiped): one
  // finger, not starting in a field, where a drag moves the caret or
  // selects.
  let from: { x: number; y: number; at: number } | undefined;
  addEventListener("touchstart", (e) => {
    const t = e.touches;
    const field = (e.target as Element | null)?.closest?.("input, textarea");
    from = t.length === 1 && !field ? { x: t[0].clientX, y: t[0].clientY, at: e.timeStamp } : undefined;
  }, { passive: true });
  addEventListener("touchcancel", () => { from = undefined; }, { passive: true });
  addEventListener("touchend", (e) => {
    const t = e.changedTouches[0];
    const dir = from && t ? swipeDir(from, { x: t.clientX, y: t.clientY }, e.timeStamp - from.at) : undefined;
    from = undefined;
    if (dir) swiped(store, dir);
  }, { passive: true });
  // Once Kiln runs, tell it what the page sees of you and the pane's size.
  const stop = store.subscribe(() => {
    const { phase, act } = store.state;
    if (phase !== "running" || !act) return;
    stop();
    // Measured inside the scroller: its text box, less any scrollbar.
    const main = document.querySelector("main")!;
    watch(act, main, () => paneSize(main.querySelector<HTMLElement>(".scroll") ?? main), () => logShows(store.state, store.state.active));
  });
}
