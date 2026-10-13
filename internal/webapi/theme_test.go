package webapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// startThemed runs a loop that reads themes from dir.
func startThemed(t *testing.T, a *app.App, dir string) *harness {
	t.Helper()
	h := &harness{t: t, a: a, dir: dir, batches: make(chan []map[string]any, 100)}
	h.l = New(a, func(b []byte) { h.batches <- decode(t, b) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go h.l.Run(ctx, Options{ConfigDir: dir})
	return h
}

// setTheme puts theme = name at the top of dir's config.toml.
func setTheme(t *testing.T, dir, name string) {
	t.Helper()
	p := filepath.Join(dir, "config.toml")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append([]byte("theme = \""+name+"\"\n"), b...), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// reload has the loop reload the config, as a settings save does, and
// returns that turn's batch. Each call adds a world named id.
func (h *harness) reload(id string) []map[string]any {
	return h.act(`{"Do":"add_world","ID":"` + id + `","WorldSettings":{"Host":"x.test","Port":7777}}`)
}

func evLooks(t *testing.T, ev map[string]any, ap string) map[string]any {
	t.Helper()
	if ev == nil {
		t.Fatal("no Theme event")
	}
	return ev[ap].(map[string]any)
}

func TestThemeComesAfterStart(t *testing.T) {
	a, dir := newApp(t)
	h := startThemed(t, a, dir)
	b := h.next()
	if ts := types(b); len(ts) < 2 || ts[0] != "Start" || ts[1] != "Theme" {
		t.Fatalf("first batch %v", ts)
	}
	ev := event(b, "Theme")
	if ev["Appearance"] != "auto" {
		t.Errorf("appearance %v", ev["Appearance"])
	}
	dark := evLooks(t, ev, "Dark")["Roles"].(map[string]any)["sidebar.active"]
	light := evLooks(t, ev, "Light")["Roles"].(map[string]any)["sidebar.active"]
	if dark == nil || light == nil || reflect.DeepEqual(dark, light) {
		t.Errorf("sidebar.active dark %v, light %v", dark, light)
	}
	if b := h.act(`{"Do":"here"}`); event(b, "Theme") != nil {
		t.Error("a turn with no config change sent the theme again")
	}
}

func TestThemeFollowsTheConfig(t *testing.T) {
	a, dir := newApp(t)
	h := startThemed(t, a, dir)
	h.next()
	writeFile(t, filepath.Join(dir, "themes", "mine.toml"), "extends = \"kiln\"\n[tags]\npage = { fg = \"#123456\" }\n")
	setTheme(t, dir, "mine")
	ev := event(h.reload("zz"), "Theme")
	tags := evLooks(t, ev, "Dark")["Tags"].(map[string]any)
	if fg := tags["page"].(map[string]any)["FG"]; fg != "#123456" {
		t.Errorf("page fg %v", fg)
	}
}

const kitLooks = `host = "muck.test"
port = 8888
tls = true
use = ["fuzzball"]

[[characters]]
id = "kit"
name = "Kit"
[characters.tags]
"page/in" = { fg = "%s" }

[[characters]]
id = "rook"
name = "Rook"
`

// withKitLooks rewrites fm so Kit styles page/in, which the built-in
// theme leaves to page. The fuzzball pack tags "Mira pages: hi" page and
// page/in.
func withKitLooks(t *testing.T, a *app.App, dir, fg string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "worlds", "fm.toml"), strings.Replace(kitLooks, "%s", fg, 1))
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
}

// feedKit opens Kit, gives it a session and a line from the server, and
// returns the turn's batches until a Lines event for Kit arrives.
func feedKit(t *testing.T, h *harness, text string) map[string]any {
	t.Helper()
	h.act(`{"Do":"open","Key":"fm/kit"}`)
	s := attachTo(h.a, "fm/kit")
	h.l.post(app.SessionMsg{Key: "fm/kit", Sess: s, OK: true, Ev: session.Event{Kind: session.EventLine, Entry: logEntry(text)}})
	for range 5 {
		if ls := event(h.next(), "Lines"); ls != nil && ls["Key"] == "fm/kit" {
			return ls
		}
	}
	t.Fatal("no lines for Kit")
	return nil
}

func lastSpanTags(ls map[string]any) []any {
	lines := ls["Lines"].([]any)
	sp := lines[len(lines)-1].(map[string]any)["Spans"].([]any)
	tags, _ := sp[0].(map[string]any)["Tags"].([]any)
	return tags
}

func TestCharLooksAndSpans(t *testing.T) {
	a, dir := newApp(t)
	withKitLooks(t, a, dir, "#abcdef")
	h := startThemed(t, a, dir)
	ev := event(h.next(), "Theme")
	chars, _ := evLooks(t, ev, "Dark")["Chars"].(map[string]any)
	kit, _ := chars["fm/kit"].(map[string]any)
	if kit == nil || kit["page/in"].(map[string]any)["FG"] != "#abcdef" {
		t.Errorf("Kit's tags %v", chars)
	}
	if _, ok := chars["fm/rook"]; ok {
		t.Error("Rook has no looks of its own but got an entry")
	}
	if tags := lastSpanTags(feedKit(t, h, "Mira pages: hi")); !slices.Equal(tags, []any{"page", "page/in"}) {
		t.Errorf("span tags %v", tags)
	}
}

func TestBadLookFallsBackAndSaysSo(t *testing.T) {
	a, dir := newApp(t)
	withKitLooks(t, a, dir, "nosuchcolor")
	h := startThemed(t, a, dir)
	ev := event(h.next(), "Theme")
	if chars, _ := evLooks(t, ev, "Dark")["Chars"].(map[string]any); chars["fm/kit"] != nil {
		t.Error("a look that didn't resolve was sent")
	}
	b := h.act(`{"Do":"open","Key":"fm/kit"}`)
	want := upTo(str.StatusCharError("fm/kit", errors.New(mark)))
	if st := event(b, "Status"); st == nil || st["Err"] != true || !strings.HasPrefix(st["Text"].(string), want) {
		t.Errorf("status %v", st)
	}
	s := attachTo(h.a, "fm/kit")
	h.l.post(app.SessionMsg{Key: "fm/kit", Sess: s, OK: true, Ev: session.Event{Kind: session.EventLine, Entry: logEntry("Mira pages: hi")}})
	for range 5 {
		if ls := event(h.next(), "Lines"); ls != nil {
			if tags := lastSpanTags(ls); !slices.Equal(tags, []any{"page"}) {
				t.Errorf("span tags %v (the theme's own page style should apply)", tags)
			}
			return
		}
	}
	t.Fatal("no lines for Kit")
}

func TestBrokenThemeKeepsWhatItHad(t *testing.T) {
	a, dir := newApp(t)
	setTheme(t, dir, "missing")
	if _, ok := a.Reload(); !ok {
		t.Fatal(a.Status().Text)
	}
	h := startThemed(t, a, dir)
	b := h.next()
	if st := event(b, "Status"); st == nil || st["Err"] != true {
		t.Errorf("no error for a missing theme: %v", st)
	}
	builtin, _ := theme.BuiltinFor(theme.Dark).Looks()
	got := evLooks(t, event(b, "Theme"), "Dark")["Roles"].(map[string]any)["sidebar.active"].(map[string]any)
	if got["FG"] != nilIfEmpty(builtin[theme.SidebarActive].FG) {
		t.Errorf("sidebar.active %v, built-in %+v", got, builtin[theme.SidebarActive])
	}

	writeFile(t, filepath.Join(dir, "themes", "mine.toml"), "extends = \"kiln\"\n[tags]\npage = { fg = \"#123456\" }\n")
	writeFile(t, filepath.Join(dir, "config.toml"), "theme = \"mine\"\n")
	if ev := event(h.reload("zz"), "Theme"); ev == nil {
		t.Fatal("a good theme after a missing one sent nothing")
	}
	writeFile(t, filepath.Join(dir, "themes", "mine.toml"), "this isn't toml [")
	b = h.reload("yy")
	if event(b, "Theme") != nil {
		t.Error("a broken theme replaced the one that loaded")
	}
	if st := event(b, "Status"); st == nil || st["Err"] != true {
		t.Errorf("no error for a broken theme: %v", st)
	}
}

func logEntry(text string) logstore.Entry {
	return logstore.Entry{Time: day, Dir: logstore.In, Text: text}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// TestNewLooksRestyleLinesAlreadySent: when the styled tag names a line
// gets change (new looks, a new theme), the page gets its lines again,
// as the TUI repaints.
func TestNewLooksRestyleLinesAlreadySent(t *testing.T) {
	a, dir := newApp(t)
	h := startThemed(t, a, dir)
	h.next()
	if tags := lastSpanTags(feedKit(t, h, "Mira pages: hi")); !slices.Equal(tags, []any{"page"}) {
		t.Fatalf("before: span tags %v", tags)
	}
	l, _ := h.a.LogLayout(h.a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, day.AddDate(0, 0, -1), "Mira pages: earlier")
	if b := h.act(`{"Do":"log","Key":"fm/kit"}`); event(b, "LogLines") == nil {
		t.Fatalf("log mode has no lines: %v", types(b))
	}
	writeFile(t, filepath.Join(dir, "worlds", "fm.toml"), strings.Replace(kitLooks, "%s", "#abcdef", 1))
	b := h.reload("zz")
	ls := event(b, "Lines")
	if ls == nil || ls["Key"] != "fm/kit" || ls["Op"] != "replace" {
		t.Fatalf("no restyled lines for Kit: %v", types(b))
	}
	if tags := lastSpanTags(ls); !slices.Equal(tags, []any{"page", "page/in"}) {
		t.Errorf("after: span tags %v", tags)
	}
	if ll := event(b, "LogLines"); ll == nil || ll["Op"] != "replace" {
		t.Errorf("log mode's lines weren't restyled: %v", types(b))
	}
	if b := h.reload("yy"); event(b, "Lines") != nil {
		t.Error("a reload that changes no styled names resent lines")
	}
}
