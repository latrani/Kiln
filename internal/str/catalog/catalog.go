// Package catalog parses Kiln's string catalogs: TOML files that map dotted
// keys to message templates. A template is plain text with {name}
// placeholders, optionally with a fmt verb ({name:%q}); {{ and }} are a
// literal brace. An entry that is a table of CLDR plural forms (one, other,
// …) picks its form by the {n} placeholder.
//
// Both the runtime (package str) and the code generator use it, so what the
// generator promises and what the runtime fills in can't drift apart.
package catalog

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Forms are the CLDR plural categories an entry may define.
var Forms = []string{"zero", "one", "two", "few", "many", "other"}

// Entry is one message: a single template, or plural forms keyed by
// category.
type Entry struct {
	Key   string
	Forms map[string]Template // "" for a plain message
}

// Plural reports whether the entry picks a form by {n}.
func (e Entry) Plural() bool { _, ok := e.Forms[""]; return !ok }

// Params are the entry's placeholder names in reading order (a plural's
// other form first), with a plural's count, n, always first.
func (e Entry) Params() []string {
	var out []string
	if e.Plural() {
		out = []string{"n"}
	}
	for _, form := range append([]string{"", "other"}, Forms...) {
		for _, p := range e.Forms[form].Params() {
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// Segment is literal text (Name == "") or a placeholder.
type Segment struct {
	Text string
	Name string
	Verb string // fmt verb, "%v" when the template gives none
}

// Template is a parsed message.
type Template []Segment

// Params are the template's placeholder names, in order, without repeats.
func (t Template) Params() []string {
	var out []string
	for _, s := range t {
		if s.Name != "" && !slices.Contains(out, s.Name) {
			out = append(out, s.Name)
		}
	}
	return out
}

// Fill renders the template with args keyed by placeholder name.
func (t Template) Fill(args map[string]any) string {
	var b strings.Builder
	for _, s := range t {
		if s.Name == "" {
			b.WriteString(s.Text)
		} else {
			fmt.Fprintf(&b, s.Verb, args[s.Name])
		}
	}
	return b.String()
}

// String is the template's source text.
func (t Template) String() string {
	var b strings.Builder
	for _, s := range t {
		switch {
		case s.Name == "":
			b.WriteString(strings.NewReplacer("{", "{{", "}", "}}").Replace(s.Text))
		case s.Verb == "%v":
			b.WriteString("{" + s.Name + "}")
		default:
			b.WriteString("{" + s.Name + ":" + s.Verb + "}")
		}
	}
	return b.String()
}

// String is the entry's source text: its template, or its plural forms
// in CLDR order.
func (e Entry) String() string {
	if !e.Plural() {
		return e.Forms[""].String()
	}
	var parts []string
	for _, form := range Forms {
		if t, ok := e.Forms[form]; ok {
			parts = append(parts, form+": "+t.String())
		}
	}
	return strings.Join(parts, " · ")
}

var placeholder = regexp.MustCompile(`^([a-z][a-z0-9_]*)(?::(%[-+# 0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z]))?$`)

// ParseTemplate parses one message template.
func ParseTemplate(s string) (Template, error) {
	var t Template
	var lit strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '{' && strings.HasPrefix(s[i:], "{{"), c == '}' && strings.HasPrefix(s[i:], "}}"):
			lit.WriteByte(c)
			i++
		case c == '{':
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return nil, fmt.Errorf("unclosed { in %q", s)
			}
			m := placeholder.FindStringSubmatch(s[i+1 : i+end])
			if m == nil {
				return nil, fmt.Errorf("bad placeholder {%s} in %q (want {name} or {name:%%v}; {{ for a brace)", s[i+1:i+end], s)
			}
			if lit.Len() > 0 {
				t = append(t, Segment{Text: lit.String()})
				lit.Reset()
			}
			verb := m[2]
			if verb == "" {
				verb = "%v"
			}
			t = append(t, Segment{Name: m[1], Verb: verb})
			i += end
		case c == '}':
			return nil, fmt.Errorf("stray } in %q (use }} for a brace)", s)
		default:
			lit.WriteByte(c)
		}
	}
	if lit.Len() > 0 {
		t = append(t, Segment{Text: lit.String()})
	}
	return t, nil
}

var keyPart = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Parse reads a catalog. Tables nest keys (a [session] table holding
// connected is session.connected); a table whose keys are all plural
// categories, other among them, is a plural entry.
func Parse(data []byte) (map[string]Entry, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := map[string]Entry{}
	return out, walk(raw, "", out)
}

func walk(tbl map[string]any, prefix string, out map[string]Entry) error {
	for k, v := range tbl {
		if !keyPart.MatchString(k) {
			return fmt.Errorf("%s%s: keys are lowercase letters, digits and _", prefix, k)
		}
		key := prefix + k
		switch v := v.(type) {
		case string:
			t, err := ParseTemplate(v)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			out[key] = Entry{Key: key, Forms: map[string]Template{"": t}}
		case map[string]any:
			if !isPlural(v) {
				if err := walk(v, key+".", out); err != nil {
					return err
				}
				continue
			}
			e := Entry{Key: key, Forms: map[string]Template{}}
			for form, s := range v {
				str, ok := s.(string)
				if !ok {
					return fmt.Errorf("%s.%s: want a string", key, form)
				}
				t, err := ParseTemplate(str)
				if err != nil {
					return fmt.Errorf("%s.%s: %w", key, form, err)
				}
				e.Forms[form] = t
			}
			out[key] = e
		default:
			return fmt.Errorf("%s: want a string or a table", key)
		}
	}
	return nil
}

func isPlural(tbl map[string]any) bool {
	if _, ok := tbl["other"]; !ok {
		return false
	}
	for k := range tbl {
		if !slices.Contains(Forms, k) {
			return false
		}
	}
	return true
}
