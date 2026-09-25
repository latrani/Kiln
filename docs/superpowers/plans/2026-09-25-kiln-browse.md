# Kiln Browse Mode Implementation Plan (Plan 3 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add browse mode. It's a dedicated screen over a character's full log history, with tri-state tag chips, find, range + exclude selection, and export to plain, ANSI, or HTML files or to the clipboard. The plan also adds `/highlight`, which appends a highlight rule to the world file.

**Architecture:** There are three new pure packages:
- `history` pages day files backward.
- `scene` holds the chip visibility rules, what counts as exportable, and the three formatters.
- `ansi.Spans` gets added for HTML.

`config` gains `export_dir` and `AppendHighlight`. The UI gains `browse.go` (per-character browse state and its view) and `keys.go` (the single binding table and glyph constants, so they can be tuned). The model routes keys, mouse, and live lines to an open browse. Lines are held by pointer, so paging older history never shifts marks.

**Tech Stack:** Go 1.27, plus the Plan 1 and Plan 2 dependencies. There are no new modules.

**Spec:** `docs/superpowers/specs/2026-09-24-kiln-design.md`, §6 "Browse mode" (revised 2026-09-25) and §4 (`export_dir`, `/highlight`).

**Verification note:** every code block here compiled and passed `go vet` and `go test -race -count=5` on Go 1.27.1 before this plan was written. The built client was then driven in tmux against FurryMUCK. Browse opened on the live log, the chip cycled, a marked range with one excluded line exported to HTML with exactly the right lines, and the save prompt scrolled so the filename stayed visible.

## Decisions made while planning (review these)

Everything the spec leaves open. Keys and glyphs are expected to change with use, and they all live in `internal/ui/keys.go`.

**Keys and navigation**
- `Ctrl+B` or `/browse` opens browse. `Esc` or `Ctrl+C` closes it.
- `↑/↓` (also `k`/`j`), `PgUp/PgDn`, and the mouse wheel move the cursor.
- `Home` loads all history and goes to the oldest line. `End` goes to the newest.
- `Ctrl+↑/↓` still switch characters. Browse state is kept per character.

**Chips**
- Chips are numbered `1[page] 2[say] …` in tag order, so `1`–`9` cycle the first nine. Clicking any chip cycles it too.

**Find**
- Matching lines are drawn as plain text with the match in reverse video, which drops the server's colors on those lines while find is active.

**Layout**
- The body is top-aligned. A short history leaves blank rows at the bottom, as in a document viewer.
- Scrolling keeps the cursor visible and never leaves blank rows below the newest line when more history exists above.

**Initial load**
- Browse loads whole days, newest first, until it has at least 200 lines. The header shows `…` while older days remain unloaded.

**Selecting**
- Shift-click sets the range end. If there's no start yet, the cursor's line becomes the start.

**Export**
- The save prompt is one line and scrolls horizontally with a leading `…`, so the filename stays visible.
- Export refuses to overwrite an existing file; edit the name instead. The file is `0644`, and its directory is created if needed.
- The HTML `<title>` is `<world> <Name> — <date of range start>`.
- The export filename's time comes from the first exported line.

**`/highlight`**
- `/highlight <text>` adds a case-insensitive literal match styled bold `#ffd166`. The appended rule uses inline tables, so it can't be misread as a sub-table of the preceding `[characters.x]`.

**Test fix**
- The Plan 2 test harness's `connected()` raced the auto-login line (seen as a rare flake). It now waits for that line, too.

## Global Constraints

- Everything from Plans 1 and 2 still holds (module `github.com/latrani/Kiln`, SGR-only rendering, a UI that never blocks, and no password in history or logs).
- Exports contain only received lines (`Dir == In`), inside the marked range, not excluded, and visible under the current chips. There are no timestamps.
- Browse never writes to logs. It only reads them via `history` and receives live lines from session events.
- Commits end with the Co-Authored-By and Claude-Session trailers shown in each commit step.

## Review Focus

1. **Hostile or odd server bytes in exports:** OSC and other escapes must not reach the HTML/ANSI/plain output, and HTML must be escaped (`<`, `&`, quotes). Pinned by Task 3 `TestHTML`/`TestANSI`/`TestPlain`, and Task 1 `TestSpans` "drops non-sgr".
2. **Marks staying put while history pages in:** marks and exclusions must not shift. Pinned by Task 5 `TestBrowseMarksSurvivePaging`.
3. **Chip logic edge cases:** untagged lines under Only/Hide, and hide beating only. Pinned by Task 3 `TestVisible`.
4. **Writing into the user's hand-edited world file:** comments are preserved, the rule lands at world level after a `[characters.x]` table, quotes and regex metacharacters are matched literally, and path traversal in the world id is rejected. Pinned by Task 4 `TestAppendHighlightAfterCharacterTable` and `TestAppendHighlightErrors`.
5. **Overwriting an existing export, and long paths in a narrow prompt:** refuse and say so, and keep the filename visible. Pinned by Task 5 `TestBrowseMarkExcludeExport` and `TestPromptWindow`.

---

### Task 1: ANSI → styled spans

**Files:**
- Create: `internal/ansi/spans.go`
- Test: `internal/ansi/spans_test.go`

**Interfaces:**
- Consumes: the unexported `skipEscape` in `internal/ansi/strip.go` (from the Plan 2 review fix)
- Produces: `ansi.SpanStyle{FG, BG string; Bold, Italic, Underline bool}` (colors are `#rrggbb`, or `""` for default), `ansi.Span{Text string; Style SpanStyle}`, `ansi.Spans(s string) []Span` (16, 256 and truecolor SGR; non-SGR escapes dropped; adjacent equal styles merged)

- [ ] **Step 1: Write the failing tests** (full file contents; files that already exist are replaced wholesale, and they keep every earlier test)

`internal/ansi/spans_test.go`:

```go
package ansi

import (
	"reflect"
	"testing"
)

func TestSpans(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Span
	}{
		{"plain", "hi", []Span{{"hi", SpanStyle{}}}},
		{"empty", "", nil},
		{"bold then reset", "\x1b[1mRook\x1b[0m says", []Span{{"Rook", SpanStyle{Bold: true}}, {" says", SpanStyle{}}}},
		{"16 color", "\x1b[31mred\x1b[39m plain", []Span{{"red", SpanStyle{FG: "#cd0000"}}, {" plain", SpanStyle{}}}},
		{"bright bg", "\x1b[101mx", []Span{{"x", SpanStyle{BG: "#ff0000"}}}},
		{"256 color", "\x1b[38;5;208mx", []Span{{"x", SpanStyle{FG: "#ff8700"}}}},
		{"256 gray", "\x1b[48;5;244mx", []Span{{"x", SpanStyle{BG: "#808080"}}}},
		{"truecolor", "\x1b[38;2;255;159;67mx", []Span{{"x", SpanStyle{FG: "#ff9f43"}}}},
		{"combined params", "\x1b[1;3;4;32mx\x1b[22;23;24mY", []Span{{"x", SpanStyle{FG: "#00cd00", Bold: true, Italic: true, Underline: true}}, {"Y", SpanStyle{FG: "#00cd00"}}}},
		{"empty sgr resets", "\x1b[1mA\x1b[mB", []Span{{"A", SpanStyle{Bold: true}}, {"B", SpanStyle{}}}},
		{"merges equal styles", "a\x1b[0mb", []Span{{"ab", SpanStyle{}}}},
		{"drops non-sgr", "a\x1b]0;t\x07b\x1b[2Jc", []Span{{"abc", SpanStyle{}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Spans(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Spans(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ansi/`
Expected: FAIL with build errors such as `undefined: Spans`.

- [ ] **Step 3: Write the implementation** (full file contents)

`internal/ansi/spans.go`:

```go
package ansi

import (
	"fmt"
	"strconv"
	"strings"
)

// SpanStyle is the SGR state for a run of text. Colors are "#rrggbb", or
// "" for the terminal default.
type SpanStyle struct {
	FG, BG    string
	Bold      bool
	Italic    bool
	Underline bool
}

// Span is a run of text drawn in one style.
type Span struct {
	Text  string
	Style SpanStyle
}

// Spans splits s into styled runs by interpreting its SGR sequences
// (16-color, 256-color and truecolor; bold, italic, underline). Other
// escape sequences are dropped. Adjacent runs never share a style.
func Spans(s string) []Span {
	var out []Span
	var st SpanStyle
	var text strings.Builder
	flush := func() {
		if text.Len() == 0 {
			return
		}
		if n := len(out); n > 0 && out[n-1].Style == st {
			out[n-1].Text += text.String()
		} else {
			out = append(out, Span{Text: text.String(), Style: st})
		}
		text.Reset()
	}
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			text.WriteByte(s[i])
			i++
			continue
		}
		j := skipEscape(s, i)
		if i+1 < len(s) && s[i+1] == '[' && s[j-1] == 'm' {
			flush()
			st = applySGR(st, s[i+2:j-1])
		}
		i = j
	}
	flush()
	return out
}

func applySGR(st SpanStyle, params string) SpanStyle {
	if params == "" {
		return SpanStyle{}
	}
	ps := strings.Split(strings.ReplaceAll(params, ":", ";"), ";")
	num := func(k int) int {
		if k >= len(ps) {
			return -1
		}
		n, err := strconv.Atoi(ps[k])
		if err != nil {
			return -1
		}
		return n
	}
	for k := 0; k < len(ps); k++ {
		switch n := num(k); {
		case n == 0 || ps[k] == "":
			st = SpanStyle{}
		case n == 1:
			st.Bold = true
		case n == 22:
			st.Bold = false
		case n == 3:
			st.Italic = true
		case n == 23:
			st.Italic = false
		case n == 4:
			st.Underline = true
		case n == 24:
			st.Underline = false
		case n >= 30 && n <= 37:
			st.FG = palette16[n-30]
		case n >= 90 && n <= 97:
			st.FG = palette16[n-90+8]
		case n == 39:
			st.FG = ""
		case n >= 40 && n <= 47:
			st.BG = palette16[n-40]
		case n >= 100 && n <= 107:
			st.BG = palette16[n-100+8]
		case n == 49:
			st.BG = ""
		case n == 38 || n == 48:
			var c string
			switch num(k + 1) {
			case 5:
				c = color256(num(k + 2))
				k += 2
			case 2:
				r, g, b := num(k+2), num(k+3), num(k+4)
				if r >= 0 && g >= 0 && b >= 0 {
					c = fmt.Sprintf("#%02x%02x%02x", r&255, g&255, b&255)
				}
				k += 4
			}
			if n == 38 {
				st.FG = c
			} else {
				st.BG = c
			}
		}
	}
	return st
}

// palette16 is the xterm default 16-color palette.
var palette16 = [16]string{
	"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
	"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
}

// color256 maps an xterm 256-color index to "#rrggbb" ("" if invalid).
func color256(n int) string {
	switch {
	case n < 0 || n > 255:
		return ""
	case n < 16:
		return palette16[n]
	case n < 232:
		n -= 16
		level := func(v int) int {
			if v == 0 {
				return 0
			}
			return 55 + v*40
		}
		return fmt.Sprintf("#%02x%02x%02x", level(n/36), level(n/6%6), level(n%6))
	default:
		v := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/ansi/ && go test ./internal/ansi/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/ansi/spans_test.go internal/ansi/spans.go
git commit -m "feat(ansi): parse SGR into styled spans

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 2: History pager

**Files:**
- Create: `internal/history/history.go`
- Test: `internal/history/history_test.go`

**Interfaces:**
- Consumes: `logstore.Days`, `logstore.ReadDay`
- Produces: `history.NewReader(root, world, char string) (*Reader, error)`, `(*Reader).LoadOlder() (entries []logstore.Entry, day string, ok bool, err error)` (newest day first, then older), `(*Reader).Exhausted() bool`, `(*Reader).Oldest() string`

- [ ] **Step 1: Write the failing tests** (full file contents; files that already exist are replaced wholesale, and they keep every earlier test)

`internal/history/history_test.go`:

```go
package history

import (
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/logstore"
)

func TestLoadOlderWalksBackward(t *testing.T) {
	root := t.TempDir()
	w := logstore.NewWriter(root, "fm", "kit")
	for _, d := range []int{22, 24, 23} {
		w.Append(logstore.Entry{Time: time.Date(2026, 9, d, 12, 0, 0, 0, time.UTC), Dir: logstore.In, Text: "day"})
	}
	w.Close()
	r, err := NewReader(root, "fm", "kit")
	if err != nil {
		t.Fatal(err)
	}
	if r.Oldest() != "2026-09-22" {
		t.Errorf("Oldest = %q", r.Oldest())
	}
	var got []string
	for {
		es, day, ok, err := r.LoadOlder()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		if len(es) != 1 {
			t.Errorf("%s: %d entries", day, len(es))
		}
		got = append(got, day)
	}
	if want := "2026-09-24 2026-09-23 2026-09-22"; join(got) != want {
		t.Errorf("days = %q, want %q", join(got), want)
	}
	if !r.Exhausted() {
		t.Error("not exhausted")
	}
}

func TestNoLogs(t *testing.T) {
	r, err := NewReader(t.TempDir(), "fm", "nobody")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := r.LoadOlder(); ok || !r.Exhausted() || r.Oldest() != "" {
		t.Error("expected empty, exhausted reader")
	}
}

func join(xs []string) string {
	s := ""
	for i, x := range xs {
		if i > 0 {
			s += " "
		}
		s += x
	}
	return s
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/history/`
Expected: FAIL with build errors such as `undefined: NewReader`.

- [ ] **Step 3: Write the implementation** (full file contents)

`internal/history/history.go`:

```go
// Package history pages a character's logs backward, one day at a time.
package history

import "github.com/latrani/Kiln/internal/logstore"

// Reader walks a character's day files from newest to oldest.
type Reader struct {
	root, world, char string
	days              []string // oldest first
	next              int      // index of the next day LoadOlder returns
}

// NewReader lists the character's log days. A character with no logs
// yields a Reader that is immediately exhausted.
func NewReader(root, world, char string) (*Reader, error) {
	days, err := logstore.Days(root, world, char)
	if err != nil {
		return nil, err
	}
	return &Reader{root: root, world: world, char: char, days: days, next: len(days) - 1}, nil
}

// LoadOlder returns the next older day's entries and its date
// ("YYYY-MM-DD"). ok is false once every day has been returned.
func (r *Reader) LoadOlder() (entries []logstore.Entry, day string, ok bool, err error) {
	if r.next < 0 {
		return nil, "", false, nil
	}
	day = r.days[r.next]
	r.next--
	entries, err = logstore.ReadDay(r.root, r.world, r.char, day)
	return entries, day, true, err
}

// Exhausted reports whether every day has been loaded.
func (r *Reader) Exhausted() bool { return r.next < 0 }

// Oldest is the earliest day with logs, or "" if there are none.
func (r *Reader) Oldest() string {
	if len(r.days) == 0 {
		return ""
	}
	return r.days[0]
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/history/ && go test ./internal/history/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/history/history_test.go internal/history/history.go
git commit -m "feat(history): page a character's logs backward by day

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 3: Scene selection rules and export formats

**Files:**
- Create: `internal/scene/scene.go`
- Test: `internal/scene/scene_test.go`

**Interfaces:**
- Consumes: `ansi.Strip`, `ansi.Sanitize`, `ansi.Spans` (Task 1)
- Produces: `scene.Chip` (`Neutral`, `Only`, `Hide`) with `Next()`, `scene.Visible(tags []string, chips map[string]Chip) bool` (hide wins; any Only chip means the line must carry an Only tag), `scene.Exportable(logstore.Entry) bool` (received lines only), `scene.Plain`/`ANSI`/`HTML`, `scene.Render(format, entries, title) string`, `scene.Ext(format) string`, `scene.FileName(dir, t, world, name, format) string`

- [ ] **Step 1: Write the failing tests** (full file contents; files that already exist are replaced wholesale, and they keep every earlier test)

`internal/scene/scene_test.go`:

```go
package scene

import (
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/logstore"
)

func TestVisible(t *testing.T) {
	cases := []struct {
		name  string
		tags  []string
		chips map[string]Chip
		want  bool
	}{
		{"no chips", []string{"page"}, nil, true},
		{"neutral chip", []string{"page"}, map[string]Chip{"page": Neutral}, true},
		{"hidden tag", []string{"page"}, map[string]Chip{"page": Hide}, false},
		{"untagged survives hide", nil, map[string]Chip{"page": Hide}, true},
		{"only: has it", []string{"page", "self"}, map[string]Chip{"page": Only}, true},
		{"only: lacks it", []string{"say"}, map[string]Chip{"page": Only}, false},
		{"only: untagged hidden", nil, map[string]Chip{"page": Only}, false},
		{"only either of two", []string{"whisper"}, map[string]Chip{"page": Only, "whisper": Only}, true},
		{"hide beats only", []string{"page", "ooc"}, map[string]Chip{"page": Only, "ooc": Hide}, false},
	}
	for _, c := range cases {
		if got := Visible(c.tags, c.chips); got != c.want {
			t.Errorf("%s: Visible = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestChipCycle(t *testing.T) {
	if Neutral.Next() != Only || Only.Next() != Hide || Hide.Next() != Neutral {
		t.Error("cycle wrong")
	}
}

func TestExportable(t *testing.T) {
	if !Exportable(logstore.Entry{Dir: logstore.In}) || Exportable(logstore.Entry{Dir: logstore.Out}) || Exportable(logstore.Entry{Dir: logstore.Sys}) {
		t.Error("only received lines are exportable")
	}
}

var sample = []logstore.Entry{
	{Dir: logstore.In, Text: "\x1b[1mSable\x1b[0m waves a paw."},
	{Dir: logstore.In, Text: "Kit says, \"<3 & hi\"\x1b]0;evil\x07"},
}

func TestPlain(t *testing.T) {
	want := "Sable waves a paw.\nKit says, \"<3 & hi\"\n"
	if got := Plain(sample); got != want {
		t.Errorf("Plain = %q, want %q", got, want)
	}
}

func TestANSI(t *testing.T) {
	got := ANSI(sample)
	if !strings.Contains(got, "\x1b[1mSable\x1b[0m waves a paw.\x1b[0m\n") || strings.Contains(got, "evil") {
		t.Errorf("ANSI = %q", got)
	}
}

func TestHTML(t *testing.T) {
	got := HTML(sample, "Tavern <scene>")
	for _, want := range []string{
		"<title>Tavern &lt;scene&gt;</title>",
		`<span style="font-weight:bold">Sable</span> waves a paw.` + "\n",
		"Kit says, &#34;&lt;3 &amp; hi&#34;\n",
		"</pre>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "evil") {
		t.Error("OSC leaked into HTML")
	}
}

func TestFileName(t *testing.T) {
	ts := time.Date(2026, 9, 24, 21, 14, 0, 0, time.UTC)
	got := FileName("/x/Kiln Scenes", ts, "furrymuck", "Kit", "html")
	if got != "/x/Kiln Scenes/2026-09-24 2114 furrymuck Kit.html" {
		t.Errorf("FileName = %q", got)
	}
	if Ext("ansi") != "ans" || Ext("plain") != "txt" || Ext("bogus") != "txt" {
		t.Error("Ext wrong")
	}
	if Render("plain", sample, "") != Plain(sample) || Render("html", sample, "t") != HTML(sample, "t") {
		t.Error("Render dispatch wrong")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/scene/`
Expected: FAIL with build errors such as `undefined: Visible`.

- [ ] **Step 3: Write the implementation** (full file contents)

`internal/scene/scene.go`:

```go
// Package scene decides which lines belong to an exported scene and
// renders them as plain text, ANSI or HTML.
package scene

import (
	"fmt"
	"html"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/logstore"
)

// Chip is a tag filter's state.
type Chip int

const (
	Neutral Chip = iota
	Only         // show only lines carrying this tag
	Hide         // hide lines carrying this tag
)

// Next cycles Neutral → Only → Hide → Neutral.
func (c Chip) Next() Chip { return (c + 1) % 3 }

// Visible applies chip filters to a line's tags. A line is hidden if it
// carries any Hide tag. If any chip is Only, the line must carry at least
// one Only tag.
func Visible(tags []string, chips map[string]Chip) bool {
	wantOnly := false
	hasOnly := false
	for tag, c := range chips {
		switch c {
		case Hide:
			if slices.Contains(tags, tag) {
				return false
			}
		case Only:
			wantOnly = true
			if slices.Contains(tags, tag) {
				hasOnly = true
			}
		}
	}
	return !wantOnly || hasOnly
}

// Exportable reports whether an entry belongs in an exported scene at
// all: only text received from the server (sent commands are echoed by
// the server anyway, and client lines are noise).
func Exportable(e logstore.Entry) bool { return e.Dir == logstore.In }

// Plain renders entries as plain text, one line each.
func Plain(entries []logstore.Entry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(ansi.Strip(ansi.Sanitize(e.Text)))
		b.WriteByte('\n')
	}
	return b.String()
}

// ANSI renders entries with their (sanitized) colors, each line reset.
func ANSI(entries []logstore.Entry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(ansi.Sanitize(e.Text))
		b.WriteString("\x1b[0m\n")
	}
	return b.String()
}

// HTML renders entries as a standalone dark-background page.
func HTML(entries []logstore.Entry, title string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>%s</title>
<style>
body { background: #1b1b1f; color: #d8d8d8; margin: 2rem; }
pre { font: 14px/1.45 ui-monospace, SFMono-Regular, Menlo, monospace; white-space: pre-wrap; }
</style>
</head>
<body>
<pre>
`, html.EscapeString(title))
	for _, e := range entries {
		for _, sp := range ansi.Spans(ansi.Sanitize(e.Text)) {
			if css := spanCSS(sp.Style); css != "" {
				fmt.Fprintf(&b, `<span style="%s">%s</span>`, css, html.EscapeString(sp.Text))
			} else {
				b.WriteString(html.EscapeString(sp.Text))
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("</pre>\n</body>\n</html>\n")
	return b.String()
}

func spanCSS(s ansi.SpanStyle) string {
	var parts []string
	if s.FG != "" {
		parts = append(parts, "color:"+s.FG)
	}
	if s.BG != "" {
		parts = append(parts, "background:"+s.BG)
	}
	if s.Bold {
		parts = append(parts, "font-weight:bold")
	}
	if s.Italic {
		parts = append(parts, "font-style:italic")
	}
	if s.Underline {
		parts = append(parts, "text-decoration:underline")
	}
	return strings.Join(parts, ";")
}

// Ext is the file extension for a format: "plain", "ansi" or "html".
func Ext(format string) string {
	switch format {
	case "html":
		return "html"
	case "ansi":
		return "ans"
	default:
		return "txt"
	}
}

// Render renders entries in format ("plain", "ansi" or "html").
func Render(format string, entries []logstore.Entry, title string) string {
	switch format {
	case "html":
		return HTML(entries, title)
	case "ansi":
		return ANSI(entries)
	default:
		return Plain(entries)
	}
}

// FileName is the default export path:
// "<dir>/YYYY-MM-DD HHMM <world> <name>.<ext>", timed by the scene's first line.
func FileName(dir string, t time.Time, world, name, format string) string {
	return filepath.Join(dir, fmt.Sprintf("%s %s %s.%s", t.Format("2006-01-02 1504"), world, name, Ext(format)))
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/scene/ && go test ./internal/scene/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/scene/scene_test.go internal/scene/scene.go
git commit -m "feat(scene): tri-state tag filtering and plain/ANSI/HTML export

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 4: Config: export_dir and /highlight write-back

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/write.go`
- Test: `internal/config/config_test.go`, `internal/config/write_test.go`

**Interfaces:**
- Produces: `config.Config.ExportDir string` (top-level `export_dir` in `config.toml`, `~` expanded, default `config.DefaultExportDir` = `~/Documents/Kiln Scenes`), `config.AppendHighlight(dir, world, text string) error` (appends a case-insensitive literal `[[highlight]]` rule with inline tables to `worlds/<world>.toml`), `config.HighlightStyle`

- [ ] **Step 1: Write the failing tests** (full file contents; files that already exist are replaced wholesale, and they keep every earlier test)

`internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write creates files under dir from a map of relative path → content.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadResolvesInheritance(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"config.toml": "[defaults]\nmax_line_bytes = 1000\n",
		"packs/p.toml": `
[[classify]]
tag = "page"
pattern = '^\S+ pages: '
`,
		"worlds/fm.toml": `
host = "example.org"
port = 8899
tls = true
use = ["p"]
login = "connect {name} {password}"

[[classify]]
tag = "ooc"
pattern = '^OOC'

[characters.kit]
name = "Kit"
aliases = ["Kitty"]

[characters.rook]
name = "Rook"
max_line_bytes = 500
login = "co {name} {password}"

[[characters.rook.classify]]
tag = "mine"
pattern = 'Rook'
`,
	})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, ok := cfg.Find("fm", "kit")
	if !ok {
		t.Fatal("kit not found")
	}
	if kit.Name != "Kit" || kit.Aliases[0] != "Kitty" || kit.Host != "example.org" || kit.Port != 8899 || !kit.TLS {
		t.Errorf("kit basics wrong: %+v", kit)
	}
	if kit.TLSTrust != "pin" {
		t.Errorf("TLSTrust = %q, want default pin", kit.TLSTrust)
	}
	if kit.MaxLineBytes != 1000 {
		t.Errorf("kit MaxLineBytes = %d, want 1000 from [defaults]", kit.MaxLineBytes)
	}
	if kit.NewlineMode != "batch" {
		t.Errorf("kit NewlineMode = %q, want built-in batch", kit.NewlineMode)
	}
	if kit.Login != "connect {name} {password}" {
		t.Errorf("kit Login = %q, want world's", kit.Login)
	}
	if tags := classifyTags(kit.Rules); tags != "page,ooc" {
		t.Errorf("kit classify tags = %s, want pack then world: page,ooc", tags)
	}

	rook, _ := cfg.Find("fm", "rook")
	if rook.MaxLineBytes != 500 || rook.Login != "co {name} {password}" {
		t.Errorf("rook overrides lost: %+v", rook)
	}
	if tags := classifyTags(rook.Rules); tags != "page,ooc,mine" {
		t.Errorf("rook classify tags = %s, want page,ooc,mine", tags)
	}
	if tags := classifyTags(kit.Rules); strings.Contains(tags, "mine") {
		t.Error("rook's rule leaked into kit")
	}
}

func classifyTags(r Rules) string {
	var tags []string
	for _, c := range r.Classify {
		tags = append(tags, c.Tag)
	}
	return strings.Join(tags, ",")
}

func TestLoadEmptyDirIsEmptyConfig(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil || len(cfg.Worlds) != 0 {
		t.Errorf("Load(empty) = %+v, %v", cfg, err)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name, world, wantErr string
	}{
		{"missing host", "port = 1\n", "host is required"},
		{"bad port", "host = \"h\"\nport = 0\n", "port must be"},
		{"missing name", "host = \"h\"\nport = 1\n[characters.kit]\n", "name is required"},
		{"unknown key", "host = \"h\"\nport = 1\nhots = \"typo\"\n", `unknown key "hots"`},
		{"unknown pack", "host = \"h\"\nport = 1\nuse = [\"nope\"]\n", `unknown pack "nope"`},
		{"bad regex", "host = \"h\"\nport = 1\n[[classify]]\ntag = \"x\"\npattern = '('\n[characters.kit]\nname = \"Kit\"\n", "classify rule 1"},
		{"empty highlight match", "host = \"h\"\nport = 1\n[[highlight]]\nattention = true\n[characters.kit]\nname = \"Kit\"\n", "match needs tags or pattern"},
		{"bad trust", "host = \"h\"\nport = 1\ntls_trust = \"yolo\"\n", "tls_trust"},
		{"bad newline mode", "host = \"h\"\nport = 1\nnewline_mode = \"x\"\n[characters.kit]\nname = \"Kit\"\n", "newline_mode"},
		{"bad char id", "host = \"h\"\nport = 1\n[characters.\"a/b\"]\nname = \"X\"\n", "id may only use"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, map[string]string{"worlds/w.toml": c.world})
			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want containing %q", err, c.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "w.toml") {
				t.Errorf("err %q should name the file", err)
			}
		})
	}
}

func TestEnsureDefaultsWritesStarterFilesOnce(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"config.toml", "packs/fuzzball.toml"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("%s not written: %v", rel, err)
		}
	}
	// User edits must survive a second run.
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("# mine\n"), 0o600)
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if string(b) != "# mine\n" {
		t.Errorf("EnsureDefaults overwrote config.toml: %q", b)
	}
}

func TestStarterPackLoads(t *testing.T) {
	dir := t.TempDir()
	EnsureDefaults(dir)
	write(t, dir, map[string]string{"worlds/fm.toml": "host = \"h\"\nport = 1\nuse = [\"fuzzball\"]\n[characters.kit]\nname = \"Kit\"\n"})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "kit")
	if len(kit.Rules.Classify) == 0 || len(kit.Rules.Highlight) == 0 {
		t.Errorf("fuzzball pack rules missing: %+v", kit.Rules)
	}
	if kit.MaxLineBytes != 2047 {
		t.Errorf("MaxLineBytes = %d, want 2047", kit.MaxLineBytes)
	}
}

func TestAutoconnectInherits(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"worlds/a.toml": "host = \"h\"\nport = 1\nautoconnect = true\n[characters.kit]\nname = \"Kit\"\n[characters.rook]\nname = \"Rook\"\nautoconnect = false\n",
		"worlds/b.toml": "host = \"h\"\nport = 1\n[characters.ash]\nname = \"Ash\"\n",
	})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		world, char string
		want        bool
	}{{"a", "kit", true}, {"a", "rook", false}, {"b", "ash", false}} {
		ch, _ := cfg.Find(c.world, c.char)
		if ch.Autoconnect != c.want {
			t.Errorf("%s/%s Autoconnect = %v, want %v", c.world, c.char, ch.Autoconnect, c.want)
		}
	}
}

func TestExportDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	for _, c := range []struct{ toml, want string }{
		{"", filepath.Join(home, "Documents", "Kiln Scenes")},
		{"export_dir = \"~/scenes\"\n", filepath.Join(home, "scenes")},
		{"export_dir = \"/tmp/x\"\n", "/tmp/x"},
	} {
		dir := t.TempDir()
		write(t, dir, map[string]string{"config.toml": c.toml})
		cfg, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ExportDir != c.want {
			t.Errorf("export_dir %q → %q, want %q", c.toml, cfg.ExportDir, c.want)
		}
	}
}
```

`internal/config/write_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAppendHighlightAfterCharacterTable(t *testing.T) {
	dir := t.TempDir()
	world := "# my world\nhost = \"h\"\nport = 1\n\n[characters.kit]\nname = \"Kit\"\n"
	write(t, dir, map[string]string{"worlds/fm.toml": world})
	if err := AppendHighlight(dir, "fm", `the "lighthouse" (old)`); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "worlds", "fm.toml"))
	if !strings.HasPrefix(string(b), world) {
		t.Errorf("existing content changed:\n%s", b)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("appended file no longer loads: %v\n%s", err, b)
	}
	kit, _ := cfg.Find("fm", "kit")
	if len(kit.Rules.Highlight) != 1 {
		t.Fatalf("rules = %+v (the rule must land at world level, not inside [characters.kit])", kit.Rules.Highlight)
	}
	r := kit.Rules.Highlight[0]
	re := regexp.MustCompile(r.Match.Pattern)
	if !re.MatchString(`I saw THE "LIGHTHOUSE" (OLD) glow`) || re.MatchString("the lighthouse old") {
		t.Errorf("pattern %q should match the text literally, case-insensitively", r.Match.Pattern)
	}
	if r.Style != HighlightStyle {
		t.Errorf("style = %+v", r.Style)
	}
}

func TestAppendHighlightErrors(t *testing.T) {
	dir := t.TempDir()
	if err := AppendHighlight(dir, "fm", "   "); err == nil {
		t.Error("empty text accepted")
	}
	if err := AppendHighlight(dir, "../evil", "x"); err == nil {
		t.Error("bad world id accepted")
	}
	if err := AppendHighlight(dir, "missing", "x"); err == nil {
		t.Error("missing world file accepted")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -race ./internal/config/`
Expected: FAIL with build errors such as `cfg.ExportDir undefined` and `undefined: AppendHighlight`.

- [ ] **Step 3: Write the implementation** (full file contents)

`internal/config/config.go`:

```go
// Package config loads Kiln's config directory:
//
//	<dir>/config.toml        [defaults] + global prefs
//	<dir>/worlds/<id>.toml   one world and its characters
//	<dir>/packs/<id>.toml    reusable classify/highlight rule sets
//
// and resolves inheritance: defaults → packs (in `use` order) → world →
// character. Rule lists append along that chain; scalars override.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Style is how a highlighted line is drawn. Empty colors mean "unchanged".
type Style struct {
	FG        string `toml:"fg"`
	BG        string `toml:"bg"`
	Bold      bool   `toml:"bold"`
	Italic    bool   `toml:"italic"`
	Underline bool   `toml:"underline"`
}

// ClassifyRule tags lines whose plain text matches Pattern.
type ClassifyRule struct {
	Tag     string `toml:"tag"`
	Pattern string `toml:"pattern"`
}

// Match selects lines for a highlight rule. A line matches when it has at
// least one of Tags (if any are given) AND matches Pattern (if given).
type Match struct {
	Tags    []string `toml:"tags"`
	Pattern string   `toml:"pattern"`
}

// HighlightRule styles matching lines and optionally flags them for attention.
type HighlightRule struct {
	Match     Match `toml:"match"`
	Style     Style `toml:"style"`
	Attention bool  `toml:"attention"`
}

// Rules is the rule set carried by packs, worlds, and characters.
type Rules struct {
	Classify  []ClassifyRule  `toml:"classify"`
	Highlight []HighlightRule `toml:"highlight"`
}

// Character is a fully resolved character: everything a session needs.
type Character struct {
	World        string // world id (filename without .toml)
	ID           string // key under [characters]
	Name         string // canonical in-game name
	Aliases      []string
	Host         string
	Port         int
	TLS          bool
	TLSTrust     string // "pin" or "ca"
	Login        string // template with {name} and {password}; "" = no auto-login
	MaxLineBytes int
	NewlineMode  string // "batch" or "flatten"
	Autoconnect  bool   // connect when Kiln starts
	Rules        Rules
}

// World groups resolved characters under their world id.
type World struct {
	ID         string
	Characters []Character // sorted by ID
}

// Config is the resolved configuration.
type Config struct {
	Worlds    []World // sorted by ID
	ExportDir string  // where browse-mode exports go; "~" already expanded
}

// Find returns the resolved character, or false.
func (c *Config) Find(world, char string) (Character, bool) {
	for _, w := range c.Worlds {
		if w.ID != world {
			continue
		}
		for _, ch := range w.Characters {
			if ch.ID == char {
				return ch, true
			}
		}
	}
	return Character{}, false
}

// settings are the inheritable scalars. Pointers distinguish "unset" from
// zero values so later levels only override what they actually set.
type settings struct {
	MaxLineBytes *int    `toml:"max_line_bytes"`
	NewlineMode  *string `toml:"newline_mode"`
	Login        *string `toml:"login"`
	Autoconnect  *bool   `toml:"autoconnect"`
}

func (s *settings) overlay(o settings) {
	if o.MaxLineBytes != nil {
		s.MaxLineBytes = o.MaxLineBytes
	}
	if o.NewlineMode != nil {
		s.NewlineMode = o.NewlineMode
	}
	if o.Login != nil {
		s.Login = o.Login
	}
	if o.Autoconnect != nil {
		s.Autoconnect = o.Autoconnect
	}
}

type globalFile struct {
	ExportDir string   `toml:"export_dir"`
	Defaults  settings `toml:"defaults"`
}

type charFile struct {
	settings
	Rules
	Name    string   `toml:"name"`
	Aliases []string `toml:"aliases"`
}

type worldFile struct {
	settings
	Rules
	Host       string              `toml:"host"`
	Port       int                 `toml:"port"`
	TLS        bool                `toml:"tls"`
	TLSTrust   string              `toml:"tls_trust"`
	Use        []string            `toml:"use"`
	Characters map[string]charFile `toml:"characters"`
}

// Built-in defaults, applied beneath config.toml's [defaults].
const (
	DefaultMaxLineBytes = 2047 // Fuzzball MAX_COMMAND_LEN (2048) minus the NUL
	DefaultNewlineMode  = "batch"
)

var idRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Load reads and resolves the config directory. config.toml and the
// worlds/ and packs/ directories are all optional.
func Load(dir string) (*Config, error) {
	var g globalFile
	if err := decodeFile(filepath.Join(dir, "config.toml"), &g, true); err != nil {
		return nil, err
	}
	base := settings{MaxLineBytes: ptr(DefaultMaxLineBytes), NewlineMode: ptr(DefaultNewlineMode), Login: ptr(""), Autoconnect: ptr(false)}
	base.overlay(g.Defaults)

	worldPaths, err := filepath.Glob(filepath.Join(dir, "worlds", "*.toml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(worldPaths)

	packs := map[string]Rules{}
	exportDir, err := expandHome(g.ExportDir)
	if err != nil {
		return nil, err
	}
	cfg := &Config{ExportDir: exportDir}
	for _, wp := range worldPaths {
		w, err := loadWorld(dir, wp, base, packs)
		if err != nil {
			return nil, err
		}
		cfg.Worlds = append(cfg.Worlds, w)
	}
	return cfg, nil
}

func loadWorld(dir, path string, base settings, packs map[string]Rules) (World, error) {
	id := strings.TrimSuffix(filepath.Base(path), ".toml")
	rel := filepath.Join("worlds", filepath.Base(path))
	if !idRE.MatchString(id) {
		return World{}, fmt.Errorf("%s: world id %q may only use letters, digits, _ and -", rel, id)
	}
	var wf worldFile
	if err := decodeFile(path, &wf, false); err != nil {
		return World{}, err
	}
	if wf.Host == "" {
		return World{}, fmt.Errorf("%s: host is required", rel)
	}
	if wf.Port <= 0 || wf.Port > 65535 {
		return World{}, fmt.Errorf("%s: port must be 1-65535", rel)
	}
	switch wf.TLSTrust {
	case "":
		wf.TLSTrust = "pin"
	case "pin", "ca":
	default:
		return World{}, fmt.Errorf("%s: tls_trust must be \"pin\" or \"ca\"", rel)
	}

	var worldRules Rules
	for _, p := range wf.Use {
		r, err := loadPack(dir, p, packs)
		if err != nil {
			return World{}, fmt.Errorf("%s: %w", rel, err)
		}
		worldRules = appendRules(worldRules, r)
	}
	worldRules = appendRules(worldRules, wf.Rules)
	ws := base
	ws.overlay(wf.settings)

	w := World{ID: id}
	charIDs := make([]string, 0, len(wf.Characters))
	for cid := range wf.Characters {
		charIDs = append(charIDs, cid)
	}
	sort.Strings(charIDs)
	for _, cid := range charIDs {
		cf := wf.Characters[cid]
		where := fmt.Sprintf("%s: characters.%s", rel, cid)
		if !idRE.MatchString(cid) {
			return World{}, fmt.Errorf("%s: id may only use letters, digits, _ and -", where)
		}
		if strings.TrimSpace(cf.Name) == "" {
			return World{}, fmt.Errorf("%s: name is required", where)
		}
		cs := ws
		cs.overlay(cf.settings)
		ch := Character{
			World: id, ID: cid, Name: cf.Name, Aliases: cf.Aliases,
			Host: wf.Host, Port: wf.Port, TLS: wf.TLS, TLSTrust: wf.TLSTrust,
			Login: *cs.Login, MaxLineBytes: *cs.MaxLineBytes, NewlineMode: *cs.NewlineMode, Autoconnect: *cs.Autoconnect,
			Rules: appendRules(worldRules, cf.Rules),
		}
		if err := validate(ch); err != nil {
			return World{}, fmt.Errorf("%s: %w", where, err)
		}
		w.Characters = append(w.Characters, ch)
	}
	return w, nil
}

func loadPack(dir, id string, cache map[string]Rules) (Rules, error) {
	if r, ok := cache[id]; ok {
		return r, nil
	}
	if !idRE.MatchString(id) {
		return Rules{}, fmt.Errorf("pack id %q may only use letters, digits, _ and -", id)
	}
	var r Rules
	path := filepath.Join(dir, "packs", id+".toml")
	if _, err := os.Stat(path); err != nil {
		return Rules{}, fmt.Errorf("unknown pack %q (no packs/%s.toml)", id, id)
	}
	if err := decodeFile(path, &r, false); err != nil {
		return Rules{}, err
	}
	cache[id] = r
	return r, nil
}

func validate(ch Character) error {
	if ch.MaxLineBytes <= 0 {
		return errors.New("max_line_bytes must be positive")
	}
	if ch.NewlineMode != "batch" && ch.NewlineMode != "flatten" {
		return errors.New(`newline_mode must be "batch" or "flatten"`)
	}
	for i, r := range ch.Rules.Classify {
		if r.Tag == "" {
			return fmt.Errorf("classify rule %d: tag is required", i+1)
		}
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("classify rule %d (%s): %w", i+1, r.Tag, err)
		}
	}
	for i, r := range ch.Rules.Highlight {
		if len(r.Match.Tags) == 0 && r.Match.Pattern == "" {
			return fmt.Errorf("highlight rule %d: match needs tags or pattern", i+1)
		}
		if _, err := regexp.Compile(r.Match.Pattern); err != nil {
			return fmt.Errorf("highlight rule %d: %w", i+1, err)
		}
	}
	return nil
}

// decodeFile decodes TOML into v, rejecting unknown keys so typos surface.
func decodeFile(path string, v any, optional bool) error {
	b, err := os.ReadFile(path)
	if optional && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	md, err := toml.Decode(string(b), v)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return fmt.Errorf("%s: unknown key %q", filepath.Base(path), und[0].String())
	}
	return nil
}

// DefaultExportDir is used when config.toml sets no export_dir.
const DefaultExportDir = "~/Documents/Kiln Scenes"

// expandHome expands a leading "~/" (and defaults an empty path).
func expandHome(p string) (string, error) {
	if p == "" {
		p = DefaultExportDir
	}
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
}

func appendRules(a, b Rules) Rules {
	return Rules{
		Classify:  append(append([]ClassifyRule(nil), a.Classify...), b.Classify...),
		Highlight: append(append([]HighlightRule(nil), a.Highlight...), b.Highlight...),
	}
}

func ptr[T any](v T) *T { return &v }
```

`internal/config/write.go`:

```go
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// HighlightStyle is the style /highlight gives new rules.
var HighlightStyle = Style{FG: "#ffd166", Bold: true}

// AppendHighlight adds a literal-text, case-insensitive highlight rule to
// the end of worlds/<world>.toml. Appending is safe because TOML table
// headers are absolute: "[[highlight]]" always means the world's top-level
// list, even after a [characters.x] table. The rule uses inline tables so
// nothing after the header can be mistaken for a sub-table. Existing
// content and comments are kept.
func AppendHighlight(dir, world, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("nothing to highlight")
	}
	if !idRE.MatchString(world) {
		return fmt.Errorf("bad world id %q", world)
	}
	pattern, err := tomlString("(?i)" + regexp.QuoteMeta(text))
	if err != nil {
		return err
	}
	rule := fmt.Sprintf("\n# added by /highlight\n[[highlight]]\nmatch = { pattern = %s }\nstyle = { fg = %q, bold = %t }\n",
		pattern, HighlightStyle.FG, HighlightStyle.Bold)
	path := filepath.Join(dir, "worlds", world+".toml")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(rule)
	return err
}

// tomlString quotes s as a TOML basic string. JSON string escapes
// (\" \\ \n \uXXXX) are all valid TOML escapes.
func tomlString(s string) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/config/ && go test -race ./internal/config/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/config/config_test.go internal/config/write_test.go internal/config/config.go internal/config/write.go
git commit -m "feat(config): export_dir and AppendHighlight for /highlight

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 5: Browse mode UI, and a fix for a Plan 2 test race

**Files:**
- Create: `internal/ui/keys.go`, `internal/ui/browse.go`
- Modify: `internal/ui/model.go` (browse state per character, `Ctrl+B` / `/browse`, `/highlight`, live lines, mouse routing), `internal/ui/view.go` (the right pane draws browse when open)
- Test: `internal/ui/browse_test.go`, `internal/ui/model_test.go` (the harness `connected()` helper now also waits for the auto-login line, which fixes a pre-existing flake in `TestAutoconnectOnlyFlaggedCharacters`)

**Interfaces:**
- Consumes: `history` (Task 2), `scene` (Task 3), `config.ExportDir`/`AppendHighlight` (Task 4), `ansi.Wrap`/`Strip`/`Sanitize`, `tea.SetClipboard`
- Produces: `browseKeys` (the single binding table), `openBrowseKey`, glyph constants, `(*Model).openBrowse`, and `charState.browse *browse`

- [ ] **Step 1: Write the failing tests** (full file contents; files that already exist are replaced wholesale, and they keep every earlier test)

`internal/ui/browse_test.go`:

```go
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
)

var day24 = time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)

// writeLog puts entries in fm/kit's log, one minute apart from start.
func (h *harness) writeLog(start time.Time, lines ...string) {
	h.t.Helper()
	w := logstore.NewWriter(h.m.d.LogRoot, "fm", "kit")
	defer w.Close()
	for i, l := range lines {
		dir := logstore.In
		if strings.HasPrefix(l, "> ") {
			dir, l = logstore.Out, strings.TrimPrefix(l, "> ")
		}
		if err := w.Append(logstore.Entry{Time: start.Add(time.Duration(i) * time.Minute), Dir: dir, Text: l}); err != nil {
			h.t.Fatal(err)
		}
	}
}

func (h *harness) key(s string) tea.Cmd {
	var k tea.KeyPressMsg
	switch s {
	case "esc":
		k = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		k = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "up":
		k = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		k = tea.KeyPressMsg{Code: tea.KeyDown}
	case "home":
		k = tea.KeyPressMsg{Code: tea.KeyHome}
	case "space":
		k = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+b":
		k = tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}
	default:
		r := []rune(s)[0]
		k = tea.KeyPressMsg{Code: r, Text: s}
	}
	_, cmd := h.m.Update(k)
	return cmd
}

func (h *harness) keys(ss ...string) {
	for _, s := range ss {
		h.key(s)
	}
}

func (h *harness) br() *browse { return h.m.chars["fm/kit"].browse }

var scene1 = []string{
	"Rook says, \"Evening!\"",
	"Sable waves a paw.",
	"Mira pages: you around?",
	"> :grins.",
	"Kit grins.",
	"Rook says, \"The lighthouse is dark.\"",
	"Rook yawns.",
}

func TestBrowseOpensAndCloses(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	s := h.screen()
	for _, want := range []string{"BROWSE Kit · Thu Sep 24 → today", "── Thu Sep 24 ──", "21:00", "Rook says", "21:06", "m mark"} {
		if !strings.Contains(s, want) {
			t.Errorf("screen missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "> ") && strings.Contains(s, "fm/Kit 🔒") {
		t.Error("normal input/statusline still shown in browse mode")
	}
	h.key("esc")
	if h.br() != nil || !strings.Contains(h.screen(), "fm/Kit 🔒") {
		t.Errorf("esc did not close browse:\n%s", h.screen())
	}
}

func TestBrowseMarkExcludeExport(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	// Cursor starts on the last line (Rook yawns). Mark 21:01..21:05.
	h.keys("up", "m")                   // Rook says lighthouse = end
	h.keys("up", "up", "up", "up", "m") // Sable = start (marks may go either way)
	h.keys("down", "space")             // exclude the page
	b := h.br()
	var got []string
	for _, e := range b.selection() {
		got = append(got, e.Text)
	}
	want := "Sable waves a paw.|Kit grins.|Rook says, \"The lighthouse is dark.\""
	if strings.Join(got, "|") != want {
		t.Errorf("selection = %q, want %q (page excluded, sent line dropped)", strings.Join(got, "|"), want)
	}
	s := h.screen()
	if !strings.Contains(s, glyphExcluded+" Mira pages") || !strings.Contains(s, glyphSelected+" Sable") {
		t.Errorf("gutter glyphs missing:\n%s", s)
	}

	exportDir := t.TempDir()
	h.m.exportDir = exportDir
	h.keys("e", "h")
	if !strings.Contains(h.screen(), "save as: ") || !strings.Contains(h.screen(), "fm Kit.html") {
		t.Fatalf("filename prompt missing:\n%s", h.screen())
	}
	h.key("enter")
	path := filepath.Join(exportDir, "2026-09-24 2101 fm Kit.html")
	b2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("export not written: %v\n%s", err, h.screen())
	}
	if !strings.Contains(string(b2), "Sable waves a paw.") || strings.Contains(string(b2), "Mira") {
		t.Errorf("export content wrong:\n%s", b2)
	}
	h.keys("e", "h", "enter")
	if !strings.Contains(h.screen(), "exists; pick another name") {
		t.Errorf("overwrite not refused:\n%s", h.screen())
	}
}

func TestBrowseExportNeedsRange(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.key("e")
	if !strings.Contains(h.screen(), "mark a range with m") {
		t.Errorf("screen:\n%s", h.screen())
	}
	if cmd := h.key("c"); cmd != nil {
		t.Error("copy without range should do nothing")
	}
}

func TestBrowseCopy(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.keys("m", "up", "m")
	if cmd := h.key("c"); cmd == nil {
		t.Fatal("copy returned no command")
	}
	if !strings.Contains(h.screen(), "copied 2 lines") {
		t.Errorf("screen:\n%s", h.screen())
	}
}

func TestBrowseChips(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	s := h.screen()
	if !strings.Contains(s, "tags: 1[page] 2[say] 3[self]") {
		t.Fatalf("chips:\n%s", s)
	}
	h.key("1") // page → only
	s = h.screen()
	if !strings.Contains(s, "1[+page]") || strings.Contains(s, "Sable") || !strings.Contains(s, "Mira pages") {
		t.Errorf("only-page filter wrong:\n%s", s)
	}
	h.key("1") // page → hide
	s = h.screen()
	if !strings.Contains(s, "1[−page]") || strings.Contains(s, "Mira pages") || !strings.Contains(s, "Sable") {
		t.Errorf("hide-page filter wrong:\n%s", s)
	}
	h.key("1") // neutral
	if !strings.Contains(h.screen(), "Mira pages") {
		t.Error("neutral chip should show everything")
	}
}

func TestBrowseFind(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.keys("home") // cursor to the oldest line
	h.key("/")
	h.typeText("rook")
	h.key("enter")
	b := h.br()
	if b.cursor.e.Text != scene1[0] || !strings.Contains(h.screen(), "find: rook 1/3") {
		t.Errorf("first match wrong: %q\n%s", b.cursor.e.Text, h.screen())
	}
	h.key("n")
	if b.cursor.e.Text != scene1[5] {
		t.Errorf("n went to %q", b.cursor.e.Text)
	}
	h.key("N")
	if b.cursor.e.Text != scene1[0] {
		t.Errorf("N went to %q", b.cursor.e.Text)
	}
	if !strings.Contains(h.m.View().Content, reverse+"Rook") {
		t.Error("matches not highlighted")
	}
	if !strings.Contains(h.screen(), "Sable") {
		t.Error("find must not hide lines")
	}
}

func TestBrowsePagesOlderDaysAndDateJump(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	for d := 21; d <= 24; d++ {
		lines := make([]string, 150)
		for i := range lines {
			lines[i] = fmt.Sprintf("day %d line %d", d, i)
		}
		h.writeLog(time.Date(2026, 9, d, 8, 0, 0, 0, time.Local), lines...)
	}
	h.key("ctrl+b")
	b := h.br()
	if len(b.lines) != 300 {
		t.Errorf("initially loaded %d lines, want 300 (two days)", len(b.lines))
	}
	if !strings.Contains(h.screen(), "…Wed Sep 23 → today") {
		t.Errorf("header should show more history exists:\n%s", h.screen())
	}
	h.key("g")
	h.typeText("2026-09-21")
	h.key("enter")
	if b.cursor.e.Text != "day 21 line 0" {
		t.Errorf("date jump landed on %q", b.cursor.e.Text)
	}
	if !strings.Contains(h.screen(), "Mon Sep 21 → today") || strings.Contains(h.screen(), "…Mon") {
		t.Errorf("header after loading everything:\n%s", h.screen())
	}
	h.key("g")
	h.typeText("yesterday")
	h.key("enter")
	if !strings.Contains(h.screen(), "dates look like") {
		t.Errorf("bad date not reported:\n%s", h.screen())
	}
}

func TestBrowseMarksSurvivePaging(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(time.Date(2026, 9, 23, 8, 0, 0, 0, time.Local), make([]string, 250)...)
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	b := h.br()
	h.keys("m", "up", "m")
	start, end := b.start, b.end
	h.key("home") // loads everything older
	if b.start != start || b.end != end || len(b.selection()) != 2 {
		t.Error("marks moved when older history loaded")
	}
}

func TestBrowseLiveLinesArrive(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.key("ctrl+b")
	h.conn("fm/kit").lines <- "Brand new line"
	h.settle("fm/kit", func() bool { return strings.Contains(h.screen(), "Brand new line") })
	if h.br().cursor.e.Text != "Brand new line" {
		t.Error("cursor at the bottom should follow live lines")
	}
}

func TestBrowseMouse(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+b")
	h.screen()
	b := h.br()
	l := h.m.layout()
	rowOf := func(text string) int {
		for i, bl := range b.rowLines {
			if bl != nil && bl.e.Text == text {
				return i + 3
			}
		}
		t.Fatalf("%q not on screen", text)
		return 0
	}
	x := l.sw + 1 + 10
	h.m.Update(tea.MouseClickMsg{X: x, Y: rowOf(scene1[1]), Button: tea.MouseLeft})
	if b.cursor.e.Text != scene1[1] {
		t.Errorf("click moved cursor to %q", b.cursor.e.Text)
	}
	h.m.Update(tea.MouseClickMsg{X: x, Y: rowOf(scene1[4]), Button: tea.MouseLeft, Mod: tea.ModShift})
	if b.start == nil || b.end == nil || b.start.e.Text != scene1[1] || b.end.e.Text != scene1[4] {
		t.Fatalf("shift-click range wrong")
	}
	h.screen()
	h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + browsePrefixW - 2, Y: rowOf(scene1[2]), Button: tea.MouseLeft})
	if !b.excluded[b.lines[2]] {
		t.Error("gutter click did not exclude")
	}
	h.m.Update(tea.MouseClickMsg{X: l.sw + 1 + b.chipSpans[0].from, Y: 1, Button: tea.MouseLeft})
	if b.chips["page"] == 0 {
		t.Error("chip click did not cycle")
	}
}

func TestHighlightCommand(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/highlight the lighthouse")
	h.enter()
	if !strings.Contains(h.screen(), `added highlight for "the lighthouse"`) {
		t.Fatalf("screen:\n%s", h.screen())
	}
	h.m.Update(reloadMsg{})
	cs := h.m.chars["fm/kit"]
	text, _ := cs.render(logstore.Entry{Dir: logstore.In, Text: "Rook: The Lighthouse is dark."})
	if !strings.Contains(text, "\x1b[1;38;2;255;209;102m") {
		t.Errorf("new rule not applied: %q", text)
	}
	h.typeText("/highlight")
	h.enter()
	if !strings.Contains(h.screen(), "nothing to highlight") {
		t.Errorf("screen:\n%s", h.screen())
	}
}

func TestBrowseCommandAndSwitching(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/browse")
	h.enter()
	if h.br() == nil || !strings.Contains(h.screen(), "no logs yet") {
		t.Fatalf("screen:\n%s", h.screen())
	}
	h.press(tea.KeyDown, tea.ModCtrl) // switch to Rook: normal view
	if strings.Contains(h.screen(), "BROWSE") {
		t.Error("rook should not be in browse mode")
	}
	h.press(tea.KeyUp, tea.ModCtrl)
	if !strings.Contains(h.screen(), "BROWSE Kit") {
		t.Error("kit's browse state was lost")
	}
	_ = session.Connected
}

func TestPromptWindow(t *testing.T) {
	cases := []struct {
		in       string
		col, w   int
		want     string
		wantCurX int
	}{
		{"short", 5, 20, "short", 5},
		{"/a/very/long/path/Scene.html", 28, 12, "…Scene.html", 11},
		{"/a/very/long/path/Scene.html", 0, 12, "/a/very/long", 0}, // cursor at start: text may fill every cell
		{"日本語日本語", 6, 7, "…本語", 5},                                 // "…" + 4 cells + the cursor's cell = 6 ≤ 7
	}
	for _, c := range cases {
		got, x := promptWindow([]rune(c.in), c.col, c.w)
		if got != c.want || x != c.wantCurX {
			t.Errorf("promptWindow(%q, %d, %d) = %q, %d; want %q, %d", c.in, c.col, c.w, got, x, c.want, c.wantCurX)
		}
	}
}
```

`internal/ui/model_test.go`:

```go
package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
)

// testConn is a scripted server connection.
type testConn struct {
	lines     chan string
	prompts   chan string
	mu        sync.Mutex
	sent      []string
	closeOnce sync.Once
}

func newTestConn() *testConn {
	return &testConn{lines: make(chan string, 100), prompts: make(chan string, 1)}
}
func (c *testConn) Lines() <-chan string   { return c.lines }
func (c *testConn) Prompts() <-chan string { return c.prompts }
func (c *testConn) Err() error             { return nil }
func (c *testConn) Close() error           { c.closeOnce.Do(func() { close(c.lines) }); return nil }
func (c *testConn) Send(l string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, l)
	return nil
}
func (c *testConn) Sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.sent...)
}

type memLog struct{}

func (memLog) Append(logstore.Entry) error { return nil }

type harness struct {
	t       *testing.T
	m       *Model
	dir     string
	mu      sync.Mutex
	conns   map[string]*testConn
	dialErr map[string]error
	saved   map[string]string
	pw      map[string]string
}

const fmWorld = `host = "muck.test"
port = 8888
tls = true
use = ["fuzzball"]
login = "connect {name} {password}"
max_line_bytes = 20

[characters.kit]
name = "Kit"
autoconnect = true

[characters.rook]
name = "Rook"
`

func newHarness(t *testing.T, worlds map[string]string) *harness {
	t.Helper()
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	for name, body := range worlds {
		os.WriteFile(filepath.Join(dir, "worlds", name+".toml"), []byte(body), 0o600)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, dir: dir, conns: map[string]*testConn{}, dialErr: map[string]error{},
		saved: map[string]string{}, pw: map[string]string{"fm/kit": "hunter2", "fm/rook": "pw"}}
	d := Deps{
		ConfigDir:  dir,
		LogRoot:    filepath.Join(dir, "logs"),
		KnownHosts: conn.KnownHosts{Path: filepath.Join(dir, "known_hosts")},
		Load:       config.Load,
		Dial: func(ctx context.Context, ch config.Character) (session.LineConn, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			k := key(ch.World, ch.ID)
			if err := h.dialErr[k]; err != nil {
				delete(h.dialErr, k)
				return nil, err
			}
			c := newTestConn()
			h.conns[k] = c
			return c, nil
		},
		NewLog: func(world, char string) session.Appender { return memLog{} },
		Password: func(world, char string) (string, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			if p, ok := h.pw[key(world, char)]; ok {
				return p, nil
			}
			return "", errors.New("not found")
		},
		SavePassword: func(world, char, pw string) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.saved[key(world, char)] = pw
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local) },
	}
	h.m = New(d, cfg)
	h.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return h
}

func (h *harness) conn(k string) *testConn {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conns[k]
}

// settle feeds session events for k into the model until pred holds.
func (h *harness) settle(k string, pred func() bool) {
	h.t.Helper()
	deadline := time.After(3 * time.Second)
	for !pred() {
		cs := h.m.chars[k]
		select {
		case ev, ok := <-cs.sess.Events():
			h.m.Update(eventMsg{key: k, sess: cs.sess, ev: ev, ok: ok})
		case <-deadline:
			h.t.Fatalf("timed out; screen:\n%s", h.screen())
		}
	}
}

// connected waits for the Connected state and, when the character has a
// saved password, for the auto-login line (sent just after the state
// change) so tests never race it.
func (h *harness) connected(k string) func() bool {
	return func() bool {
		if h.m.chars[k].state != session.Connected {
			return false
		}
		h.mu.Lock()
		_, hasPW := h.pw[k]
		h.mu.Unlock()
		c := h.conn(k)
		return !hasPW || (c != nil && len(c.Sent()) > 0)
	}
}

func (h *harness) screen() string { return ansi.Strip(h.m.View().Content) }

func (h *harness) typeText(s string) {
	for _, r := range s {
		h.m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func (h *harness) press(code rune, mod tea.KeyMod) tea.Cmd {
	_, cmd := h.m.Update(tea.KeyPressMsg{Code: code, Mod: mod})
	return cmd
}

func (h *harness) enter() tea.Cmd { return h.press(tea.KeyEnter, 0) }

func (h *harness) init() {
	h.m.Init() // starts autoconnect sessions; the returned cmds (clock, waits) are not run
}

func TestLayoutShowsSidebarAndStatus(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	s := h.screen()
	for _, want := range []string{"▾ fm", "✕ Kit", "✕ Rook", "fm/Kit 🔒 · disconnected · 21:14", "> "} {
		if !strings.Contains(s, want) {
			t.Errorf("screen missing %q:\n%s", want, s)
		}
	}
	if rows := strings.Split(h.m.View().Content, "\n"); len(rows) != 24 {
		t.Errorf("screen has %d rows, want 24", len(rows))
	}
}

func TestTooSmall(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(tea.WindowSizeMsg{Width: 20, Height: 5})
	if !strings.Contains(h.screen(), "Kiln needs at least") {
		t.Errorf("screen = %q", h.screen())
	}
}

func TestAutoconnectOnlyFlaggedCharacters(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	if h.m.chars["fm/rook"].sess != nil {
		t.Error("rook connected without autoconnect")
	}
	if got := h.conn("fm/kit").Sent(); len(got) != 1 || got[0] != "connect Kit hunter2" {
		t.Errorf("auto-login sent %q", got)
	}
	if s := h.screen(); !strings.Contains(s, "○ Kit") || !strings.Contains(s, "connected to muck.test:8888") {
		t.Errorf("screen:\n%s", s)
	}
}

func TestIncomingLinesHighlightAndBadges(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.press(tea.KeyDown, tea.ModCtrl) // look at rook; kit is now in the background
	c := h.conn("fm/kit")
	c.lines <- "Rook says, \"hi\""
	c.lines <- "Mira pages: \x1b]0;pwned\x07you around?"
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].unread == 2 })
	if !h.m.chars["fm/kit"].attention {
		t.Error("page did not set attention")
	}
	if s := h.screen(); !strings.Contains(s, "● Kit        2") {
		t.Errorf("sidebar badge missing:\n%s", s)
	}
	h.press(tea.KeyUp, tea.ModCtrl)
	s := h.screen()
	if strings.Contains(s, "● Kit") || !strings.Contains(s, "○ Kit") {
		t.Errorf("switching back should clear badges:\n%s", s)
	}
	if !strings.Contains(s, "Mira pages: you around?") || strings.Contains(h.m.View().Content, "pwned") {
		t.Errorf("page not shown sanitized:\n%s", s)
	}
	if !strings.Contains(h.m.View().Content, "\x1b[1;38;2;255;159;67mMira pages") {
		t.Error("page not styled by the fuzzball pack")
	}
}

func TestSendBatchAndFlatten(t *testing.T) {
	flat := strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nnewline_mode = \"flatten\"", 1)
	for _, c := range []struct {
		world string
		want  []string
	}{{fmWorld, []string{"connect Kit hunter2", ":waves.", "say hi"}}, {flat, []string{"connect Kit hunter2", ":waves. say hi"}}} {
		h := newHarness(t, map[string]string{"fm": c.world})
		h.init()
		h.settle("fm/kit", h.connected("fm/kit"))
		h.typeText(":waves.")
		h.press(tea.KeyEnter, tea.ModShift)
		h.typeText("say hi")
		h.enter()
		if got := h.conn("fm/kit").Sent(); strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("sent %q, want %q", got, c.want)
		}
		if !strings.Contains(h.screen(), "> :waves.") {
			t.Errorf("echo missing:\n%s", h.screen())
		}
		if !h.m.chars["fm/kit"].in.Empty() {
			t.Error("input not cleared")
		}
	}
}

func TestOverLimitNeedsConfirm(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld}) // max_line_bytes = 20
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("this line is definitely too long")
	h.enter()
	if n := len(h.conn("fm/kit").Sent()); n != 1 {
		t.Fatalf("sent %d lines before confirm", n)
	}
	if !strings.Contains(h.screen(), "over 20 bytes") {
		t.Errorf("no warning:\n%s", h.screen())
	}
	h.enter()
	if n := len(h.conn("fm/kit").Sent()); n != 2 {
		t.Errorf("confirm Enter did not send")
	}
}

func TestSlashCommandsAndEscape(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("//me waves")
	h.enter()
	if got := h.conn("fm/kit").Sent(); got[len(got)-1] != "/me waves" {
		t.Errorf("sent %q", got)
	}
	h.typeText("/bogus")
	h.enter()
	if !strings.Contains(h.screen(), "unknown command /bogus") {
		t.Errorf("screen:\n%s", h.screen())
	}
	h.typeText("/disconnect")
	h.enter()
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].state == session.Disconnected })
	h.typeText("/connect")
	h.enter()
	h.settle("fm/kit", h.connected("fm/kit"))
}

func TestNotConnectedStatus(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("hello")
	h.enter()
	if !strings.Contains(h.screen(), "Kit is not connected (/connect)") {
		t.Errorf("screen:\n%s", h.screen())
	}
	if h.m.chars["fm/kit"].in.Value() != "hello" {
		t.Error("input lost")
	}
}

func TestPasswordPromptAndSave(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	delete(h.pw, "fm/kit")
	h.init()
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].needPW })
	h.typeText("s3cret")
	s := h.screen()
	if !strings.Contains(s, "> ••••••") || strings.Contains(s, "s3cret") {
		t.Errorf("password not masked:\n%s", s)
	}
	h.enter()
	if got := h.conn("fm/kit").Sent(); len(got) != 1 || got[0] != "connect Kit s3cret" {
		t.Errorf("sent %q", got)
	}
	if !strings.Contains(h.screen(), "connect Kit ***") {
		t.Errorf("redacted echo missing:\n%s", h.screen())
	}
	h.m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if h.saved["fm/kit"] != "s3cret" {
		t.Errorf("saved = %q", h.saved)
	}
	if len(h.m.chars["fm/kit"].in.history) != 0 {
		t.Error("password leaked into input history")
	}
}

func TestPromptShown(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.conn("fm/kit").prompts <- "Name: "
	h.settle("fm/kit", func() bool { return strings.Contains(h.screen(), "Name: ") })
}

func TestPreloadsHistory(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "logs")
	w := logstore.NewWriter(root, "fm", "kit")
	ts := time.Date(2026, 9, 23, 20, 0, 0, 0, time.Local)
	w.Append(logstore.Entry{Time: ts, Dir: logstore.In, Text: "yesterday's news"})
	w.Close()
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.d.LogRoot = root
	h.m.preload(h.m.chars["fm/kit"])
	s := h.screen()
	if !strings.Contains(s, "yesterday's news") || !strings.Contains(s, "history ends Wed Sep 23 20:00") {
		t.Errorf("screen:\n%s", s)
	}
}

func TestReloadAddsCharactersAndKeepsOldOnError(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	p := filepath.Join(h.dir, "worlds", "fm.toml")
	os.WriteFile(p, []byte(fmWorld+"\n[characters.ash]\nname = \"Ash\"\n"), 0o600)
	h.m.Update(reloadMsg{})
	if !strings.Contains(h.screen(), "Ash") {
		t.Errorf("new character missing:\n%s", h.screen())
	}
	os.WriteFile(p, []byte("host = \n"), 0o600)
	h.m.Update(reloadMsg{})
	s := h.screen()
	if !strings.Contains(s, "config not reloaded") || !strings.Contains(s, "Ash") {
		t.Errorf("screen:\n%s", s)
	}
}

func TestScrollPill(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	c := h.conn("fm/kit")
	for i := 0; i < 60; i++ {
		c.lines <- "filler"
	}
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].sb.Len() >= 61 })
	h.press(tea.KeyPgUp, 0)
	if !strings.Contains(h.screen(), "▼ more") {
		t.Errorf("no pill:\n%s", h.screen())
	}
	c.lines <- "fresh"
	h.settle("fm/kit", func() bool {
		sb := &h.m.chars["fm/kit"].sb
		return strings.Contains(sb.lines[sb.Len()-1].text, "fresh")
	})
	if !strings.Contains(h.screen(), " new ") {
		t.Errorf("pill should count new lines:\n%s", h.screen())
	}
	l := h.m.layout()
	h.m.Update(tea.MouseClickMsg{X: 79, Y: l.sbH - 1, Button: tea.MouseLeft})
	if h.m.chars["fm/kit"].sb.Scrolled() || !strings.Contains(h.screen(), "fresh") {
		t.Errorf("pill click did not jump to live:\n%s", h.screen())
	}
}

func TestSidebarClick(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(tea.MouseClickMsg{X: 3, Y: 2, Button: tea.MouseLeft}) // row 2 = Rook
	if h.m.active != "fm/rook" {
		t.Errorf("active = %q", h.m.active)
	}
	h.m.Update(tea.MouseClickMsg{X: 3, Y: 0, Button: tea.MouseLeft}) // collapse fm
	if s := h.screen(); !strings.Contains(s, "▸ fm") || strings.Contains(s, "Kit") && strings.Contains(s, "✕ Kit") {
		t.Errorf("collapse failed:\n%s", s)
	}
}

func TestTrustAfterPinMismatch(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.dialErr["fm/kit"] = &conn.PinMismatchError{HostPort: "muck.test:8888", Pinned: "sha256:aa", Got: "sha256:bb"}
	h.init()
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].pin != nil })
	if !strings.Contains(h.screen(), "/trust to accept") {
		t.Errorf("screen:\n%s", h.screen())
	}
	h.typeText("/trust")
	h.enter()
	h.settle("fm/kit", h.connected("fm/kit"))
	if fp, ok, _ := h.m.d.KnownHosts.Lookup("muck.test:8888"); !ok || fp != "sha256:bb" {
		t.Errorf("pin = %q %v", fp, ok)
	}
}

func TestCtrlCClearsThenQuits(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("oops")
	if cmd := h.press('c', tea.ModCtrl); cmd != nil {
		t.Error("ctrl+c with text should not quit")
	}
	if !h.m.chars["fm/kit"].in.Empty() {
		t.Error("ctrl+c did not clear")
	}
	cmd := h.press('c', tea.ModCtrl)
	if cmd == nil {
		t.Fatal("ctrl+c on empty input should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("not a quit")
	}
}

func TestPasteInsertsMultiline(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(tea.PasteMsg{Content: "one\ntwo"})
	if got := h.m.chars["fm/kit"].in.Value(); got != "one\ntwo" {
		t.Errorf("input = %q", got)
	}
}

func TestSavePasswordAnswerBindsToPromptingCharacter(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	delete(h.pw, "fm/kit")
	h.init()
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].needPW })
	h.typeText("s3cret")
	h.enter()
	h.m.Update(tea.MouseClickMsg{X: 3, Y: 2, Button: tea.MouseLeft}) // click Rook mid-question
	h.m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if h.saved["fm/kit"] != "s3cret" || h.saved["fm/rook"] != "" {
		t.Errorf("saved = %q, want only fm/kit", h.saved)
	}
}

func TestTypedPasswordNotInHistory(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("connect Kit hunter2")
	h.enter()
	h.typeText("say hi")
	h.enter()
	h.press(tea.KeyUp, 0)
	h.press(tea.KeyUp, 0)
	if v := h.m.chars["fm/kit"].in.Value(); strings.Contains(v, "hunter2") {
		t.Errorf("history recalled %q", v)
	}
	if strings.Contains(h.screen(), "hunter2") {
		t.Errorf("password on screen:\n%s", h.screen())
	}
}

func TestConnectWhenAlreadyConnected(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("/connect")
	h.enter()
	if !strings.Contains(h.screen(), "already connected") {
		t.Errorf("screen:\n%s", h.screen())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -race ./internal/ui/`
Expected: FAIL with build errors such as `h.m.exportDir undefined` and `undefined: browse`.

- [ ] **Step 3: Write the implementation** (full file contents)

`internal/ui/keys.go`:

```go
package ui

// Browse-mode key bindings and glyphs live here so they can be retuned in
// one place. Keys are tea.KeyPressMsg.String() values.

type browseAction int

const (
	actNone browseAction = iota
	actBack
	actUp
	actDown
	actPageUp
	actPageDown
	actTop
	actBottom
	actMark
	actExclude
	actFind
	actNextMatch
	actPrevMatch
	actDate
	actExport
	actCopy
)

// openBrowseKey opens browse mode from the normal view.
const openBrowseKey = "ctrl+b"

var browseKeys = map[string]browseAction{
	"esc":    actBack,
	"ctrl+c": actBack,
	"up":     actUp,
	"k":      actUp,
	"down":   actDown,
	"j":      actDown,
	"pgup":   actPageUp,
	"pgdown": actPageDown,
	"home":   actTop,
	"end":    actBottom,
	"m":      actMark,
	"space":  actExclude,
	" ":      actExclude,
	"/":      actFind,
	"n":      actNextMatch,
	"N":      actPrevMatch,
	"g":      actDate,
	"e":      actExport,
	"c":      actCopy,
}

// Export format keys, pressed after actExport.
var exportFormatKeys = map[string]string{"p": "plain", "a": "ansi", "h": "html"}

// Browse glyphs.
const (
	glyphSelected = "▌"
	glyphExcluded = "░"
	glyphChipOnly = "+"
	glyphChipHide = "−"
	browseHints   = "m mark · space exclude · / find · n/N · g date · 1-9 tags · e export · c copy · esc back"
)
```

`internal/ui/browse.go`:

```go
package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/history"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/style"
)

// browseInitialLines is how much history browse loads up front (whole
// days, newest first, until at least this many lines).
const browseInitialLines = 200

// browsePrefixW is the width of "HH:MM" + space + gutter + space.
const browsePrefixW = 8

type bline struct {
	e    logstore.Entry
	tags []string
	text string // sanitized and styled for display
	day  string // local "2006-01-02"
}

type promptKind int

const (
	promptNone promptKind = iota
	promptFind
	promptDate
	promptFormat
	promptFilename
)

// browse is one character's browse-mode state. Lines are referenced by
// pointer so paging in older history never disturbs marks or exclusions.
type browse struct {
	cs        *charState
	hist      *history.Reader
	lines     []*bline // oldest first
	cursor    *bline
	top       *bline // first line drawn at the top of the body
	start     *bline
	end       *bline
	excluded  map[*bline]bool
	chips     map[string]scene.Chip
	find      string
	prompt    promptKind
	pin       *Input
	format    string
	status    string
	statusErr bool
	rowLines  []*bline // body row → line (nil for dividers), from the last draw
	chipSpans []chipSpan
}

type chipSpan struct {
	tag      string
	from, to int // columns within the right pane
}

func newBrowse(cs *charState, logRoot string) *browse {
	b := &browse{cs: cs, excluded: map[*bline]bool{}, chips: map[string]scene.Chip{}, pin: NewInput()}
	if logRoot != "" {
		if h, err := history.NewReader(logRoot, cs.ch.World, cs.ch.ID); err == nil {
			b.hist = h
		}
	}
	for len(b.lines) < browseInitialLines && b.loadOlder() {
	}
	b.cursor = b.last()
	return b
}

func (b *browse) newLine(e logstore.Entry) *bline {
	text, _ := b.cs.render(e)
	plain := ansi.Strip(ansi.Sanitize(e.Text))
	var tags []string
	if e.Dir == logstore.In {
		tags = b.cs.cls.Classify(plain)
	}
	return &bline{e: e, tags: tags, text: text, day: e.Time.Local().Format("2006-01-02")}
}

// loadOlder prepends the next older day. It reports false when there is
// nothing more to load.
func (b *browse) loadOlder() bool {
	if b.hist == nil || b.hist.Exhausted() {
		return false
	}
	es, _, ok, err := b.hist.LoadOlder()
	if err != nil {
		b.setStatus(true, "reading logs: %v", err)
	}
	if !ok {
		return false
	}
	older := make([]*bline, 0, len(es))
	for _, e := range es {
		older = append(older, b.newLine(e))
	}
	b.lines = append(older, b.lines...)
	return true
}

// appendLive adds a line that arrived while browsing. A line that is
// already the newest loaded one (logged just before browse opened) is
// skipped.
func (b *browse) appendLive(e logstore.Entry) {
	if l := b.last(); l != nil && l.e.Time.Equal(e.Time) && l.e.Text == e.Text {
		return
	}
	atEnd := b.cursor == nil || b.cursor == b.lastVisible()
	b.lines = append(b.lines, b.newLine(e))
	if atEnd {
		b.cursor = b.lastVisible()
	}
}

func (b *browse) last() *bline {
	if len(b.lines) == 0 {
		return nil
	}
	return b.lines[len(b.lines)-1]
}

func (b *browse) setStatus(isErr bool, format string, args ...any) {
	b.status, b.statusErr = fmt.Sprintf(format, args...), isErr
}

func (b *browse) visible() []*bline {
	out := make([]*bline, 0, len(b.lines))
	for _, l := range b.lines {
		if scene.Visible(l.tags, b.chips) {
			out = append(out, l)
		}
	}
	return out
}

func (b *browse) lastVisible() *bline {
	v := b.visible()
	if len(v) == 0 {
		return nil
	}
	return v[len(v)-1]
}

func (b *browse) index(l *bline) int { return slices.Index(b.lines, l) }

// inRange reports whether l is inside the marked range. With only a
// start marked, just the start counts.
func (b *browse) inRange(l *bline) bool {
	if b.start == nil {
		return false
	}
	if b.end == nil {
		return l == b.start
	}
	i := b.index(l)
	return i >= b.index(b.start) && i <= b.index(b.end)
}

// tagList is every tag in the loaded lines, sorted, for the chip row.
func (b *browse) tagList() []string {
	seen := map[string]bool{}
	for _, l := range b.lines {
		for _, t := range l.tags {
			seen[t] = true
		}
	}
	tags := make([]string, 0, len(seen))
	for t := range seen {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

// moveCursor moves by delta visible lines, paging in older history when
// moving up past the oldest loaded line.
func (b *browse) moveCursor(delta int) {
	v := b.visible()
	if len(v) == 0 {
		return
	}
	i := slices.Index(v, b.cursor)
	if i < 0 {
		i = len(v) - 1
	}
	for i+delta < 0 && b.loadOlder() {
		nv := b.visible()
		i += len(nv) - len(v)
		v = nv
	}
	i = min(max(0, i+delta), len(v)-1)
	b.cursor = v[i]
}

func (b *browse) mark() {
	if b.cursor == nil {
		return
	}
	if b.start == nil || b.end != nil {
		b.start, b.end = b.cursor, nil
		b.setStatus(false, "range start marked; m again at the end")
		return
	}
	b.end = b.cursor
	if b.index(b.end) < b.index(b.start) {
		b.start, b.end = b.end, b.start
	}
	b.setStatus(false, "%d lines in range", b.index(b.end)-b.index(b.start)+1)
}

func (b *browse) toggleExclude(l *bline) {
	if l == nil || b.end == nil || !b.inRange(l) {
		b.setStatus(true, "exclude works inside a marked range")
		return
	}
	b.excluded[l] = !b.excluded[l]
}

func (b *browse) matches() []*bline {
	if b.find == "" {
		return nil
	}
	needle := strings.ToLower(b.find)
	var out []*bline
	for _, l := range b.visible() {
		if strings.Contains(strings.ToLower(ansi.Strip(l.text)), needle) {
			out = append(out, l)
		}
	}
	return out
}

// jumpMatch moves to the next (dir=1) or previous (dir=-1) match,
// wrapping around. With from=true the cursor's own line counts.
func (b *browse) jumpMatch(dir int, includeCursor bool) {
	ms := b.matches()
	if len(ms) == 0 {
		b.setStatus(true, "no matches for %q", b.find)
		return
	}
	ci := b.index(b.cursor)
	pick := -1
	if dir > 0 {
		for k, m := range ms {
			if i := b.index(m); i > ci || (includeCursor && i == ci) {
				pick = k
				break
			}
		}
		if pick < 0 {
			pick = 0
		}
	} else {
		for k := len(ms) - 1; k >= 0; k-- {
			if b.index(ms[k]) < ci {
				pick = k
				break
			}
		}
		if pick < 0 {
			pick = len(ms) - 1
		}
	}
	b.cursor = ms[pick]
	b.status = ""
}

func (b *browse) matchPos() (int, int) {
	ms := b.matches()
	return slices.Index(ms, b.cursor) + 1, len(ms)
}

// gotoDate loads history back to day and moves to its first visible line.
func (b *browse) gotoDate(day string) {
	if _, err := time.Parse("2006-01-02", day); err != nil {
		b.setStatus(true, "dates look like 2026-09-24")
		return
	}
	for (len(b.lines) == 0 || b.lines[0].day > day) && b.loadOlder() {
	}
	for _, l := range b.visible() {
		if l.day >= day {
			b.cursor = l
			b.top = l
			b.status = ""
			return
		}
	}
	b.setStatus(true, "no logs on or after %s", day)
}

// selection is what an export contains: received lines inside the range,
// not excluded and not hidden by chips.
func (b *browse) selection() []logstore.Entry {
	if b.start == nil || b.end == nil {
		return nil
	}
	var out []logstore.Entry
	for _, l := range b.lines[b.index(b.start) : b.index(b.end)+1] {
		if !b.excluded[l] && scene.Visible(l.tags, b.chips) && scene.Exportable(l.e) {
			out = append(out, l.e)
		}
	}
	return out
}

func (b *browse) title() string {
	ch := b.cs.ch
	when := ""
	if b.start != nil {
		when = " — " + b.start.e.Time.Local().Format("Mon Jan 2 2006")
	}
	return ch.World + " " + ch.Name + when
}

// save writes the export to path, refusing to overwrite.
func (b *browse) save(path string) {
	sel := b.selection()
	if len(sel) == 0 {
		b.setStatus(true, "nothing to export")
		return
	}
	if _, err := os.Stat(path); err == nil {
		b.setStatus(true, "%s exists; pick another name", filepath.Base(path))
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		b.setStatus(true, "%v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.setStatus(true, "%v", err)
		return
	}
	if err := os.WriteFile(path, []byte(scene.Render(b.format, sel, b.title())), 0o644); err != nil {
		b.setStatus(true, "%v", err)
		return
	}
	b.setStatus(false, "saved %s", path)
}

// key handles a key press in browse mode. It returns (cmd, close).
func (b *browse) key(k tea.KeyPressMsg, exportDir string, pageH int) (tea.Cmd, bool) {
	s := k.String()
	if b.prompt != promptNone {
		return b.promptKey(k, exportDir), false
	}
	if len(s) == 1 && s >= "1" && s <= "9" {
		tags := b.tagList()
		if i := int(s[0] - '1'); i < len(tags) {
			b.cycleChip(tags[i])
		}
		return nil, false
	}
	b.status = ""
	switch browseKeys[s] {
	case actBack:
		return nil, true
	case actUp:
		b.moveCursor(-1)
	case actDown:
		b.moveCursor(1)
	case actPageUp:
		b.moveCursor(-max(1, pageH-1))
	case actPageDown:
		b.moveCursor(max(1, pageH-1))
	case actTop:
		for b.loadOlder() {
		}
		if v := b.visible(); len(v) > 0 {
			b.cursor = v[0]
		}
	case actBottom:
		b.cursor = b.lastVisible()
	case actMark:
		b.mark()
	case actExclude:
		b.toggleExclude(b.cursor)
	case actFind:
		b.prompt = promptFind
		b.pin.SetValue(b.find)
	case actNextMatch:
		b.jumpMatch(1, false)
	case actPrevMatch:
		b.jumpMatch(-1, false)
	case actDate:
		b.prompt = promptDate
		b.pin.SetValue("")
	case actExport:
		if len(b.selection()) == 0 {
			b.setStatus(true, "mark a range with m (only received lines export)")
			break
		}
		b.prompt = promptFormat
	case actCopy:
		sel := b.selection()
		if len(sel) == 0 {
			b.setStatus(true, "mark a range with m (only received lines export)")
			break
		}
		b.setStatus(false, "copied %d lines", len(sel))
		return tea.SetClipboard(scene.Plain(sel)), false
	}
	return nil, false
}

func (b *browse) cycleChip(tag string) {
	b.chips[tag] = b.chips[tag].Next()
	if b.cursor != nil && !scene.Visible(b.cursor.tags, b.chips) {
		b.moveCursor(0)
		if v := b.visible(); len(v) > 0 && !slices.Contains(v, b.cursor) {
			b.cursor = v[len(v)-1]
		}
	}
}

func (b *browse) promptKey(k tea.KeyPressMsg, exportDir string) tea.Cmd {
	s := k.String()
	if s == "esc" {
		b.prompt = promptNone
		return nil
	}
	if b.prompt == promptFormat {
		if f, ok := exportFormatKeys[s]; ok {
			b.format = f
			b.prompt = promptFilename
			first := b.selection()[0]
			b.pin.SetValue(scene.FileName(exportDir, first.Time.Local(), b.cs.ch.World, b.cs.ch.Name, f))
		}
		return nil
	}
	switch s {
	case "enter":
		v := strings.TrimSpace(b.pin.Value())
		kind := b.prompt
		b.prompt = promptNone
		switch kind {
		case promptFind:
			b.find = v
			if v != "" {
				b.jumpMatch(1, true)
			}
		case promptDate:
			b.gotoDate(v)
		case promptFilename:
			b.save(v)
		}
	case "backspace":
		b.pin.Backspace()
	case "left":
		b.pin.Left()
	case "right":
		b.pin.Right()
	case "home", "ctrl+a":
		b.pin.Home()
	case "end", "ctrl+e":
		b.pin.End()
	default:
		if k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			b.pin.InsertText(k.Text)
		}
	}
	return nil
}

// promptText is the action bar's prompt label.
func (b *browse) promptLabel() string {
	switch b.prompt {
	case promptFind:
		return "find: "
	case promptDate:
		return "go to date (YYYY-MM-DD): "
	case promptFormat:
		return "export as (p)lain · (a)nsi · (h)tml   esc cancel"
	case promptFilename:
		return "save as: "
	}
	return ""
}

// rowsFor is how many body rows a line takes, including a day divider.
func (b *browse) rowsFor(l *bline, prev *bline, w int) int {
	n := len(ansi.Wrap(l.text, max(1, w-browsePrefixW)))
	if prev == nil || prev.day != l.day {
		n++
	}
	return n
}

// scrollToCursor adjusts top so the cursor is within h body rows and no
// blank rows are left below the newest line.
func (b *browse) scrollToCursor(v []*bline, h, w int) {
	if len(v) == 0 {
		b.top = nil
		return
	}
	ci := slices.Index(v, b.cursor)
	if ci < 0 {
		b.cursor, ci = v[len(v)-1], len(v)-1
	}
	rows := func(i int) int {
		var prev *bline
		if i > 0 {
			prev = v[i-1]
		}
		return b.rowsFor(v[i], prev, w)
	}
	ti := slices.Index(v, b.top)
	if ti < 0 || ci < ti {
		ti = ci
	}
	if ci-ti > h { // every line takes at least one row
		ti = ci - h
	}
	for ti < ci {
		n := 0
		for i := ti; i <= ci; i++ {
			n += rows(i)
		}
		if n <= h {
			break
		}
		ti++
	}
	// The earliest top that still fits everything through the last line.
	fill, n := len(v)-1, 0
	for ; fill >= 0; fill-- {
		if n+rows(fill) > h {
			break
		}
		n += rows(fill)
	}
	if fill+1 < ti {
		ti = fill + 1
	}
	b.top = v[ti]
}

// view draws the right pane (w×h) in browse mode. When a prompt is being
// typed, showCur is true and (curX, curY) is the cursor within the pane.
func (b *browse) view(w, h int) (rows []string, curX, curY int, showCur bool) {
	v := b.visible()
	bodyH := max(1, h-5)
	b.scrollToCursor(v, bodyH, w)

	// Header row 1.
	span := "no logs yet"
	if len(b.lines) > 0 {
		span = dayLabel(b.lines[0].day) + " → today"
		if b.hist != nil && !b.hist.Exhausted() {
			span = "…" + span
		}
	}
	head := bold + "BROWSE " + b.cs.ch.Name + style.Reset + " · " + span
	if b.find != "" {
		i, n := b.matchPos()
		head += fmt.Sprintf("   find: %s %d/%d", b.find, i, n)
	}
	// Header row 2: chips.
	chipRow := "tags:"
	b.chipSpans = b.chipSpans[:0]
	for i, t := range b.tagList() {
		label := t
		switch b.chips[t] {
		case scene.Only:
			label = glyphChipOnly + t
		case scene.Hide:
			label = glyphChipHide + t
		}
		chip := fmt.Sprintf(" %d[%s]", i+1, label)
		from := xansi.StringWidth(chipRow) + 1
		chipRow += chip
		b.chipSpans = append(b.chipSpans, chipSpan{tag: t, from: from, to: xansi.StringWidth(chipRow)})
		if b.chips[t] != scene.Neutral {
			chipRow = chipRow[:len(chipRow)-len(chip)] + " " + reverse + chip[1:] + style.Reset
		}
	}
	rows = []string{head, chipRow, style.Dim(strings.Repeat("─", w))}

	// Body.
	b.rowLines = b.rowLines[:0]
	ms := b.matches()
	start := slices.Index(v, b.top)
	for i := max(0, start); i < len(v) && len(b.rowLines) < bodyH; i++ {
		l := v[i]
		if i == 0 || v[i-1].day != l.day {
			rows = append(rows, style.Dim("── "+dayLabel(l.day)+" ──"))
			b.rowLines = append(b.rowLines, nil)
			if len(b.rowLines) >= bodyH {
				break
			}
		}
		ts := l.e.Time.Local().Format("15:04")
		if l == b.cursor {
			ts = reverse + ts + style.Reset
		} else {
			ts = style.Dim(ts)
		}
		gutter := " "
		if b.inRange(l) {
			gutter = glyphSelected
			if b.excluded[l] {
				gutter = glyphExcluded
			}
		}
		text := l.text
		if b.find != "" && slices.Contains(ms, l) {
			text = highlightFind(ansi.Strip(l.text), b.find)
		}
		for j, row := range ansi.Wrap(text, max(1, w-browsePrefixW)) {
			prefix := ts + " " + gutter + " "
			if j > 0 {
				prefix = "      " + gutter + " "
			}
			rows = append(rows, prefix+row)
			b.rowLines = append(b.rowLines, l)
			if len(b.rowLines) >= bodyH {
				break
			}
		}
	}
	for len(rows) < 3+bodyH {
		rows = append(rows, "")
		b.rowLines = append(b.rowLines, nil)
	}

	// Action bar.
	rows = append(rows, style.Dim(strings.Repeat("─", w)))
	switch {
	case b.prompt == promptFormat:
		rows = append(rows, b.promptLabel())
	case b.prompt != promptNone:
		label := b.promptLabel()
		text, x := promptWindow([]rune(b.pin.Value()), b.pin.col, w-xansi.StringWidth(label)-1)
		rows = append(rows, label+text)
		curX, curY, showCur = xansi.StringWidth(label)+x, h-1, true
	case b.status != "":
		msg := b.status
		if b.statusErr {
			msg = red + msg + style.Reset
		}
		rows = append(rows, msg)
	default:
		rows = append(rows, style.Dim(browseHints))
	}
	return rows, curX, curY, showCur
}

// dayLabel formats "2026-09-24" as "Thu Sep 24".
func dayLabel(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.Format("Mon Jan 2")
}

// highlightFind reverses every case-insensitive occurrence of needle.
func highlightFind(plain, needle string) string {
	if needle == "" {
		return plain
	}
	lower, ln := strings.ToLower(plain), strings.ToLower(needle)
	var out strings.Builder
	for {
		i := strings.Index(lower, ln)
		if i < 0 || len(ln) == 0 {
			out.WriteString(plain)
			return out.String()
		}
		out.WriteString(plain[:i] + reverse + plain[i:i+len(ln)] + style.Reset)
		plain, lower = plain[i+len(ln):], lower[i+len(ln):]
	}
}

// click handles a left click at (x, y) within the right pane.
func (b *browse) click(x, y int, shift bool) {
	if y == 1 {
		for _, c := range b.chipSpans {
			if x >= c.from && x < c.to {
				b.cycleChip(c.tag)
			}
		}
		return
	}
	row := y - 3
	if row < 0 || row >= len(b.rowLines) || b.rowLines[row] == nil {
		return
	}
	l := b.rowLines[row]
	switch {
	case shift:
		if b.start == nil {
			b.start = b.cursor
		}
		b.cursor = l
		b.end = nil
		b.mark()
	case x == browsePrefixW-2: // the gutter column
		b.toggleExclude(l)
	default:
		b.cursor = l
	}
}

// promptWindow fits a one-line value into avail cells, scrolling
// horizontally so the cursor (at rune index col) stays visible; hidden
// text on the left is marked with "…". It returns the text to draw and
// the cursor's cell offset within it.
func promptWindow(rs []rune, col, avail int) (string, int) {
	avail = max(2, avail)
	width := func(r []rune) int { return xansi.StringWidth(string(r)) }
	if width(rs) <= avail {
		return string(rs), width(rs[:col])
	}
	start := 0
	for start < col && width(rs[start:col])+1 > avail-1 {
		start++
	}
	prefix := ""
	if start > 0 {
		prefix = "…"
	}
	shown := string(rs[start:])
	shown = xansi.Truncate(shown, avail-xansi.StringWidth(prefix), "")
	return prefix + shown, xansi.StringWidth(prefix) + width(rs[start:col])
}
```

`internal/ui/model.go`:

```go
// Package ui is Kiln's terminal interface: a sidebar of worlds and
// characters, and a right pane with scrollback, input box and statusline.
package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/style"
)

// HistoryLines is how many logged lines are preloaded per character.
const HistoryLines = 200

// Deps are the UI's connections to the outside world. Tests substitute
// fakes; cmd/kiln wires the real ones.
type Deps struct {
	ConfigDir    string
	LogRoot      string
	KnownHosts   conn.KnownHosts
	Load         func(dir string) (*config.Config, error)
	Dial         func(ctx context.Context, ch config.Character) (session.LineConn, error)
	NewLog       func(world, char string) session.Appender
	Password     func(world, char string) (string, error)
	SavePassword func(world, char, password string) error // nil: never offer
	Changes      <-chan struct{}                          // config changes; nil: no hot reload
	Now          func() time.Time
}

type mode int

const (
	modeNormal mode = iota
	modeSavePassword
)

// Model is the Bubble Tea model.
type Model struct {
	d         Deps
	chars     map[string]*charState
	order     []string // sidebar order of character keys
	collapsed map[string]bool
	active    string
	width     int
	height    int
	status    string
	statusErr bool
	mode      mode
	pendingPW string    // entered password awaiting the save y/n answer
	pendingCh [2]string // world and character id the pending password belongs to
	exportDir string
	confirm   bool // next Enter sends an over-limit line anyway
}

type charState struct {
	key       string
	ch        config.Character
	sess      *session.Session
	cancel    context.CancelFunc
	state     session.State
	cls       *classify.Classifier
	hl        *rules.Highlighter
	sb        Scrollback
	in        *Input
	unread    int
	attention bool
	pin       *conn.PinMismatchError
	needPW    bool
	browse    *browse // non-nil while browse mode is open
}

func key(world, char string) string { return world + "/" + char }

// Messages.
type (
	eventMsg struct {
		key  string
		sess *session.Session
		ev   session.Event
		ok   bool
	}
	reloadMsg struct{}
	tickMsg   time.Time
)

// New builds the model from an already-loaded config and preloads each
// character's recent history. Nothing connects until Init.
func New(d Deps, cfg *config.Config) *Model {
	if d.Now == nil {
		d.Now = time.Now
	}
	m := &Model{d: d, chars: map[string]*charState{}, collapsed: map[string]bool{}}
	m.applyConfig(cfg)
	for _, k := range m.order {
		m.preload(m.chars[k])
	}
	if len(m.order) > 0 {
		m.active = m.order[0]
	}
	return m
}

// Init connects autoconnect characters and starts the clock and watcher.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(m.d.Now()), m.watch()}
	for _, k := range m.order {
		if m.chars[k].ch.Autoconnect {
			cmds = append(cmds, m.connect(m.chars[k]))
		}
	}
	return tea.Batch(cmds...)
}

func tick(now time.Time) tea.Cmd {
	next := now.Truncate(time.Minute).Add(time.Minute)
	return tea.Tick(next.Sub(now), func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) watch() tea.Cmd {
	if m.d.Changes == nil {
		return nil
	}
	ch := m.d.Changes
	return func() tea.Msg {
		if _, ok := <-ch; !ok {
			return nil
		}
		return reloadMsg{}
	}
}

func waitEvent(k string, s *session.Session) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-s.Events()
		return eventMsg{key: k, sess: s, ev: ev, ok: ok}
	}
}

// applyConfig adds, updates and removes characters to match cfg.
// Connected characters that vanished from the config stay until they
// disconnect.
func (m *Model) applyConfig(cfg *config.Config) {
	m.exportDir = cfg.ExportDir
	var order []string
	seen := map[string]bool{}
	for _, w := range cfg.Worlds {
		for _, ch := range w.Characters {
			k := key(ch.World, ch.ID)
			seen[k] = true
			order = append(order, k)
			cs, ok := m.chars[k]
			if !ok {
				cs = &charState{key: k, in: NewInput()}
				m.chars[k] = cs
			}
			cs.ch = ch
			if err := cs.compile(); err != nil {
				m.setStatus(true, "%s: %v", k, err)
			}
			if cs.sess != nil {
				cs.sess.SetChar(ch)
			}
		}
	}
	for _, k := range m.order {
		if seen[k] {
			continue
		}
		cs := m.chars[k]
		if cs.sess != nil && cs.state != session.Disconnected && cs.state != session.Failed {
			order = append(order, k) // keep until it disconnects
			continue
		}
		if cs.cancel != nil {
			cs.cancel()
		}
		delete(m.chars, k)
	}
	m.order = order
	if _, ok := m.chars[m.active]; !ok {
		m.active = ""
		if len(order) > 0 {
			m.active = order[0]
		}
	}
}

func (cs *charState) compile() error {
	cls, err := classify.New(cs.ch.Rules.Classify, cs.ch.Name, cs.ch.Aliases)
	if err != nil {
		return err
	}
	hl, err := rules.New(cs.ch.Rules.Highlight)
	if err != nil {
		return err
	}
	cs.cls, cs.hl = cls, hl
	return nil
}

// preload fills the scrollback with the tail of the most recent log days.
func (m *Model) preload(cs *charState) {
	if m.d.LogRoot == "" {
		return
	}
	days, err := logstore.Days(m.d.LogRoot, cs.ch.World, cs.ch.ID)
	if err != nil || len(days) == 0 {
		return
	}
	var entries []logstore.Entry
	for i := len(days) - 1; i >= 0 && len(entries) < HistoryLines; i-- {
		es, err := logstore.ReadDay(m.d.LogRoot, cs.ch.World, cs.ch.ID, days[i])
		if err != nil {
			continue
		}
		entries = append(es, entries...)
	}
	if len(entries) > HistoryLines {
		entries = entries[len(entries)-HistoryLines:]
	}
	for _, e := range entries {
		text, _ := cs.render(e)
		cs.sb.Append(text)
	}
	cs.sb.Append(style.Dim("─── history ends " + entries[len(entries)-1].Time.Format("Mon Jan 2 15:04") + " ───"))
}

// render turns a log entry into a drawable line; the bool reports whether
// a highlight rule asked for attention.
func (cs *charState) render(e logstore.Entry) (string, bool) {
	text := ansi.Sanitize(e.Text)
	switch e.Dir {
	case logstore.Out:
		return style.Dim("> " + text), false
	case logstore.Sys:
		return style.Dim("* " + text), false
	}
	plain := ansi.Strip(text)
	res := cs.hl.Apply(plain, cs.cls.Classify(plain))
	if !res.Styled {
		return text + style.Reset, res.Attention
	}
	return style.Apply(text, res.Style), res.Attention
}

// connect starts (or restarts) a character's session.
func (m *Model) connect(cs *charState) tea.Cmd {
	if cs.sess != nil {
		cs.sess.Reconnect()
		return nil
	}
	var s *session.Session
	s = session.New(session.Options{
		Char: cs.ch,
		Log:  m.d.NewLog(cs.ch.World, cs.ch.ID),
		Dial: func(ctx context.Context) (session.LineConn, error) { return m.d.Dial(ctx, s.Char()) },
		Password: func() (string, error) {
			ch := s.Char()
			return m.d.Password(ch.World, ch.ID)
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cs.sess, cs.cancel = s, cancel
	go s.Run(ctx)
	return waitEvent(cs.key, s)
}

// Update handles one message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		return m, tick(time.Time(msg))
	case reloadMsg:
		cfg, err := m.d.Load(m.d.ConfigDir)
		if err != nil {
			m.setStatus(true, "config not reloaded: %v", err)
		} else {
			m.applyConfig(cfg)
			m.setStatus(false, "config reloaded")
		}
		return m, m.watch()
	case eventMsg:
		return m, m.handleEvent(msg)
	case tea.PasteMsg:
		if cs := m.cur(); cs != nil && m.mode == modeNormal {
			cs.in.InsertText(msg.Content)
			m.confirm = false
		}
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	case tea.MouseWheelMsg:
		m.handleWheel(msg)
	case tea.MouseClickMsg:
		m.handleClick(msg)
	}
	return m, nil
}

func (m *Model) cur() *charState { return m.chars[m.active] }

func (m *Model) setStatus(isErr bool, format string, args ...any) {
	m.status, m.statusErr = fmt.Sprintf(format, args...), isErr
}

func (m *Model) handleEvent(msg eventMsg) tea.Cmd {
	cs, ok := m.chars[msg.key]
	if !ok || cs.sess != msg.sess || !msg.ok {
		return nil // stale session, or it has shut down
	}
	ev := msg.ev
	switch ev.Kind {
	case session.EventLine:
		text, attn := cs.render(ev.Entry)
		cs.sb.Append(text)
		if cs.browse != nil {
			cs.browse.appendLive(ev.Entry)
		}
		if msg.key != m.active && ev.Entry.Dir == logstore.In {
			cs.unread++
			cs.attention = cs.attention || attn
		}
	case session.EventPrompt:
		cs.sb.SetPrompt(ansi.Sanitize(ev.Entry.Text))
	case session.EventState:
		cs.state = ev.State
		var pin *conn.PinMismatchError
		if ev.State == session.Failed && errors.As(ev.Err, &pin) {
			cs.pin = pin
			m.setStatus(true, "%s: certificate changed; /trust to accept", cs.ch.Name)
		}
		if ev.State == session.Connected {
			cs.pin = nil
		}
		if ev.State != session.Connected {
			cs.needPW = false
		}
	case session.EventLogError:
		m.setStatus(true, "%s: log write failed: %v", cs.ch.Name, ev.Err)
	case session.EventNeedPassword:
		cs.needPW = true
		if msg.key == m.active {
			m.setStatus(false, "enter password for %s (Esc to skip)", cs.ch.Name)
		}
	}
	return waitEvent(msg.key, msg.sess)
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	cs := m.cur()
	if m.mode == modeSavePassword {
		switch k.String() {
		case "y", "Y":
			// Save for the character that was asked about, even if another
			// one is active now.
			if err := m.d.SavePassword(m.pendingCh[0], m.pendingCh[1], m.pendingPW); err != nil {
				m.setStatus(true, "keychain: %v", err)
			} else {
				m.setStatus(false, "password saved")
			}
		default:
			m.setStatus(false, "password not saved")
		}
		m.mode, m.pendingPW, m.pendingCh = modeNormal, "", [2]string{}
		return nil
	}
	switch k.String() {
	case "ctrl+up":
		m.switchBy(-1)
		return nil
	case "ctrl+down":
		m.switchBy(1)
		return nil
	}
	if cs != nil && cs.browse != nil {
		cmd, closed := cs.browse.key(k, m.exportDir, m.browseBodyH())
		if closed {
			cs.browse = nil
		}
		return cmd
	}
	switch k.String() {
	case openBrowseKey:
		if cs != nil {
			m.openBrowse(cs)
		}
		return nil
	case "ctrl+c":
		if cs == nil || cs.in.Empty() {
			return m.quit()
		}
		cs.in.Reset()
	case "pgup":
		if cs != nil {
			cs.sb.ScrollUp(max(1, m.layout().sbH-1))
		}
	case "pgdown":
		if cs != nil {
			cs.sb.ScrollDown(max(1, m.layout().sbH-1))
		}
	case "esc":
		m.confirm = false
		if cs != nil && cs.needPW {
			cs.needPW = false
			cs.in.Reset()
			m.setStatus(false, "skipped login")
		}
	case "enter":
		return m.submit()
	}
	if cs == nil {
		return nil
	}
	switch k.String() {
	case "shift+enter", "alt+enter":
		cs.in.Newline()
	case "up":
		cs.in.Up()
	case "down":
		cs.in.Down()
	case "left":
		cs.in.Left()
	case "right":
		cs.in.Right()
	case "home", "ctrl+a":
		cs.in.Home()
	case "end", "ctrl+e":
		cs.in.End()
	case "backspace":
		cs.in.Backspace()
	case "delete":
		cs.in.Delete()
	default:
		if k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			cs.in.InsertText(k.Text)
		} else {
			return nil
		}
	}
	m.confirm = false
	return nil
}

func (m *Model) quit() tea.Cmd {
	for _, cs := range m.chars {
		if cs.cancel != nil {
			cs.cancel()
		}
	}
	return tea.Quit
}

// switchBy moves the active character through the sidebar order.
func (m *Model) switchBy(delta int) {
	if len(m.order) == 0 {
		return
	}
	i := 0
	for j, k := range m.order {
		if k == m.active {
			i = j
		}
	}
	i = (i + delta + len(m.order)) % len(m.order)
	m.switchTo(m.order[i])
}

func (m *Model) switchTo(k string) {
	cs, ok := m.chars[k]
	if !ok {
		return
	}
	m.active, m.confirm = k, false
	cs.unread, cs.attention = 0, false
	delete(m.collapsed, cs.ch.World)
	if cs.needPW {
		m.setStatus(false, "enter password for %s (Esc to skip)", cs.ch.Name)
	}
}

// submit handles Enter: a command, a password, or lines for the server.
func (m *Model) submit() tea.Cmd {
	cs := m.cur()
	if cs == nil {
		return nil
	}
	if cs.needPW {
		pw := cs.in.CommitSecret()
		e, err := cs.sess.Login(pw)
		if err != nil && !isLogErr(err) {
			m.setStatus(true, "login not sent: %v", err)
			return nil
		}
		cs.needPW = false
		cs.sb.Append(style.Dim("> " + e.Text))
		if m.d.SavePassword != nil && pw != "" {
			m.mode, m.pendingPW = modeSavePassword, pw
			m.pendingCh = [2]string{cs.ch.World, cs.ch.ID}
			m.setStatus(false, "save password for %s in the keychain? [y/n]", cs.ch.Name)
		}
		return nil
	}
	m.status = ""
	text := cs.in.Value()
	if strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "//") {
		cs.in.Commit()
		return m.command(cs, strings.Fields(text))
	}
	text = strings.TrimPrefix(text, "/") // "//foo" sends "/foo"
	if cs.sess == nil || cs.state != session.Connected {
		m.setStatus(true, "%s is not connected (/connect)", cs.ch.Name)
		return nil
	}
	if cs.in.OverLimit(cs.ch.MaxLineBytes) && !m.confirm {
		m.confirm = true
		m.setStatus(true, "over %d bytes: Enter again to send anyway", cs.ch.MaxLineBytes)
		return nil
	}
	m.confirm = false
	lines := strings.Split(text, "\n")
	if cs.ch.NewlineMode == "flatten" {
		lines = []string{strings.Join(lines, " ")}
	}
	secret := false
	for _, line := range lines {
		e, err := cs.sess.Send(line)
		if err != nil && !isLogErr(err) {
			m.setStatus(true, "not sent: %v", err)
			break
		}
		if err != nil {
			m.setStatus(true, "%v", err)
		}
		secret = secret || e.Text != line // the session redacted a typed password
		cs.sb.Append(style.Dim("> " + ansi.Sanitize(e.Text)))
	}
	if secret {
		cs.in.CommitSecret()
	} else {
		cs.in.Commit()
	}
	cs.sb.ToBottom()
	return nil
}

func isLogErr(err error) bool {
	var le *session.LogError
	return errors.As(err, &le)
}

func (m *Model) command(cs *charState, args []string) tea.Cmd {
	switch args[0] {
	case "/connect":
		if cs.sess != nil && cs.state == session.Connected {
			m.setStatus(false, "%s is already connected", cs.ch.Name)
			return nil
		}
		return m.connect(cs)
	case "/reconnect":
		return m.connect(cs)
	case "/disconnect":
		if cs.sess != nil {
			cs.sess.Disconnect()
		}
	case "/trust":
		if cs.pin == nil {
			m.setStatus(true, "no changed certificate to trust")
			return nil
		}
		if err := m.d.KnownHosts.Trust(cs.pin.HostPort, cs.pin.Got); err != nil {
			m.setStatus(true, "trust: %v", err)
			return nil
		}
		m.setStatus(false, "trusted new certificate for %s", cs.pin.HostPort)
		cs.pin = nil
		return m.connect(cs)
	case "/quit":
		return m.quit()
	case "/browse":
		m.openBrowse(cs)
	case "/highlight":
		text := strings.Join(args[1:], " ")
		if err := config.AppendHighlight(m.d.ConfigDir, cs.ch.World, text); err != nil {
			m.setStatus(true, "highlight: %v", err)
			return nil
		}
		m.setStatus(false, "added highlight for %q", text)
	default:
		m.setStatus(true, "unknown command %s", args[0])
	}
	return nil
}

// openBrowse opens browse mode for cs.
func (m *Model) openBrowse(cs *charState) {
	cs.browse = newBrowse(cs, m.d.LogRoot)
	m.status = ""
}

// browseBodyH is the number of line rows in browse mode.
func (m *Model) browseBodyH() int { return max(1, m.height-5) }

func (m *Model) handleWheel(msg tea.MouseWheelMsg) {
	l := m.layout()
	cs := m.cur()
	if cs != nil && cs.browse != nil && msg.X > l.sw {
		switch msg.Button {
		case tea.MouseWheelUp:
			cs.browse.moveCursor(-3)
		case tea.MouseWheelDown:
			cs.browse.moveCursor(3)
		}
		return
	}
	if cs == nil || msg.X <= l.sw || msg.Y >= l.sbH {
		return
	}
	switch msg.Button {
	case tea.MouseWheelUp:
		cs.sb.ScrollUp(3)
	case tea.MouseWheelDown:
		cs.sb.ScrollDown(3)
	}
}

func (m *Model) handleClick(msg tea.MouseClickMsg) {
	if msg.Button != tea.MouseLeft {
		return
	}
	l := m.layout()
	if msg.X < l.sw {
		rows := m.sidebarRows()
		if msg.Y < len(rows) {
			r := rows[msg.Y]
			if r.char == "" {
				m.collapsed[r.world] = !m.collapsed[r.world]
			} else {
				m.switchTo(r.char)
			}
		}
		return
	}
	if cs := m.cur(); cs != nil && cs.browse != nil {
		cs.browse.click(msg.X-l.sw-1, msg.Y, msg.Mod&tea.ModShift != 0)
		return
	}
	if cs := m.cur(); cs != nil && cs.sb.Scrolled() && msg.Y == l.sbH-1 && msg.X >= m.width-l.pillW {
		cs.sb.ToBottom()
	}
}
```

`internal/ui/view.go`:

```go
package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/style"
)

// Minimum usable terminal size.
const (
	MinWidth  = 40
	MinHeight = 10
)

const (
	reverse = "\x1b[7m"
	red     = "\x1b[31m"
	bold    = "\x1b[1m"
)

type layout struct {
	sw, rw     int // sidebar width; right pane width
	sbH        int // scrollback rows
	inRows     []string
	curRow     int // cursor row within inRows
	curCol     int
	pillW      int
	masked     bool
	inputLimit int
}

func (m *Model) layout() layout {
	l := layout{sw: min(22, max(12, m.width/5))}
	l.rw = max(1, m.width-l.sw-1)
	cs := m.cur()
	if cs == nil {
		l.inRows = []string{"> "}
		l.curCol = 2
	} else {
		l.masked = cs.needPW
		l.inputLimit = cs.ch.MaxLineBytes
		rows, r, c := cs.in.Render(l.rw, l.inputLimit, l.masked)
		maxIn := max(1, m.height/3)
		top := 0
		if len(rows) > maxIn {
			top = min(max(0, r-maxIn+1), len(rows)-maxIn)
		}
		l.inRows = rows[top:min(len(rows), top+maxIn)]
		l.curRow, l.curCol = r-top, c
	}
	l.sbH = max(1, m.height-len(l.inRows)-3) // two rules + statusline
	if cs != nil && cs.sb.Scrolled() {
		l.pillW = xansi.StringWidth(pillText(cs))
	}
	return l
}

// pillText is the "jump to live" marker shown while scrolled up.
func pillText(cs *charState) string {
	if n := cs.sb.Unseen(); n > 0 {
		return fmt.Sprintf(" ▼ %d new ", n)
	}
	return " ▼ more "
}

type sidebarRow struct {
	world string
	char  string // "" for a world header
}

func (m *Model) sidebarRows() []sidebarRow {
	var rows []sidebarRow
	lastWorld := ""
	for _, k := range m.order {
		cs := m.chars[k]
		if cs.ch.World != lastWorld {
			lastWorld = cs.ch.World
			rows = append(rows, sidebarRow{world: lastWorld})
		}
		if !m.collapsed[lastWorld] {
			rows = append(rows, sidebarRow{world: lastWorld, char: k})
		}
	}
	return rows
}

func (m *Model) sidebarLine(r sidebarRow, w int) string {
	if r.char == "" {
		arrow := "▾ "
		if m.collapsed[r.world] {
			arrow = "▸ "
		}
		return bold + fit(arrow+r.world, w) + style.Reset
	}
	cs := m.chars[r.char]
	badge := "✕"
	switch {
	case cs.attention:
		badge = "●"
	case cs.state == session.Connected:
		badge = "○"
	case cs.state == session.Connecting:
		badge = "…"
	}
	count := ""
	if cs.unread > 0 {
		count = fmt.Sprintf(" %d", cs.unread)
	}
	name := fit("  "+badge+" "+cs.ch.Name, w-xansi.StringWidth(count))
	line := name + count
	if r.char == m.active {
		return reverse + line + style.Reset
	}
	return line
}

// fit truncates or pads s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = xansi.Truncate(s, w, "")
	if pad := w - xansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// statusLine shows the active character, its connection and the clock,
// or, while there is a status message, just the character and the message.
func (m *Model) statusLine(w int) string {
	cs := m.cur()
	name := ""
	if cs != nil {
		name = cs.ch.World + "/" + cs.ch.Name
		if cs.ch.TLS {
			name += " 🔒"
		}
	}
	if m.status != "" {
		msg := m.status
		if m.statusErr {
			msg = red + msg + style.Reset
		}
		if name == "" {
			return fit(msg, w)
		}
		return fit(name+" · "+msg, w)
	}
	parts := []string{}
	if cs != nil {
		parts = append(parts, name, cs.state.String())
	}
	parts = append(parts, m.d.Now().Format("15:04"))
	return fit(strings.Join(parts, " · "), w)
}

// View draws the whole screen.
func (m *Model) View() tea.View {
	v := tea.View{AltScreen: true, MouseMode: tea.MouseModeCellMotion}
	if m.width < MinWidth || m.height < MinHeight {
		v.Content = fmt.Sprintf("Kiln needs at least %dx%d (now %dx%d)", MinWidth, MinHeight, m.width, m.height)
		return v
	}
	l := m.layout()
	right := make([]string, 0, m.height)
	cs := m.cur()
	var cursor *tea.Cursor
	if cs != nil && cs.browse != nil {
		rows, x, y, show := cs.browse.view(l.rw, m.height)
		right = rows
		if show {
			cursor = tea.NewCursor(l.sw+1+x, y)
		}
	} else if cs == nil {
		right = append(right, make([]string, l.sbH)...)
		right[0] = style.Dim("No characters yet: add one in " + m.d.ConfigDir + "/worlds/")
	} else {
		cs.sb.SetWidth(l.rw)
		rows := cs.sb.View(l.sbH)
		if cs.sb.Scrolled() {
			pill := pillText(cs)
			last := len(rows) - 1
			rows[last] = fit(rows[last], l.rw-xansi.StringWidth(pill)) + style.Reset + reverse + pill + style.Reset
		}
		right = append(right, rows...)
	}
	if cs == nil || cs.browse == nil {
		rule := style.Dim(strings.Repeat("─", l.rw))
		right = append(right, rule)
		right = append(right, l.inRows...)
		right = append(right, rule, m.statusLine(l.rw))
		cursor = tea.NewCursor(l.sw+1+l.curCol, l.sbH+1+l.curRow)
	}

	side := m.sidebarRows()
	var b strings.Builder
	for y := 0; y < m.height; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		if y < len(side) {
			b.WriteString(m.sidebarLine(side[y], l.sw))
		} else {
			b.WriteString(strings.Repeat(" ", l.sw))
		}
		b.WriteString(style.Dim("│"))
		if y < len(right) {
			b.WriteString(fit(right[y], l.rw) + style.Reset)
		}
	}
	v.Content = b.String()
	v.Cursor = cursor
	return v
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/ui/ && go test -race ./internal/ui/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Step 5: Whole suite, repeated to shake out races**

Run: `go vet ./... && go test -race -count=5 ./... && go build -o kiln ./cmd/kiln`
Expected: all 14 packages report `ok` on every run, and `kiln` builds.


- [ ] **Commit**

```bash
git add internal/ui/browse_test.go internal/ui/model_test.go internal/ui/keys.go internal/ui/browse.go internal/ui/model.go internal/ui/view.go
git commit -m "feat(ui): browse mode (history paging, marks, exclusions, chips, find, export, copy) and /highlight

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 6: Manual check against a real MUCK

**Files:** none (verification only)

- [ ] **Step 1: Throwaway config with an export dir**

```bash
export XDG_CONFIG_HOME=$(mktemp -d) XDG_DATA_HOME=$(mktemp -d) SCENES=$(mktemp -d)
mkdir -p $XDG_CONFIG_HOME/kiln/worlds
printf 'export_dir = "%s"
' $SCENES > $XDG_CONFIG_HOME/kiln/config.toml
printf 'host = "furrymuck.com"
port = 8899
tls = true
use = ["fuzzball"]
autoconnect = true
[characters.guest]
name = "Guest"
' > $XDG_CONFIG_HOME/kiln/worlds/furrymuck.toml
./kiln
```

- [ ] **Step 2: Check each behavior**

Expected:
- After the banner arrives, `Ctrl+B` shows `BROWSE Guest · <today> → today`, the chip row `tags: 1[self]`, a `── <day> ──` divider, and `HH:MM` timestamps. The cursor's timestamp is in reverse video.
- `1` cycles the chip through `[+self]` (only the "connect guest guest" line) and `[−self]` (that line hidden), then back to neutral.
- `↑` a few lines, `m`, `↑` a few more, `m`: the range shows `▌` in the gutter. `↓`, `space`: that line shows `░`.
- `e`, `h`: `save as: …<date> <time> furrymuck Guest.html` (the path scrolls so the filename is visible). Enter shows `saved …`. The file contains the range minus the excluded line, with HTML-escaped quotes. Exporting again with the same name is refused.
- `c` shows `copied N lines`, and pasting in another app gives the plain text (if your terminal supports OSC 52).
- `/` + `WHO` + Enter jumps to and highlights the match. `g` + today's date + Enter jumps to the day's first line.
- `Esc` returns to the normal view. `/highlight furrymuck` then shows `added highlight for "furrymuck"`, the world file gains a `[[highlight]]` rule, and within a second new lines containing "furrymuck" appear bold yellow.
