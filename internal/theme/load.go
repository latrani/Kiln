package theme

import (
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"

	"github.com/latrani/Kiln/internal/str"
)

//go:embed default.toml
var builtinSrc []byte

var builtin = func() *Theme {
	f, err := parse(themePath("default"), builtinSrc)
	if err != nil {
		panic(err)
	}
	t, err := build([]file{f})
	if err != nil {
		panic(err)
	}
	return t
}()

// Builtin is the theme Kiln ships with.
func Builtin() *Theme { return builtin }

// Load reads dir/themes/default.toml, following extends, or returns the
// built-in theme when there's no such file. On an error it returns the
// built-in theme too, so there's always something to draw with.
func Load(dir string) (*Theme, error) {
	chain, err := chainFor(dir, "default", nil)
	if err != nil {
		return builtin, err
	}
	t, err := build(chain)
	if err != nil {
		return builtin, err
	}
	return t, nil
}

// chainFor is the files theme name is built from, base first. seen is the
// user files already on the chain: naming one of those again means the
// built-in theme of that name (only default has one), and otherwise a loop.
func chainFor(dir, name string, seen []string) ([]file, error) {
	builtinFile := func() ([]file, error) {
		if name != "default" {
			return nil, errors.New(str.ThemeNoTheme(name))
		}
		f, _ := parse(themePath("default"), builtinSrc)
		return []file{f}, nil
	}
	if slices.Contains(seen, name) {
		if name == "default" { // the user's default.toml is on the chain: this means the built-in
			return builtinFile()
		}
		return nil, errors.New(str.ThemeExtendsLoop(themePath(seen[len(seen)-1]), name))
	}
	data, err := os.ReadFile(filepath.Join(dir, "themes", name+".toml"))
	if errors.Is(err, os.ErrNotExist) {
		return builtinFile()
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

func init() { active.Store(builtin) }

// Active is the theme to draw with.
func Active() *Theme { return active.Load() }

// SetActive makes t the theme to draw with.
func SetActive(t *Theme) { active.Store(t) }

// Paint is Active().Paint.
func Paint(r Role, text string) string { return Active().Paint(r, text) }

// SGR is Active().SGR.
func SGR(r Role) string { return Active().SGR(r) }

// FromTOML builds a theme from src on top of the built-in, for tests and
// tools.
func FromTOML(src string) (*Theme, error) {
	base, _ := parse(themePath("default"), builtinSrc)
	f, err := parse(themePath("test"), []byte(src))
	if err != nil {
		return nil, err
	}
	return build([]file{base, f})
}
