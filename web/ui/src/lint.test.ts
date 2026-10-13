import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";

// Text people read or hear comes from the catalog
// (internal/str/locales/en.toml) through str.*, as in Go. This parses
// each component and flags JSX text with a letter in it, and string
// literals with a letter that a label people hear would show: the
// attributes below, given as a literal or an expression, and expressions
// drawn as children. A node on a line ending in "// str:ok" is exempt.
const read = new Set(["aria-label", "aria-description", "aria-placeholder", "aria-roledescription", "aria-valuetext", "title", "placeholder", "alt", "value"]);
const letter = /\p{L}/u;

// results are the literals expression e can come out as: itself, either
// side of a ?:, what && gives, either side of || ?? and +.
function results(e: ts.Expression): ts.Node[] {
  if (ts.isStringLiteral(e) || ts.isNoSubstitutionTemplateLiteral(e) || ts.isTemplateExpression(e)) return [e];
  if (ts.isParenthesizedExpression(e)) return results(e.expression);
  if (ts.isConditionalExpression(e)) return [...results(e.whenTrue), ...results(e.whenFalse)];
  if (ts.isBinaryExpression(e)) {
    switch (e.operatorToken.kind) {
      case ts.SyntaxKind.AmpersandAmpersandToken: return results(e.right);
      case ts.SyntaxKind.BarBarToken:
      case ts.SyntaxKind.QuestionQuestionToken:
      case ts.SyntaxKind.PlusToken: return [...results(e.left), ...results(e.right)];
    }
  }
  return [];
}

// text is a literal's own words, without what's substituted into it.
function text(n: ts.Node): string {
  if (ts.isStringLiteral(n) || ts.isNoSubstitutionTemplateLiteral(n) || ts.isJsxText(n)) return n.text;
  if (ts.isTemplateExpression(n)) return n.head.text + n.templateSpans.map((s) => s.literal.text).join("");
  return "";
}

// stray is where file's source has text for people outside the catalog.
export function stray(file: string, src: string): string[] {
  const sf = ts.createSourceFile(file, src, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const lines = src.split("\n");
  const found: string[] = [];
  const flag = (n: ts.Node) => {
    if (!letter.test(text(n))) return;
    const line = sf.getLineAndCharacterOfPosition(n.getStart(sf)).line;
    if (lines[line].trimEnd().endsWith("// str:ok")) return;
    found.push(`${file}:${line + 1}: ${text(n).trim()}`);
  };
  const visit = (n: ts.Node) => {
    if (ts.isJsxText(n)) flag(n);
    else if (ts.isJsxAttribute(n) && read.has(n.name.getText(sf)) && n.initializer) {
      if (ts.isStringLiteral(n.initializer)) flag(n.initializer);
      else if (ts.isJsxExpression(n.initializer) && n.initializer.expression) results(n.initializer.expression).forEach(flag);
    } else if (ts.isJsxExpression(n) && n.expression && (ts.isJsxElement(n.parent) || ts.isJsxFragment(n.parent))) {
      results(n.expression).forEach(flag);
    }
    ts.forEachChild(n, visit);
  };
  visit(sf);
  return found;
}

test("no stray text in the page", () => {
  const dir = join(process.cwd(), "src", "ui");
  const found: string[] = [];
  for (const f of readdirSync(dir).filter((f) => f.endsWith(".tsx") && !f.endsWith(".test.tsx"))) {
    found.push(...stray(f, readFileSync(join(dir, f), "utf8")));
  }
  assert.deepEqual(found, [], "text for people goes in the catalog (internal/str/locales/en.toml) and comes from str.*");
});

test("the lint catches text however it's written", () => {
  const caught = (src: string) => stray("x.tsx", src).length;
  assert.equal(caught(`const a = <p>\n  Hello there\n</p>;`), 1, "text on its own lines");
  assert.equal(caught(`const a = <nav aria-label={"Characters"} />;`), 1, "a label as an expression");
  assert.equal(caught(`const a = <i title="Close" />;`), 1, "title");
  assert.equal(caught(`const a = <input value={on ? "Yes" : "No"} aria-description={\`Say \${x}\`} />;`), 3, "conditional and template");
  assert.equal(caught(`const a = <p>{err || "oops"}</p>;`), 1, "a child expression's fallback");
  assert.equal(caught(`const a = <p>{on && "Ready"}</p>;`), 1, "a child expression's && result");
});

test("the lint leaves code alone", () => {
  const caught = (src: string) => stray("x.tsx", src).length;
  assert.equal(caught(`const a = <p class="line r-x" key="k">{str.x()}{" "}{n}</p>;`), 0, "classes, keys, spaces, catalog calls");
  assert.equal(caught(`const a = <p>{mode === "log" && <b>{str.y()}</b>}</p>;`), 0, "comparisons");
  assert.equal(caught(`const a = <p onClick={() => act({ Do: "away" })}>{f("vocab")}</p>;`), 0, "handlers and arguments");
  assert.equal(caught(`const a = <p>Hello</p>; // str:ok`), 0, "exempt");
});
