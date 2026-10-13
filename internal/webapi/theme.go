package webapi

import (
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/theme"
)

// Theme is how things look, as data, for dark and light backgrounds;
// Appearance is the setting (auto, light or dark). The page draws with
// it: roles for Kiln's own parts, tags laid over server text (see Span).
type Theme struct {
	Appearance  string
	Dark, Light Looks
}

// Looks is one appearance's styles. Chars has, by character key, the tag
// table of each character whose own [palette] or [tags] change it; the
// rest use Tags.
type Looks struct {
	Roles map[theme.Role]theme.Look
	Tags  map[string]theme.TagLook
	Chars map[string]map[string]theme.TagLook `json:",omitempty"`
}

// looks is the loop's theme state: the theme it last sent and each
// character's highlighter.
type looks struct {
	cfg         *config.Config // the config they were made for
	loaded      bool
	dark, light *theme.Theme
	ev          Theme
	base        *rules.Highlighter            // the theme's own tags
	hl          map[string]*rules.Highlighter // characters with looks of their own, by key
	lookErrs    map[string]error              // characters whose own looks didn't resolve
	styled      map[string]string             // each character's styled tags and their scopes, as styledOf says them
}

// load reads the theme cfg names from dir, for both appearances, with
// each character's own looks on top. A theme that won't load keeps the
// one loaded before (at first, the built-in) and returns why. changed
// says whether what goes to the page changed; restyle are the characters
// whose lines name different tags now (a tag gained or lost a style, or
// changed scope), so lines already sent need sending again. Highlighters are built on
// the dark theme: which tags have styles, and their scope, don't depend
// on the appearance.
func (lk *looks) load(dir string, cfg *config.Config, chars []config.Character) (changed bool, restyle []string, err error) {
	lk.cfg = cfg
	name, appearance := "kiln", "auto" //str:ok: theme vocabulary
	if cfg != nil {
		name, appearance = cfg.Theme, cfg.Appearance
	}
	dark, light := theme.BuiltinFor(theme.Dark), theme.BuiltinFor(theme.Light)
	if dir != "" {
		if dark, err = theme.Load(dir, name, theme.Dark); err == nil {
			light, err = theme.Load(dir, name, theme.Light)
		}
	}
	if err == nil || !lk.loaded {
		lk.dark, lk.light, lk.loaded = dark, light, true
	}
	lk.base = rules.New(lk.dark)
	lk.hl, lk.lookErrs = map[string]*rules.Highlighter{}, map[string]error{}
	ev := Theme{Appearance: appearance, Dark: looksOf(lk.dark), Light: looksOf(lk.light)}
	baseStyled, styled := styledOf(lk.dark), map[string]string{}
	for _, ch := range chars {
		k := app.Key(ch.World, ch.ID)
		styled[k] = baseStyled
		if len(ch.Looks) == 0 {
			continue
		}
		d, derr := lk.dark.With(ch.Looks...)
		l, lerr := lk.light.With(ch.Looks...)
		if derr != nil || lerr != nil {
			lk.lookErrs[k] = firstErr(derr, lerr)
			continue
		}
		lk.hl[k], styled[k] = rules.New(d), styledOf(d)
		for _, p := range []struct {
			th *theme.Theme
			to *Looks
		}{{d, &ev.Dark}, {l, &ev.Light}} {
			if p.to.Chars == nil {
				p.to.Chars = map[string]map[string]theme.TagLook{}
			}
			_, p.to.Chars[k] = p.th.Looks()
		}
	}
	for k, was := range lk.styled {
		if now, ok := styled[k]; ok && now != was {
			restyle = append(restyle, k)
		}
	}
	changed = !reflect.DeepEqual(ev, lk.ev)
	lk.ev, lk.styled = ev, styled
	return changed, restyle, err
}

// styledOf is which tags t styles, and with what scope: all that decides
// the tag names a line's spans carry (see rules.TagRuns).
func styledOf(t *theme.Theme) string {
	_, tags := t.Looks()
	var b strings.Builder
	for _, n := range slices.Sorted(maps.Keys(tags)) {
		b.WriteString(n)
		if tags[n].Match {
			b.WriteString(" match") //str:ok: a comparison key
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func looksOf(t *theme.Theme) Looks {
	roles, tags := t.Looks()
	return Looks{Roles: roles, Tags: tags}
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// highlighter is character k's, or the theme's own when k has none.
func (lk *looks) highlighter(k string) *rules.Highlighter {
	if h := lk.hl[k]; h != nil {
		return h
	}
	return lk.base
}
