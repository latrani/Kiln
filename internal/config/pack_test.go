package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// The starter fuzzball pack tags both sides of a page or whisper
// conversation, and only the side you receive is a priority.
func TestStarterPackPages(t *testing.T) {
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\nuse = [\"fuzzball\"]\n[[characters]]\nname = \"Kit\"\n"), 0o600)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "Kit")
	cls, err := classify.New(kit.Rules.Classify, kit.Name, kit.Aliases)
	if err != nil {
		t.Fatal(err)
	}
	th, err := theme.Builtin().Stack(kit.PackLooks, kit.Looks)
	if err != nil {
		t.Fatal(err)
	}
	hl := rules.New(th)
	judge := rules.Judge{Priority: kit.Rules.Priority, Quiet: kit.Rules.Quiet}
	for _, c := range []struct {
		line string
		dir  string // e.g. "page/in" or "whisper/out"
		in   bool
	}{
		{`Mira whispers, "psst"`, "whisper/in", true},
		{`Mira whispers "psst"`, "whisper/in", true},
		{`You whisper, "hi" to Mira.`, "whisper/out", false},
		{"Mira pages: you around?", "page/in", true},
		{"In a page-pose to you, Mira grins.", "page/in", true},
		{"You sense that Mira is paging you from the Docks.", "page/in", true},
		{`You page, "yep!" to Mira.`, "page/out", false},
		{`You page-pose, "Rook grins." to Mira.`, "page/out", false}, // naming Kit would tag it self
	} {
		kind, _, _ := strings.Cut(c.dir, "/")
		tags := cls.Classify(c.line)
		if !slices.Contains(tags, kind) || !slices.Contains(tags, c.dir) {
			t.Errorf("%q: tags %v, want %s and %s", c.line, tags, kind, c.dir)
		}
		ts := cls.Tags(c.line)
		priority, runs := judge.Of(ts).Priority, hl.Runs(c.line, ts)
		// Only pages and whispers to you are priority; the built-in
		// theme styles both ways alike.
		if priority != c.in || runs == nil {
			t.Errorf("%q: priority %v, styled %v; want priority %v, styled", c.line, priority, runs != nil, c.in)
		}
	}
	// The pack's looks, as the built-in theme had them: dark, then light.
	light, err := theme.BuiltinFor(theme.Light).Stack(kit.PackLooks, kit.Looks)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		th        *theme.Theme
		tag, want string
	}{
		{th, "page/in", "\x1b[1;38;2;138;173;244m"},
		{th, "whisper/in", "\x1b[1;38;2;145;215;227m"},
		{light, "page/in", "\x1b[1;38;2;30;102;245m"},
	} {
		if _, ts, _ := c.th.Tag(c.tag); ts.Style.SGR() != c.want {
			t.Errorf("%s = %q, want %q", c.tag, ts.Style.SGR(), c.want)
		}
	}
	if len(kit.PackLooks) == 0 {
		t.Error("the starter fuzzball pack has no looks")
	}
}

// The starter fuzzball pack tags watchfor's notices: Fuzzball's own
// ("Somewhere on the muck, Rook connected.") and the older fbmuf one's
// ("Somewhere on the muck, Rook has connected."). Reconnecting counts as
// connecting, which is a priority.
func TestStarterPackWatchfor(t *testing.T) {
	dir := t.TempDir()
	if err := config.EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\nuse = [\"fuzzball\"]\n[[characters]]\nname = \"Kit\"\n"), 0o600)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "Kit")
	cls, err := classify.New(kit.Rules.Classify, kit.Name, kit.Aliases)
	if err != nil {
		t.Fatal(err)
	}
	judge := rules.Judge{Priority: kit.Rules.Priority, Quiet: kit.Rules.Quiet}
	for _, c := range []struct{ line, tag string }{
		{"Somewhere on the muck, Rook connected.", "watchfor/connect"},
		{"Somewhere on the muck, Rook reconnected.", "watchfor/connect"},
		{"Somewhere on the muck, Rook disconnected.", "watchfor/disconnect"},
		{"Somewhere on the muck, Rook has connected.", "watchfor/connect"},
		{"Somewhere on the muck, Rook has reconnected.", "watchfor/connect"},
		{"Somewhere on the muck, Rook has disconnected.", "watchfor/disconnect"},
		{`Rook says, "Somewhere on the muck, Ash connected."`, ""},
	} {
		tags := cls.Classify(c.line)
		if c.tag == "" {
			if slices.Contains(tags, "watchfor") {
				t.Errorf("%q: tags %v, want no watchfor", c.line, tags)
			}
			continue
		}
		if !slices.Contains(tags, "watchfor") || !slices.Contains(tags, c.tag) {
			t.Errorf("%q: tags %v, want watchfor and %s", c.line, tags, c.tag)
		}
		// Someone you watch for connecting is a priority; leaving isn't.
		if got, want := judge.Of(cls.Tags(c.line)).Priority, c.tag == "watchfor/connect"; got != want {
			t.Errorf("%q: priority %v, want %v", c.line, got, want)
		}
	}
}

// packWorld writes a pack named extra and a world fm using it, and
// returns Kit.
func packWorld(t *testing.T, pack string) config.Character {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"worlds", "packs"} {
		os.MkdirAll(filepath.Join(dir, sub), 0o700)
	}
	os.WriteFile(filepath.Join(dir, "packs", "extra.toml"), []byte(pack), 0o600)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\nuse = [\"extra\"]\n[[characters]]\nname = \"Kit\"\n"), 0o600)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "Kit")
	return kit
}

// A pack's [palette] and [tags] go under the theme, light variants and all.
func TestPackLooks(t *testing.T) {
	kit := packWorld(t, "[palette]\nsea = \"#123456\"\n[palette.light]\nsea = \"#0a0b0c\"\n[tags]\n\"ooc\" = { fg = \"sea\" }\n\n[[classify]]\ntag = \"ooc\"\npattern = '^OOC'\n")
	if len(kit.PackLooks) != 1 || len(kit.Looks) != 0 {
		t.Fatalf("pack looks %d, own looks %d", len(kit.PackLooks), len(kit.Looks))
	}
	for _, c := range []struct {
		th   *theme.Theme
		want string
	}{{theme.Builtin(), "\x1b[38;2;18;52;86m"}, {theme.BuiltinFor(theme.Light), "\x1b[38;2;10;11;12m"}} {
		th, err := c.th.Stack(kit.PackLooks, kit.Looks)
		if err != nil {
			t.Fatal(err)
		}
		if _, ts, _ := th.Tag("ooc"); ts.Style.SGR() != c.want {
			t.Errorf("ooc = %q, want %q", ts.Style.SGR(), c.want)
		}
	}
}

// A pack's look that names no color loads, and says which pack when it's
// put on a theme.
func TestPackBadColorNamesThePack(t *testing.T) {
	kit := packWorld(t, "[tags]\n\"ooc\" = { fg = \"nowhere\" }\n")
	_, err := theme.Builtin().Stack(kit.PackLooks, nil)
	want := str.ThemeBadColor(filepath.Join("packs", "extra.toml"), str.ThemeTagEntry("ooc"), "nowhere")
	if err == nil || err.Error() != want {
		t.Errorf("err %v, want %q", err, want)
	}
}

// A pack still rejects keys that aren't rules or looks.
func TestPackUnknownKey(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o700)
	os.MkdirAll(filepath.Join(dir, "packs"), 0o700)
	os.WriteFile(filepath.Join(dir, "packs", "extra.toml"), []byte("[frob]\nx = 1\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\nuse = [\"extra\"]\n[[characters]]\nname = \"Kit\"\n"), 0o600)
	_, err := config.Load(dir)
	if err == nil || !strings.Contains(err.Error(), str.ConfigUnknownKey("extra.toml", "frob")) {
		t.Errorf("err %v", err)
	}
}
