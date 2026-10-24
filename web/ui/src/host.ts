// What the page needs from whatever runs Go: the wasm in a browser
// (webhost.ts), the golden test's server (golden.tsx), the desktop app.
// The page asks its store's host, never the host's globals.
import type { Permission } from "./notify.ts";
import type { Event, Store } from "./store.ts";

// Ranges are [start, end) in the browser's string offsets (UTF-16).
export type Ranges = [number, number][];

// Host is what the page asks of Go's host. An optional member is a
// capability: the page does what it's for only if the host has it.
export interface Host {
  // check is what Go says of a settings field as it's typed, "" when
  // it's fine (app.CheckField). It may answer late: callers drop an
  // answer for a value that has moved on.
  check(field: string, value: string): Promise<string>;
  // over is where text runs past the line limit (webapi.OverRanges). It
  // may answer late: callers keep only the latest answer.
  over(text: string, maxLineBytes: number, newlineMode: string): Promise<Ranges>;
  // saveFile saves an export or a backup, named name.
  saveFile(name: string, bytes: Uint8Array): void;
  // notify shows a Notify effect, best effort.
  notify(store: Store, e: Event): void;
  // permission is whether notifications can show: "default" if they
  // haven't been asked for, "unsupported" if the host can't show any.
  permission(): Promise<Permission>;
  // askPermission asks for them, in answer to a click, and gives the
  // answer; a host that pushes subscribes to pushes once it's granted.
  askPermission(): Promise<Permission>;
  // whyNoNotify is what the status line says when notifications can't
  // show for p, "" when they can.
  whyNoNotify(p: Permission): string;
  // restore asks for a backup to restore and gives done its bytes.
  restore?(done: (bytes: Uint8Array) => void): void;
  // reset wipes what the host saved and starts over (/reset). Without
  // it /reset does nothing.
  reset?(): Promise<void>;
  // useHere takes Kiln over from another tab. The splash offers it only
  // with this.
  useHere?(): Promise<void>;
  // worlds says the host can add worlds and change a world's host,
  // port, TLS and cert trust (the desktop app dials them itself). The
  // page offers + World and edits them only with this; on the web
  // they're the relay's.
  worlds?: true;
  // asksWhere says saveFile asks where to save, offering the name, as
  // the desktop app's Save dialog does: an export then goes straight to
  // it, with no name asked for on the log bar first.
  asksWhere?: true;
  // storageFailed tells Go the page has stopped saving.
  storageFailed?(): void;
}

// toBase64 is bytes as base64, a chunk at a time: one call with every
// byte as an argument would overflow the stack for a large file.
export function toBase64(bytes: Uint8Array): string {
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}

// noHost is a host with nothing behind it: no checks, no ranges, nowhere
// to save or show anything, and no capabilities.
export const noHost: Host = {
  check: async () => "",
  over: async () => [],
  saveFile: () => {},
  notify: () => {},
  permission: async () => "unsupported",
  askPermission: async () => "unsupported",
  whyNoNotify: () => "",
};
