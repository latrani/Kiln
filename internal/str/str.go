// Package str holds every string Kiln shows a person. The text lives in
// locales/en.toml (and any translations beside it), embedded at build time;
// keys_gen.go gives each entry a function, so a missing key or a wrong
// number of arguments is a compile error.
//
// After editing locales/en.toml, run go generate ./internal/str.
//
// Build with -ldflags "-X github.com/latrani/Kiln/internal/str.lang=de" to
// pick locales/de.toml; entries it lacks fall back to English.
package str

//go:generate go run ./gen

import (
	"embed"
	"fmt"

	"github.com/latrani/Kiln/internal/str/catalog"
	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

//go:embed locales/*.toml
var locales embed.FS

// lang is the locale to show, set at build time.
var lang = "en"

var (
	tag      language.Tag
	active   map[string]catalog.Entry
	fallback map[string]catalog.Entry
)

func init() {
	fallback = mustLoad("en")
	active = fallback
	tag = language.English
	if lang != "en" {
		active = mustLoad(lang)
		tag = language.MustParse(lang)
	}
}

func mustLoad(name string) map[string]catalog.Entry {
	data, err := locales.ReadFile("locales/" + name + ".toml")
	if err != nil {
		panic(err)
	}
	c, err := catalog.Parse(data)
	if err != nil {
		panic(fmt.Sprintf("locales/%s.toml: %v", name, err))
	}
	return c
}

func lookup(key string) catalog.Entry {
	if e, ok := active[key]; ok {
		return e
	}
	return fallback[key]
}

// get fills a plain entry.
func get(key string, args map[string]any) string {
	return lookup(key).Forms[""].Fill(args)
}

// formNames are plural.Form's categories, in its iota order.
var formNames = [...]string{"other", "zero", "one", "two", "few", "many"}

// count fills a plural entry, picking its form by n.
func count(key string, n int, args map[string]any) string {
	e := lookup(key)
	abs := n
	if abs < 0 {
		abs = -abs
	}
	form := formNames[plural.Cardinal.MatchPlural(tag, abs, 0, 0, 0, 0)]
	t, ok := e.Forms[form]
	if !ok {
		t = e.Forms["other"]
	}
	return t.Fill(args)
}

// Wrap is an error that reads as msg (which should already say what err
// said) and unwraps to err, so errors.Is still sees through it.
func Wrap(msg string, err error) error { return wrapped{msg, err} }

type wrapped struct {
	msg string
	err error
}

func (w wrapped) Error() string { return w.msg }
func (w wrapped) Unwrap() error { return w.err }
