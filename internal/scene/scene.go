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
