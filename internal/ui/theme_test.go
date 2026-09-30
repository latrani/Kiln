package ui

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// withTheme draws with src (on top of the built-in theme) for one test.
func withTheme(t *testing.T, src string) *theme.Theme {
	t.Helper()
	th, err := theme.FromTOML(src)
	if err != nil {
		t.Fatal(err)
	}
	theme.SetActive(th)
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	return th
}

func TestSidebarUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.m.chars["fm/rook"].unread, h.m.chars["fm/rook"].attention = 2, true
	th := withTheme(t, `[ui]
sidebar = { bg = "#010203" }
"sidebar.world" = { fg = "#0a0b0c" }
"sidebar.active" = { fg = "#0d0e0f" }
"sidebar.attention" = { fg = "#101112" }
divider = { fg = "#131415" }`)
	s := h.drawn()
	for _, role := range []theme.Role{theme.Sidebar, theme.SidebarWorld, theme.SidebarActive, theme.SidebarAttention, theme.Divider} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("screen doesn't draw %s", role)
		}
	}
}

func TestPickerUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
"picker.world" = { fg = "#0a0b0c" }
"picker.selected" = { fg = "#0d0e0f" }
"picker.add" = { fg = "#101112" }`)
	h.press('o', tea.ModCtrl)
	s := h.drawn()
	for _, role := range []theme.Role{theme.PickerWorld, theme.PickerSelected, theme.PickerAdd} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("picker doesn't draw %s", role)
		}
	}
}

func TestInputAreaUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
input = { bg = "#010203" }
"input.over_limit" = { fg = "#0a0b0c" }
"input.hint" = { fg = "#0d0e0f" }`)
	s := h.drawn() // disconnected: the input area shows a hint
	if !strings.Contains(s, th.SGR(theme.InputHint)) {
		t.Error("hint not drawn in input.hint")
	}
	h.typeText("this line runs past twenty bytes")
	if !strings.Contains(h.drawn(), th.SGR(theme.InputOverLimit)) {
		t.Error("over-limit text not drawn in input.over_limit")
	}
}

// TestInputAreaHasNoHoles: every input row is painted edge to edge, and
// a styled stretch inside one doesn't leave the terminal's background
// after it.
func TestInputAreaHasNoHoles(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, "[ui]\ninput = { bg = \"#010203\" }\n")
	h.typeText("this line runs past twenty bytes")
	base := th.SGR(theme.Input)
	l := h.m.layout()
	row := strings.Split(h.drawn(), "\n")[l.top+l.sbH+1]
	_, right, _ := strings.Cut(row, "│")
	right = strings.TrimPrefix(right, theme.Reset) // the divider's own reset
	if !strings.HasPrefix(right, base) {
		t.Errorf("input row doesn't start painted: %q", right)
	}
	for _, part := range strings.Split(right, theme.Reset)[1:] {
		if part != "" && !strings.HasPrefix(part, base) {
			t.Errorf("a reset inside the input row isn't followed by the area's colors: %q", right)
			break
		}
	}
}

func TestFormUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
"form.label" = { fg = "#0a0b0c" }
"form.focus" = { fg = "#0d0e0f" }
"form.title" = { fg = "#101112" }
"form.error" = { fg = "#131415" }`)
	h.typeText("/edit world")
	h.enter()
	h.focusOn(saveLabel) // a button draws focus; a text field shows a cursor
	s := h.drawn()
	for _, role := range []theme.Role{theme.FormLabel, theme.FormFocus, theme.FormTitle} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("editor doesn't draw %s", role)
		}
	}
	h.focusOn(portLabel)
	h.typeText("x")
	if !strings.Contains(h.drawn(), th.SGR(theme.FormError)) {
		t.Error("field error not drawn in form.error")
	}
}

func TestStatusUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
status = { bg = "#010203" }
"status.error" = { fg = "#0a0b0c" }`)
	h.m.setStatus(true, "x")
	s := h.drawn()
	for _, role := range []theme.Role{theme.Status, theme.StatusError} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("screen doesn't draw %s", role)
		}
	}
}

func TestScrollbackUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": goldenWorld}) // before the theme: New loads the built-in
	th := withTheme(t, `[ui]
"scrollback.sys" = { fg = "#0a0b0c" }
"scrollback.echo" = { fg = "#0d0e0f" }
"scrollback.pill" = { fg = "#101112" }
link = { fg = "#131415" }`)
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.show("see https://kiln.test/map")
	h.typeText(":waves.")
	h.enter()
	s := h.drawn()
	for _, role := range []theme.Role{theme.ScrollbackSys, theme.ScrollbackEcho, theme.Link} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("scrollback doesn't draw %s", role)
		}
	}
	for i := 0; i < 40; i++ {
		h.advance(2 * time.Second)
		h.show(fmt.Sprintf("filler %d", i))
	}
	h.press(tea.KeyPgUp, 0)
	if !strings.Contains(h.drawn(), th.SGR(theme.ScrollbackPill)) {
		t.Error("pill not drawn in scrollback.pill")
	}
}

func TestLogModeUsesTheme(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	th := withTheme(t, `[ui]
"status.filter.on" = { fg = "#0d0e0f" }
"log.time" = { fg = "#101112" }
"log.cursor" = { fg = "#131415" }
"log.find" = { fg = "#161718" }
"log.day" = { fg = "#191a1b" }
"log.bar" = { bg = "#1c1d1e" }
"log.bar.hints" = { fg = "#1f2021" }`)
	h.key("ctrl+l")
	s := h.drawn()
	for _, role := range []theme.Role{theme.LogTime, theme.LogCursor, theme.LogDay, theme.LogBar, theme.LogHints} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("log mode doesn't draw %s", role)
		}
	}
	h.kitFilter().PressOnly(scene.Item{Name: "page"}, h.br().items())
	h.key("/")
	h.typeText("Mira") // only pages show
	h.key("enter")
	h.key("f") // the panel open: the Filter chip on
	s = h.drawn()
	for _, role := range []theme.Role{theme.StatusFilterOn, theme.LogFind} {
		if !strings.Contains(s, th.SGR(role)) {
			t.Errorf("log mode doesn't draw %s", role)
		}
	}
}

func TestBackdropBehindModals(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld}) // before the theme: New loads the built-in
	th := withTheme(t, "[ui]\n\"scrollback.inactive\" = { fg = \"#0a0b0c\" }\n")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.show("Rook pages: see https://kiln.test/map")
	inactive := th.SGR(theme.ScrollbackInactive)
	if strings.Contains(h.drawn(), inactive) {
		t.Fatal("backdrop drawn with no modal open")
	}
	h.press('o', tea.ModCtrl)
	s := h.drawn()
	if !strings.Contains(s, inactive) || strings.Contains(s, th.SGR(theme.Link)) {
		t.Errorf("behind the picker, the scrollback should be one plain inactive color:\n%q", s)
	}
	h.press(tea.KeyEscape, 0)
	if strings.Contains(h.drawn(), inactive) {
		t.Error("backdrop outlived the picker")
	}
}

func writeUserTheme(t *testing.T, h *harness, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(h.dir, "themes", "default.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestThemeReloads(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": goldenWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText(":waves.")
	h.enter()
	writeUserTheme(t, h, "extends = \"kiln\"\n[ui]\n\"scrollback.echo\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{})
	want := theme.Active().SGR(theme.ScrollbackEcho)
	if want != "\x1b[2;38;2;10;11;12m" {
		t.Fatalf("theme not reloaded: %q", want)
	}
	if !strings.Contains(h.drawn(), want) {
		t.Error("an echo line already on screen wasn't restyled")
	}
}

func TestThemeReloadErrorKeepsTheme(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	writeUserTheme(t, h, "extends = \"kiln\"\n[ui]\n\"status.error\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{})
	good := theme.Active()
	writeUserTheme(t, h, "[ui]\n\"status.eror\" = { fg = \"red\" }\n")
	h.m.Update(reloadMsg{})
	if theme.Active() != good {
		t.Error("a broken theme replaced a working one")
	}
	if !strings.Contains(h.screen(), upTo(str.StatusThemeNotLoaded(errors.New(mark)))) {
		t.Errorf("no word about the broken theme:\n%s", h.screen())
	}
}

func TestBrokenThemeAtStartUsesBuiltin(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	dir := t.TempDir()
	config.EnsureDefaults(dir)
	os.WriteFile(filepath.Join(dir, "themes", "default.toml"), []byte("[ui\n"), 0o644)
	cfg, _ := config.Load(dir)
	m := New(Deps{ConfigDir: dir, Load: config.Load, Now: func() time.Time { return time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local) }}, cfg)
	if theme.Active().SGR(theme.StatusError) != theme.Builtin().SGR(theme.StatusError) || !strings.Contains(m.status, upTo(str.StatusThemeNotLoaded(errors.New(mark)))) {
		t.Errorf("want the built-in theme and a status, got %q", m.status)
	}
}

func TestThemeReloadKeepsScroll(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	for i := 0; i < 40; i++ {
		h.advance(2 * time.Second)
		h.show(fmt.Sprintf("line %d", i))
	}
	h.press(tea.KeyPgUp, 0)
	sb := &h.m.chars["fm/kit"].sb
	offset := sb.offset
	sb.StartSelect(sbPos{line: 5})
	writeUserTheme(t, h, "extends = \"kiln\"\n[ui]\n\"scrollback.sys\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{})
	if !sb.Scrolled() || sb.offset != offset {
		t.Errorf("reload moved the view: offset %d, was %d", sb.offset, offset)
	}
	h.screen() // must not panic with the selection dropped
}

// A broken theme is only a theme problem: adding a character still opens
// and connects it, and the theme's error stays up over any success message.
func TestBrokenThemeDoesNotBlockAddingACharacter(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	writeUserTheme(t, h, "[ui]\n\"status.eror\" = { fg = \"red\" }\n")
	h.press('o', tea.ModCtrl)
	h.press(tea.KeyDown, 0) // Rook, then fm's add row
	h.enter()
	h.typeText("Ash")
	h.enter()
	if cs := h.m.chars["fm/Ash"]; cs == nil || cs.sess == nil || h.m.active != "fm/Ash" {
		t.Fatalf("a broken theme stopped Ash from opening:\n%s", h.screen())
	}
	if !strings.Contains(h.screen(), upTo(str.StatusThemeNotLoaded(errors.New(mark)))) {
		t.Errorf("the theme's error should stay up:\n%s", h.screen())
	}
}

// TestActiveRowWinsOverItsParts: with the sidebar colored (so every
// sidebar.* child inherits a color), the active row's name and unread
// count still draw in sidebar.active, and the attention dot's reset
// doesn't end the highlight.
func TestActiveRowWinsOverItsParts(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.m.switchTo("fm/rook")
	h.m.chars["fm/rook"].unread, h.m.chars["fm/rook"].attention = 3, true
	withTheme(t, `[ui]
sidebar = { fg = "#6b6f7a", bg = "#1f2029" }
"sidebar.char" = { fg = "#0a0b0c" }
"sidebar.unread" = { fg = "#0d0e0f" }
"sidebar.active" = { fg = "#ffffff", bold = true }`)
	row := strings.Split(cells(h.drawn()), "\n")
	for i, r := range row {
		if i > 0 && strings.Contains(r, " Rook") && i+1 < len(row) { // not the top bar, which names the character too
			runs := row[i+1]
			if strings.Contains(runs, "fg=#0a0b0c") || strings.Contains(runs, "fg=#0d0e0f") {
				t.Errorf("a child role drew over the active row: %s\n%s", r, runs)
			}
			if !strings.HasPrefix(strings.TrimSpace(runs), "@0-") || strings.Count(runs, "fg=#ffffff") < 2 {
				t.Errorf("the active style should cover the name and, past the dot, the count: %s\n%s", r, runs)
			}
			return
		}
	}
	t.Fatalf("no Rook row:\n%s", h.screen())
}

// Reloading a config that leaves the theme as it was doesn't restyle, so
// a live selection survives.
func TestUnchangedThemeReloadKeepsSelection(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.show("line")
	writeUserTheme(t, h, "extends = \"kiln\"\n[ui]\n\"scrollback.sys\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{})
	sb := &h.m.chars["fm/kit"].sb
	sb.StartSelect(sbPos{line: 0})
	h.m.Update(reloadMsg{})
	if sb.sel == nil {
		t.Error("a reload with the same theme dropped the selection")
	}
}

// History read under one theme and arriving after a change is restyled.
func TestOlderHistoryArrivingAfterThemeChange(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	dir := t.TempDir()
	root := filepath.Join(dir, "logs")
	w := logstore.NewWriter(logstore.Layout{Root: root, World: "fm", Char: "kit", CharName: "Kit"})
	for _, d := range []int{22, 23, 24} {
		start := time.Date(2026, 9, d, 8, 0, 0, 0, time.Local)
		for i := 0; i < 150; i++ {
			w.Append(logstore.Entry{Time: start.Add(time.Duration(i) * time.Minute), Dir: logstore.In, Text: fmt.Sprintf("d%d-%03d", d, i)})
		}
	}
	w.Close()
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.d.LogRoot = root
	cs := h.m.chars["fm/kit"]
	cs.sb = Scrollback{}
	h.m.preload(cs)
	var msg sbOlderMsg
	for i := 0; i < 100 && msg.lines == nil; i++ {
		if cmd := h.press(tea.KeyPgUp, 0); cmd != nil {
			msg = cmd().(sbOlderMsg) // read under the old theme...
		}
		h.screen()
	}
	if msg.lines == nil {
		t.Fatal("no older batch was read")
	}
	writeUserTheme(t, h, "extends = \"kiln\"\n[ui]\n\"scrollback.day\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{}) // ...the theme changes...
	h.m.Update(msg)         // ...then the batch arrives
	day := theme.Active().SGR(theme.ScrollbackDay)
	n := 0
	for _, l := range cs.sb.lines {
		if l.role == theme.ScrollbackDay {
			n++
			if !strings.HasPrefix(l.text, day) {
				t.Errorf("divider %q kept the old style", l.text)
			}
		}
	}
	if n < 2 {
		t.Errorf("dividers = %d, want the preload's and the batch's", n)
	}
}

// Each rule takes the color of the area below it: the input's or, while
// a form is up, the form's; the lower one, the statusline's.
func TestRulesTakeTheirAreas(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
"rule.input" = { fg = "#0a0b0c" }
"rule.form" = { fg = "#0d0e0f" }
"rule.status" = { fg = "#101112" }`)
	rules := func() (top, bottom string) {
		l := h.m.layout()
		rows := strings.Split(h.drawn(), "\n")
		return rows[l.top+l.sbH], rows[l.top+l.sbH+len(l.inRows)+1]
	}
	top, bottom := rules()
	if !strings.Contains(top, th.SGR(theme.RuleInput)) || !strings.Contains(bottom, th.SGR(theme.RuleStatus)) {
		t.Errorf("rules: top %q, bottom %q", top, bottom)
	}
	h.typeText("/edit world")
	h.enter()
	top, bottom = rules()
	if !strings.Contains(top, th.SGR(theme.RuleForm)) || !strings.Contains(bottom, th.SGR(theme.RuleStatus)) {
		t.Errorf("rules with a form up: top %q, bottom %q", top, bottom)
	}
}

// While the picker is open the sidebar is the picker's area, and a form
// fills the input area with the form's.
func TestPickerAndFormAreAreas(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
sidebar = { bg = "#010203" }
picker = { bg = "#040506" }
form = { bg = "#070809" }`)
	h.press('o', tea.ModCtrl)
	rows := strings.Split(h.drawn(), "\n")
	for y, r := range rows {
		if !strings.HasPrefix(r, th.SGR(theme.Picker)) {
			t.Fatalf("sidebar row %d isn't the picker's area: %q", y, r)
		}
	}
	l := h.m.layout()
	_, in, _ := strings.Cut(rows[l.top+l.sbH+1], "│")
	if in = strings.TrimPrefix(in, theme.Reset); !strings.HasPrefix(in, th.SGR(theme.Form)) {
		t.Errorf("the picker's filter row isn't the form's area: %q", in)
	}
}

// Everything you can click is a chip: its label with a space either
// side, in its role.
func TestClickablesAreChips(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
"sidebar.add" = { bg = "#010203" }
"picker.add" = { bg = "#040506" }
"form.button" = { bg = "#070809" }`)
	chip := func(r theme.Role, label string) string { return th.Paint(r, " "+label+" ") }
	if s := h.drawn(); !strings.Contains(s, chip(theme.SidebarAdd, addLabel)) {
		t.Errorf("no %s chip:\n%q", addLabel, s)
	}
	h.press('o', tea.ModCtrl)
	if s := h.drawn(); !strings.Contains(s, chip(theme.PickerAdd, addWorldLabel)) {
		t.Errorf("no %s chip:\n%q", addWorldLabel, s)
	}
	h.press(tea.KeyEsc, 0)
	h.typeText("/edit world")
	h.enter()
	if s := h.drawn(); !strings.Contains(s, chip(theme.FormButton, saveLabel)) {
		t.Errorf("no %s chip:\n%q", saveLabel, s)
	}
}

// Save is a form's primary button; the rest are secondary.
func TestSecondaryButtons(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	th := withTheme(t, `[ui]
"form.button" = { bg = "#070809" }
"form.button.secondary" = { bg = "#0a0b0c" }`)
	chip := func(r theme.Role, label string) string { return th.Paint(r, " "+label+" ") }
	h.typeText("/edit world")
	h.enter()
	s := h.drawn()
	if !strings.Contains(s, chip(theme.FormButton, saveLabel)) || !strings.Contains(s, chip(theme.FormButtonSecondary, delWorldLabel)) {
		t.Errorf("want %s primary and %s secondary:\n%q", saveLabel, delWorldLabel, s)
	}
	h.press(tea.KeyEsc, 0)
	h.typeText("/edit")
	h.enter()
	s = h.drawn()
	for _, l := range []string{forgetPWLabel, delCharLabel} {
		if !strings.Contains(s, chip(theme.FormButtonSecondary, l)) {
			t.Errorf("want %s secondary:\n%q", l, s)
		}
	}
}

// Editing the theme's [tags] restyles lines already on screen.
func TestThemeTagChangeRestyles(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.show("Mira pages: you around?")
	writeUserTheme(t, h, "extends = \"kiln\"\n[tags]\n\"page/in\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{})
	if !strings.Contains(h.drawn(), "\x1b[1;38;2;10;11;12m") {
		t.Error("the page wasn't restyled with the theme's new page/in color (bold from the built-in)")
	}
}

// A world's look naming a color nobody defines is reported, and the
// character still draws with the theme's own tag styles.
func TestBadWorldLookFallsBackToTheme(t *testing.T) {
	world := fmWorld + "\n[tags]\n\"page/in\" = { fg = \"nowhere\" }\n"
	h := newHarness(t, map[string]string{"fm": world})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	want := str.StatusCharError("fm/kit", errors.New(str.ThemeBadColor(filepath.Join("worlds", "fm.toml"), str.ThemeTagEntry("page/in"), "nowhere")))
	if !strings.Contains(h.m.status, want) {
		t.Errorf("status = %q, want the bad color reported", h.m.status)
	}
	h.show("Mira pages: you around?")
	_, ts, _ := theme.Active().Tag("page/in")
	if !strings.Contains(h.drawn(), ts.Style.SGR()+"Mira pages") {
		t.Error("the page should draw in the theme's own page/in style")
	}
}

// Editing a world's look into one that doesn't resolve redraws what's on
// screen in the theme's own tag styles, not the old look.
func TestBrokenLookEditRestyles(t *testing.T) {
	world := fmWorld + "\n[tags]\n\"page/in\" = { fg = \"#0a0b0c\" }\n"
	h := newHarness(t, map[string]string{"fm": world})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.show("Mira pages: you around?")
	broken := fmWorld + "\n[tags]\n\"page/in\" = { fg = \"nowhere\" }\n"
	if err := os.WriteFile(filepath.Join(h.dir, "worlds", "fm.toml"), []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	h.m.Update(reloadMsg{})
	_, ts, _ := theme.Active().Tag("page/in")
	if s := h.drawn(); !strings.Contains(s, ts.Style.SGR()+"Mira pages") {
		t.Errorf("the page kept its old look after the edit:\n%q", s)
	}
}

var (
	lightBG = tea.BackgroundColorMsg{Color: color.RGBA{0xfb, 0xf8, 0xf1, 0xff}}
	darkBG  = tea.BackgroundColorMsg{Color: color.RGBA{0x31, 0x2a, 0x54, 0xff}}
)

func withAppearance(t *testing.T, h *harness, setting string) {
	t.Helper()
	os.WriteFile(filepath.Join(h.dir, "config.toml"), []byte("appearance = \""+setting+"\"\n"), 0o600)
	h.m.Update(reloadMsg{})
}

func TestLightAnswerRestyles(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(lightBG)
	if theme.Active().SGR(theme.Sidebar) != theme.BuiltinFor(theme.Light).SGR(theme.Sidebar) {
		t.Error("a light answer should switch to the light palette")
	}
	h.m.Update(darkBG)
	if theme.Active().SGR(theme.Sidebar) != theme.Builtin().SGR(theme.Sidebar) {
		t.Error("a dark answer should switch back")
	}
}

func TestNoAnswerStaysDark(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if theme.Active().SGR(theme.Sidebar) != theme.Builtin().SGR(theme.Sidebar) || h.m.status != "" {
		t.Errorf("with no answer: sidebar %q, status %q", theme.Active().SGR(theme.Sidebar), h.m.status)
	}
}

func TestPinnedAppearanceIgnoresAnswer(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	withAppearance(t, h, "light")
	light := theme.Active()
	if light.SGR(theme.Sidebar) != theme.BuiltinFor(theme.Light).SGR(theme.Sidebar) {
		t.Fatal("appearance = light should draw light")
	}
	h.m.Update(darkBG)
	if theme.Active() != light {
		t.Error("a pinned appearance must ignore the terminal's answer")
	}
	if cmd := h.m.Init(); cmdAsks(cmd) {
		t.Error("a pinned appearance must not ask")
	}
}

func TestSameAnswerKeepsSelection(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.show("line")
	sb := &h.m.chars["fm/kit"].sb
	sb.StartSelect(sbPos{line: 0})
	h.m.Update(darkBG) // already dark
	if sb.sel == nil {
		t.Error("an answer that doesn't change the appearance dropped the selection")
	}
}

func TestAsksOnStartAndFocus(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if !cmdAsks(h.m.Init()) {
		t.Error("auto should ask at start")
	}
	_, cmd := h.m.Update(tea.FocusMsg{})
	if !cmdAsks(cmd) {
		t.Error("auto should ask again on focus-in")
	}
}

func TestMissingNamedTheme(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	writeUserTheme(t, h, "extends = \"kiln\"\n[ui]\n\"status.error\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{})
	had := theme.Active()
	os.WriteFile(filepath.Join(h.dir, "config.toml"), []byte("theme = \"nope\"\n"), 0o600)
	h.m.Update(reloadMsg{})
	if !strings.Contains(h.m.status, str.ThemeNoTheme("nope")) {
		t.Errorf("status = %q", h.m.status)
	}
	if theme.Active() != had {
		t.Error("a missing theme on reload should keep the colors Kiln had")
	}
}

// cmdAsks reports whether cmd is, or is a batch holding, a
// background-color request. It compares functions, and only ever calls
// the top-level command (a tea.Batch's function just returns its list),
// so nothing else in the batch runs.
func cmdAsks(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	want := reflect.ValueOf(tea.RequestBackgroundColor).Pointer()
	if reflect.ValueOf(cmd).Pointer() == want {
		return true
	}
	b, ok := cmd().(tea.BatchMsg)
	return ok && slices.ContainsFunc(b, func(c tea.Cmd) bool { return c != nil && reflect.ValueOf(c).Pointer() == want })
}

// A day log mode reads under one theme and receives after a change is
// restyled on arrival, like the scrollback's.
func TestBrowseOlderDayAfterThemeChange(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	h := newHarness(t, map[string]string{"fm": fmWorld})
	for d := 21; d <= 24; d++ {
		lines := make([]string, 150)
		for i := range lines {
			lines[i] = fmt.Sprintf("Mira pages: day %d line %d", d, i)
		}
		h.writeLog(time.Date(2026, 9, d, 8, 0, 0, 0, time.Local), lines...)
	}
	h.key("ctrl+l")
	b := h.br()
	_, cmd := h.m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if cmd == nil {
		t.Fatal("Home should start a background load")
	}
	msg := cmd() // read under the old theme...
	writeUserTheme(t, h, "extends = \"kiln\"\n[tags]\n\"page/in\" = { fg = \"#0a0b0c\" }\n")
	h.m.Update(reloadMsg{}) // ...the theme changes...
	h.m.Update(msg)         // ...then the day arrives
	cs := h.m.chars["fm/kit"]
	for _, l := range b.lines {
		if want, _ := cs.render(l.e); l.text != want {
			t.Fatalf("%q kept its old style", l.e.Text)
		}
	}
}

// A light answer switches a world's own [palette.light] in, even when
// the theme itself looks the same either way.
func TestLayerLightPaletteFollowsAnswer(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	world := fmWorld + "\n[palette]\nx = \"#aaaaaa\"\n\n[palette.light]\nx = \"#111111\"\n\n[tags]\n\"page/in\" = { fg = \"x\" }\n"
	h := newHarness(t, map[string]string{"fm": world})
	writeUserTheme(t, h, "[ui]\nsidebar = { bold = true }\n") // no extends: the same in both appearances
	h.m.Update(reloadMsg{})
	h.m.Update(lightBG)
	text, _ := h.m.chars["fm/kit"].render(logstore.Entry{Dir: logstore.In, Text: "Mira pages: hi"})
	if !strings.Contains(text, "\x1b[38;2;17;17;17m") {
		t.Errorf("the world's light palette wasn't used: %q", text)
	}
}

// A broken theme at start stands in the built-in, which still follows
// the terminal's answer.
func TestStandInBuiltinFollowsAnswer(t *testing.T) {
	t.Cleanup(func() { theme.SetActive(theme.Builtin()) })
	dir := t.TempDir()
	config.EnsureDefaults(dir)
	os.WriteFile(filepath.Join(dir, "themes", "default.toml"), []byte("[ui\n"), 0o644)
	cfg, _ := config.Load(dir)
	m := New(Deps{ConfigDir: dir, Load: config.Load, Now: func() time.Time { return time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local) }}, cfg)
	m.Update(lightBG)
	if theme.Active() != theme.BuiltinFor(theme.Light) {
		t.Error("the stand-in built-in should turn light with a light answer")
	}
}
