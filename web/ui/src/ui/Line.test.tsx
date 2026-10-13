import { test } from "node:test";
import assert from "node:assert/strict";
import { render } from "preact-render-to-string";
import { Line } from "./Line.tsx";
import { roleClass } from "../theme.ts";
import type { LineData } from "../store.ts";

const L = (over: Partial<LineData>): LineData => ({ Seq: 1, Kind: "server", Day: "", Out: false, Plain: "", Spans: [], Attention: false, Quiet: false, ...over });
const text = (html: string) => html.replace(/<[^>]*>/g, "").replace(/&amp;/g, "&").replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&quot;/g, '"');
const tags = { dark: {}, light: {} };

test("a line's text is exactly its plain text, spaces at span edges and all", () => {
  const l = L({ Plain: "a  b\tc ", Spans: [{ Text: "a " }, { Text: " b", Bold: true }, { Text: "\tc " }] });
  assert.equal(text(render(<Line line={l} tags={tags} />)), l.Plain);
});

test("styled spans carry their look; plain ones are bare text", () => {
  const html = render(<Line line={L({ Plain: "ab", Spans: [{ Text: "a" }, { Text: "b", FG: "ansi:1" }] })} tags={tags} />);
  assert.match(html, />a<span class="s" style="--d-fg:var\(--ansi-1\);--l-fg:var\(--ansi-1\)">b<\/span>/);
});

test("links are links, opening a new tab", () => {
  const html = render(<Line line={L({ Plain: "see https://x.test", Spans: [{ Text: "see " }, { Text: "https://x.test", Link: "https://x.test" }] })} tags={tags} />);
  assert.match(html, /<a [^>]*href="https:\/\/x.test"[^>]*target="_blank"[^>]*rel="noopener noreferrer"/);
  assert.ok(html.includes(roleClass("link")));
});

test("Kiln's lines wear their roles", () => {
  for (const [Kind, role] of [["echo", "scrollback.echo"], ["sys", "scrollback.sys"], ["day", "scrollback.day"], ["history_end", "scrollback.history_end"]] as const) {
    assert.ok(render(<Line line={L({ Kind, Plain: "x", Spans: [{ Text: "x" }] })} tags={tags} />).includes(roleClass(role)), Kind);
  }
});

test("a link that isn't http(s) isn't a link", () => {
  const html = render(<Line line={L({ Plain: "x", Spans: [{ Text: "x", Link: "javascript:alert(1)" }] })} tags={tags} />);
  assert.ok(!html.includes("<a "));
});

test("no spans: nothing breaks", () => {
  assert.equal(text(render(<Line line={L({ Spans: null })} tags={tags} />)), "");
});
