package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// write creates files under dir from a map of relative path → content.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadResolvesInheritance(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"config.toml": "[defaults]\nmax_line_bytes = 1000\n",
		"packs/p.toml": `
[[classify]]
tag = "page"
pattern = '^\S+ pages: '
`,
		"worlds/fm.toml": `
host = "example.org"
port = 8899
tls = true
use = ["p"]
login = "connect {name} {password}"

[[classify]]
tag = "ooc"
pattern = '^OOC'

[[characters]]
id = "kit"
name = "Kit"
aliases = ["Kitty"]

[[characters]]
id = "rook"
name = "Rook"
max_line_bytes = 500
login = "co {name} {password}"

[[characters.classify]]
tag = "mine"
pattern = 'Rook'
`,
	})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, ok := cfg.Find("fm", "kit")
	if !ok {
		t.Fatal("kit not found")
	}
	if kit.Name != "Kit" || kit.Aliases[0] != "Kitty" || kit.Host != "example.org" || kit.Port != 8899 || !kit.TLS {
		t.Errorf("kit basics wrong: %+v", kit)
	}
	if kit.TLSTrust != "pin" {
		t.Errorf("TLSTrust = %q, want default pin", kit.TLSTrust)
	}
	if kit.MaxLineBytes != 1000 {
		t.Errorf("kit MaxLineBytes = %d, want 1000 from [defaults]", kit.MaxLineBytes)
	}
	if kit.NewlineMode != "batch" {
		t.Errorf("kit NewlineMode = %q, want built-in batch", kit.NewlineMode)
	}
	if kit.Login != "connect {name} {password}" {
		t.Errorf("kit Login = %q, want world's", kit.Login)
	}
	if tags := classifyTags(kit.Rules); tags != "page,ooc" {
		t.Errorf("kit classify tags = %s, want pack then world: page,ooc", tags)
	}

	rook, _ := cfg.Find("fm", "rook")
	if rook.MaxLineBytes != 500 || rook.Login != "co {name} {password}" {
		t.Errorf("rook overrides lost: %+v", rook)
	}
	if tags := classifyTags(rook.Rules); tags != "page,ooc,mine" {
		t.Errorf("rook classify tags = %s, want page,ooc,mine", tags)
	}
	if tags := classifyTags(kit.Rules); strings.Contains(tags, "mine") {
		t.Error("rook's rule leaked into kit")
	}
}

func classifyTags(r Rules) string {
	var tags []string
	for _, c := range r.Classify {
		tags = append(tags, c.Tag)
	}
	return strings.Join(tags, ",")
}

func TestLoadEmptyDirIsEmptyConfig(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil || len(cfg.Worlds) != 0 {
		t.Errorf("Load(empty) = %+v, %v", cfg, err)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name, world, wantErr string
	}{
		{"missing host", "port = 1\n", "host is required"},
		{"bad port", "host = \"h\"\nport = 0\n", "port must be"},
		{"missing name", "host = \"h\"\nport = 1\n[[characters]]\nid = \"kit\"\n", str.ConfigNameRequired("")},
		{"unknown key", "host = \"h\"\nport = 1\nhots = \"typo\"\n", `unknown key "hots"`},
		{"missing pack", "host = \"h\"\nport = 1\nuse = [\"nope\"]\n", str.ConfigUnknownPack("nope")},
		{"bad regex", "host = \"h\"\nport = 1\n[[classify]]\ntag = \"x\"\npattern = '('\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n", upTo(str.ConfigClassifyBadPattern(1, mark, nil))},
		{"bad trust", "host = \"h\"\nport = 1\ntls_trust = \"yolo\"\n", "tls_trust"},
		{"bad newline mode", "host = \"h\"\nport = 1\nnewline_mode = \"x\"\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n", "newline_mode"},
		{"bad char id", "host = \"h\"\nport = 1\n[[characters]]\nid = \"a/b\"\nname = \"X\"\n", "id may only use"},
		{"name needs an id", "host = \"h\"\nport = 1\n[[characters]]\nname = \"Big Kit\"\n", str.ConfigBadCharacterId("")},
		{"duplicate id", "host = \"h\"\nport = 1\n[[characters]]\nname = \"Kit\"\n[[characters]]\nid = \"kit\"\nname = \"Other\"\n", str.ConfigDuplicateCharacterId("")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, map[string]string{"worlds/w.toml": c.world})
			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want containing %q", err, c.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "w.toml") {
				t.Errorf("err %q should name the file", err)
			}
		})
	}
}

func TestEnsureDefaultsWritesStarterFilesOnce(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"config.toml", "packs/fuzzball.toml", "themes/default.toml"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("%s not written: %v", rel, err)
		}
	}
	// User edits must survive a second run.
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("# mine\n"), 0o600)
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if string(b) != "# mine\n" {
		t.Errorf("EnsureDefaults overwrote config.toml: %q", b)
	}
}

// The starter theme changes nothing until you uncomment something.
func TestStarterThemeIsBuiltin(t *testing.T) {
	dir := t.TempDir()
	EnsureDefaults(dir)
	for _, ap := range []theme.Appearance{theme.Dark, theme.Light} {
		th, err := theme.Load(dir, "default", ap)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range theme.Roles {
			if th.SGR(r) != theme.BuiltinFor(ap).SGR(r) {
				t.Errorf("%s: %q, want %q", r, th.SGR(r), theme.BuiltinFor(ap).SGR(r))
			}
		}
	}
}

func TestClassifyRuleTags(t *testing.T) {
	r := ClassifyRule{Tag: "page", Tags: []string{"page/in", "", "page"}}
	if got := strings.Join(r.AllTags(), ","); got != "page,page/in" {
		t.Errorf("AllTags = %q", got)
	}
	for body, ok := range map[string]bool{
		"[[classify]]\ntags = [\"page\", \"page/in\"]\npattern = \"x\"\n": true,
		"[[classify]]\npattern = \"x\"\n":                                 false,
		"[[classify]]\ntags = []\npattern = \"x\"\n":                      false,
	} {
		dir := t.TempDir()
		write(t, dir, map[string]string{"worlds/a.toml": "host = \"h\"\nport = 1\n" + body + "[[characters]]\nname = \"Kit\"\n"})
		if _, err := Load(dir); (err == nil) != ok {
			t.Errorf("%q: err = %v", body, err)
		}
	}
}

func TestStarterPackLoads(t *testing.T) {
	dir := t.TempDir()
	EnsureDefaults(dir)
	write(t, dir, map[string]string{"worlds/fm.toml": "host = \"h\"\nport = 1\nuse = [\"fuzzball\"]\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n"})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "kit")
	if len(kit.Rules.Classify) == 0 || len(kit.Rules.Attention) == 0 {
		t.Errorf("fuzzball pack rules missing: %+v", kit.Rules)
	}
	if kit.MaxLineBytes != 2047 {
		t.Errorf("MaxLineBytes = %d, want 2047", kit.MaxLineBytes)
	}
}

func TestAutoconnectInherits(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"worlds/a.toml": "host = \"h\"\nport = 1\nautoconnect = true\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n[[characters]]\nid = \"rook\"\nname = \"Rook\"\nautoconnect = false\n",
		"worlds/b.toml": "host = \"h\"\nport = 1\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n",
	})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		world, char string
		want        bool
	}{{"a", "kit", true}, {"a", "rook", false}, {"b", "ash", false}} {
		ch, _ := cfg.Find(c.world, c.char)
		if ch.Autoconnect != c.want {
			t.Errorf("%s/%s Autoconnect = %v, want %v", c.world, c.char, ch.Autoconnect, c.want)
		}
	}
}

func TestLocalEchoInherits(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"worlds/a.toml": "host = \"h\"\nport = 1\nlocal_echo = true\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n[[characters]]\nid = \"rook\"\nname = \"Rook\"\nlocal_echo = false\n",
		"worlds/b.toml": "host = \"h\"\nport = 1\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n",
	})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		world, char string
		want        bool
	}{{"a", "kit", true}, {"a", "rook", false}, {"b", "ash", false}} {
		ch, _ := cfg.Find(c.world, c.char)
		if ch.LocalEcho != c.want {
			t.Errorf("%s/%s LocalEcho = %v, want %v", c.world, c.char, ch.LocalEcho, c.want)
		}
	}
}

func TestReconnectDefaultsOffAndInherits(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"worlds/a.toml": "host = \"h\"\nport = 1\nreconnect = true\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n[[characters]]\nid = \"rook\"\nname = \"Rook\"\nreconnect = false\n",
		"worlds/b.toml": "host = \"h\"\nport = 1\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n",
	})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		world, char string
		want        bool
	}{{"a", "kit", true}, {"a", "rook", false}, {"b", "ash", false}} {
		ch, _ := cfg.Find(c.world, c.char)
		if ch.Reconnect != c.want {
			t.Errorf("%s/%s Reconnect = %v, want %v", c.world, c.char, ch.Reconnect, c.want)
		}
	}
}

func TestLoginDefaultsToConnect(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"worlds/a.toml": "host = \"h\"\nport = 1\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n",
		"worlds/b.toml": "host = \"h\"\nport = 1\nlogin = \"\"\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n",
	})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if kit, _ := cfg.Find("a", "kit"); kit.Login != DefaultLogin {
		t.Errorf("a/kit Login = %q, want %q", kit.Login, DefaultLogin)
	}
	if ash, _ := cfg.Find("b", "ash"); ash.Login != "" {
		t.Errorf("b/ash Login = %q, want \"\" (login = \"\" turns it off)", ash.Login)
	}
}

// The stock config.toml says what the built-in defaults are, so a fresh
// install and one with no config.toml behave the same.
func TestStockConfigMatchesBuiltIns(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"login = \"" + DefaultLogin + "\"", "reconnect = false"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("stock config.toml lacks %q", want)
		}
	}
}

func TestExportDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	for _, c := range []struct{ toml, want string }{
		{"", filepath.Join(home, "Documents", "Kiln Scenes")},
		{"export_dir = \"~/scenes\"\n", filepath.Join(home, "scenes")},
		{"export_dir = \"/tmp/x\"\n", "/tmp/x"},
	} {
		dir := t.TempDir()
		write(t, dir, map[string]string{"config.toml": c.toml})
		cfg, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ExportDir != c.want {
			t.Errorf("export_dir %q → %q, want %q", c.toml, cfg.ExportDir, c.want)
		}
	}
}

func TestPasswordStore(t *testing.T) {
	for _, c := range []struct{ toml, want string }{
		{"", "keychain"},
		{"password_store = \"file\"\n", "file"},
		{"password_store = \"none\"\n", "none"},
		{"password_store = \"vault\"\n", ""},
	} {
		dir := t.TempDir()
		write(t, dir, map[string]string{"config.toml": c.toml})
		cfg, err := Load(dir)
		if c.want == "" {
			if err == nil {
				t.Errorf("password_store %q: no error", c.toml)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if cfg.PasswordStore != c.want {
			t.Errorf("password_store %q → %q, want %q", c.toml, cfg.PasswordStore, c.want)
		}
	}
}

func TestCharacterIDsAndOrder(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"worlds/w.toml": `host = "h"
port = 1

[[characters]]
name = "Zed"

[[characters]]
id = "big-kit"
name = "Big Kit"

[[characters.classify]]
tag = "x"
pattern = 'x'

[[characters]]
name = "Ash"
`})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, ch := range cfg.Worlds[0].Characters {
		ids = append(ids, ch.ID)
	}
	if want := []string{"Zed", "big-kit", "Ash"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %q, want %q (id defaults to name; file order kept)", ids, want)
	}
	if kit, _ := cfg.Find("w", "big-kit"); len(kit.Rules.Classify) != 1 {
		t.Errorf("per-character rule missing: %+v", kit.Rules)
	}
	if ash, _ := cfg.Find("w", "Ash"); len(ash.Rules.Classify) != 0 {
		t.Errorf("rule leaked to the next character: %+v", ash.Rules)
	}
}

func TestLogNameAndStrftime(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"config.toml": "log_dir = \"/logs/{world}/{name}/%Y/%m\"\nlog_name = \"%Y-%m-%d.%H.%M.%S\"\nexport_name = \"%Y-%m-%d {name}\"\n"})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogDir != "/logs/{world}/{name}/%Y/%m" || cfg.LogName != "%Y-%m-%d.%H.%M.%S" || cfg.ExportName != "%Y-%m-%d {name}" {
		t.Errorf("LogDir %q, LogName %q, ExportName %q", cfg.LogDir, cfg.LogName, cfg.ExportName)
	}
}

func TestLogAndExportSettings(t *testing.T) {
	home, _ := os.UserHomeDir()
	dir := t.TempDir()
	write(t, dir, map[string]string{"config.toml": "log_dir = \"~/Logs/{world}/{char}\"\nexport_name = \"{name} {date}\"\nexport_format = \"ansi\"\n"})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "Logs/{world}/{char}"); cfg.LogDir != want {
		t.Errorf("LogDir = %q, want %q", cfg.LogDir, want)
	}
	if cfg.ExportName != "{name} {date}" || cfg.ExportFormat != "ansi" {
		t.Errorf("ExportName %q, ExportFormat %q", cfg.ExportName, cfg.ExportFormat)
	}

	dir = t.TempDir()
	cfg, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogDir != "" || cfg.ExportName != DefaultExportName || cfg.ExportFormat != "" {
		t.Errorf("defaults: LogDir %q, ExportName %q, ExportFormat %q", cfg.LogDir, cfg.ExportName, cfg.ExportFormat)
	}

	for _, bad := range []string{
		"log_dir = \"/logs/{wrld}\"\n",
		"export_name = \"{date} {character}\"\n",
		"export_format = \"pdf\"\n",
		"log_name = \"{world}/%Y\"\n",
		"log_name = \"%Q\"\n",
		"log_dir = \"/logs/%Y/%\"\n",
	} {
		dir := t.TempDir()
		write(t, dir, map[string]string{"config.toml": bad})
		if _, err := Load(dir); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestNotifyInheritsAndDefaults(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"config.toml":   "[defaults]\nnotify = \"attention\"\n",
		"worlds/a.toml": "host = \"h\"\nport = 1\nnotify = \"all\"\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n[[characters]]\nid = \"rook\"\nname = \"Rook\"\nnotify = \"none\"\n",
		"worlds/b.toml": "host = \"h\"\nport = 1\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n",
	})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		world, char string
		want        notify.Level
	}{{"a", "kit", notify.All}, {"a", "rook", notify.None}, {"b", "ash", notify.Attention}} {
		ch, _ := cfg.Find(c.world, c.char)
		if ch.Notify != c.want {
			t.Errorf("%s/%s Notify = %q, want %q", c.world, c.char, ch.Notify, c.want)
		}
	}
}

func TestNotifyGlobalDefaults(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"worlds/b.toml": "host = \"h\"\nport = 1\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n"})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	ash, _ := cfg.Find("b", "ash")
	if ash.Notify != notify.First || cfg.NotifyIdle != 5*time.Minute || cfg.NotifyMethod != notify.OSC {
		t.Errorf("notify %q, idle %v, method %q", ash.Notify, cfg.NotifyIdle, cfg.NotifyMethod)
	}
}

func TestNotifyGlobalKeys(t *testing.T) {
	for _, c := range []struct {
		toml   string
		idle   time.Duration
		method notify.Method
		bad    bool
	}{
		{"notify_idle = \"90s\"\nnotify_method = \"both\"\n", 90 * time.Second, notify.Both, false},
		{"notify_idle = \"0\"\n", 0, notify.OSC, false},
		{"notify_idle = \"-1m\"\n", 0, "", true},
		{"notify_idle = \"soon\"\n", 0, "", true},
		{"notify_method = \"smoke\"\n", 0, "", true},
	} {
		dir := t.TempDir()
		write(t, dir, map[string]string{"config.toml": c.toml})
		cfg, err := Load(dir)
		if c.bad {
			if err == nil {
				t.Errorf("%q: loaded", c.toml)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.toml, err)
			continue
		}
		if cfg.NotifyIdle != c.idle || cfg.NotifyMethod != c.method {
			t.Errorf("%q: idle %v, method %q", c.toml, cfg.NotifyIdle, cfg.NotifyMethod)
		}
	}
}

// scroll_lines defaults to 1, and anything but a whole number of at
// least 1 falls back to that rather than failing the load.
func TestScrollLines(t *testing.T) {
	for toml, want := range map[string]int{
		"":                       1,
		"scroll_lines = 3\n":     3,
		"scroll_lines = 0\n":     1,
		"scroll_lines = -2\n":    1,
		"scroll_lines = 2.5\n":   1,
		"scroll_lines = \"3\"\n": 1,
	} {
		dir := t.TempDir()
		write(t, dir, map[string]string{"config.toml": toml})
		cfg, err := Load(dir)
		if err != nil {
			t.Errorf("%q: %v", toml, err)
		} else if cfg.ScrollLines != want {
			t.Errorf("%q: scroll_lines %d, want %d", toml, cfg.ScrollLines, want)
		}
	}
}

func TestNotifyLevelValidated(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"worlds/a.toml": "host = \"h\"\nport = 1\nnotify = \"loud\"\n[[characters]]\nid = \"kit\"\nname = \"Kit\"\n"})
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "notify must be") {
		t.Errorf("err = %v", err)
	}
}

func TestNotifyIdleNumberIsFriendly(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"config.toml": "notify_idle = 300\n"})
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), `notify_idle must be a duration like "5m"`) {
		t.Errorf("err = %v", err)
	}
}
func TestAttentionAndQuietAddUp(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o700)
	os.MkdirAll(filepath.Join(dir, "packs"), 0o700)
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[defaults]\nattention = [\"self\"]\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "packs", "p.toml"), []byte("attention = [\"page/in\", \"self\"]\nquiet = [\"spam\"]\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(`host = "h"
port = 1
use = ["p"]
attention = ["whisper/in"]

[[characters]]
name = "Kit"
quiet = ["wiki"]
`), 0o600)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "Kit")
	if want := []string{"self", "page/in", "whisper/in"}; !slices.Equal(kit.Rules.Attention, want) {
		t.Errorf("attention = %v, want %v", kit.Rules.Attention, want)
	}
	if want := []string{"spam", "wiki"}; !slices.Equal(kit.Rules.Quiet, want) {
		t.Errorf("quiet = %v, want %v", kit.Rules.Quiet, want)
	}
}

func TestWorldAndCharacterLooks(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o700)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte(`host = "h"
port = 1

[palette]
beacon = "#ffd166"

[tags]
highlight = { fg = "beacon", scope = "match" }

[[characters]]
name = "Kit"
tags = { highlight = { bold = true } }

[[characters]]
name = "Rook"
`), 0o600)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "Kit")
	rook, _ := cfg.Find("fm", "Rook")
	if len(kit.Looks) != 2 || len(rook.Looks) != 1 {
		t.Fatalf("looks: kit %d, rook %d; want 2 and 1", len(kit.Looks), len(rook.Looks))
	}
	th, err := theme.Builtin().With(kit.Looks...)
	if err != nil {
		t.Fatal(err)
	}
	if _, ts, _ := th.Tag("highlight"); ts.Style.SGR() != "\x1b[1;38;2;255;209;102m" || !ts.Match {
		t.Errorf("kit's highlight = %q, match %v", ts.Style.SGR(), ts.Match)
	}
}

func TestBadWorldTagsFailLoad(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o700)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\n[tags]\npage = \"red\"\n"), 0o600)
	if _, err := Load(dir); err == nil || err.Error() != str.ThemeTagNotTable(filepath.Join("worlds", "fm.toml"), "page") {
		t.Errorf("err = %v", err)
	}
}

func TestHighlightIsGone(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worlds"), 0o700)
	os.WriteFile(filepath.Join(dir, "worlds", "fm.toml"), []byte("host = \"h\"\nport = 1\n[[highlight]]\nmatch = { pattern = 'x' }\n"), 0o600)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "fm.toml") || !strings.Contains(err.Error(), "highlight") {
		t.Errorf("err = %v, want the unknown-key error naming fm.toml and highlight", err)
	}
}

// Only worlds and characters have looks: a [tags] table in a pack or
// config.toml is an unknown key, not silently ignored.
func TestLooksOnlyInWorlds(t *testing.T) {
	for _, c := range []struct{ file, body, key string }{
		{"packs/p.toml", "[tags.page]\nfg = \"red\"\n", "tags.page"},
		{"config.toml", "[tags.page]\nfg = \"red\"\n", "tags.page"},
		{"config.toml", "[palette.x]\ny = 1\n", "palette.x"},
	} {
		dir := t.TempDir()
		write(t, dir, map[string]string{c.file: c.body, "worlds/fm.toml": "host = \"h\"\nport = 1\nuse = [\"p\"]\n", "packs/p.toml": ""})
		write(t, dir, map[string]string{c.file: c.body})
		if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), str.ConfigUnknownKey(filepath.Base(c.file), c.key)) {
			t.Errorf("%s with %q: err = %v, want an unknown key", c.file, c.body, err)
		}
	}
}

func TestThemeAndAppearance(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil || cfg.Theme != "default" || cfg.Appearance != "auto" {
		t.Fatalf("defaults: %v", err)
	}
	write(t, dir, map[string]string{"config.toml": "theme = \"ember\"\nappearance = \"light\"\n"})
	if cfg, err = Load(dir); err != nil || cfg.Theme != "ember" || cfg.Appearance != "light" {
		t.Errorf("set: %+v %v", cfg, err)
	}
	write(t, dir, map[string]string{"config.toml": "appearance = \"dusk\"\n"})
	if _, err := Load(dir); err == nil || err.Error() != str.ConfigBadAppearance() {
		t.Errorf("bad appearance: %v", err)
	}
}
