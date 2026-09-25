# Kiln Core Engine Implementation Plan (Plan 1 of 3)

> **Superseded detail:** the final review of this plan changed `Session.Send(line) error` to `Send(line) (logstore.Entry, error)`. It never emits events, and it returns the logged entry for the caller to echo. Plan 2 (`2026-09-25-kiln-tui.md`) builds on the new signature. The code blocks below show the original.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Kiln's headless engine (config, logs, telnet/TLS connections, sessions, classification, highlighting) plus a `kiln tail` command that makes it a working, if plain, MUCK client.

**Architecture:** Small `internal/` packages with one job each. Everything after `session` is a pure function over lines. `session` owns one character's connection lifecycle and logs every line *before* emitting it as an event. `cmd/kiln` wires the packages together. Plans 2 (TUI) and 3 (browse mode) consume these packages unchanged.

**Tech Stack:** Go 1.27; `github.com/BurntSushi/toml` v1.6.0; `github.com/zalando/go-keyring` v0.2.8; `golang.org/x/term` v0.46.0. The standard library covers TLS, x509, and networking.

**Spec:** `docs/superpowers/specs/2026-09-24-kiln-design.md`

**Scope split:** The spec is delivered in three plans, and each produces working software.

- **Plan 1 (this one):** spec §3 (every package except `ui`), §4 (minus hot-reload and write-back), §5, §7, and the connection, log, and pinning rows of §8.
- **Plan 2 (TUI):** the §6 layout, sidebar, scrollback, input box (growth, newline modes, over-limit highlight), statusline, the masked password prompt, config hot-reload and write-back, `/trust`, and `/reconnect`.
- **Plan 3 (browse mode):** disk paging, filters, range selection, and export.

**Verification note:** every code block in this plan was compiled and passed `go vet` and `go test -race` on Go 1.27.1 before the plan was written, and `kiln tail` was smoke-tested against FurryMUCK's TLS port.

## Global Constraints

- Module path is `kiln`. All packages live under `internal/`, except `cmd/kiln`.
- Config lives in `$XDG_CONFIG_HOME/kiln`, else `~/.config/kiln`. Data lives in `$XDG_DATA_HOME/kiln`, else `~/.local/share/kiln`.
- Log path is `<data>/logs/<world>/<char>/YYYY-MM-DD.log`. The first line is exactly `#kiln-log v1`. Each entry is `<RFC3339 millis with numeric offset> <dir>\t<raw>`, with dir one of `<` `>` `*`. Raw text is never escaped, and tags are never stored.
- Default `max_line_bytes = 2047` (Fuzzball's `MAX_COMMAND_LEN` of 2048, minus the NUL). Default `newline_mode = "batch"`.
- `tls_trust` is `"pin"` (the default, trust-on-first-use) or `"ca"`. Pins live in `<data>/known_hosts` as `host:port sha256:<hex>`.
- Passwords live only in the OS keychain (service `kiln`, account `<world>/<char>`). A logged login line always has `***` in place of the password.
- Log files are `0600`, and log directories are `0700`.
- Reconnect backoff is 1s doubling, capped at 60s. A pin mismatch and a user `QUIT` do not auto-reconnect.
- Commits end with the Co-Authored-By and Claude-Session trailers shown in each commit step.

## Review Focus

These are the input classes most likely to bite a real user, the ones a happy-path test wouldn't catch. Each is pinned by a test in its owning task.

1. **Non-UTF-8 servers:** Latin-1 bytes must display as the right characters (`été`), not mojibake or a crash. Pinned by Task 8 `TestSplitterLatin1Fallback` and Task 9 `TestPlainConnReceivesLinesAndNegotiates`.
2. **Bytes split across TCP reads:** IAC sequences and multi-byte UTF-8 split mid-sequence must reassemble. Pinned by Task 7 `TestSequencesSplitAcrossFeeds` and Task 8 `TestSplitterUTF8SplitAcrossPushes`.
3. **Awkward character names:** regex metacharacters (`K.i.t`, `(Ash)`) and non-ASCII (`Zoë`) must self-match literally, and substrings (`Kitten`, `Kitë`) must not. Pinned by Task 5 `TestSelfNamesAreLiteralAndUnicode`.
4. **Password leakage:** a password containing `{name}`, spaces, or `$1` must go over the wire exactly as typed and never appear in the log. Pinned by Task 10 `TestLoginRedactsPasswordInLog`.
5. **A deliberate `QUIT`:** the server hangs up in response, and the client must not auto-reconnect. Found during the smoke test. Pinned by Task 10 `TestQuitDoesNotReconnect`.

Also pinned, just below the top five: midnight rollover and crash-truncated log lines (Task 2 `TestWriterRollsOverAtMidnight` and `TestReadDaySkipsCorruptLines`).

---

### Task 0: Project scaffold

**Files:**
- Create: `go.mod`, `go.sum`, `.gitignore`

- [ ] **Step 1: Initialize the module and dependencies**

```bash
go mod init kiln
go get github.com/BurntSushi/toml@v1.6.0 github.com/zalando/go-keyring@v0.2.8 golang.org/x/term@v0.46.0
printf '/kiln\n' > .gitignore
```

- [ ] **Step 2: Verify**

Run: `go version && cat go.mod`
Expected: `go1.27.x`. `go.mod` lists the three dependencies, marked `// indirect` until code imports them (Task 11's `go mod tidy` fixes that).

- [ ] **Commit**

```bash
git add go.mod go.sum .gitignore
git commit -m "chore: initialize Go module

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 1: Log line format

**Files:**
- Create: `internal/logstore/format.go`
- Test: `internal/logstore/format_test.go`

**Interfaces:**
- Produces: `logstore.Dir` (`In '<'`, `Out '>'`, `Sys '*'`), `logstore.Entry{Time time.Time; Dir Dir; Text string}`, `logstore.Header`, `logstore.Format(Entry) string`, `logstore.Parse(string) (Entry, error)`

- [ ] **Step 1: Write the failing tests**

`internal/logstore/format_test.go`:

```go
package logstore

import (
	"testing"
	"time"
)

func TestFormat(t *testing.T) {
	pdt := time.FixedZone("PDT", -7*3600)
	ts := time.Date(2026, 9, 24, 21, 14, 3, 120_000_000, pdt)
	cases := []struct {
		name string
		in   Entry
		want string
	}{
		{"received", Entry{ts, In, `Rook says, "Evening!"`}, "2026-09-24T21:14:03.120-07:00 <\tRook says, \"Evening!\""},
		{"sent", Entry{ts, Out, ":grins."}, "2026-09-24T21:14:03.120-07:00 >\t:grins."},
		{"sys", Entry{ts, Sys, "connected"}, "2026-09-24T21:14:03.120-07:00 *\tconnected"},
		{"utc keeps numeric offset", Entry{ts.UTC(), In, "x"}, "2026-09-25T04:14:03.120+00:00 <\tx"},
		{"ansi and tabs untouched", Entry{ts, In, "\x1b[1mA\x1b[0m\tB"}, "2026-09-24T21:14:03.120-07:00 <\t\x1b[1mA\x1b[0m\tB"},
		{"newlines flattened", Entry{ts, Sys, "a\nb\r\nc"}, "2026-09-24T21:14:03.120-07:00 *\ta b c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Format(c.in); got != c.want {
				t.Errorf("Format() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestParseRoundTrip(t *testing.T) {
	pdt := time.FixedZone("PDT", -7*3600)
	in := Entry{time.Date(2026, 9, 24, 21, 14, 3, 120_000_000, pdt), In, "Mira pages: \"you\taround?\" \x1b[0m"}
	got, err := Parse(Format(in))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Time.Equal(in.Time) || got.Dir != in.Dir || got.Text != in.Text {
		t.Errorf("round trip: got %+v, want %+v", got, in)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	for _, line := range []string{
		"",
		"no tab here",
		"2026-09-24T21:14:03.120-07:00 <",    // no tab
		"2026-09-24T21:14:03.120-07:00 ?\tx", // bad dir
		"yesterday <\tx",                     // bad time
		"2026-09-24T21:14:0",                 // truncated by crash
	} {
		if _, err := Parse(line); err == nil {
			t.Errorf("Parse(%q) = nil error, want error", line)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/logstore/`
Expected: FAIL with build errors such as `undefined: Entry`.

- [ ] **Step 3: Write the implementation**

`internal/logstore/format.go`:

```go
// Package logstore reads and writes Kiln's plain-text log files.
//
// Each file holds one character's traffic for one local day:
//
//	#kiln-log v1
//	2026-09-24T21:14:03.120-07:00 <	Rook says, "Evening!"
//
// The prefix is fixed-width; everything after the first TAB is the raw
// line, unescaped, with ANSI bytes intact.
package logstore

import (
	"fmt"
	"strings"
	"time"
)

// Dir is the direction of a logged line.
type Dir byte

const (
	In  Dir = '<' // received from the server
	Out Dir = '>' // sent by the user
	Sys Dir = '*' // client/system event
)

// Header is the first line of every log file.
const Header = "#kiln-log v1"

// timeLayout always renders a numeric offset (never "Z") so the prefix
// stays fixed-width.
const timeLayout = "2006-01-02T15:04:05.000-07:00"

// Entry is one logged line.
type Entry struct {
	Time time.Time
	Dir  Dir
	Text string
}

// Format renders e as a single log line without a trailing newline.
// Any CR/LF inside Text is replaced with a space so one entry is always
// one line.
func Format(e Entry) string {
	text := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(e.Text)
	return e.Time.Format(timeLayout) + " " + string(rune(e.Dir)) + "\t" + text
}

// Parse is the inverse of Format.
func Parse(line string) (Entry, error) {
	tab := strings.IndexByte(line, '\t')
	if tab < 0 {
		return Entry{}, fmt.Errorf("logstore: no tab in %q", line)
	}
	prefix, text := line[:tab], line[tab+1:]
	sp := strings.LastIndexByte(prefix, ' ')
	if sp < 0 || sp != len(prefix)-2 {
		return Entry{}, fmt.Errorf("logstore: bad prefix %q", prefix)
	}
	t, err := time.Parse(timeLayout, prefix[:sp])
	if err != nil {
		return Entry{}, fmt.Errorf("logstore: bad time: %w", err)
	}
	d := Dir(prefix[sp+1])
	if d != In && d != Out && d != Sys {
		return Entry{}, fmt.Errorf("logstore: bad direction %q", prefix[sp+1])
	}
	return Entry{Time: t, Dir: d, Text: text}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/logstore/ && go test ./internal/logstore/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/logstore/format_test.go internal/logstore/format.go
git commit -m "feat(logstore): fixed-prefix log line format

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 2: Day-file writer and reader

**Files:**
- Create: `internal/logstore/store.go`
- Test: `internal/logstore/store_test.go`

**Interfaces:**
- Consumes: `Entry`, `Format`, `Parse`, `Header` (Task 1)
- Produces: `logstore.NewWriter(root, world, char string) *Writer`, `(*Writer).Append(Entry) error`, `(*Writer).Close() error`, `logstore.Days(root, world, char string) ([]string, error)` (oldest first), `logstore.ReadDay(root, world, char, day string) ([]Entry, error)`, `logstore.CharDir(root, world, char string) string`

- [ ] **Step 1: Write the failing tests**

`internal/logstore/store_test.go`:

```go
package logstore

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWriterCreatesFileWithHeader(t *testing.T) {
	root := t.TempDir()
	w := NewWriter(root, "furrymuck", "kit")
	pdt := time.FixedZone("PDT", -7*3600)
	if err := w.Append(Entry{time.Date(2026, 9, 24, 21, 0, 0, 0, pdt), In, "hello"}); err != nil {
		t.Fatal(err)
	}
	w.Close()
	b, err := os.ReadFile(filepath.Join(root, "furrymuck", "kit", "2026-09-24.log"))
	if err != nil {
		t.Fatal(err)
	}
	want := Header + "\n2026-09-24T21:00:00.000-07:00 <\thello\n"
	if string(b) != want {
		t.Errorf("file = %q, want %q", b, want)
	}
}

func TestWriterAppendsWithoutSecondHeader(t *testing.T) {
	root := t.TempDir()
	ts := time.Date(2026, 9, 24, 21, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ { // two separate writers = two app runs
		w := NewWriter(root, "w", "c")
		if err := w.Append(Entry{ts, In, "line"}); err != nil {
			t.Fatal(err)
		}
		w.Close()
	}
	b, _ := os.ReadFile(filepath.Join(root, "w", "c", "2026-09-24.log"))
	if n := strings.Count(string(b), Header); n != 1 {
		t.Errorf("header count = %d, want 1\n%s", n, b)
	}
}

func TestWriterRollsOverAtMidnight(t *testing.T) {
	root := t.TempDir()
	w := NewWriter(root, "w", "c")
	defer w.Close()
	pdt := time.FixedZone("PDT", -7*3600)
	w.Append(Entry{time.Date(2026, 9, 24, 23, 59, 59, 0, pdt), In, "before"})
	w.Append(Entry{time.Date(2026, 9, 25, 0, 0, 1, 0, pdt), In, "after"})
	days, err := Days(root, "w", "c")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2026-09-24", "2026-09-25"}; !reflect.DeepEqual(days, want) {
		t.Fatalf("Days = %v, want %v", days, want)
	}
	got, _ := ReadDay(root, "w", "c", "2026-09-25")
	if len(got) != 1 || got[0].Text != "after" {
		t.Errorf("2026-09-25 entries = %+v", got)
	}
}

func TestDaysMissingDirIsEmpty(t *testing.T) {
	days, err := Days(t.TempDir(), "nope", "nobody")
	if err != nil || len(days) != 0 {
		t.Errorf("Days = %v, %v; want empty, nil", days, err)
	}
}

func TestReadDaySkipsCorruptLines(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "w", "c")
	os.MkdirAll(dir, 0o700)
	content := Header + "\n" +
		"2026-09-24T21:00:00.000-07:00 <\tgood one\n" +
		"2026-09-24T21:0\n" + // crash-truncated
		"2026-09-24T21:00:02.000-07:00 >\tgood two\n"
	os.WriteFile(filepath.Join(dir, "2026-09-24.log"), []byte(content), 0o600)
	got, err := ReadDay(root, "w", "c", "2026-09-24")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Text != "good one" || got[1].Text != "good two" {
		t.Errorf("entries = %+v", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/logstore/`
Expected: FAIL with build errors such as `undefined: NewWriter`.

- [ ] **Step 3: Write the implementation**

`internal/logstore/store.go`:

```go
package logstore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const dayLayout = "2006-01-02"

// CharDir is the directory holding one character's day files.
func CharDir(root, world, char string) string {
	return filepath.Join(root, world, char)
}

// Writer appends entries to per-day files under CharDir. The day is taken
// from each entry's own timestamp (in its own location), so a session
// running past midnight rolls over to a new file automatically.
type Writer struct {
	dir string
	day string
	f   *os.File
}

// NewWriter returns a Writer for one character. No files are touched
// until the first Append.
func NewWriter(root, world, char string) *Writer {
	return &Writer{dir: CharDir(root, world, char)}
}

// Append writes e to the day file for e.Time, creating it (with Header)
// if needed.
func (w *Writer) Append(e Entry) error {
	day := e.Time.Format(dayLayout)
	if w.f == nil || day != w.day {
		if err := w.open(day); err != nil {
			return err
		}
	}
	_, err := w.f.WriteString(Format(e) + "\n")
	return err
}

func (w *Writer) open(day string) error {
	if w.f != nil {
		w.f.Close()
		w.f = nil
	}
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(w.dir, day+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	if st.Size() == 0 {
		if _, err := f.WriteString(Header + "\n"); err != nil {
			f.Close()
			return err
		}
	}
	w.f, w.day = f, day
	return nil
}

// Close closes the current file, if any.
func (w *Writer) Close() error {
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}

// Days lists the days that have log files for a character, oldest first,
// as "YYYY-MM-DD" strings. A missing directory yields no days, not an error.
func Days(root, world, char string) ([]string, error) {
	ents, err := os.ReadDir(CharDir(root, world, char))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var days []string
	for _, e := range ents {
		name, ok := strings.CutSuffix(e.Name(), ".log")
		if !ok || e.IsDir() {
			continue
		}
		days = append(days, name)
	}
	sort.Strings(days)
	return days, nil
}

// ReadDay reads every entry from one day file. Lines that fail to parse
// (e.g. a line cut short by a crash) are skipped rather than failing the
// whole read.
func ReadDay(root, world, char, day string) ([]Entry, error) {
	f, err := os.Open(filepath.Join(CharDir(root, world, char), day+".log"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		e, err := Parse(line)
		if err != nil {
			continue
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("logstore: reading %s: %w", day, err)
	}
	return out, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/logstore/ && go test ./internal/logstore/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/logstore/store_test.go internal/logstore/store.go
git commit -m "feat(logstore): per-day writer with midnight rollover and tolerant reader

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 3: Config loading and inheritance

**Files:**
- Create: `internal/config/config.go`, `internal/config/paths.go`, `internal/config/defaults/config.toml`, `internal/config/defaults/packs/fuzzball.toml`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Style{FG, BG string; Bold, Italic, Underline bool}`, `config.ClassifyRule{Tag, Pattern string}`, `config.Match{Tags []string; Pattern string}`, `config.HighlightRule{Match Match; Style Style; Attention bool}`, `config.Rules{Classify []ClassifyRule; Highlight []HighlightRule}`, `config.Character{World, ID, Name string; Aliases []string; Host string; Port int; TLS bool; TLSTrust, Login string; MaxLineBytes int; NewlineMode string; Rules Rules}`, `config.World{ID string; Characters []Character}`, `config.Config{Worlds []World}`, `(*Config).Find(world, char string) (Character, bool)`, `config.Load(dir string) (*Config, error)`, `config.EnsureDefaults(dir string) error`, `config.Dir() (string, error)`, `config.DataDir() (string, error)`, `config.DefaultMaxLineBytes`, `config.DefaultNewlineMode`

> The fuzzball pack patterns come from Fuzzball's `src/speech.c` (`says, "…"`, `whispers, "…"`, `pages from X: "…"`) and the stock `cmd-page` MUF in `dbs/starterdb/muf/101.m` (`pages: `, `pages, "…" to`, `In a page-pose to`). Individual MUCKs customize these, which is why they're starter rules.
>
> BurntSushi/toml flattens embedded structs (`settings`, `Rules`) the way `encoding/json` does, so world and character files carry those keys at their own level.

- [ ] **Step 1: Write the failing tests**

`internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

[characters.kit]
name = "Kit"
aliases = ["Kitty"]

[characters.rook]
name = "Rook"
max_line_bytes = 500
login = "co {name} {password}"

[[characters.rook.classify]]
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
		{"missing name", "host = \"h\"\nport = 1\n[characters.kit]\n", "name is required"},
		{"unknown key", "host = \"h\"\nport = 1\nhots = \"typo\"\n", `unknown key "hots"`},
		{"unknown pack", "host = \"h\"\nport = 1\nuse = [\"nope\"]\n", `unknown pack "nope"`},
		{"bad regex", "host = \"h\"\nport = 1\n[[classify]]\ntag = \"x\"\npattern = '('\n[characters.kit]\nname = \"Kit\"\n", "classify rule 1"},
		{"empty highlight match", "host = \"h\"\nport = 1\n[[highlight]]\nattention = true\n[characters.kit]\nname = \"Kit\"\n", "match needs tags or pattern"},
		{"bad trust", "host = \"h\"\nport = 1\ntls_trust = \"yolo\"\n", "tls_trust"},
		{"bad newline mode", "host = \"h\"\nport = 1\nnewline_mode = \"x\"\n[characters.kit]\nname = \"Kit\"\n", "newline_mode"},
		{"bad char id", "host = \"h\"\nport = 1\n[characters.\"a/b\"]\nname = \"X\"\n", "id may only use"},
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
	for _, rel := range []string{"config.toml", "packs/fuzzball.toml"} {
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

func TestStarterPackLoads(t *testing.T) {
	dir := t.TempDir()
	EnsureDefaults(dir)
	write(t, dir, map[string]string{"worlds/fm.toml": "host = \"h\"\nport = 1\nuse = [\"fuzzball\"]\n[characters.kit]\nname = \"Kit\"\n"})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	kit, _ := cfg.Find("fm", "kit")
	if len(kit.Rules.Classify) == 0 || len(kit.Rules.Highlight) == 0 {
		t.Errorf("fuzzball pack rules missing: %+v", kit.Rules)
	}
	if kit.MaxLineBytes != 2047 {
		t.Errorf("MaxLineBytes = %d, want 2047", kit.MaxLineBytes)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL with build errors such as `undefined: Load`.

- [ ] **Step 3: Write the implementation**

`internal/config/config.go`:

```go
// Package config loads Kiln's config directory:
//
//	<dir>/config.toml        [defaults] + global prefs
//	<dir>/worlds/<id>.toml   one world and its characters
//	<dir>/packs/<id>.toml    reusable classify/highlight rule sets
//
// and resolves inheritance: defaults → packs (in `use` order) → world →
// character. Rule lists append along that chain; scalars override.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Style is how a highlighted line is drawn. Empty colors mean "unchanged".
type Style struct {
	FG        string `toml:"fg"`
	BG        string `toml:"bg"`
	Bold      bool   `toml:"bold"`
	Italic    bool   `toml:"italic"`
	Underline bool   `toml:"underline"`
}

// ClassifyRule tags lines whose plain text matches Pattern.
type ClassifyRule struct {
	Tag     string `toml:"tag"`
	Pattern string `toml:"pattern"`
}

// Match selects lines for a highlight rule. A line matches when it has at
// least one of Tags (if any are given) AND matches Pattern (if given).
type Match struct {
	Tags    []string `toml:"tags"`
	Pattern string   `toml:"pattern"`
}

// HighlightRule styles matching lines and optionally flags them for attention.
type HighlightRule struct {
	Match     Match `toml:"match"`
	Style     Style `toml:"style"`
	Attention bool  `toml:"attention"`
}

// Rules is the rule set carried by packs, worlds, and characters.
type Rules struct {
	Classify  []ClassifyRule  `toml:"classify"`
	Highlight []HighlightRule `toml:"highlight"`
}

// Character is a fully resolved character: everything a session needs.
type Character struct {
	World        string // world id (filename without .toml)
	ID           string // key under [characters]
	Name         string // canonical in-game name
	Aliases      []string
	Host         string
	Port         int
	TLS          bool
	TLSTrust     string // "pin" or "ca"
	Login        string // template with {name} and {password}; "" = no auto-login
	MaxLineBytes int
	NewlineMode  string // "batch" or "flatten"
	Rules        Rules
}

// World groups resolved characters under their world id.
type World struct {
	ID         string
	Characters []Character // sorted by ID
}

// Config is the resolved configuration.
type Config struct {
	Worlds []World // sorted by ID
}

// Find returns the resolved character, or false.
func (c *Config) Find(world, char string) (Character, bool) {
	for _, w := range c.Worlds {
		if w.ID != world {
			continue
		}
		for _, ch := range w.Characters {
			if ch.ID == char {
				return ch, true
			}
		}
	}
	return Character{}, false
}

// settings are the inheritable scalars. Pointers distinguish "unset" from
// zero values so later levels only override what they actually set.
type settings struct {
	MaxLineBytes *int    `toml:"max_line_bytes"`
	NewlineMode  *string `toml:"newline_mode"`
	Login        *string `toml:"login"`
}

func (s *settings) overlay(o settings) {
	if o.MaxLineBytes != nil {
		s.MaxLineBytes = o.MaxLineBytes
	}
	if o.NewlineMode != nil {
		s.NewlineMode = o.NewlineMode
	}
	if o.Login != nil {
		s.Login = o.Login
	}
}

type globalFile struct {
	Defaults settings `toml:"defaults"`
}

type charFile struct {
	settings
	Rules
	Name    string   `toml:"name"`
	Aliases []string `toml:"aliases"`
}

type worldFile struct {
	settings
	Rules
	Host       string              `toml:"host"`
	Port       int                 `toml:"port"`
	TLS        bool                `toml:"tls"`
	TLSTrust   string              `toml:"tls_trust"`
	Use        []string            `toml:"use"`
	Characters map[string]charFile `toml:"characters"`
}

// Built-in defaults, applied beneath config.toml's [defaults].
const (
	DefaultMaxLineBytes = 2047 // Fuzzball MAX_COMMAND_LEN (2048) minus the NUL
	DefaultNewlineMode  = "batch"
)

var idRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Load reads and resolves the config directory. config.toml and the
// worlds/ and packs/ directories are all optional.
func Load(dir string) (*Config, error) {
	var g globalFile
	if err := decodeFile(filepath.Join(dir, "config.toml"), &g, true); err != nil {
		return nil, err
	}
	base := settings{MaxLineBytes: ptr(DefaultMaxLineBytes), NewlineMode: ptr(DefaultNewlineMode), Login: ptr("")}
	base.overlay(g.Defaults)

	worldPaths, err := filepath.Glob(filepath.Join(dir, "worlds", "*.toml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(worldPaths)

	packs := map[string]Rules{}
	cfg := &Config{}
	for _, wp := range worldPaths {
		w, err := loadWorld(dir, wp, base, packs)
		if err != nil {
			return nil, err
		}
		cfg.Worlds = append(cfg.Worlds, w)
	}
	return cfg, nil
}

func loadWorld(dir, path string, base settings, packs map[string]Rules) (World, error) {
	id := strings.TrimSuffix(filepath.Base(path), ".toml")
	rel := filepath.Join("worlds", filepath.Base(path))
	if !idRE.MatchString(id) {
		return World{}, fmt.Errorf("%s: world id %q may only use letters, digits, _ and -", rel, id)
	}
	var wf worldFile
	if err := decodeFile(path, &wf, false); err != nil {
		return World{}, err
	}
	if wf.Host == "" {
		return World{}, fmt.Errorf("%s: host is required", rel)
	}
	if wf.Port <= 0 || wf.Port > 65535 {
		return World{}, fmt.Errorf("%s: port must be 1-65535", rel)
	}
	switch wf.TLSTrust {
	case "":
		wf.TLSTrust = "pin"
	case "pin", "ca":
	default:
		return World{}, fmt.Errorf("%s: tls_trust must be \"pin\" or \"ca\"", rel)
	}

	var worldRules Rules
	for _, p := range wf.Use {
		r, err := loadPack(dir, p, packs)
		if err != nil {
			return World{}, fmt.Errorf("%s: %w", rel, err)
		}
		worldRules = appendRules(worldRules, r)
	}
	worldRules = appendRules(worldRules, wf.Rules)
	ws := base
	ws.overlay(wf.settings)

	w := World{ID: id}
	charIDs := make([]string, 0, len(wf.Characters))
	for cid := range wf.Characters {
		charIDs = append(charIDs, cid)
	}
	sort.Strings(charIDs)
	for _, cid := range charIDs {
		cf := wf.Characters[cid]
		where := fmt.Sprintf("%s: characters.%s", rel, cid)
		if !idRE.MatchString(cid) {
			return World{}, fmt.Errorf("%s: id may only use letters, digits, _ and -", where)
		}
		if strings.TrimSpace(cf.Name) == "" {
			return World{}, fmt.Errorf("%s: name is required", where)
		}
		cs := ws
		cs.overlay(cf.settings)
		ch := Character{
			World: id, ID: cid, Name: cf.Name, Aliases: cf.Aliases,
			Host: wf.Host, Port: wf.Port, TLS: wf.TLS, TLSTrust: wf.TLSTrust,
			Login: *cs.Login, MaxLineBytes: *cs.MaxLineBytes, NewlineMode: *cs.NewlineMode,
			Rules: appendRules(worldRules, cf.Rules),
		}
		if err := validate(ch); err != nil {
			return World{}, fmt.Errorf("%s: %w", where, err)
		}
		w.Characters = append(w.Characters, ch)
	}
	return w, nil
}

func loadPack(dir, id string, cache map[string]Rules) (Rules, error) {
	if r, ok := cache[id]; ok {
		return r, nil
	}
	if !idRE.MatchString(id) {
		return Rules{}, fmt.Errorf("pack id %q may only use letters, digits, _ and -", id)
	}
	var r Rules
	path := filepath.Join(dir, "packs", id+".toml")
	if _, err := os.Stat(path); err != nil {
		return Rules{}, fmt.Errorf("unknown pack %q (no packs/%s.toml)", id, id)
	}
	if err := decodeFile(path, &r, false); err != nil {
		return Rules{}, err
	}
	cache[id] = r
	return r, nil
}

func validate(ch Character) error {
	if ch.MaxLineBytes <= 0 {
		return errors.New("max_line_bytes must be positive")
	}
	if ch.NewlineMode != "batch" && ch.NewlineMode != "flatten" {
		return errors.New(`newline_mode must be "batch" or "flatten"`)
	}
	for i, r := range ch.Rules.Classify {
		if r.Tag == "" {
			return fmt.Errorf("classify rule %d: tag is required", i+1)
		}
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("classify rule %d (%s): %w", i+1, r.Tag, err)
		}
	}
	for i, r := range ch.Rules.Highlight {
		if len(r.Match.Tags) == 0 && r.Match.Pattern == "" {
			return fmt.Errorf("highlight rule %d: match needs tags or pattern", i+1)
		}
		if _, err := regexp.Compile(r.Match.Pattern); err != nil {
			return fmt.Errorf("highlight rule %d: %w", i+1, err)
		}
	}
	return nil
}

// decodeFile decodes TOML into v, rejecting unknown keys so typos surface.
func decodeFile(path string, v any, optional bool) error {
	b, err := os.ReadFile(path)
	if optional && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	md, err := toml.Decode(string(b), v)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return fmt.Errorf("%s: unknown key %q", filepath.Base(path), und[0].String())
	}
	return nil
}

func appendRules(a, b Rules) Rules {
	return Rules{
		Classify:  append(append([]ClassifyRule(nil), a.Classify...), b.Classify...),
		Highlight: append(append([]HighlightRule(nil), a.Highlight...), b.Highlight...),
	}
}

func ptr[T any](v T) *T { return &v }
```

`internal/config/paths.go`:

```go
package config

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Dir is Kiln's config directory: $XDG_CONFIG_HOME/kiln, else ~/.config/kiln.
func Dir() (string, error) {
	return xdg("XDG_CONFIG_HOME", ".config")
}

// DataDir is Kiln's data directory: $XDG_DATA_HOME/kiln, else ~/.local/share/kiln.
func DataDir() (string, error) {
	return xdg("XDG_DATA_HOME", filepath.Join(".local", "share"))
}

func xdg(env, fallback string) (string, error) {
	if d := os.Getenv(env); d != "" {
		return filepath.Join(d, "kiln"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, fallback, "kiln"), nil
}

//go:embed defaults
var defaults embed.FS

// EnsureDefaults creates the config directory layout and writes the
// starter config.toml and packs/fuzzball.toml if they don't exist yet.
// Existing files are never overwritten.
func EnsureDefaults(dir string) error {
	for _, sub := range []string{"worlds", "packs"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return err
		}
	}
	return fs.WalkDir(defaults, "defaults", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel("defaults", path)
		dst := filepath.Join(dir, rel)
		if _, err := os.Stat(dst); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		b, err := defaults.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o600)
	})
}
```

`internal/config/defaults/config.toml`:

```toml
# Kiln global config. World files live in worlds/<id>.toml.

[defaults]
max_line_bytes = 2047   # Fuzzball's limit; override per world if yours differs
newline_mode = "batch"  # "batch": each line is its own command; "flatten": join with spaces
```

`internal/config/defaults/packs/fuzzball.toml`:

```toml
# Starter rules for Fuzzball MUCKs (built-in commands plus the stock
# cmd-page and cmd-whisper programs). Servers customize these heavily;
# edit freely.

[[classify]]
tag = "say"
pattern = '^(You say|\S+ says), "'

[[classify]]
tag = "whisper"
pattern = '^\S+ whispers,? "'

[[classify]]
tag = "page"
pattern = '^\S+ pages( from [^:]+)?: '

[[classify]]
tag = "page"
pattern = '^\S+ pages, ".*" to '

[[classify]]
tag = "page"
pattern = '^In a page-pose to '

[[classify]]
tag = "page"
pattern = '^You sense that \S+ is paging you'

[[highlight]]
match = { tags = ["page"] }
style = { fg = "#ff9f43", bold = true }
attention = true

[[highlight]]
match = { tags = ["whisper"] }
style = { fg = "#c39bd3", italic = true }
attention = true

[[highlight]]
match = { tags = ["self"] }
style = { bold = true }
attention = true
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/config/ && go test ./internal/config/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/config/config_test.go internal/config/config.go internal/config/paths.go internal/config/defaults/config.toml internal/config/defaults/packs/fuzzball.toml
git commit -m "feat(config): directory config with packs, inheritance, validation, starter fuzzball pack

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 4: ANSI stripping

**Files:**
- Create: `internal/ansi/strip.go`
- Test: `internal/ansi/strip_test.go`

**Interfaces:**
- Produces: `ansi.Strip(s string) string`, which classification and highlighting use on raw lines. (Plan 2 adds span parsing to this package.)

- [ ] **Step 1: Write the failing tests**

`internal/ansi/strip_test.go`:

```go
package ansi

import "testing"

func TestStrip(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"\x1b[1mRook\x1b[0m says, \"hi\"", "Rook says, \"hi\""},
		{"\x1b[38;2;255;159;67morange\x1b[m", "orange"},
		{"a\x1b]8;;http://x\x07link\x1b]8;;\x07b", "alinkb"},
		{"a\x1b]0;title\x1b\\b", "ab"},
		{"a\x1b(Bb", "ab"},
		{"Zoë 🦊 \x1b[31m日本\x1b[0m", "Zoë 🦊 日本"},
		{"cut\x1b[3", "cut"},
		{"cut\x1b", "cut"},
	}
	for _, c := range cases {
		if got := Strip(c.in); got != c.want {
			t.Errorf("Strip(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ansi/`
Expected: FAIL with build errors such as `undefined: Strip`.

- [ ] **Step 3: Write the implementation**

`internal/ansi/strip.go`:

```go
// Package ansi handles ANSI escape sequences in server output.
package ansi

import "strings"

// Strip removes ANSI escape sequences, leaving the visible text. It
// handles CSI (ESC [ … final), OSC (ESC ] … BEL or ESC \), and short ESC
// sequences with optional intermediates (ESC ( B). An unterminated
// sequence at the end is dropped.
func Strip(s string) string {
	if strings.IndexByte(s, 0x1b) < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 >= len(s) {
			break
		}
		switch s[i+1] {
		case '[': // CSI: parameters/intermediates, then a final byte 0x40–0x7E
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j + 1
		case ']': // OSC: terminated by BEL or ESC \
			j := i + 2
			for j < len(s) {
				if s[j] == 0x07 {
					j++
					break
				}
				if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
					j += 2
					break
				}
				j++
			}
			i = j
		default: // ESC, optional intermediates 0x20–0x2F (e.g. "(" in ESC ( B), final byte
			j := i + 1
			for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
				j++
			}
			i = j + 1
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/ansi/ && go test ./internal/ansi/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/ansi/strip_test.go internal/ansi/strip.go
git commit -m "feat(ansi): strip CSI/OSC/ESC sequences

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 5: Classifier with built-in self tag

**Files:**
- Create: `internal/classify/classify.go`
- Test: `internal/classify/classify_test.go`

**Interfaces:**
- Consumes: `config.ClassifyRule` (Task 3)
- Produces: `classify.SelfTag = "self"`, `classify.New(rules []config.ClassifyRule, name string, aliases []string) (*Classifier, error)`, `(*Classifier).Classify(plain string) []string` (rule order, deduped, `self` last, nil when none)

- [ ] **Step 1: Write the failing tests**

`internal/classify/classify_test.go`:

```go
package classify

import (
	"reflect"
	"testing"

	"kiln/internal/config"
)

func TestClassify(t *testing.T) {
	c, err := New([]config.ClassifyRule{
		{Tag: "page", Pattern: `^\S+ pages: `},
		{Tag: "page", Pattern: `^In a page-pose`},
		{Tag: "ooc", Pattern: `OOC`},
	}, "Kit", []string{"Kitty"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in   string
		want []string
	}{
		{"Rook says, \"Evening!\"", nil},
		{"Mira pages: you around?", []string{"page"}},
		{"In a page-pose to you, Mira grins.", []string{"page"}},
		{"Mira pages: OOC brb", []string{"page", "ooc"}},
		{"Rook waves to Kit.", []string{SelfTag}},
		{"Rook waves to kitty!", []string{SelfTag}},
		{"Mira pages: hi Kit", []string{"page", SelfTag}},
		{"Kit", []string{SelfTag}},
		{"Kitten wanders by.", nil},       // not a whole word
		{"The skit was funny.", nil},      // not a whole word
		{"Kitë is a different name", nil}, // non-ASCII letter continues the word
	}
	for _, tc := range cases {
		if got := c.Classify(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Classify(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSelfNamesAreLiteralAndUnicode(t *testing.T) {
	c, err := New(nil, "K.i.t", []string{"Zoë", "(Ash)"})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"hi K.i.t!":        true,
		"hi Kxixt":         false, // dots are literal, not wildcards
		"Zoë waves.":       true,
		"ZOË waves.":       true,
		"hello (Ash) here": true,
	}
	for in, want := range cases {
		got := len(c.Classify(in)) == 1
		if got != want {
			t.Errorf("self match %q = %v, want %v", in, got, want)
		}
	}
}

func TestNoNamesNoSelf(t *testing.T) {
	c, _ := New(nil, "", nil)
	if got := c.Classify("anything"); got != nil {
		t.Errorf("got %v", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/classify/`
Expected: FAIL with build errors such as `undefined: New`.

- [ ] **Step 3: Write the implementation**

`internal/classify/classify.go`:

```go
// Package classify tags lines with semantic kinds (page, whisper, …).
package classify

import (
	"regexp"
	"strings"

	"kiln/internal/config"
)

// SelfTag is added to any line that mentions the character's own name
// or one of its aliases.
const SelfTag = "self"

// Classifier holds one character's compiled classify rules.
type Classifier struct {
	rules []rule
	self  *regexp.Regexp // nil if no names
}

type rule struct {
	tag string
	re  *regexp.Regexp
}

// New compiles rules plus the built-in self rule for name and aliases.
func New(rules []config.ClassifyRule, name string, aliases []string) (*Classifier, error) {
	c := &Classifier{}
	for _, r := range rules {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, err
		}
		c.rules = append(c.rules, rule{r.Tag, re})
	}
	var names []string
	for _, n := range append([]string{name}, aliases...) {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, regexp.QuoteMeta(n))
		}
	}
	if len(names) > 0 {
		// Unicode-aware word boundaries: Go's \b is ASCII-only, which
		// would miss names like "Zoë".
		c.self = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_])(?:` + strings.Join(names, "|") + `)(?:$|[^\p{L}\p{N}_])`)
	}
	return c, nil
}

// Classify returns the tags for plain (ANSI-stripped) text, in rule order,
// without duplicates, with SelfTag last.
func (c *Classifier) Classify(plain string) []string {
	var tags []string
	seen := map[string]bool{}
	for _, r := range c.rules {
		if !seen[r.tag] && r.re.MatchString(plain) {
			seen[r.tag] = true
			tags = append(tags, r.tag)
		}
	}
	if c.self != nil && !seen[SelfTag] && c.self.MatchString(plain) {
		tags = append(tags, SelfTag)
	}
	return tags
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/classify/ && go test ./internal/classify/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/classify/classify_test.go internal/classify/classify.go
git commit -m "feat(classify): semantic tags plus unicode-aware self tag

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 6: Highlight rules

**Files:**
- Create: `internal/rules/rules.go`
- Test: `internal/rules/rules_test.go`

**Interfaces:**
- Consumes: `config.HighlightRule`, `config.Style` (Task 3)
- Produces: `rules.New(rules []config.HighlightRule) (*Highlighter, error)`, `(*Highlighter).Apply(plain string, tags []string) rules.Result`, `rules.Result{Style config.Style; Styled bool; Attention bool}`

- [ ] **Step 1: Write the failing tests**

`internal/rules/rules_test.go`:

```go
package rules

import (
	"testing"

	"kiln/internal/config"
)

func TestApply(t *testing.T) {
	h, err := New([]config.HighlightRule{
		{Match: config.Match{Tags: []string{"page"}}, Style: config.Style{FG: "#ff9f43", Bold: true}, Attention: true},
		{Match: config.Match{Pattern: `lighthouse`}, Style: config.Style{FG: "#00ffff", Underline: true}},
		{Match: config.Match{Tags: []string{"whisper", "self"}}, Style: config.Style{Italic: true}},
		{Match: config.Match{Tags: []string{"page"}, Pattern: `urgent`}, Style: config.Style{BG: "#330000"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		plain string
		tags  []string
		want  Result
	}{
		{"no match", "Rook waves.", nil, Result{}},
		{"tag match", "Mira pages: hi", []string{"page"}, Result{config.Style{FG: "#ff9f43", Bold: true}, true, true}},
		{"pattern match", "the lighthouse glows", nil, Result{config.Style{FG: "#00ffff", Underline: true}, true, false}},
		{"any-of tags", "Rook waves to Kit.", []string{"self"}, Result{config.Style{Italic: true}, true, false}},
		{"later color overrides, flags accumulate", "Mira pages: the lighthouse", []string{"page"},
			Result{config.Style{FG: "#00ffff", Bold: true, Underline: true}, true, true}},
		{"tags AND pattern: pattern missing", "Mira pages: hi", []string{"page"}, Result{config.Style{FG: "#ff9f43", Bold: true}, true, true}},
		{"tags AND pattern: both", "Mira pages: urgent", []string{"page"}, Result{config.Style{FG: "#ff9f43", BG: "#330000", Bold: true}, true, true}},
		{"tags AND pattern: tag missing", "urgent news", nil, Result{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := h.Apply(c.plain, c.tags); got != c.want {
				t.Errorf("Apply = %+v, want %+v", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/rules/`
Expected: FAIL with build errors such as `undefined: New`.

- [ ] **Step 3: Write the implementation**

`internal/rules/rules.go`:

```go
// Package rules applies highlight rules to classified lines.
package rules

import (
	"regexp"
	"slices"

	"kiln/internal/config"
)

// Highlighter holds one character's compiled highlight rules.
type Highlighter struct {
	rules []compiled
}

type compiled struct {
	tags      []string
	re        *regexp.Regexp // nil = no pattern constraint
	style     config.Style
	attention bool
}

// Result is the combined effect of every matching rule.
type Result struct {
	Style     config.Style
	Styled    bool // at least one rule matched
	Attention bool
}

// New compiles rules.
func New(rules []config.HighlightRule) (*Highlighter, error) {
	h := &Highlighter{}
	for _, r := range rules {
		c := compiled{tags: r.Match.Tags, style: r.Style, attention: r.Attention}
		if r.Match.Pattern != "" {
			re, err := regexp.Compile(r.Match.Pattern)
			if err != nil {
				return nil, err
			}
			c.re = re
		}
		h.rules = append(h.rules, c)
	}
	return h, nil
}

// Apply evaluates every rule against a line's plain text and tags. All
// matching rules apply in order: later non-empty colors override earlier
// ones, and bold/italic/underline/attention accumulate.
func (h *Highlighter) Apply(plain string, tags []string) Result {
	var res Result
	for _, r := range h.rules {
		if len(r.tags) > 0 && !slices.ContainsFunc(r.tags, func(t string) bool { return slices.Contains(tags, t) }) {
			continue
		}
		if r.re != nil && !r.re.MatchString(plain) {
			continue
		}
		res.Styled = true
		res.Attention = res.Attention || r.attention
		if r.style.FG != "" {
			res.Style.FG = r.style.FG
		}
		if r.style.BG != "" {
			res.Style.BG = r.style.BG
		}
		res.Style.Bold = res.Style.Bold || r.style.Bold
		res.Style.Italic = res.Style.Italic || r.style.Italic
		res.Style.Underline = res.Style.Underline || r.style.Underline
	}
	return res
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/rules/ && go test ./internal/rules/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/rules/rules_test.go internal/rules/rules.go
git commit -m "feat(rules): tag/pattern highlight matching with style merge

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 7: Telnet protocol parser

**Files:**
- Create: `internal/telnet/telnet.go`
- Test: `internal/telnet/telnet_test.go`

**Interfaces:**
- Produces: byte constants `IAC, DO, DONT, WILL, WONT, SB, SE, GA, NOP`, `OptNAWS`, `OptCharset`; `telnet.NewParser(width, height int) *Parser`, `(*Parser).Feed([]byte) (data, reply []byte)`, `(*Parser).Resize(w, h int) []byte` (nil until NAWS is negotiated), `(*Parser).UTF8() bool`, `telnet.Escape([]byte) []byte`

- [ ] **Step 1: Write the failing tests**

`internal/telnet/telnet_test.go`:

```go
package telnet

import (
	"bytes"
	"testing"
)

func TestPlainDataPassesThrough(t *testing.T) {
	p := NewParser(80, 24)
	data, reply := p.Feed([]byte("hello\r\n"))
	if string(data) != "hello\r\n" || reply != nil {
		t.Errorf("data=%q reply=%v", data, reply)
	}
}

func TestEscapedIACIsData(t *testing.T) {
	p := NewParser(80, 24)
	data, _ := p.Feed([]byte{'a', IAC, IAC, 'b'})
	if !bytes.Equal(data, []byte{'a', IAC, 'b'}) {
		t.Errorf("data=%v", data)
	}
}

func TestRefusesUnknownOptions(t *testing.T) {
	p := NewParser(80, 24)
	// MCCP2 (86) offered, TTYPE (24) requested.
	data, reply := p.Feed([]byte{IAC, WILL, 86, IAC, DO, 24, 'x'})
	if string(data) != "x" {
		t.Errorf("data=%q", data)
	}
	want := []byte{IAC, DONT, 86, IAC, WONT, 24}
	if !bytes.Equal(reply, want) {
		t.Errorf("reply=%v want %v", reply, want)
	}
}

func TestIgnoresGAAndWontDont(t *testing.T) {
	p := NewParser(80, 24)
	data, reply := p.Feed([]byte{'a', IAC, GA, IAC, WONT, 1, IAC, DONT, 3, 'b'})
	if string(data) != "ab" || reply != nil {
		t.Errorf("data=%q reply=%v", data, reply)
	}
}

func TestNAWS(t *testing.T) {
	p := NewParser(100, 40)
	if msg := p.Resize(120, 50); msg != nil {
		t.Errorf("Resize before negotiation = %v, want nil", msg)
	}
	_, reply := p.Feed([]byte{IAC, DO, OptNAWS})
	want := []byte{IAC, WILL, OptNAWS, IAC, SB, OptNAWS, 0, 120, 0, 50, IAC, SE}
	if !bytes.Equal(reply, want) {
		t.Errorf("reply=%v want %v", reply, want)
	}
	got := p.Resize(255, 300) // 255 must be escaped
	want = []byte{IAC, SB, OptNAWS, 0, IAC, IAC, 1, 44, IAC, SE}
	if !bytes.Equal(got, want) {
		t.Errorf("Resize=%v want %v", got, want)
	}
}

func TestCharsetAcceptsUTF8(t *testing.T) {
	p := NewParser(80, 24)
	_, reply := p.Feed([]byte{IAC, DO, OptCharset})
	if !bytes.Equal(reply, []byte{IAC, WILL, OptCharset}) {
		t.Errorf("reply=%v", reply)
	}
	req := append([]byte{IAC, SB, OptCharset, charsetRequest}, []byte(";ISO-8859-1;utf-8")...)
	req = append(req, IAC, SE)
	_, reply = p.Feed(req)
	want := append([]byte{IAC, SB, OptCharset, charsetAccepted}, []byte("utf-8")...)
	want = append(want, IAC, SE)
	if !bytes.Equal(reply, want) {
		t.Errorf("reply=%q want %q", reply, want)
	}
	if !p.UTF8() {
		t.Error("UTF8() = false")
	}
}

func TestCharsetRejectsOthers(t *testing.T) {
	p := NewParser(80, 24)
	req := append([]byte{IAC, SB, OptCharset, charsetRequest}, []byte(" ASCII KOI8-R")...)
	_, reply := p.Feed(append(req, IAC, SE))
	if !bytes.Equal(reply, []byte{IAC, SB, OptCharset, charsetRejected, IAC, SE}) || p.UTF8() {
		t.Errorf("reply=%v utf8=%v", reply, p.UTF8())
	}
}

func TestSequencesSplitAcrossFeeds(t *testing.T) {
	p := NewParser(80, 24)
	stream := []byte{'h', 'i', IAC, DO, OptNAWS, IAC, SB, 99, 1, IAC, IAC, 2, IAC, SE, IAC, IAC, '!'}
	var data, reply []byte
	for _, b := range stream { // worst case: one byte per read
		d, r := p.Feed([]byte{b})
		data = append(data, d...)
		reply = append(reply, r...)
	}
	if !bytes.Equal(data, []byte{'h', 'i', IAC, '!'}) {
		t.Errorf("data=%v", data)
	}
	if !bytes.HasPrefix(reply, []byte{IAC, WILL, OptNAWS}) {
		t.Errorf("reply=%v", reply)
	}
}

func TestEscape(t *testing.T) {
	if got := Escape([]byte{'a', IAC, 'b'}); !bytes.Equal(got, []byte{'a', IAC, IAC, 'b'}) {
		t.Errorf("Escape=%v", got)
	}
	if got := Escape([]byte("plain")); string(got) != "plain" {
		t.Errorf("Escape=%q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/telnet/`
Expected: FAIL with build errors such as `undefined: NewParser`.

- [ ] **Step 3: Write the implementation**

`internal/telnet/telnet.go`:

```go
// Package telnet is a minimal client-side telnet protocol parser. It
// separates application data from IAC sequences and produces replies:
// NAWS and CHARSET (UTF-8) are accepted; every other option is refused.
package telnet

import (
	"bytes"
	"strings"
)

// Protocol bytes.
const (
	SE   byte = 240
	NOP  byte = 241
	GA   byte = 249
	SB   byte = 250
	WILL byte = 251
	WONT byte = 252
	DO   byte = 253
	DONT byte = 254
	IAC  byte = 255
)

// Options.
const (
	OptNAWS    byte = 31
	OptCharset byte = 42
)

// CHARSET subnegotiation commands (RFC 2066).
const (
	charsetRequest  byte = 1
	charsetAccepted byte = 2
	charsetRejected byte = 3
)

type state int

const (
	stData state = iota
	stIAC
	stOpt   // after WILL/WONT/DO/DONT, waiting for the option byte
	stSBOpt // after SB, waiting for the option byte
	stSB    // inside subnegotiation data
	stSBIAC // IAC inside subnegotiation
)

// Parser is fed raw bytes from the server. It is not safe for concurrent use.
type Parser struct {
	st     state
	verb   byte
	sbOpt  byte
	sbBuf  []byte
	width  uint16
	height uint16
	naws   bool // server asked for NAWS and we agreed
	utf8   bool // server accepted/negotiated UTF-8
}

// NewParser returns a parser that reports the given window size via NAWS.
func NewParser(width, height int) *Parser {
	return &Parser{width: clamp16(width), height: clamp16(height)}
}

// UTF8 reports whether UTF-8 was negotiated via CHARSET.
func (p *Parser) UTF8() bool { return p.utf8 }

// Feed consumes bytes from the server and returns the application data
// they contain plus any bytes that must be written back to the server.
// Sequences split across calls are handled.
func (p *Parser) Feed(in []byte) (data, reply []byte) {
	for _, b := range in {
		switch p.st {
		case stData:
			if b == IAC {
				p.st = stIAC
			} else {
				data = append(data, b)
			}
		case stIAC:
			switch b {
			case IAC:
				data = append(data, IAC)
				p.st = stData
			case WILL, WONT, DO, DONT:
				p.verb = b
				p.st = stOpt
			case SB:
				p.st = stSBOpt
			default: // GA, NOP, and anything else: ignore
				p.st = stData
			}
		case stOpt:
			reply = append(reply, p.negotiate(p.verb, b)...)
			p.st = stData
		case stSBOpt:
			p.sbOpt = b
			p.sbBuf = p.sbBuf[:0]
			p.st = stSB
		case stSB:
			if b == IAC {
				p.st = stSBIAC
			} else {
				p.sbBuf = append(p.sbBuf, b)
			}
		case stSBIAC:
			switch b {
			case SE:
				reply = append(reply, p.subneg(p.sbOpt, p.sbBuf)...)
				p.st = stData
			case IAC:
				p.sbBuf = append(p.sbBuf, IAC)
				p.st = stSB
			default: // malformed; abandon the subnegotiation
				p.st = stData
			}
		}
	}
	return data, reply
}

func (p *Parser) negotiate(verb, opt byte) []byte {
	switch verb {
	case DO:
		switch opt {
		case OptNAWS:
			p.naws = true
			return append([]byte{IAC, WILL, OptNAWS}, p.nawsMsg()...)
		case OptCharset:
			return []byte{IAC, WILL, OptCharset}
		}
		return []byte{IAC, WONT, opt}
	case WILL:
		if opt == OptCharset {
			return []byte{IAC, DO, OptCharset}
		}
		return []byte{IAC, DONT, opt}
	}
	return nil // WONT/DONT: nothing to acknowledge for options we never enabled
}

func (p *Parser) subneg(opt byte, data []byte) []byte {
	if opt != OptCharset || len(data) < 2 || data[0] != charsetRequest {
		return nil
	}
	sep := data[1]
	for _, name := range bytes.Split(data[2:], []byte{sep}) {
		if n := strings.ToUpper(string(name)); n == "UTF-8" || n == "UTF8" {
			p.utf8 = true
			msg := []byte{IAC, SB, OptCharset, charsetAccepted}
			msg = append(msg, name...)
			return append(msg, IAC, SE)
		}
	}
	return []byte{IAC, SB, OptCharset, charsetRejected, IAC, SE}
}

// Resize records a new window size and returns the NAWS message to send,
// or nil if NAWS has not been negotiated.
func (p *Parser) Resize(width, height int) []byte {
	p.width, p.height = clamp16(width), clamp16(height)
	if !p.naws {
		return nil
	}
	return p.nawsMsg()
}

func (p *Parser) nawsMsg() []byte {
	msg := []byte{IAC, SB, OptNAWS}
	for _, v := range []uint16{p.width, p.height} {
		hi, lo := byte(v>>8), byte(v)
		msg = append(msg, hi)
		if hi == IAC {
			msg = append(msg, IAC)
		}
		msg = append(msg, lo)
		if lo == IAC {
			msg = append(msg, IAC)
		}
	}
	return append(msg, IAC, SE)
}

// Escape doubles IAC bytes in outgoing application data.
func Escape(b []byte) []byte {
	if bytes.IndexByte(b, IAC) < 0 {
		return b
	}
	return bytes.ReplaceAll(b, []byte{IAC}, []byte{IAC, IAC})
}

func clamp16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 0xffff {
		return 0xffff
	}
	return uint16(v)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/telnet/ && go test ./internal/telnet/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/telnet/telnet_test.go internal/telnet/telnet.go
git commit -m "feat(telnet): IAC parser with NAWS and CHARSET UTF-8

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 8: Line splitting, MCP, and certificate pins

**Files:**
- Create: `internal/conn/lines.go`, `internal/conn/pin.go`
- Test: `internal/conn/lines_test.go`, `internal/conn/pin_test.go`

**Interfaces:**
- Produces (package-internal): `splitter` with `push([]byte) []string` and `flush() []string`
- Produces (exported): `conn.Fingerprint(*x509.Certificate) string`, `conn.PinMismatchError{HostPort, Pinned, Got string}`, `conn.KnownHosts{Path string}` with `Lookup(hostport) (string, bool, error)` and `Trust(hostport, fp string) error`

- [ ] **Step 1: Write the failing tests**

`internal/conn/lines_test.go`:

```go
package conn

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitterLines(t *testing.T) {
	var s splitter
	got := s.push([]byte("one\r\ntwo\nthr"))
	if want := []string{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	got = s.push([]byte("ee\r\n"))
	if want := []string{"three"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSplitterUTF8SplitAcrossPushes(t *testing.T) {
	var s splitter
	fox := []byte("🦊 hi\n")
	var got []string
	for _, b := range fox {
		got = append(got, s.push([]byte{b})...)
	}
	if len(got) != 1 || got[0] != "🦊 hi" {
		t.Errorf("got %q", got)
	}
}

func TestSplitterLatin1Fallback(t *testing.T) {
	var s splitter
	got := s.push([]byte{'c', 'a', 'f', 0xe9, '\n'}) // "café" in Latin-1
	if len(got) != 1 || got[0] != "café" {
		t.Errorf("got %q", got)
	}
}

func TestSplitterMCP(t *testing.T) {
	var s splitter
	got := s.push([]byte("#$#mcp version: 2.1 to: 2.1\n#$\"#$#not mcp\nnormal\n"))
	if want := []string{"#$#not mcp", "normal"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSplitterFlushAndBlankLines(t *testing.T) {
	var s splitter
	got := s.push([]byte("\r\n\npartial"))
	if want := []string{"", ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
	if got := s.flush(); !reflect.DeepEqual(got, []string{"partial"}) {
		t.Errorf("flush = %q", got)
	}
	if got := s.flush(); got != nil {
		t.Errorf("second flush = %q", got)
	}
}

func TestSplitterBoundsRunawayLine(t *testing.T) {
	var s splitter
	got := s.push([]byte(strings.Repeat("x", maxLine+10)))
	if len(got) != 1 || len(got[0]) != maxLine {
		t.Errorf("got %d lines", len(got))
	}
}
```

`internal/conn/pin_test.go`:

```go
package conn

import (
	"path/filepath"
	"testing"
)

func TestKnownHosts(t *testing.T) {
	k := KnownHosts{Path: filepath.Join(t.TempDir(), "sub", "known_hosts")}
	if _, ok, err := k.Lookup("a:1"); ok || err != nil {
		t.Fatalf("empty lookup = %v, %v", ok, err)
	}
	if err := k.Trust("a:1", "sha256:aa"); err != nil {
		t.Fatal(err)
	}
	k.Trust("b:2", "sha256:bb")
	k.Trust("a:1", "sha256:cc") // replace
	for host, want := range map[string]string{"a:1": "sha256:cc", "b:2": "sha256:bb"} {
		got, ok, err := k.Lookup(host)
		if !ok || err != nil || got != want {
			t.Errorf("Lookup(%s) = %q, %v, %v; want %q", host, got, ok, err, want)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/conn/`
Expected: FAIL with build errors such as `undefined: splitter`.

- [ ] **Step 3: Write the implementation**

`internal/conn/lines.go`:

```go
package conn

import (
	"strings"
	"unicode/utf8"
)

// maxLine bounds a single unterminated line so a server that never sends
// a newline can't grow memory without limit.
const maxLine = 64 * 1024

// splitter turns a byte stream (telnet already removed) into lines.
type splitter struct {
	buf []byte
}

// push appends data and returns every completed line: CR stripped,
// decoded to UTF-8, MCP out-of-band lines removed.
func (s *splitter) push(data []byte) []string {
	var out []string
	for _, b := range data {
		if b == '\n' {
			out = appendLine(out, s.buf)
			s.buf = s.buf[:0]
			continue
		}
		s.buf = append(s.buf, b)
		if len(s.buf) >= maxLine {
			out = appendLine(out, s.buf)
			s.buf = s.buf[:0]
		}
	}
	return out
}

// flush returns any partial line left at end of stream.
func (s *splitter) flush() []string {
	if len(s.buf) == 0 {
		return nil
	}
	out := appendLine(nil, s.buf)
	s.buf = s.buf[:0]
	return out
}

func appendLine(out []string, raw []byte) []string {
	line := strings.ReplaceAll(decode(raw), "\r", "")
	// MCP 2.1: "#$#" lines are out-of-band messages (swallowed);
	// "#$\"" quotes a normal line that happens to start with "#$#".
	if strings.HasPrefix(line, "#$#") {
		return out
	}
	line = strings.TrimPrefix(line, "#$\"")
	return append(out, line)
}

// decode returns raw as a string if it is valid UTF-8, otherwise treats it
// as Latin-1 (what most older MUCKs actually send).
func decode(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	var b strings.Builder
	b.Grow(len(raw) * 2)
	for _, c := range raw {
		b.WriteRune(rune(c))
	}
	return b.String()
}
```

`internal/conn/pin.go`:

```go
package conn

import (
	"bufio"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Fingerprint is the pin for a certificate: "sha256:<hex>".
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// PinMismatchError means a server presented a different certificate than
// the one pinned on first use.
type PinMismatchError struct {
	HostPort string
	Pinned   string
	Got      string
}

func (e *PinMismatchError) Error() string {
	return fmt.Sprintf("certificate for %s changed: pinned %s, got %s", e.HostPort, e.Pinned, e.Got)
}

// KnownHosts is a file of "host:port fingerprint" lines.
type KnownHosts struct {
	Path string
}

var knownHostsMu sync.Mutex

// Lookup returns the pinned fingerprint for hostport.
func (k KnownHosts) Lookup(hostport string) (string, bool, error) {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()
	m, err := k.read()
	if err != nil {
		return "", false, err
	}
	fp, ok := m[hostport]
	return fp, ok, nil
}

// Trust pins fp for hostport, replacing any existing pin.
func (k KnownHosts) Trust(hostport, fp string) error {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()
	m, err := k.read()
	if err != nil {
		return err
	}
	m[hostport] = fp
	keys := make([]string, 0, len(m))
	for h := range m {
		keys = append(keys, h)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, h := range keys {
		fmt.Fprintf(&b, "%s %s\n", h, m[h])
	}
	if err := os.MkdirAll(filepath.Dir(k.Path), 0o700); err != nil {
		return err
	}
	tmp := k.Path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.Path)
}

func (k KnownHosts) read() (map[string]string, error) {
	m := map[string]string{}
	f, err := os.Open(k.Path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		host, fp, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if ok {
			m[host] = fp
		}
	}
	return m, sc.Err()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/conn/ && go test ./internal/conn/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/conn/lines_test.go internal/conn/pin_test.go internal/conn/lines.go internal/conn/pin.go
git commit -m "feat(conn): line splitter with Latin-1 fallback, MCP swallowing, known_hosts pins

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 9: Dialing: TCP/TLS with TOFU pinning

**Files:**
- Create: `internal/conn/conn.go`
- Test: `internal/conn/conn_test.go`

**Interfaces:**
- Consumes: `telnet.Parser`, `telnet.Escape` (Task 7); `splitter`, `KnownHosts`, `Fingerprint`, `PinMismatchError` (Task 8)
- Produces: `conn.Options{Host string; Port int; TLS bool; TLSTrust string; KnownHosts KnownHosts; Width, Height int}`, `conn.Dial(ctx, Options) (*Conn, error)`, `(*Conn).Lines() <-chan string` (closed when the connection ends, including after `Close`), `(*Conn).Err() error`, `(*Conn).Send(line string) error`, `(*Conn).Resize(w, h int) error`, `(*Conn).Close() error`

- [ ] **Step 1: Write the failing tests**

`internal/conn/conn_test.go`:

```go
package conn

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"kiln/internal/telnet"
)

// fakeServer accepts one connection and hands it to handle.
func fakeServer(t *testing.T, ln net.Listener, handle func(net.Conn)) (host string, port int) {
	t.Helper()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		handle(c)
	}()
	t.Cleanup(func() { ln.Close() })
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	port, _ = strconv.Atoi(p)
	return h, port
}

func tcpListener(t *testing.T) net.Listener {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func collect(t *testing.T, c *Conn) []string {
	t.Helper()
	var got []string
	timeout := time.After(5 * time.Second)
	for {
		select {
		case l, ok := <-c.Lines():
			if !ok {
				return got
			}
			got = append(got, l)
		case <-timeout:
			t.Fatal("timed out waiting for lines")
		}
	}
}

func TestPlainConnReceivesLinesAndNegotiates(t *testing.T) {
	replies := make(chan []byte, 1)
	host, port := fakeServer(t, tcpListener(t), func(c net.Conn) {
		c.Write([]byte{telnet.IAC, telnet.DO, telnet.OptNAWS})
		c.Write([]byte("#$#mcp version: 2.1 to: 2.1\r\nWelcome to \xe9t\xe9\r\n"))
		c.Write([]byte("Rook says, \"hi\"\r\nno newline"))
		buf := make([]byte, 64)
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := c.Read(buf)
		replies <- buf[:n]
	})
	c, err := Dial(context.Background(), Options{Host: host, Port: port, Width: 80, Height: 24})
	if err != nil {
		t.Fatal(err)
	}
	got := collect(t, c)
	want := []string{"Welcome to été", "Rook says, \"hi\"", "no newline"}
	if len(got) != len(want) {
		t.Fatalf("got %q want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q want %q", i, got[i], want[i])
		}
	}
	if c.Err() != nil {
		t.Errorf("Err = %v, want nil on clean close", c.Err())
	}
	r := <-replies
	if len(r) < 3 || r[0] != telnet.IAC || r[1] != telnet.WILL || r[2] != telnet.OptNAWS {
		t.Errorf("server got reply %v, want IAC WILL NAWS …", r)
	}
}

func TestSendTerminatesAndEscapes(t *testing.T) {
	got := make(chan string, 1)
	host, port := fakeServer(t, tcpListener(t), func(c net.Conn) {
		line, _ := bufio.NewReader(c).ReadString('\n')
		got <- line
	})
	c, err := Dial(context.Background(), Options{Host: host, Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Send(":grins.\xff"); err != nil {
		t.Fatal(err)
	}
	if line := <-got; line != ":grins.\xff\xff\r\n" {
		t.Errorf("server got %q", line)
	}
}

func selfSignedListener(t *testing.T) (net.Listener, *x509.Certificate) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "muck.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ln, cert
}

func greet(c net.Conn) { c.Write([]byte("secure hello\r\n")) }

func TestTLSPinsOnFirstUseThenAccepts(t *testing.T) {
	kh := KnownHosts{Path: filepath.Join(t.TempDir(), "known_hosts")}
	ln, cert := selfSignedListener(t)
	host, port := fakeServer(t, ln, greet)
	c, err := Dial(context.Background(), Options{Host: host, Port: port, TLS: true, TLSTrust: "pin", KnownHosts: kh})
	if err != nil {
		t.Fatalf("first connect: %v", err)
	}
	if got := collect(t, c); len(got) != 1 || got[0] != "secure hello" {
		t.Errorf("got %q", got)
	}
	pinned, ok, _ := kh.Lookup(net.JoinHostPort(host, strconv.Itoa(port)))
	if !ok || pinned != Fingerprint(cert) {
		t.Errorf("pinned %q, want %q", pinned, Fingerprint(cert))
	}
}

func TestTLSPinMismatchRefuses(t *testing.T) {
	kh := KnownHosts{Path: filepath.Join(t.TempDir(), "known_hosts")}
	ln, cert := selfSignedListener(t)
	host, port := fakeServer(t, ln, greet)
	hp := net.JoinHostPort(host, strconv.Itoa(port))
	kh.Trust(hp, "sha256:0000")
	_, err := Dial(context.Background(), Options{Host: host, Port: port, TLS: true, TLSTrust: "pin", KnownHosts: kh})
	var pin *PinMismatchError
	if !errors.As(err, &pin) {
		t.Fatalf("err = %v, want PinMismatchError", err)
	}
	if pin.Pinned != "sha256:0000" || pin.Got != Fingerprint(cert) || pin.HostPort != hp {
		t.Errorf("mismatch = %+v", pin)
	}
	if fp, _, _ := kh.Lookup(hp); fp != "sha256:0000" {
		t.Error("mismatch must not overwrite the pin")
	}
}

func TestTLSCAModeRejectsSelfSigned(t *testing.T) {
	ln, _ := selfSignedListener(t)
	host, port := fakeServer(t, ln, greet)
	_, err := Dial(context.Background(), Options{Host: host, Port: port, TLS: true, TLSTrust: "ca"})
	if err == nil {
		t.Fatal("ca mode accepted a self-signed cert")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -race ./internal/conn/`
Expected: FAIL with build errors such as `undefined: Dial`.

- [ ] **Step 3: Write the implementation**

`internal/conn/conn.go`:

```go
// Package conn is a MUCK connection: TCP or TLS (with trust-on-first-use
// pinning), telnet negotiation, and line splitting.
package conn

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"kiln/internal/telnet"
)

// Options configures Dial.
type Options struct {
	Host       string
	Port       int
	TLS        bool
	TLSTrust   string // "pin" or "ca"
	KnownHosts KnownHosts
	Width      int // reported via NAWS
	Height     int
}

// Conn is an open connection. Received lines arrive on Lines().
type Conn struct {
	nc     net.Conn
	lines  chan string
	mu     sync.Mutex // guards writes and parser
	parser *telnet.Parser
	err    error // set before lines is closed
}

// Dial connects and starts reading. The context bounds only the dial.
func Dial(ctx context.Context, o Options) (*Conn, error) {
	hostport := net.JoinHostPort(o.Host, strconv.Itoa(o.Port))
	d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	var nc net.Conn
	var err error
	if o.TLS {
		td := &tls.Dialer{NetDialer: d, Config: tlsConfig(o, hostport)}
		nc, err = td.DialContext(ctx, "tcp", hostport)
	} else {
		nc, err = d.DialContext(ctx, "tcp", hostport)
	}
	if err != nil {
		var pin *PinMismatchError
		if errors.As(err, &pin) {
			return nil, pin
		}
		return nil, err
	}
	c := &Conn{nc: nc, lines: make(chan string, 64), parser: telnet.NewParser(o.Width, o.Height)}
	go c.readLoop()
	return c, nil
}

func tlsConfig(o Options, hostport string) *tls.Config {
	cfg := &tls.Config{ServerName: o.Host}
	if o.TLSTrust == "ca" {
		return cfg
	}
	// Pin mode: skip CA verification (MUCK certs are usually self-signed)
	// and verify against the pinned fingerprint instead.
	cfg.InsecureSkipVerify = true
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return errors.New("server sent no certificate")
		}
		got := Fingerprint(cs.PeerCertificates[0])
		pinned, ok, err := o.KnownHosts.Lookup(hostport)
		if err != nil {
			return fmt.Errorf("reading known_hosts: %w", err)
		}
		if !ok {
			return o.KnownHosts.Trust(hostport, got)
		}
		if pinned != got {
			return &PinMismatchError{HostPort: hostport, Pinned: pinned, Got: got}
		}
		return nil
	}
	return cfg
}

func (c *Conn) readLoop() {
	var s splitter
	buf := make([]byte, 16*1024)
	for {
		n, err := c.nc.Read(buf)
		if n > 0 {
			c.mu.Lock()
			data, reply := c.parser.Feed(buf[:n])
			if len(reply) > 0 {
				c.nc.Write(reply)
			}
			c.mu.Unlock()
			for _, line := range s.push(data) {
				c.lines <- line
			}
		}
		if err != nil {
			for _, line := range s.flush() {
				c.lines <- line
			}
			if !errors.Is(err, io.EOF) {
				c.err = err
			}
			close(c.lines)
			return
		}
	}
}

// Lines delivers received lines. It is closed when the connection ends;
// Err then reports why (nil for a clean close by the server).
func (c *Conn) Lines() <-chan string { return c.lines }

// Err is valid after Lines() is closed.
func (c *Conn) Err() error { return c.err }

// Send writes one line (IAC-escaped, CRLF-terminated).
func (c *Conn) Send(line string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.nc.Write(append(telnet.Escape([]byte(line)), '\r', '\n'))
	return err
}

// Resize reports a new window size to the server if NAWS was negotiated.
func (c *Conn) Resize(width, height int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if msg := c.parser.Resize(width, height); msg != nil {
		_, err := c.nc.Write(msg)
		return err
	}
	return nil
}

// Close closes the connection; Lines() will then close.
func (c *Conn) Close() error { return c.nc.Close() }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/conn/ && go test -race ./internal/conn/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/conn/conn_test.go internal/conn/conn.go
git commit -m "feat(conn): dial TCP/TLS with trust-on-first-use pinning

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 10: Session lifecycle

**Files:**
- Create: `internal/session/session.go`
- Test: `internal/session/session_test.go`

**Interfaces:**
- Consumes: `config.Character` (Task 3), `logstore.Entry`/`Dir` (Task 1), `conn.PinMismatchError` (Task 8)
- Produces: `session.State` (`Disconnected, Connecting, Connected, Failed`), `session.EventKind` (`EventLine, EventState, EventLogError`), `session.Event{Kind EventKind; Entry logstore.Entry; State State; Err error}`, `session.LineConn` interface (`Lines, Err, Send, Close`; `Lines` must close after `Close`), `session.Appender` interface (`Append(logstore.Entry) error`), `session.Options{Char config.Character; Dial func(ctx) (LineConn, error); Log Appender; Password func() (string, error); Now func() time.Time; Backoff func(int) time.Duration}`, `session.New(Options) *Session`, `(*Session).Run(ctx)`, `(*Session).Events() <-chan Event`, `(*Session).Send(line) error`, `(*Session).Reconnect()`, `session.ErrNotConnected`, `session.QuitCommand`, `session.DefaultBackoff`

- [ ] **Step 1: Write the failing tests**

`internal/session/session_test.go`:

```go
package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"kiln/internal/config"
	"kiln/internal/conn"
	"kiln/internal/logstore"
)

// fakeConn is a scripted LineConn.
type fakeConn struct {
	lines     chan string
	err       error
	mu        sync.Mutex
	sent      []string
	closeOnce sync.Once
}

func newFakeConn(lines ...string) *fakeConn {
	f := &fakeConn{lines: make(chan string, len(lines)+1)}
	for _, l := range lines {
		f.lines <- l
	}
	return f
}
func (f *fakeConn) Lines() <-chan string { return f.lines }
func (f *fakeConn) Err() error           { return f.err }
func (f *fakeConn) Close() error {
	f.closeOnce.Do(func() { close(f.lines) })
	return nil
}
func (f *fakeConn) Send(l string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, l)
	return nil
}
func (f *fakeConn) Sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

type memLog struct {
	mu      sync.Mutex
	entries []logstore.Entry
	fail    error
}

func (m *memLog) Append(e logstore.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, e)
	return m.fail
}
func (m *memLog) Texts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, e := range m.entries {
		out = append(out, string(rune(e.Dir))+" "+e.Text)
	}
	return out
}

var kit = config.Character{World: "fm", ID: "kit", Name: "Kit", Host: "h", Port: 1, Login: "connect {name} {password}"}

// waitFor reads events until pred is true or times out.
func waitFor(t *testing.T, s *Session, pred func(Event) bool) Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				t.Fatal("events closed")
			}
			if pred(ev) {
				return ev
			}
		case <-timeout:
			t.Fatal("timed out")
		}
	}
}

func isState(st State) func(Event) bool {
	return func(e Event) bool { return e.Kind == EventState && e.State == st }
}

func TestLoginRedactsPasswordInLog(t *testing.T) {
	fc := newFakeConn("Welcome!")
	log := &memLog{}
	pw := `hunter2 {name} $1`
	s := New(Options{
		Char:     kit,
		Dial:     func(context.Context) (LineConn, error) { return fc, nil },
		Log:      log,
		Password: func() (string, error) { return pw, nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == "Welcome!" })

	if sent := fc.Sent(); len(sent) != 1 || sent[0] != "connect Kit "+pw {
		t.Errorf("wire = %q", sent)
	}
	for _, line := range log.Texts() {
		if strings.Contains(line, "hunter2") {
			t.Errorf("password leaked into log: %q", line)
		}
	}
	if got := log.Texts(); !contains(got, "> connect Kit ***") {
		t.Errorf("log = %q, want redacted login line", got)
	}
}

func TestMissingPasswordSkipsLogin(t *testing.T) {
	fc := newFakeConn("Welcome!")
	log := &memLog{}
	s := New(Options{
		Char:     kit,
		Dial:     func(context.Context) (LineConn, error) { return fc, nil },
		Log:      log,
		Password: func() (string, error) { return "", errors.New("not found") },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == "Welcome!" })
	if sent := fc.Sent(); len(sent) != 0 {
		t.Errorf("sent %q, want nothing", sent)
	}
	if !containsPrefix(log.Texts(), "* no saved password for fm/kit") {
		t.Errorf("log = %q", log.Texts())
	}
}

func TestNoLoginTemplateSendsNothing(t *testing.T) {
	fc := newFakeConn("hi")
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Dial: func(context.Context) (LineConn, error) { return fc, nil }, Log: &memLog{}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == "hi" })
	if len(fc.Sent()) != 0 {
		t.Errorf("sent %q", fc.Sent())
	}
}

func TestSendLogsOutgoing(t *testing.T) {
	fc := newFakeConn()
	ch := kit
	ch.Login = ""
	log := &memLog{}
	s := New(Options{Char: ch, Dial: func(context.Context) (LineConn, error) { return fc, nil }, Log: log})
	if err := s.Send("x"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Send before connect = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	if err := s.Send(":grins."); err != nil {
		t.Fatal(err)
	}
	if !contains(log.Texts(), "> :grins.") || fc.Sent()[0] != ":grins." {
		t.Errorf("log=%q sent=%q", log.Texts(), fc.Sent())
	}
}

func TestReconnectsWithBackoffAfterDrop(t *testing.T) {
	first := newFakeConn("one")
	first.Close() // server hangs up after one line
	first.err = errors.New("connection reset")
	second := newFakeConn("two")
	conns := []*fakeConn{first, second}
	var mu sync.Mutex
	var delays []int
	ch := kit
	ch.Login = ""
	log := &memLog{}
	s := New(Options{
		Char: ch,
		Log:  log,
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			c := conns[0]
			conns = conns[1:]
			return c, nil
		},
		Backoff: func(a int) time.Duration {
			mu.Lock()
			delays = append(delays, a)
			mu.Unlock()
			return time.Millisecond
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == "two" })
	if !containsPrefix(log.Texts(), "* disconnected (connection reset); retrying in") {
		t.Errorf("log = %q", log.Texts())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(delays) != 1 || delays[0] != 0 {
		t.Errorf("backoff attempts = %v, want [0]", delays)
	}
}

func TestDialFailuresBackOffIncreasingly(t *testing.T) {
	var mu sync.Mutex
	var attempts []int
	calls := 0
	s := New(Options{
		Char: kit,
		Log:  &memLog{},
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls <= 3 {
				return nil, errors.New("refused")
			}
			return newFakeConn(), nil
		},
		Backoff: func(a int) time.Duration {
			mu.Lock()
			attempts = append(attempts, a)
			mu.Unlock()
			return time.Millisecond
		},
		Password: func() (string, error) { return "pw", nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	mu.Lock()
	defer mu.Unlock()
	if len(attempts) != 3 || attempts[0] != 0 || attempts[2] != 2 {
		t.Errorf("attempts = %v, want [0 1 2]", attempts)
	}
}

func TestQuitDoesNotReconnect(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	first := newFakeConn()
	ch := kit
	ch.Login = ""
	log := &memLog{}
	s := New(Options{
		Char: ch,
		Log:  log,
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls == 1 {
				return first, nil
			}
			return newFakeConn(), nil
		},
		Backoff: func(int) time.Duration { t.Error("quit must not use backoff"); return 0 },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, isState(Connected))
	if err := s.Send("QUIT"); err != nil {
		t.Fatal(err)
	}
	first.Close() // server hangs up in response
	waitFor(t, s, isState(Disconnected))
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if calls != 1 {
		t.Errorf("redialed %d times after QUIT", calls-1)
	}
	mu.Unlock()
	if !contains(log.Texts(), "* disconnected (quit)") {
		t.Errorf("log = %q", log.Texts())
	}
	s.Reconnect()
	waitFor(t, s, isState(Connected))
}

func TestPinMismatchFailsUntilReconnect(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	s := New(Options{
		Char: kit,
		Log:  &memLog{},
		Dial: func(context.Context) (LineConn, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls == 1 {
				return nil, &conn.PinMismatchError{HostPort: "h:1", Pinned: "sha256:a", Got: "sha256:b"}
			}
			return newFakeConn(), nil
		},
		Backoff: func(int) time.Duration { t.Error("pin mismatch must not use backoff"); return 0 },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	ev := waitFor(t, s, isState(Failed))
	var pin *conn.PinMismatchError
	if !errors.As(ev.Err, &pin) {
		t.Errorf("Failed err = %v", ev.Err)
	}
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if calls != 1 {
		t.Errorf("redialed %d times without Reconnect", calls-1)
	}
	mu.Unlock()
	s.Reconnect()
	waitFor(t, s, isState(Connected))
}

func TestLogFailureStillDeliversLine(t *testing.T) {
	ch := kit
	ch.Login = ""
	s := New(Options{
		Char: ch,
		Log:  &memLog{fail: errors.New("disk full")},
		Dial: func(context.Context) (LineConn, error) { return newFakeConn("still here"), nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLogError })
	waitFor(t, s, func(e Event) bool { return e.Kind == EventLine && e.Entry.Text == "still here" })
}

func TestRunStopsOnCancel(t *testing.T) {
	ch := kit
	ch.Login = ""
	s := New(Options{Char: ch, Log: &memLog{}, Dial: func(context.Context) (LineConn, error) { return newFakeConn(), nil }})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	waitFor(t, s, isState(Connected))
	cancel()
	go func() {
		for range s.Events() {
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func containsPrefix(xs []string, prefix string) bool {
	for _, x := range xs {
		if strings.HasPrefix(x, prefix) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -race ./internal/session/`
Expected: FAIL with build errors such as `undefined: New`.

- [ ] **Step 3: Write the implementation**

`internal/session/session.go`:

```go
// Package session runs one character's connection: dialing, auto-login,
// logging, and reconnecting with backoff.
package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"kiln/internal/config"
	"kiln/internal/conn"
	"kiln/internal/logstore"
)

// State is a session's connection state.
type State int

const (
	Disconnected State = iota
	Connecting
	Connected
	Failed // stopped retrying (e.g. certificate changed); waits for Reconnect
)

func (s State) String() string {
	return [...]string{"disconnected", "connecting", "connected", "failed"}[s]
}

// EventKind says which fields of an Event are set.
type EventKind int

const (
	EventLine     EventKind = iota // Entry is set (already logged)
	EventState                     // State is set; Err is set for Failed
	EventLogError                  // Err is set: writing the log failed
)

// Event is something the UI should hear about.
type Event struct {
	Kind  EventKind
	Entry logstore.Entry
	State State
	Err   error
}

// LineConn is the part of *conn.Conn a session uses. Lines must be closed
// when the connection ends, including after Close.
type LineConn interface {
	Lines() <-chan string
	Err() error
	Send(line string) error
	Close() error
}

// Appender is where entries are logged.
type Appender interface {
	Append(logstore.Entry) error
}

// Options configures a Session. Dial, Log and Char are required.
type Options struct {
	Char     config.Character
	Dial     func(ctx context.Context) (LineConn, error)
	Log      Appender
	Password func() (string, error)          // nil: no password available
	Now      func() time.Time                // default time.Now
	Backoff  func(attempt int) time.Duration // default DefaultBackoff
}

// DefaultBackoff is 1s, 2s, 4s, … capped at 60s.
func DefaultBackoff(attempt int) time.Duration {
	d := time.Second << min(attempt, 6)
	return min(d, 60*time.Second)
}

// ErrNotConnected is returned by Send while no connection is open.
var ErrNotConnected = errors.New("not connected")

// Session is one character's connection lifecycle.
type Session struct {
	o      Options
	events chan Event
	kick   chan struct{}

	mu       sync.Mutex
	c        LineConn
	quitting bool // user sent QUIT on the current connection
}

// New returns a Session; call Run to start it.
func New(o Options) *Session {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Backoff == nil {
		o.Backoff = DefaultBackoff
	}
	return &Session{o: o, events: make(chan Event, 256), kick: make(chan struct{}, 1)}
}

// Events delivers session events. It is closed when Run returns.
func (s *Session) Events() <-chan Event { return s.events }

// Reconnect skips the current backoff wait, or resumes a Failed session.
func (s *Session) Reconnect() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// QuitCommand is the MUCK command that ends a connection on purpose.
// When the server hangs up after it, the session does not reconnect.
const QuitCommand = "QUIT"

// Send sends one line to the server and logs it.
func (s *Session) Send(line string) error {
	s.mu.Lock()
	c := s.c
	if c != nil && strings.TrimSpace(line) == QuitCommand {
		s.quitting = true
	}
	s.mu.Unlock()
	if c == nil {
		return ErrNotConnected
	}
	if err := c.Send(line); err != nil {
		return err
	}
	s.logLine(logstore.Out, line)
	return nil
}

// Run connects and keeps reconnecting until ctx is cancelled.
func (s *Session) Run(ctx context.Context) {
	defer close(s.events)
	attempt := 0
	for ctx.Err() == nil {
		s.state(Connecting, nil)
		c, err := s.o.Dial(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			var pin *conn.PinMismatchError
			if errors.As(err, &pin) {
				s.sys("connect failed: " + err.Error())
				s.state(Failed, err)
				if !s.wait(ctx, -1) {
					return
				}
				continue
			}
			delay := s.o.Backoff(attempt)
			attempt++
			s.sys(fmt.Sprintf("connect failed: %v; retrying in %s", err, delay))
			s.state(Disconnected, err)
			if !s.wait(ctx, delay) {
				return
			}
			continue
		}

		attempt = 0
		s.setConn(c)
		s.sys(fmt.Sprintf("connected to %s:%d", s.o.Char.Host, s.o.Char.Port))
		s.state(Connected, nil)
		s.login(c)
		s.pump(ctx, c)
		s.setConn(nil)
		if ctx.Err() != nil {
			return
		}

		s.mu.Lock()
		quit := s.quitting
		s.quitting = false
		s.mu.Unlock()
		if quit {
			s.sys("disconnected (quit)")
			s.state(Disconnected, nil)
			if !s.wait(ctx, -1) { // stay down until Reconnect
				return
			}
			continue
		}

		reason := "closed by server"
		if err := c.Err(); err != nil {
			reason = err.Error()
		}
		delay := s.o.Backoff(attempt)
		attempt++
		s.sys(fmt.Sprintf("disconnected (%s); retrying in %s", reason, delay))
		s.state(Disconnected, c.Err())
		if !s.wait(ctx, delay) {
			return
		}
	}
}

// pump forwards lines until the connection ends or ctx is cancelled.
func (s *Session) pump(ctx context.Context, c LineConn) {
	for {
		select {
		case <-ctx.Done():
			c.Close()
			for range c.Lines() { // drain so the reader goroutine exits
			}
			return
		case line, ok := <-c.Lines():
			if !ok {
				return
			}
			s.logLine(logstore.In, line)
		}
	}
}

// login sends the auto-login line. The password is substituted only into
// what goes over the wire; the log gets the template with "***".
func (s *Session) login(c LineConn) {
	tmpl := s.o.Char.Login
	if tmpl == "" {
		return
	}
	withName := strings.ReplaceAll(tmpl, "{name}", s.o.Char.Name)
	wire := withName
	if strings.Contains(tmpl, "{password}") {
		var pw string
		var err error
		if s.o.Password != nil {
			pw, err = s.o.Password()
		}
		if s.o.Password == nil || err != nil || pw == "" {
			s.sys(fmt.Sprintf("no saved password for %s/%s; skipping auto-login (run: kiln passwd %s %s)",
				s.o.Char.World, s.o.Char.ID, s.o.Char.World, s.o.Char.ID))
			return
		}
		wire = strings.ReplaceAll(withName, "{password}", pw)
	}
	if err := c.Send(wire); err != nil {
		return // the read side will notice the dead connection
	}
	s.logLine(logstore.Out, strings.ReplaceAll(withName, "{password}", "***"))
}

// wait sleeps for d (forever if d < 0) or until Reconnect or ctx ends.
// It reports false if ctx ended.
func (s *Session) wait(ctx context.Context, d time.Duration) bool {
	var timer <-chan time.Time
	if d >= 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		timer = t.C
	}
	select {
	case <-ctx.Done():
		return false
	case <-s.kick:
		return true
	case <-timer:
		return true
	}
}

func (s *Session) setConn(c LineConn) {
	s.mu.Lock()
	s.c = c
	s.mu.Unlock()
}

// logLine logs first, then tells the UI, so a UI failure never loses data.
func (s *Session) logLine(d logstore.Dir, text string) {
	e := logstore.Entry{Time: s.o.Now(), Dir: d, Text: text}
	if err := s.o.Log.Append(e); err != nil {
		s.events <- Event{Kind: EventLogError, Err: err}
	}
	s.events <- Event{Kind: EventLine, Entry: e}
}

func (s *Session) sys(text string) { s.logLine(logstore.Sys, text) }

func (s *Session) state(st State, err error) {
	s.events <- Event{Kind: EventState, State: st, Err: err}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/session/ && go test -race ./internal/session/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Commit**

```bash
git add internal/session/session_test.go internal/session/session.go
git commit -m "feat(session): auto-login with redaction, logging-first events, backoff reconnect

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 11: Keychain wrapper and `kiln` CLI

**Files:**
- Create: `internal/secrets/secrets.go`, `cmd/kiln/render.go`, `cmd/kiln/main.go`
- Test: `internal/secrets/secrets_test.go`, `cmd/kiln/render_test.go`

**Interfaces:**
- Consumes: everything above
- Produces: `secrets.Get(world, char) (string, error)`, `secrets.Set(world, char, pw) error`, and a `kiln` binary with `tail`, `passwd`, and `trust` subcommands

- [ ] **Step 1: Write the failing tests**

`internal/secrets/secrets_test.go`:

```go
package secrets

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestRoundTrip(t *testing.T) {
	keyring.MockInit() // in-memory keychain; never touches the real one
	if _, err := Get("fm", "kit"); err != keyring.ErrNotFound {
		t.Fatalf("Get before Set = %v, want ErrNotFound", err)
	}
	if err := Set("fm", "kit", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if pw, err := Get("fm", "kit"); err != nil || pw != "hunter2" {
		t.Errorf("Get = %q, %v", pw, err)
	}
	if _, err := Get("fm", "rook"); err != keyring.ErrNotFound {
		t.Errorf("other character = %v, want ErrNotFound", err)
	}
}
```

`cmd/kiln/render_test.go`:

```go
package main

import (
	"testing"

	"kiln/internal/config"
	"kiln/internal/logstore"
	"kiln/internal/rules"
)

func TestRender(t *testing.T) {
	in := func(s string) logstore.Entry { return logstore.Entry{Dir: logstore.In, Text: s} }
	cases := []struct {
		name string
		e    logstore.Entry
		res  rules.Result
		want string
	}{
		{"plain", in("hi"), rules.Result{}, "hi\x1b[0m"},
		{"sent", logstore.Entry{Dir: logstore.Out, Text: ":grins."}, rules.Result{}, "\x1b[2m> :grins.\x1b[0m"},
		{"sys", logstore.Entry{Dir: logstore.Sys, Text: "connected"}, rules.Result{}, "\x1b[2m* connected\x1b[0m"},
		{"styled attention", in("Mira pages: hi"),
			rules.Result{Style: config.Style{FG: "#ff9f43", Bold: true}, Styled: true, Attention: true},
			"\x1b[1;38;2;255;159;67m» Mira pages: hi\x1b[0m"},
		{"style survives server reset", in("\x1b[1mMira\x1b[0m pages"),
			rules.Result{Style: config.Style{Italic: true}, Styled: true},
			"\x1b[3m\x1b[1mMira\x1b[0m\x1b[3m pages\x1b[0m"},
		{"bad color ignored", in("x"), rules.Result{Style: config.Style{FG: "orange"}, Styled: true}, "x\x1b[0m"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := render(c.e, c.res); got != c.want {
				t.Errorf("render = %q, want %q", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/secrets/ ./cmd/kiln/`
Expected: FAIL with build errors such as `undefined: Set` and `undefined: render`.

- [ ] **Step 3: Write the implementation**

`internal/secrets/secrets.go`:

```go
// Package secrets stores character passwords in the OS keychain.
package secrets

import "github.com/zalando/go-keyring"

const service = "kiln"

func key(world, char string) string { return world + "/" + char }

// Get returns the saved password for a character.
func Get(world, char string) (string, error) {
	return keyring.Get(service, key(world, char))
}

// Set saves the password for a character.
func Set(world, char, password string) error {
	return keyring.Set(service, key(world, char), password)
}
```

`cmd/kiln/render.go`:

```go
package main

import (
	"fmt"
	"strconv"
	"strings"

	"kiln/internal/config"
	"kiln/internal/logstore"
	"kiln/internal/rules"
)

// render formats one entry for plain terminal output. Highlighted lines
// are wrapped in the rule's style; attention lines get a "» " marker.
// Sent lines are dimmed and sys lines are prefixed with "* ".
func render(e logstore.Entry, res rules.Result) string {
	switch e.Dir {
	case logstore.Out:
		return "\x1b[2m> " + e.Text + "\x1b[0m"
	case logstore.Sys:
		return "\x1b[2m* " + e.Text + "\x1b[0m"
	}
	marker := ""
	if res.Attention {
		marker = "» "
	}
	if !res.Styled {
		return marker + e.Text + "\x1b[0m"
	}
	sgr := sgrFor(res.Style)
	// Re-apply our style after any reset inside the server's own ANSI.
	text := strings.ReplaceAll(e.Text, "\x1b[0m", "\x1b[0m"+sgr)
	text = strings.ReplaceAll(text, "\x1b[m", "\x1b[m"+sgr)
	return sgr + marker + text + "\x1b[0m"
}

func sgrFor(s config.Style) string {
	var codes []string
	if s.Bold {
		codes = append(codes, "1")
	}
	if s.Italic {
		codes = append(codes, "3")
	}
	if s.Underline {
		codes = append(codes, "4")
	}
	if r, g, b, ok := hexRGB(s.FG); ok {
		codes = append(codes, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	}
	if r, g, b, ok := hexRGB(s.BG); ok {
		codes = append(codes, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
	}
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// hexRGB parses "#rrggbb".
func hexRGB(h string) (r, g, b uint8, ok bool) {
	if len(h) != 7 || h[0] != '#' {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(h[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v), true
}
```

`cmd/kiln/main.go`:

```go
// Command kiln is a terminal MUCK client.
//
// Until the full TUI lands, it offers:
//
//	kiln tail <world> <char>          connect, print output, send stdin lines
//	kiln passwd <world> <char>        save a character's password in the keychain
//	kiln trust <world> <fingerprint>  accept a changed server certificate
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"

	"golang.org/x/term"

	"kiln/internal/ansi"
	"kiln/internal/classify"
	"kiln/internal/config"
	"kiln/internal/conn"
	"kiln/internal/logstore"
	"kiln/internal/rules"
	"kiln/internal/secrets"
	"kiln/internal/session"
)

const usage = `usage:
  kiln tail <world> <char>
  kiln passwd <world> <char>
  kiln trust <world> <fingerprint>`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kiln:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 3 {
		return errors.New(usage)
	}
	cfgDir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := config.EnsureDefaults(cfgDir); err != nil {
		return err
	}
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return err
	}
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	switch args[0] {
	case "tail":
		ch, ok := cfg.Find(args[1], args[2])
		if !ok {
			return fmt.Errorf("no character %s/%s (define it in %s)", args[1], args[2], filepath.Join(cfgDir, "worlds", args[1]+".toml"))
		}
		return tail(ch, dataDir)
	case "passwd":
		if _, ok := cfg.Find(args[1], args[2]); !ok {
			return fmt.Errorf("no character %s/%s", args[1], args[2])
		}
		fmt.Fprintf(os.Stderr, "password for %s/%s: ", args[1], args[2])
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		return secrets.Set(args[1], args[2], string(pw))
	case "trust":
		w := findWorld(cfg, args[1])
		if w == nil {
			return fmt.Errorf("no world %s", args[1])
		}
		ch := w.Characters[0]
		hp := ch.Host + ":" + strconv.Itoa(ch.Port)
		return knownHosts(dataDir).Trust(hp, args[2])
	}
	return errors.New(usage)
}

func findWorld(cfg *config.Config, id string) *config.World {
	for i := range cfg.Worlds {
		if cfg.Worlds[i].ID == id && len(cfg.Worlds[i].Characters) > 0 {
			return &cfg.Worlds[i]
		}
	}
	return nil
}

func knownHosts(dataDir string) conn.KnownHosts {
	return conn.KnownHosts{Path: filepath.Join(dataDir, "known_hosts")}
}

func tail(ch config.Character, dataDir string) error {
	cls, err := classify.New(ch.Rules.Classify, ch.Name, ch.Aliases)
	if err != nil {
		return err
	}
	hl, err := rules.New(ch.Rules.Highlight)
	if err != nil {
		return err
	}
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		w, h = 80, 24
	}
	logw := logstore.NewWriter(filepath.Join(dataDir, "logs"), ch.World, ch.ID)
	defer logw.Close()

	s := session.New(session.Options{
		Char: ch,
		Log:  logw,
		Dial: func(ctx context.Context) (session.LineConn, error) {
			return conn.Dial(ctx, conn.Options{
				Host: ch.Host, Port: ch.Port, TLS: ch.TLS, TLSTrust: ch.TLSTrust,
				KnownHosts: knownHosts(dataDir), Width: w, Height: h,
			})
		},
		Password: func() (string, error) { return secrets.Get(ch.World, ch.ID) },
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if err := s.Send(sc.Text()); err != nil {
				fmt.Fprintln(os.Stderr, "\x1b[2m* not sent:", err, "\x1b[0m")
			}
		}
		stop() // stdin closed: quit
	}()
	go s.Run(ctx)

	for ev := range s.Events() {
		switch ev.Kind {
		case session.EventLine:
			var res rules.Result
			if ev.Entry.Dir == logstore.In {
				plain := ansi.Strip(ev.Entry.Text)
				res = hl.Apply(plain, cls.Classify(plain))
			}
			fmt.Println(render(ev.Entry, res))
		case session.EventLogError:
			fmt.Fprintln(os.Stderr, "\x1b[31m* log write failed:", ev.Err, "\x1b[0m")
		case session.EventState:
			var pin *conn.PinMismatchError
			if errors.As(ev.Err, &pin) {
				fmt.Fprintf(os.Stderr, "* if you expected this, run: kiln trust %s %s\n", ch.World, pin.Got)
				stop()
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./internal/secrets/ ./cmd/kiln/ && go test ./internal/secrets/ ./cmd/kiln/`
Expected: `gofmt` prints nothing, and every package reports `ok`.

- [ ] **Step 5: Tidy the module and run the whole suite**

Run: `go mod tidy && go vet ./... && go test -race ./... && go build -o kiln ./cmd/kiln`
Expected: every package reports `ok`, and a `kiln` binary gets built (it's gitignored).


- [ ] **Commit**

```bash
git add go.mod go.sum internal/secrets/secrets_test.go cmd/kiln/render_test.go internal/secrets/secrets.go cmd/kiln/render.go cmd/kiln/main.go
git commit -m "feat: kiln tail/passwd/trust CLI on the core engine

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01RJb5onmojpjBDQMUF8AjNT"
```

---

### Task 12: Manual smoke test against a real MUCK

**Files:** none (verification only)

- [ ] **Step 1: Point a throwaway config at FurryMUCK's guest login**

```bash
export XDG_CONFIG_HOME=$(mktemp -d) XDG_DATA_HOME=$(mktemp -d)
mkdir -p $XDG_CONFIG_HOME/kiln/worlds
printf 'host = "furrymuck.com"\nport = 8899\ntls = true\nuse = ["fuzzball"]\n[characters.guest]\nname = "Guest"\n' > $XDG_CONFIG_HOME/kiln/worlds/fm.toml
./kiln tail fm guest
```

- [ ] **Step 2: Check behavior interactively**

Expected:
- `* connected to furrymuck.com:8899` (dim), then the ASCII-art banner.
- The line containing `connect guest guest` is bold with a `»` marker. That comes from the `self` tag plus the fuzzball pack.
- Typing `QUIT` shows `> QUIT` and then `* disconnected (quit)`, and it does **not** reconnect. Ctrl-C exits.
- `cat $XDG_DATA_HOME/kiln/known_hosts` shows one `furrymuck.com:8899 sha256:…` line.
- `head -3 $XDG_DATA_HOME/kiln/logs/fm/guest/*.log` starts with `#kiln-log v1`, followed by tab-separated entries.

- [ ] **Step 3: Check the pin-mismatch path**

```bash
sed -i '' 's/sha256:.*/sha256:0000/' $XDG_DATA_HOME/kiln/known_hosts
./kiln tail fm guest
```

Expected: `* connect failed: certificate for furrymuck.com:8899 changed: …`, then `* if you expected this, run: kiln trust fm sha256:…`, and the process exits. After you run that `kiln trust` command, `./kiln tail fm guest` connects normally again.
