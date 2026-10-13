// The shapes of what Go sends the page (internal/webapi, internal/feed).

export type Color = string; // "" | "#rrggbb" | "ansi:N", N 0–15

// Look is a style as data (theme.Look).
export interface Look {
  FG?: Color;
  BG?: Color;
  Bold?: boolean;
  Faint?: boolean;
  Italic?: boolean;
  Underline?: boolean;
  Reverse?: boolean;
}

export interface TagLook extends Look {
  Match?: boolean;
}

// Looks is one appearance's styles; Chars has the tag tables of
// characters whose own looks change them.
export interface Looks {
  Roles: Record<string, Look>;
  Tags: Record<string, TagLook>;
  Chars?: Record<string, Record<string, TagLook>>;
}

export interface ThemeEvent {
  type: "Theme";
  Appearance: "auto" | "light" | "dark";
  Dark: Looks;
  Light: Looks;
}

// Span is a run of a line with one look: the server's style, the styled
// tags over it in the order their styles fold, and its link.
export interface Span extends Look {
  Text: string;
  Tags?: string[];
  Link?: string;
}
