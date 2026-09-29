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

// file is one parsed theme file.
type file struct {
	name    string
	extends string
	palette map[string]string
	ui      map[Role]fileStyle
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

// Theme is a loaded theme: every role's style, resolved.
type Theme struct {
	styles map[Role]style
	sgr    map[Role]string
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

var styleFields = []string{"fg", "bg", "bold", "faint", "italic", "underline", "reverse"}

// parse reads one theme file. name is its file name, for messages.
func parse(name string, data []byte) (file, error) {
	f := file{name: name, palette: map[string]string{}, ui: map[Role]fileStyle{}}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return f, errors.New(str.ThemeParse(name, err))
	}
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

// flatten reads [ui], written with quoted dotted keys ("sidebar.active"),
// nested tables ([ui.sidebar.active]), or both, into styles by role.
func flatten(name, prefix string, tbl map[string]any, out map[Role]fileStyle) error {
	var own fileStyle
	hasOwn := false
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
		if prefix == "" || !slices.Contains(styleFields, k) && slices.Contains(Roles, Role(role)) {
			return errors.New(str.ThemeRoleNotTable(name, role))
		}
		if !slices.Contains(styleFields, k) {
			return errors.New(str.ThemeUnknownField(name, prefix, k))
		}
		hasOwn = true
		switch k {
		case "fg", "bg":
			s, ok := v.(string)
			if !ok {
				return errors.New(str.ThemeBadField(name, prefix, k, str.ThemeWantColor()))
			}
			if k == "fg" {
				own.fg = &s
			} else {
				own.bg = &s
			}
		default:
			b, ok := v.(bool)
			if !ok {
				return errors.New(str.ThemeBadField(name, prefix, k, str.ThemeWantBool()))
			}
			switch k {
			case "bold":
				own.bold = &b
			case "faint":
				own.faint = &b
			case "italic":
				own.italic = &b
			case "underline":
				own.underline = &b
			case "reverse":
				own.reverse = &b
			}
		}
	}
	if hasOwn { // merge: the role may also be spelled the other way
		out[Role(prefix)] = overlay(out[Role(prefix)], own)
	}
	return nil
}

// build resolves a chain of files, base first: later files override
// earlier ones field by field, then roles inherit down their dots.
func build(chain []file) (*Theme, error) {
	palette := map[string]color{}
	merged := map[Role]fileStyle{}
	origin := map[Role]string{} // which file set a role last, for messages
	for _, f := range chain {
		for _, n := range slices.Sorted(maps.Keys(f.palette)) {
			if slices.Contains(terminalColors, n) {
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
	}
	t := &Theme{styles: map[Role]style{}, sgr: map[Role]string{}}
	for _, r := range Roles {
		s := t.styles[r.parent()]
		fs := merged[r]
		for _, c := range []struct {
			src *string
			dst **color
		}{{fs.fg, &s.fg}, {fs.bg, &s.bg}} {
			if c.src == nil {
				continue
			}
			col, ok := resolve(*c.src, palette)
			if !ok {
				return nil, errors.New(str.ThemeBadColor(origin[r], string(r), *c.src))
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
		t.styles[r], t.sgr[r] = s, s.sgr()
	}
	return t, nil
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
func (t *Theme) Equal(o *Theme) bool { return maps.Equal(t.sgr, o.sgr) }
