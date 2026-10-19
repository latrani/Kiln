// The browser as Kiln's host: Go's wasm (web/cmd/kiln-web) answers the
// checks, files download and upload as the browser does them, and
// notifications are the browser's (notify.ts).
import type { Host } from "./host.ts";
import { show } from "./notify.ts";

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

// webHost is the browser's host. The wasm's kiln is looked up on each
// call: it's there only once the wasm has loaded, and until then there's
// nothing to check against.
export function webHost(): Host {
  const kiln = () => (globalThis as any).kiln;
  return {
    check: (field, value) => kiln()?.check?.(field, value) ?? "",
    over: (text, maxLineBytes, newlineMode) => kiln()?.over?.(text, maxLineBytes, newlineMode) ?? [],
    saveFile: download,
    notify: (store, e) => show(store, e),
    restore: (done) => pickFile(".zip,application/zip", kiln().maxBackup, done),
    storageFailed: () => kiln()?.storageFailed?.(),
  };
}
