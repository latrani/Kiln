// The browser as Kiln's host: Go's wasm (web/cmd/kiln-web) answers the
// checks, files download and upload as the browser does them, and
// notifications are the browser's (notify.ts).
import type { Host } from "./host.ts";
import { permission, request, show } from "./notify.ts";
import * as str from "./str.gen.ts";

// pickFile asks for a file (a backup to restore) and gives its bytes to
// done, no more than max and one past it, so Go can say a file is too
// big without the page reading all of one; nothing if none is chosen.
export function pickFile(accept: string, max: number, done: (bytes: Uint8Array) => void): void {
  const f = Object.assign(document.createElement("input"), { type: "file", accept, hidden: true });
  document.body.append(f);
  f.addEventListener("cancel", () => f.remove());
  f.addEventListener("change", async () => {
    const file = f.files?.[0];
    f.remove();
    if (file) done(new Uint8Array(await file.slice(0, max + 1).arrayBuffer()));
  });
  f.click();
}

// download saves bytes as a file named name.
export function download(name: string, bytes: Uint8Array): void {
  const url = URL.createObjectURL(new Blob([bytes as BlobPart]));
  const a = Object.assign(document.createElement("a"), { href: url, download: name });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// WebHost is the browser's host, which gets the capabilities that need
// what boot.ts makes (the resetter, the tab lock) once they exist, so
// this file never imports boot.ts.
export interface WebHost extends Host {
  provide(parts: Pick<Host, "reset" | "useHere"> & { subscribe?: () => void }): void;
}

// webHost is the browser's host. The wasm's kiln is looked up on each
// call: it's there only once the wasm has loaded, and until then there's
// nothing to check against.
export function webHost(): WebHost {
  const kiln = () => (globalThis as any).kiln;
  let subscribe: (() => void) | undefined; // to the relay's pushes (push.ts), once boot.ts has it
  const host: WebHost = {
    check: async (field, value) => kiln()?.check?.(field, value) ?? "",
    over: async (text, maxLineBytes, newlineMode) => kiln()?.over?.(text, maxLineBytes, newlineMode) ?? [],
    saveFile: download,
    notify: (store, e) => show(store, e),
    permission: async () => permission(),
    askPermission: async () => {
      const p = await request();
      if (p === "granted") subscribe?.();
      return p;
    },
    whyNoNotify: (p) => (p === "denied" ? str.webNotifyDenied() : p === "unsupported" ? str.webNotifyUnsupported() : ""),
    restore: (done) => pickFile(".zip,application/zip", kiln().maxBackup, done),
    storageFailed: () => kiln()?.storageFailed?.(),
    provide: ({ subscribe: sub, ...parts }) => {
      if (sub) subscribe = sub;
      Object.assign(host, parts);
    },
  };
  return host;
}
