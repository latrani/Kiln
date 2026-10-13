import { spanStyle } from "../look.ts";
import type { LineData } from "../store.ts";
import { roleClass } from "../theme.ts";
import type { Span, TagLook } from "../types.ts";

export interface Tags {
  dark: Record<string, TagLook>;
  light: Record<string, TagLook>;
}

// Kiln's own lines wear their roles; kiln.css draws their glyphs.
const kindClass: Record<LineData["Kind"], string> = {
  server: "l-server",
  echo: "l-echo " + roleClass("scrollback.echo"),
  sys: "l-sys " + roleClass("scrollback.sys"),
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

// Line is one scrollback line, drawn from Go's spans: nothing between
// them, so its text is exactly the line's (kiln.css keeps the spaces).
export function Line({ line, tags, class: extra }: { line: LineData; tags: Tags; class?: string }) {
  return (
    <div class={"line " + kindClass[line.Kind] + (extra ? " " + extra : "")}>
      {(line.Spans ?? []).map((s, i) => <SpanView key={i} s={s} tags={tags} />)}
    </div>
  );
}
