// Package theme is Kiln's look: a palette of named colors, and the styles
// of the screen's parts (roles), from theme files in TOML. See the spec,
// docs/superpowers/specs/2026-09-28-theming-design.md.
package theme

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/latrani/Kiln/internal/str"
)

// Reset clears all SGR attributes.
const Reset = "\x1b[0m"

// terminalColors are the 16 colors a terminal's own theme sets, by the
// names a theme uses for them, in ANSI order.
//
//str:ok
var terminalColors = []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
	"bright-black", "bright-red", "bright-green", "bright-yellow", "bright-blue", "bright-magenta", "bright-cyan", "bright-white"}

// xterm's defaults for the 16, for the HTML export, which can't ask the
// terminal.
var terminalHex = []string{"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
	"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff"}

// defaultColor is the terminal's own foreground or background, for
// clearing a color a role inherits.
const defaultColor = "default" //str:ok

var hexRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`) //str:ok

// color is a resolved color: a terminal color (ansi 0–15) or a hex one.
type color struct {
	ansi int // -1 for hex
	hex  string
}

func (c color) code(bg bool) string {
	if c.ansi >= 0 {
		base := 30
		if c.ansi >= 8 {
			base = 90 - 8
		}
		if bg {
			base += 10
		}
		return fmt.Sprint(base + c.ansi)
	}
	var r, g, b int
	fmt.Sscanf(c.hex[1:], "%02x%02x%02x", &r, &g, &b)
	lead := "38"
	if bg {
		lead = "48"
	}
	return fmt.Sprintf("%s;2;%d;%d;%d", lead, r, g, b)
}

func (c color) css() string {
	if c.ansi >= 0 {
		return terminalHex[c.ansi]
	}
	return strings.ToLower(c.hex)
}

// fileStyle is a role's style as a file writes it: unset fields are nil,
// so a role can inherit them, and a later file can turn a flag back off.
type fileStyle struct {
	fg, bg                                  *string
	bold, faint, italic, underline, reverse *bool
}

// tagFileStyle is a tag's style as a file writes it.
type tagFileStyle struct {
	fileStyle
	scope *string // "line" or "match"; nil: unset
}

// file is one parsed theme file.
type file struct {
	name    string
	extends string
	palette map[string]string
	ui      map[Role]fileStyle
	tags    map[string]tagFileStyle
}

// style is a role's resolved style.
type style struct {
	fg, bg                                  *color
	bold, faint, italic, underline, reverse bool
}

func (s style) sgr() string {
	var codes []string
	for _, f := range []struct {
		on   bool
		code string
	}{{s.bold, "1"}, {s.faint, "2"}, {s.italic, "3"}, {s.underline, "4"}, {s.reverse, "7"}} {
		if f.on {
			codes = append(codes, f.code)
		}
	}
	if s.fg != nil {
		codes = append(codes, s.fg.code(false))
	}
	if s.bg != nil {
		codes = append(codes, s.bg.code(true))
	}
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// Style is a resolved style, for tag styles, which fold together.
type Style struct{ s style }

// SGR is the escape sequence that starts drawing in s, or "".
func (s Style) SGR() string { return s.s.sgr() }

// Over is s with b on top: b's colors win, attributes add up.
func (s Style) Over(b Style) Style {
	if b.s.fg != nil {
		s.s.fg = b.s.fg
	}
	if b.s.bg != nil {
		s.s.bg = b.s.bg
	}
	s.s.bold = s.s.bold || b.s.bold
	s.s.faint = s.s.faint || b.s.faint
	s.s.italic = s.s.italic || b.s.italic
	s.s.underline = s.s.underline || b.s.underline
	s.s.reverse = s.s.reverse || b.s.reverse
	return s
}

// TagStyle is how lines with a tag are drawn: the whole line, or with
// Match only the text the tag's pattern matched.
type TagStyle struct {
	Style Style
	Match bool
}

// Theme is a loaded theme: every role's and tag's style, resolved.
type Theme struct {
	styles map[Role]style
	sgr    map[Role]string
	tags   map[string]TagStyle
	chain  []file // what it was built from, base first
}

// Layer is a world's or character's own [palette] and [tags], drawn over
// the theme it uses (see Theme.With).
type Layer struct{ f file }

// ParseLayer reads a layer's tables, as a world or character file gives
// them. where names the file for messages. Colors are checked when the
// layer is put on a theme, since they may name the theme's palette.
func ParseLayer(where string, palette, tags map[string]any) (Layer, error) {
	raw := map[string]any{}
	if palette != nil {
		raw["palette"] = palette
	}
	if tags != nil {
		raw["tags"] = tags
	}
	f, err := parseTables(where, raw)
	return Layer{f}, err
}

// With is t with layers on top, in order, merged field by field.
func (t *Theme) With(layers ...Layer) (*Theme, error) {
	if len(layers) == 0 {
		return t, nil
	}
	chain := slices.Clone(t.chain)
	for _, l := range layers {
		chain = append(chain, l.f)
	}
	return build(chain)
}

// Tag is how a line tag is drawn: the style of the most specific styled
// name up its slashes (page/in, then page), and that name.
func (t *Theme) Tag(name string) (styled string, ts TagStyle, ok bool) {
	for n := name; ; {
		if ts, ok := t.tags[n]; ok {
			return n, ts, true
		}
		i := strings.LastIndexByte(n, '/')
		if i < 0 {
			return "", TagStyle{}, false
		}
		n = n[:i]
	}
}

// SGR is the escape sequence that starts drawing in r, or "" when r draws
// plainly.
func (t *Theme) SGR(r Role) string { return t.sgr[r] }

// Paint draws text in r and resets after it, or returns text unchanged
// when r draws plainly.
func (t *Theme) Paint(r Role, text string) string {
	if s := t.sgr[r]; s != "" {
		return s + text + Reset
	}
	return text
}

// CSS is r's colors as CSS hex colors, "" where r sets none.
func (t *Theme) CSS(r Role) (fg, bg string) {
	s := t.styles[r]
	if s.fg != nil {
		fg = s.fg.css()
	}
	if s.bg != nil {
		bg = s.bg.css()
	}
	return fg, bg
}

// parse reads one theme file. name is its path, for messages.
func parse(name string, data []byte) (file, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return file{name: name}, errors.New(str.ThemeParse(name, err))
	}
	return parseTables(name, raw)
}

// parseTables reads a theme's tables, already decoded.
func parseTables(name string, raw map[string]any) (file, error) {
	f := file{name: name, palette: map[string]string{}, ui: map[Role]fileStyle{}, tags: map[string]tagFileStyle{}}
	for k, v := range raw {
		switch k {
		case "extends":
			s, ok := v.(string)
			if !ok {
				return f, errors.New(str.ThemeBadField(name, "extends", "extends", str.ThemeWantTheme()))
			}
			if !validName(s) {
				return f, errors.New(str.ThemeBadExtends(name, s))
			}
			f.extends = s
		case "palette":
			tbl, ok := v.(map[string]any)
			if !ok {
				return f, errors.New(str.ThemeNotTable(name, k))
			}
			for pk, pv := range tbl {
				s, ok := pv.(string)
				if !ok {
					return f, errors.New(str.ThemeBadColor(name, str.ThemePaletteEntry(pk), fmt.Sprint(pv)))
				}
				f.palette[pk] = s
			}
		case "ui":
			tbl, ok := v.(map[string]any)
			if !ok {
				return f, errors.New(str.ThemeNotTable(name, k))
			}
			if err := flatten(name, "", tbl, f.ui); err != nil {
				return f, err
			}
		case "tags":
			tbl, ok := v.(map[string]any)
			if !ok {
				return f, errors.New(str.ThemeNotTable(name, k))
			}
			if err := readTags(name, tbl, f.tags); err != nil {
				return f, err
			}
		default:
			return f, errors.New(str.ThemeUnknownKey(name, k))
		}
	}
	return f, nil
}

// validName reports whether s can name a theme: a bare file name in
// themes/, without .toml.
func validName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`) && !strings.HasSuffix(s, ".toml") //str:ok
}

// readStyle reads a style's fields from tbl. where is the role or tag,
// for messages. Keys that aren't style fields come back in rest, for the
// caller to judge.
func readStyle(name, where string, tbl map[string]any) (s fileStyle, rest map[string]any, err error) {
	flags := map[string]**bool{"bold": &s.bold, "faint": &s.faint, "italic": &s.italic, "underline": &s.underline, "reverse": &s.reverse} //str:ok
	for k, v := range tbl {
		switch k {
		case "fg", "bg":
			c, ok := v.(string)
			if !ok {
				return s, nil, errors.New(str.ThemeBadField(name, where, k, str.ThemeWantColor()))
			}
			if k == "fg" {
				s.fg = &c
			} else {
				s.bg = &c
			}
		case "bold", "faint", "italic", "underline", "reverse":
			b, ok := v.(bool)
			if !ok {
				return s, nil, errors.New(str.ThemeBadField(name, where, k, str.ThemeWantBool()))
			}
			*flags[k] = &b
		default:
			if rest == nil {
				rest = map[string]any{}
			}
			rest[k] = v
		}
	}
	return s, rest, nil
}

// flatten reads [ui], written with quoted dotted keys ("sidebar.active"),
// nested tables ([ui.sidebar.active]), or both, into styles by role.
func flatten(name, prefix string, tbl map[string]any, out map[Role]fileStyle) error {
	fields := map[string]any{}
	for k, v := range tbl {
		role := k
		if prefix != "" {
			role = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			if err := flatten(name, role, sub, out); err != nil {
				return err
			}
			continue
		}
		if prefix == "" || !isStyleField(k) && slices.Contains(Roles, Role(role)) {
			return errors.New(str.ThemeRoleNotTable(name, role))
		}
		fields[k] = v
	}
	if len(fields) == 0 {
		return nil
	}
	own, rest, err := readStyle(name, prefix, fields)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return errors.New(str.ThemeUnknownField(name, prefix, slices.Sorted(maps.Keys(rest))[0]))
	}
	// Merge: the role may also be spelled the other way.
	out[Role(prefix)] = overlay(out[Role(prefix)], own)
	return nil
}

func isStyleField(k string) bool {
	switch k {
	case "fg", "bg", "bold", "faint", "italic", "underline", "reverse":
		return true
	}
	return false
}

// readTags reads [tags]: a table of settings per tag, a style plus scope.
func readTags(name string, tbl map[string]any, out map[string]tagFileStyle) error {
	for tag, v := range tbl {
		fields, ok := v.(map[string]any)
		if !ok {
			return errors.New(str.ThemeTagNotTable(name, tag))
		}
		where := str.ThemeTagEntry(tag)
		s, rest, err := readStyle(name, where, fields)
		if err != nil {
			return err
		}
		ts := tagFileStyle{fileStyle: s}
		for _, k := range slices.Sorted(maps.Keys(rest)) {
			if k != "scope" {
				return errors.New(str.ThemeUnknownField(name, where, k))
			}
			sc, ok := rest[k].(string)
			if !ok || sc != "line" && sc != "match" {
				return errors.New(str.ThemeBadScope(name, tag))
			}
			ts.scope = &sc
		}
		out[tag] = ts
	}
	return nil
}

// build resolves a chain of files, base first: later files override
// earlier ones field by field, then roles inherit down their dots.
func build(chain []file) (*Theme, error) {
	palette := map[string]color{}
	merged := map[Role]fileStyle{}
	origin := map[Role]string{} // which file set a role last, for messages
	mergedTags := map[string]tagFileStyle{}
	tagOrigin := map[string]string{}
	for _, f := range chain {
		for _, n := range slices.Sorted(maps.Keys(f.palette)) {
			if n == defaultColor || slices.Contains(terminalColors, n) {
				return nil, errors.New(str.ThemePaletteNameTaken(f.name, n))
			}
			c, ok := literal(f.palette[n])
			if !ok {
				return nil, errors.New(str.ThemeBadColor(f.name, str.ThemePaletteEntry(n), f.palette[n]))
			}
			palette[n] = c
		}
		for r, s := range f.ui {
			if !slices.Contains(Roles, r) {
				return nil, errors.New(str.ThemeUnknownRole(f.name, string(r)))
			}
			merged[r] = overlay(merged[r], s)
			origin[r] = f.name
		}
		for tag, s := range f.tags {
			m := mergedTags[tag]
			m.fileStyle = overlay(m.fileStyle, s.fileStyle)
			if s.scope != nil {
				m.scope = s.scope
			}
			mergedTags[tag], tagOrigin[tag] = m, f.name
		}
	}
	t := &Theme{styles: map[Role]style{}, sgr: map[Role]string{}, tags: map[string]TagStyle{}, chain: chain}
	for _, r := range Roles {
		s, err := resolveStyle(t.styles[r.parent()], merged[r], palette, origin[r], string(r))
		if err != nil {
			return nil, err
		}
		t.styles[r], t.sgr[r] = s, s.sgr()
	}
	for _, tag := range slices.Sorted(maps.Keys(mergedTags)) {
		fs := mergedTags[tag]
		s, err := resolveStyle(style{}, fs.fileStyle, palette, tagOrigin[tag], str.ThemeTagEntry(tag))
		if err != nil {
			return nil, err
		}
		t.tags[tag] = TagStyle{Style: Style{s}, Match: fs.scope != nil && *fs.scope == "match"}
	}
	return t, nil
}

// resolveStyle is fs's fields over base, with its colors resolved in
// palette. origin and where say where fs came from, for messages.
func resolveStyle(base style, fs fileStyle, palette map[string]color, origin, where string) (style, error) {
	s := base
	for _, c := range []struct {
		src *string
		dst **color
	}{{fs.fg, &s.fg}, {fs.bg, &s.bg}} {
		if c.src == nil {
			continue
		}
		if *c.src == defaultColor {
			*c.dst = nil
			continue
		}
		col, ok := resolve(*c.src, palette)
		if !ok {
			return s, errors.New(str.ThemeBadColor(origin, where, *c.src))
		}
		*c.dst = &col
	}
	for _, c := range []struct {
		src *bool
		dst *bool
	}{{fs.bold, &s.bold}, {fs.faint, &s.faint}, {fs.italic, &s.italic}, {fs.underline, &s.underline}, {fs.reverse, &s.reverse}} {
		if c.src != nil {
			*c.dst = *c.src
		}
	}
	return s, nil
}

// overlay is a with b's set fields on top.
func overlay(a, b fileStyle) fileStyle {
	for _, p := range []struct{ dst, src **string }{{&a.fg, &b.fg}, {&a.bg, &b.bg}} {
		if *p.src != nil {
			*p.dst = *p.src
		}
	}
	for _, p := range []struct{ dst, src **bool }{{&a.bold, &b.bold}, {&a.faint, &b.faint},
		{&a.italic, &b.italic}, {&a.underline, &b.underline}, {&a.reverse, &b.reverse}} {
		if *p.src != nil {
			*p.dst = *p.src
		}
	}
	return a
}

// literal reads a color written out: #rrggbb or a terminal color's name.
func literal(s string) (color, bool) {
	if hexRE.MatchString(s) {
		return color{ansi: -1, hex: s}, true
	}
	if i := slices.Index(terminalColors, s); i >= 0 {
		return color{ansi: i}, true
	}
	return color{}, false
}

// resolve reads a role's color: a literal, or a palette name.
func resolve(s string, palette map[string]color) (color, bool) {
	if c, ok := literal(s); ok {
		return c, true
	}
	c, ok := palette[s]
	return c, ok
}

// Equal reports whether t and o draw every role the same way. (A role's
// SGR pins its colors exactly, so the CSS matches too.)
func (t *Theme) Equal(o *Theme) bool {
	return maps.Equal(t.sgr, o.sgr) && maps.EqualFunc(t.tags, o.tags, func(a, b TagStyle) bool {
		return a.Match == b.Match && a.Style.SGR() == b.Style.SGR()
	})
}
