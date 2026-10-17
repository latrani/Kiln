import { Component } from "preact";
import { spanStyle } from "../look.ts";
import type { LineData } from "../store.ts";
import { roleClass } from "../theme.ts";
import type { Span, TagLook } from "../types.ts";

export interface Tags {
  dark: Record<string, TagLook>;
  light: Record<string, TagLook>;
}

// Dividers wear their roles; kiln.css draws the glyphs. Sent lines and
// Kiln's own wear their tags instead; see kilnStyle.
const kindClass: Record<LineData["Kind"], string> = {
  server: "l-server",
  echo: "l-echo",
  sys: "l-sys",
  day: "l-day " + roleClass("scrollback.day"),
  history_end: "l-history_end " + roleClass("scrollback.history_end"),
};

const web = /^https?:\/\//i;

// SpanView is one span: bare text when it has no look, a link when it's
// part of one the browser can open.
function SpanView({ s, tags }: { s: Span; tags: Tags }) {
  const style = spanStyle(s, tags.dark, tags.light);
  if (s.Link && web.test(s.Link)) {
    return <a class={"s " + roleClass("link")} style={style || undefined} href={s.Link} target="_blank" rel="noopener noreferrer">{s.Text}</a>;
  }
  return style ? <span class="s" style={style}>{s.Text}</span> : <>{s.Text}</>;
}

// LineProps are a Line's: the line, its character's tag tables, and for a
// scrollback line its class and number.
interface LineProps {
  line: LineData;
  tags: Tags;
  class?: string;
  seq?: number;
}

// Line is one scrollback line, drawn from Go's spans: nothing between
// them, so its text is exactly the line's (kiln.css keeps the spaces).
// It draws again only when the line is new or made again (the store keeps
// a line's object until then) or its tags restyle: not for every event.
export class Line extends Component<LineProps> {
  shouldComponentUpdate(next: LineProps): boolean {
    return !sameLine(this.props, next);
  }
  render() {
    return LineView(this.props);
  }
}

// sameLine reports whether a Line would draw the same.
export function sameLine(a: Pick<LineProps, "line" | "tags"> & Partial<LineProps>, b: Pick<LineProps, "line" | "tags"> & Partial<LineProps>): boolean {
  return a.line === b.line && a.tags === b.tags && a.class === b.class && a.seq === b.seq;
}

// kilnStyle is a sent line's or Kiln's own line's whole-line tag looks,
// worn by the line itself so the glyph kiln.css draws before it takes
// them too, as the TUI's does. Its tags match nothing, so match-scoped
// looks don't apply.
function kilnStyle(line: LineData, tags: Tags): string {
  if (line.Kind !== "echo" && line.Kind !== "sys") return "";
  const names = (line.Tags ?? []).map((t) => t.Name).filter((n) => !tags.dark[n]?.Match);
  return names.length ? spanStyle({ Text: "", Tags: names }, tags.dark, tags.light) : "";
}

function LineView({ line, tags, class: extra, seq }: LineProps) {
  const style = kilnStyle(line, tags);
  return (
    <div class={"line " + kindClass[line.Kind] + (style ? " s" : "") + (extra ? " " + extra : "")} style={style || undefined} data-seq={seq}>
      {(line.Spans ?? []).map((s, i) => <SpanView key={i} s={s} tags={tags} />)}
    </div>
  );
}
