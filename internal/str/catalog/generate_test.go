package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWebJSON(t *testing.T) {
	got, err := WebJSON([]byte(`
[web]
a = "hello {name}"
[other]
b = "x"
`))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m["a"] != "hello {name}" {
		t.Errorf("WebJSON = %v", m)
	}
	// Each must parse, so the error is WebJSON's own rejection.
	for bad, want := range map[string]string{
		"[web]\na = { one = \"1\", other = \"{n}\" }": "plural",
		"[web]\na = \"{x:%q}\"":                       "fmt verbs",
		"[web]\na = \"{{literal}}\"":                  "braces",
	} {
		if _, err := Parse([]byte(bad)); err != nil {
			t.Errorf("Parse(%q): %v", bad, err)
		}
		if _, err := WebJSON([]byte(bad)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("WebJSON(%q) = %v, want the %s error", bad, err, want)
		}
	}
}

func TestTS(t *testing.T) {
	en := []byte(`separator = " · "
[browse]
saved = "saved {path}"
quoted = "no tag {name:%q} {{here}}"
copied = { one = "copied 1 line", other = "copied {n} lines" }
[x]
tick = "a ` + "`" + `tick` + "`" + ` and ${{dollar}} \\ back"
default = "{default} {err}"
`)
	goSrc, err := Generate(en, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := TS(en, goSrc, [][2]string{{"Echo", "›"}, {"ChoicePrev", "◄"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"export function browseSaved(path: unknown): string",
		"export function browseCopied(n: number): string",
		`case "one": return ` + "`copied 1 line`",
		"default: return `copied ${n} lines`",
		"export function browseQuoted(name: unknown): string { return `no tag ${q(name)} {here}`; }",
		"export function separator(): string",
		"return `a \\`tick\\` and \\${dollar} \\\\ back`",
		"export function xDefault(default_: unknown, err: unknown): string { return `${default_} ${e(err)}`; }",
		`echo: "›"`,
		`choicePrev: "◄"`,
	} {
		if !strings.Contains(string(ts), want) {
			t.Errorf("missing %s in:\n%s", want, ts)
		}
	}
	if _, err := TS([]byte(`a = "{x:%d}"`), nil, nil); err == nil {
		t.Error("a verb the page can't fill should be an error")
	}
}

// TestTSHelpersCantBeShadowed: a placeholder named like one of the
// module's helpers gets a _, so the helper still works.
func TestTSHelpersCantBeShadowed(t *testing.T) {
	en := []byte(`a = "{q:%q} {c} {e} {plural}"`)
	goSrc, err := Generate(en, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := TS(en, goSrc, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "export function a(q_: unknown, c_: unknown, e_: unknown, plural_: unknown): string { return `${q(q_)} ${c_} ${e_} ${plural_}`; }"
	if !strings.Contains(string(ts), want) {
		t.Errorf("missing %s in:\n%s", want, ts)
	}
}
