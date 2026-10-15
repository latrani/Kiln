package theme

import (
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"

	"github.com/BurntSushi/toml"

	"github.com/latrani/Kiln/internal/str"
)

//go:embed default.toml
var builtinSrc []byte

//go:generate go run ./gen

//go:embed catppuccin.toml
var catppuccinSrc []byte

//go:embed glaze.toml
var glazeSrc []byte

// bases are the color names every theme has, under whatever it's built
// from: Catppuccin's accents (catppuccin.mauve), then Kiln's glazes
// (glaze.celadon-ink). terminalRaw is catppuccin.toml's [terminal]
// table, for Terminal.
var bases, terminalRaw = func() ([]file, map[string]any) {
	ctp, term, err := parseBase("catppuccin", catppuccinSrc) //str:ok: a namespace
	if err != nil {
		panic(err)
	}
	glaze, _, err := parseBase("glaze", glazeSrc) //str:ok: a namespace
	if err != nil {
		panic(err)
	}
	return []file{ctp, glaze}, term
}()

// parseBase reads base file ns.toml: a palette whose names it gives
// as ns.name, and catppuccin.toml's [terminal] table, returned as it's
// written.
func parseBase(ns string, data []byte) (file, map[string]any, error) {
	name := ns + ".toml" //str:ok: a path
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return file{name: name}, nil, err
	}
	term, _ := raw["terminal"].(map[string]any)
	delete(raw, "terminal")
	f, err := parseTables(name, raw)
	prefix := func(m map[string]string) map[string]string {
		out := make(map[string]string, len(m))
		for k, v := range m {
			out[ns+"."+k] = v
		}
		return out
	}
	f.palette = prefix(f.palette)
	for ap, m := range f.appearance {
		f.appearance[ap] = prefix(m)
	}
	return f, term, err
}

// builtinName is the name a theme extends to build on the built-in.
const builtinName = "kiln" //str:ok: theme vocabulary

// builtinFile is the built-in theme, parsed.
func builtinFile() file {
	f, err := parse(str.ThemeBuiltin(), builtinSrc)
	if err != nil {
		panic(err)
	}
	return f
}

var builtins = func() map[Appearance]*Theme {
	f := builtinFile()
	out := map[Appearance]*Theme{}
	for _, ap := range []Appearance{Dark, Light} {
		t, err := build([]file{f}, ap)
		if err != nil {
			panic(err)
		}
		out[ap] = t
	}
	return out
}()

// Builtin is the theme Kiln ships with, for a dark terminal.
func Builtin() *Theme { return builtins[Dark] }

// BuiltinFor is the theme Kiln ships with, for ap.
func BuiltinFor(ap Appearance) *Theme { return builtins[ap] }

// Load reads dir/themes/<name>.toml, following extends, for ap. "kiln"
// is the built-in, and so is "default" with no such file. On an error it
// returns the built-in theme too, so there's always something to draw
// with.
func Load(dir, name string, ap Appearance) (*Theme, error) {
	if !validName(name) {
		return builtins[ap], errors.New(str.ThemeBadName(name))
	}
	chain, err := chainFor(dir, name, nil)
	if err != nil {
		return builtins[ap], err
	}
	t, err := build(chain, ap)
	if err != nil {
		return builtins[ap], err
	}
	return t, nil
}

// chainFor is the files theme name is built from, base first. seen is the
// user files already on the chain, for catching loops.
func chainFor(dir, name string, seen []string) ([]file, error) {
	// Before the built-in was kiln, default.toml said extends = "default"
	// to build on it. Those files keep working.
	if name == builtinName || name == "default" && slices.Contains(seen, name) {
		return []file{builtinFile()}, nil
	}
	if slices.Contains(seen, name) {
		return nil, errors.New(str.ThemeExtendsLoop(themePath(seen[len(seen)-1]), name))
	}
	data, err := os.ReadFile(filepath.Join(dir, "themes", name+".toml"))
	if errors.Is(err, os.ErrNotExist) {
		if name != "default" {
			return nil, errors.New(str.ThemeNoTheme(name))
		}
		return []file{builtinFile()}, nil
	}
	if err != nil {
		return nil, err
	}
	f, err := parse(themePath(name), data)
	if err != nil {
		return nil, err
	}
	if f.extends == "" {
		return []file{f}, nil
	}
	base, err := chainFor(dir, f.extends, append(seen, name))
	if err != nil {
		return nil, err
	}
	return append(base, f), nil
}

// themePath is theme name's file, as messages name it.
func themePath(name string) string { return "themes/" + name + ".toml" } //str:ok: a path

var active atomic.Pointer[Theme]

func init() { active.Store(builtins[Dark]) }

// Active is the theme to draw with.
func Active() *Theme { return active.Load() }

// SetActive makes t the theme to draw with.
func SetActive(t *Theme) { active.Store(t) }

// Paint is Active().Paint.
func Paint(r Role, text string) string { return Active().Paint(r, text) }

// SGR is Active().SGR.
func SGR(r Role) string { return Active().SGR(r) }

// FromTOML builds a theme from src on top of the built-in, for a dark
// terminal, for tests and tools.
func FromTOML(src string) (*Theme, error) { return FromTOMLFor(src, Dark) }

// FromTOMLFor is FromTOML for ap.
func FromTOMLFor(src string, ap Appearance) (*Theme, error) {
	f, err := parse(themePath("test"), []byte(src))
	if err != nil {
		return nil, err
	}
	return build([]file{builtinFile(), f}, ap)
}
