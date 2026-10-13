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

export type KeyDo = "submit" | "older" | "newer" | "cancel" | "page_up" | "page_down" | undefined;

interface Keyish {
  key: string;
  shiftKey: boolean;
  altKey: boolean;
  ctrlKey: boolean;
  metaKey: boolean;
  isComposing: boolean;
}

// keyAction is what a key in the input does, the caret at start..end in
// value; undefined leaves it to the browser. ↑ and ↓ recall history only
// from the first and last rows, so they still move through a multi-line
// draft; with a modifier they're the switching keys.
export function keyAction(e: Keyish, caret: { start: number; end: number; value: string }): KeyDo {
  if (e.isComposing) return undefined;
  const plain = !e.shiftKey && !e.altKey && !e.ctrlKey && !e.metaKey;
  switch (e.key) {
    case "Enter":
      return e.shiftKey || e.altKey || e.ctrlKey || e.metaKey ? undefined : "submit";
    case "Escape":
      return "cancel";
    case "PageUp":
      return plain ? "page_up" : undefined;
    case "PageDown":
      return plain ? "page_down" : undefined;
    case "ArrowUp":
      return plain && caret.start === caret.end && !caret.value.slice(0, caret.start).includes("\n") ? "older" : undefined;
    case "ArrowDown":
      return plain && caret.start === caret.end && !caret.value.slice(caret.end).includes("\n") ? "newer" : undefined;
  }
  return undefined;
}
