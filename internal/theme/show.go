package theme

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/latrani/Kiln/internal/str"
)

// Show is theme name as one file: a theme that extends nothing as it's
// written, and otherwise the files it's built from merged into one that
// looks the same on either appearance. A theme that doesn't load is an
// error.
func Show(dir, name string) (string, error) {
	if !validName(name) {
		return "", errors.New(str.ThemeBadName(name))
	}
	chain, err := chainFor(dir, name, nil)
	if err != nil {
		return "", err
	}
	for _, ap := range []Appearance{Dark, Light} {
		if _, err := build(chain, ap); err != nil {
			return "", err
		}
	}
	if len(chain) == 1 {
		return string(chain[0].src), nil
	}
	return merged(name, chain), nil
}

// merged writes chain as one theme file.
func merged(name string, chain []file) string {
	palette := map[string]string{}
	appearance := map[Appearance]map[string]string{Dark: {}, Light: {}}
	ui := map[Role]fileStyle{}
	tags := map[string]tagFileStyle{}
	for _, f := range chain {
		// build adds each file's [palette] and then its appearance's, so
		// a later [palette] entry wins over an earlier [palette.light].
		for k, v := range f.palette {
			palette[k] = v
			for _, m := range appearance {
				delete(m, k)
			}
		}
		for ap, entries := range f.appearance {
			maps.Copy(appearance[ap], entries)
		}
		for r, s := range f.ui {
			ui[r] = overlay(ui[r], s)
		}
		for tag, s := range f.tags {
			m := tags[tag]
			m.fileStyle = overlay(m.fileStyle, s.fileStyle)
			if s.scope != nil {
				m.scope = s.scope
			}
			tags[tag] = m
		}
	}

	var b strings.Builder
	for _, line := range strings.Split(str.ThemeShowMerged(name), "\n") {
		fmt.Fprintf(&b, "# %s\n", line) //str:ok: a TOML comment
	}
	table := func(header string, entries map[string]string) {
		if len(entries) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n[%s]\n", header) //str:ok: a TOML table
		keys := slices.Sorted(maps.Keys(entries))
		w := 0
		for _, k := range keys {
			w = max(w, len(tomlKey(k)))
		}
		for _, k := range keys {
			fmt.Fprintf(&b, "%-*s = %s\n", w, tomlKey(k), tomlString(entries[k])) //str:ok: TOML
		}
	}
	table("palette", palette)
	table("palette.dark", appearance[Dark])
	table("palette.light", appearance[Light])
	uiLines := map[string]string{}
	var order []string
	for _, r := range Roles {
		if s, ok := ui[r]; ok {
			k := tomlString(string(r))
			uiLines[k], order = inline(s, nil), append(order, k)
		}
	}
	writeStyles(&b, "ui", order, uiLines)
	tagLines := map[string]string{}
	order = nil
	for _, tag := range slices.Sorted(maps.Keys(tags)) {
		k := tomlString(tag)
		tagLines[k], order = inline(tags[tag].fileStyle, tags[tag].scope), append(order, k)
	}
	writeStyles(&b, "tags", order, tagLines)
	return b.String()
}

// writeStyles writes a [header] table of inline styles, keys in order.
func writeStyles(b *strings.Builder, header string, order []string, lines map[string]string) {
	if len(order) == 0 {
		return
	}
	fmt.Fprintf(b, "\n[%s]\n", header) //str:ok: a TOML table
	w := 0
	for _, k := range order {
		w = max(w, len(k))
	}
	for _, k := range order {
		fmt.Fprintf(b, "%-*s = %s\n", w, k, lines[k]) //str:ok: TOML
	}
}

// inline is s as a TOML inline table, fields in the order the README
// lists them.
func inline(s fileStyle, scope *string) string {
	var fields []string
	for _, c := range []struct {
		key string
		v   *string
	}{{"fg", s.fg}, {"bg", s.bg}} { //str:ok: theme vocabulary
		if c.v != nil {
			fields = append(fields, c.key+" = "+tomlString(*c.v))
		}
	}
	for _, c := range []struct {
		key string
		v   *bool
	}{{"bold", s.bold}, {"faint", s.faint}, {"italic", s.italic}, {"underline", s.underline}, {"reverse", s.reverse}} { //str:ok: theme vocabulary
		if c.v != nil {
			fields = append(fields, fmt.Sprintf("%s = %t", c.key, *c.v)) //str:ok: TOML
		}
	}
	if scope != nil {
		fields = append(fields, "scope = "+tomlString(*scope)) //str:ok: TOML
	}
	if len(fields) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(fields, ", ") + " }"
}

var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`) //str:ok

// tomlKey is k as a TOML key: bare if it can be, else quoted.
func tomlKey(k string) string {
	if bareKey.MatchString(k) {
		return k
	}
	return tomlString(k)
}

// tomlString is s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04x`, r) //str:ok: TOML escape
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// swatchGap is the space between a swatch and its line.
const swatchGap = 2

// Swatches is Show's text for the theme it was made from, with a swatch
// in front of each line that sets a color: a block of it for a palette
// entry, and for a role or tag the word "Sample" drawn in its style.
// Every line moves over by the same amount, so the text lines up. It is
// for a terminal; the escape codes aren't valid TOML, so the text to
// save stays plain.
func Swatches(text string, t *Theme) string {
	sample := str.ThemeSwatchSample()
	gutter := max(utf8.RuneCountInString(sample), blockWidth) + swatchGap
	lines := strings.Split(text, "\n")
	table := ""
	for i, l := range lines {
		if strings.HasPrefix(l, "[") { //str:ok: a TOML table header
			table = strings.Trim(l, "[]")
		}
		if l == "" {
			continue
		}
		sw, w := swatch(table, l, t, sample)
		lines[i] = sw + strings.Repeat(" ", gutter-w) + l
	}
	return strings.Join(lines, "\n")
}

// blockWidth is the width of a palette entry's swatch.
const blockWidth = 3

// swatch is the swatch for one line of table, and its width; "" and 0
// if the line sets no color.
func swatch(table, line string, t *Theme, sample string) (string, int) {
	key, val, ok := strings.Cut(line, " = ")
	if !ok || strings.HasPrefix(line, "#") { //str:ok: a TOML comment
		return "", 0
	}
	key = strings.Trim(key, `" `) //str:ok: TOML quotes
	sampleW := utf8.RuneCountInString(sample)
	switch {
	case table == "palette" || strings.HasPrefix(table, "palette."): //str:ok: TOML tables
		c, ok := resolve(strings.Trim(val, `"`), t.palette) //str:ok: TOML quotes
		if !ok {
			return "", 0
		}
		return "\x1b[" + c.code(true) + "m" + strings.Repeat(" ", blockWidth) + Reset, blockWidth
	case table == "ui": //str:ok: TOML table
		r := Role(key)
		if s, ok := t.styles[r]; !ok || (s.fg == nil && s.bg == nil && !s.reverse) {
			return "", 0
		}
		return t.Paint(r, sample), sampleW
	case table == "tags": //str:ok: TOML table
		if _, ts, ok := t.Tag(key); ok {
			if sgr := ts.Style.SGR(); sgr != "" {
				return sgr + sample + Reset, sampleW
			}
		}
	}
	return "", 0
}
