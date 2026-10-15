// The input's rules, apart from the DOM: when the page takes Go's text
// (part 2's Ack numbering), and what a key in the input does.

// Typed is the page's record of one input (a character's key, or "" for
// the idle input): the number of the last action it sent, and whether
// it's been typed in since.
export interface Typed {
  n: number;
  dirty: boolean;
}

// takes reports whether the page takes Go's text for an input: only when
// Go has taken the page's newest action and nothing's been typed since,
// so a reply to an older action never overwrites newer typing.
export function takes(t: Typed | undefined, ack: number): boolean {
  return !t || (!t.dirty && ack >= t.n);
}

export type KeyDo = "submit" | "older" | "cancel" | "page_up" | "page_down" | "history_up" | "history_down" | "history_close" | undefined;

interface Keyish {
  key: string;
  shiftKey: boolean;
  altKey: boolean;
  ctrlKey: boolean;
  metaKey: boolean;
  isComposing: boolean;
  keyCode: number;
}

// composing reports whether a key belongs to an input method composing
// text: Safari's Enter confirming a conversion comes after the
// composition ends, with only keyCode 229 to say so.
export function composing(e: Keyish): boolean {
  return e.isComposing || e.keyCode === 229;
}

// Caret is where the caret is: start..end in value, and when the page
// could measure it, whether it's on the first screen row.
export interface Caret {
  start: number;
  end: number;
  value: string;
  first?: boolean;
}

// keyAction is what a key in the input does; undefined leaves it to the
// browser. ↑ on the first row opens the history panel, as the TUI's (so
// ↑ still moves through a wrapped or multi-line draft; without a
// measure, rows are lines); with a modifier it's a switching key. With
// the panel up (panel), ↑ and ↓ move its selection wherever the caret
// is, PgUp and PgDn a page, and Esc closes it.
export function keyAction(e: Keyish, caret: Caret, panel = false): KeyDo {
  if (composing(e)) return undefined;
  const plain = !e.shiftKey && !e.altKey && !e.ctrlKey && !e.metaKey;
  switch (e.key) {
    case "Enter":
      return e.shiftKey || e.altKey || e.ctrlKey || e.metaKey ? undefined : "submit";
    case "Escape":
      return panel ? "history_close" : "cancel";
    case "PageUp":
      return plain ? "page_up" : undefined;
    case "PageDown":
      return plain ? "page_down" : undefined;
    case "ArrowUp":
      if (panel) return plain ? "history_up" : undefined;
      return plain && caret.start === caret.end && (caret.first ?? !caret.value.slice(0, caret.start).includes("\n")) ? "older" : undefined;
    case "ArrowDown":
      return panel && plain ? "history_down" : undefined;
  }
  return undefined;
}

// strayKey reports whether a key pressed outside any control belongs in
// the input: a printing key (with Option or AltGr too, which make
// letters), a dead key, or an IME starting. Not one the page already
// took (a shortcut: page.tsx's listener runs before the input's, so it
// has called preventDefault), nor Ctrl, Cmd or a plain-ASCII Alt with a
// key, which are the browser's.
// The input is focused and the key isn't consumed, so the browser gives
// it, and what it composes, to the input.
export function strayKey(e: { key: string; keyCode?: number; isComposing?: boolean; ctrlKey: boolean; altKey: boolean; metaKey: boolean; defaultPrevented?: boolean }): boolean {
  if (e.defaultPrevented || e.metaKey) return false;
  if (e.isComposing || e.keyCode === 229 || e.key === "Process" || e.key === "Dead") return true;
  if ([...e.key].length !== 1) return false;
  if (e.ctrlKey) return e.altKey; // Ctrl+Alt is AltGr in some browsers
  // Alt alone makes a letter on a Mac (Option+A is å); elsewhere Alt+D is
  // the browser's, its key a plain d.
  return !e.altKey || e.key.charCodeAt(0) > 0x7f;
}
