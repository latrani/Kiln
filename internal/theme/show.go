package theme

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

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
