package str

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/str/catalog"
)

func TestGeneratedIsFresh(t *testing.T) {
	en, err := os.ReadFile("locales/en.toml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("keys_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	want, err := catalog.Generate(en, got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("keys_gen.go is stale: run go generate ./internal/str")
	}
	gotJS, err := os.ReadFile("../../web/static/strings.json")
	if err != nil {
		t.Fatal(err)
	}
	wantJS, err := catalog.WebJSON(en)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJS, wantJS) {
		t.Error("web/static/strings.json is stale: run go generate ./internal/str")
	}
}

// TestTranslationsMatchEnglish checks each translation against en.toml: no
// keys English lacks, no placeholders the generated function doesn't pass.
func TestTranslationsMatchEnglish(t *testing.T) {
	files, err := fs.Glob(locales, "locales/*.toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "locales/en.toml" {
			continue
		}
		data, _ := locales.ReadFile(f)
		c, err := catalog.Parse(data)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		for key, e := range c {
			en, ok := fallback[key]
			if !ok {
				t.Errorf("%s: %s isn't in en.toml", f, key)
				continue
			}
			if e.Plural() != en.Plural() {
				t.Errorf("%s: %s must be plural exactly when en.toml's is", f, key)
			}
			params := en.Params()
			for form, tmpl := range e.Forms {
				for _, p := range tmpl.Params() {
					if !slices.Contains(params, p) {
						t.Errorf("%s: %s %s uses {%s}; en.toml has %v", f, key, form, p, params)
					}
				}
			}
		}
	}
}

func TestParseTemplate(t *testing.T) {
	tm, err := catalog.ParseTemplate("{{literal}} {who:%q} has {n} {n}")
	if err != nil {
		t.Fatal(err)
	}
	if got := tm.Fill(map[string]any{"who": "Kit", "n": 3}); got != `{literal} "Kit" has 3 3` {
		t.Errorf("Fill = %q", got)
	}
	if got := tm.Params(); !slices.Equal(got, []string{"who", "n"}) {
		t.Errorf("Params = %v", got)
	}
	for _, bad := range []string{"{", "}", "{Who}", "{a:q}", "{}"} {
		if _, err := catalog.ParseTemplate(bad); err == nil {
			t.Errorf("ParseTemplate(%q) should fail", bad)
		}
	}
}

func TestParsePlural(t *testing.T) {
	c, err := catalog.Parse([]byte("[x]\nlines = { one = \"1 line in {where}\", other = \"{n} lines in {where}\" }\n"))
	if err != nil {
		t.Fatal(err)
	}
	e := c["x.lines"]
	if !e.Plural() || !slices.Equal(e.Params(), []string{"n", "where"}) {
		t.Errorf("x.lines = %+v, params %v", e, e.Params())
	}
}

func TestCount(t *testing.T) {
	active = map[string]catalog.Entry{}
	defer func() { active = fallback }()
	c, _ := catalog.Parse([]byte(`k = { one = "one {n}", other = "many {n}" }`))
	active["k"] = c["k"]
	if got := count("k", 1, map[string]any{"n": 1}); got != "one 1" {
		t.Errorf("count 1 = %q", got)
	}
	if got := count("k", 2, map[string]any{"n": 2}); got != "many 2" {
		t.Errorf("count 2 = %q", got)
	}
	if got := count("k", 0, map[string]any{"n": 0}); got != "many 0" {
		t.Errorf("count 0 = %q", got)
	}
}

func TestWrap(t *testing.T) {
	err := Wrap("outer: inner", fs.ErrNotExist)
	if err.Error() != "outer: inner" || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Wrap = %v", err)
	}
}

func TestFuncName(t *testing.T) {
	if got := catalog.FuncName("browse.lines_in_range"); got != "BrowseLinesInRange" {
		t.Errorf("FuncName = %q", got)
	}
	if !strings.HasPrefix(catalog.FuncName("a.b"), "A") {
		t.Error("FuncName should export")
	}
}

// TestSignaturesStayPut: rewording a template must not reorder its
// function's parameters, or every call site would silently swap
// arguments.
func TestSignaturesStayPut(t *testing.T) {
	first, err := catalog.Generate([]byte(`k = "no character {world}/{char}"`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(first, []byte("func K(world, char any)")) {
		t.Fatalf("first generation:\n%s", first)
	}
	again, err := catalog.Generate([]byte(`k = "{char} isn't in {world}, {n} {err}"`), first)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(again, []byte("func K(world, char any, n int, err error)")) {
		t.Errorf("reworded:\n%s", again)
	}
}
