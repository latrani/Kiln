package theme

import (
	"strings"
	"testing"
)

// sameLook reports the first role or tag where a and b draw differently.
func sameLook(t *testing.T, a, b *Theme, tags ...string) {
	t.Helper()
	for _, r := range Roles {
		if a.SGR(r) != b.SGR(r) {
			t.Errorf("%s: %q, want %q", r, a.SGR(r), b.SGR(r))
		}
	}
	for _, tag := range tags {
		_, at, _ := a.Tag(tag)
		_, bt, _ := b.Tag(tag)
		if at.Style.SGR() != bt.Style.SGR() || at.Match != bt.Match {
			t.Errorf("tag %s: %q %v, want %q %v", tag, at.Style.SGR(), at.Match, bt.Style.SGR(), bt.Match)
		}
	}
}

func TestShowKilnIsTheSource(t *testing.T) {
	got, err := Show(t.TempDir(), "kiln")
	if err != nil || got != string(builtinSrc) {
		t.Errorf("Show(kiln) = %.60q…, %v", got, err)
	}
}

func TestShowOneFileIsTheFile(t *testing.T) {
	dir := t.TempDir()
	body := "# mine\n[ui]\n\"status\" = { fg = \"red\" }\n"
	writeTheme(t, dir, "plain", body)
	if got, err := Show(dir, "plain"); err != nil || got != body {
		t.Errorf("Show(plain) = %q, %v", got, err)
	}
}

// Show's merged file, loaded on its own, looks just like the theme,
// on either appearance.
func TestShowMergedLoadsTheSame(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "base", "extends = \"kiln\"\n[palette]\nember = \"#010203\"\nmine = \"#0a0b0c\"\n"+
		"[palette.dark]\nmine = \"#0d0e0f\"\n[ui]\n\"sidebar.add\" = { bold = false }\n"+
		"[tags]\n\"page/in\" = { fg = \"mine\", scope = \"match\" }\n\"odd \\\"tag\\\"\" = { italic = true }\n")
	writeTheme(t, dir, "default", "extends = \"base\"\n[palette.light]\nember = \"#111213\"\n[ui]\n\"status\" = { bg = \"mine\" }\n")
	got, err := Show(dir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "\nextends") {
		t.Errorf("a merged theme extends nothing:\n%s", got)
	}
	writeTheme(t, dir, "flat", got)
	for _, ap := range []Appearance{Dark, Light} {
		want, err := Load(dir, "default", ap)
		if err != nil {
			t.Fatal(err)
		}
		flat, err := Load(dir, "flat", ap)
		if err != nil {
			t.Fatalf("%v\n%s", err, got)
		}
		sameLook(t, flat, want, "page", "page/in", "whisper/in", "self", "highlight", `odd "tag"`)
	}
}

func TestShowErrors(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "broken", "extends = \"kiln\"\n[ui]\n\"status\" = { fg = \"nope\" }\n")
	for _, name := range []string{"nope", "broken", "../x"} {
		if _, err := Show(dir, name); err == nil {
			t.Errorf("Show(%s): no error", name)
		}
	}
}
