// Package scene decides which lines belong to an exported scene and
// renders them as plain text, ANSI or HTML.
package scene

import (
	"fmt"
	"html"
	"path/filepath"
	"strings"
	"time"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/pathfmt"
	"github.com/latrani/Kiln/internal/theme"
)

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
	fg, bg := theme.Active().CSS(theme.Export)
	var body []string
	if bg != "" {
		body = append(body, "background: "+bg) //str:ok
	}
	if fg != "" {
		body = append(body, "color: "+fg) //str:ok
	}
	body = append(body, "margin: 2rem") //str:ok
	fmt.Fprintf(&b, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>%s</title>
<style>
body { %s; }
pre { font: 14px/1.45 ui-monospace, SFMono-Regular, Menlo, monospace; white-space: pre-wrap; }
</style>
</head>
<body>
<pre>
`, html.EscapeString(title), strings.Join(body, "; ")) //str:ok
	for _, e := range entries {
		for _, sp := range ansi.Spans(ansi.Sanitize(e.Text)) {
			if css := spanCSS(sp.Style); css != "" {
				fmt.Fprintf(&b, `<span style="%s">%s</span>`, css, html.EscapeString(sp.Text)) //str:ok
			} else {
				b.WriteString(html.EscapeString(sp.Text))
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("</pre>\n</body>\n</html>\n") //str:ok
	return b.String()
}

func spanCSS(s ansi.SpanStyle) string {
	var parts []string
	if s.FG != "" {
		parts = append(parts, "color:"+s.FG) //str:ok
	}
	if s.BG != "" {
		parts = append(parts, "background:"+s.BG) //str:ok
	}
	if s.Bold {
		parts = append(parts, "font-weight:bold") //str:ok
	}
	if s.Italic {
		parts = append(parts, "font-style:italic") //str:ok
	}
	if s.Underline {
		parts = append(parts, "text-decoration:underline") //str:ok
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

// FileName is the default export path: dir, then the export_name template
// (see config.ExportNameVars; "" for config.DefaultExportName) filled in
// for a scene whose first line is at t, then the format's extension.
func FileName(dir, template string, t time.Time, world, name, format string) string {
	if template == "" {
		template = config.DefaultExportName
	}
	base := pathfmt.Expand(template, map[string]string{
		"date": t.Format("2006-01-02"), "time": t.Format("1504"), "world": world, "name": name,
	}, t)
	return filepath.Join(dir, base+"."+Ext(format))
}
