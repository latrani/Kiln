package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

func TestStartupOpensOnlyAutoconnect(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if got := strings.Join(h.m.a.Order(), " "); got != "fm/kit" {
		t.Errorf("order = %q, want only the autoconnect character", got)
	}
	if h.m.a.Active() != "fm/kit" {
		t.Errorf("active = %q", h.m.a.Active())
	}
}

func TestOpenKeepsAlphabeticalOrder(t *testing.T) {
	// Case-sensitively these would sort Sp < fm and Kit < Rook < bo.
	fm := fmWorld + "\n[[characters]]\nid = \"bo\"\nname = \"bo\"\n"
	sp := "host = \"sp.test\"\nport = 1\n\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n"
	h := newHarness(t, map[string]string{"fm": fm, "Sp": sp})
	h.open("Sp/ash", "fm/rook", "fm/bo")
	if got, want := strings.Join(h.m.a.Order(), " "), "fm/bo fm/kit fm/rook Sp/ash"; got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
	if h.m.open("fm/nobody") != nil {
		t.Error("opened a character that isn't configured")
	}
}

func TestCloseHandsActiveToNeighbor(t *testing.T) {
	sp := "host = \"sp.test\"\nport = 1\n\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n"
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": sp})
	h.open("fm/rook", "sp/ash")
	h.m.switchTo("fm/rook")
	h.m.close("fm/rook") // the next one down
	if h.m.a.Active() != "sp/ash" || h.m.chars["fm/rook"] != nil {
		t.Fatalf("active = %q after closing rook", h.m.a.Active())
	}
	h.m.close("sp/ash") // the last one: the one above
	if h.m.a.Active() != "fm/kit" {
		t.Fatalf("active = %q after closing ash", h.m.a.Active())
	}
	h.m.close("fm/kit")
	if h.m.a.Active() != "" || len(h.m.a.Order()) != 0 || h.m.cur() != nil {
		t.Errorf("active = %q, order = %v; want nothing open", h.m.a.Active(), h.m.a.Order())
	}
}

func TestLateEventFromClosedCharacterIgnored(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	sess := h.m.chars["fm/kit"].Sess
	h.m.close("fm/kit")
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-sess.Events():
			h.m.Update(app.SessionMsg{Key: "fm/kit", Sess: sess, Ev: ev, OK: ok})
			if h.m.chars["fm/kit"] != nil {
				t.Fatal("a late event reopened the closed character")
			}
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("closed session still running")
		}
	}
}

// sideRow is sidebar row y as plain text, untrimmed.
func sideRow(h *harness, y int) string {
	return strings.SplitN(strings.Split(h.screen(), "\n")[y], "│", 2)[0]
}

func TestSidebarBadgesAndActivity(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	if got := strings.TrimSpace(sideRow(h, 1)); got != "Kit" {
		t.Errorf("connected row = %q, want no badge", got)
	}
	if got := strings.TrimSpace(sideRow(h, 2)); got != "× Rook" {
		t.Errorf("disconnected row = %q", got)
	}
	if got := strings.TrimSpace(sideRow(h, 3)); got != "" {
		t.Errorf("row above the add row = %q, want a gap", got)
	}
	if got := strings.TrimSpace(sideRow(h, 4)); got != addLabel {
		t.Errorf("last row = %q", got)
	}
	h.m.chars["fm/rook"].State = session.Connecting
	if got := strings.TrimSpace(sideRow(h, 2)); got != "… Rook" {
		t.Errorf("connecting row = %q", got)
	}
	kit := h.m.chars["fm/kit"]
	h.m.switchTo("fm/rook")
	kit.Unread, kit.Attention = 3, true
	if got := sideRow(h, 1); !strings.HasPrefix(strings.TrimSpace(got), "Kit") || !strings.HasSuffix(got, " ● 3") {
		t.Errorf("attention row = %q, want the ● with the count on the right", got)
	}
	kit.Attention = false
	if got := sideRow(h, 1); !strings.HasSuffix(got, " 3") || strings.Contains(got, "●") {
		t.Errorf("unread row = %q", got)
	}
}

func TestSidebarNamesTruncate(t *testing.T) {
	long := "host = \"h\"\nport = 1\n\n[[characters]]\nid = \"kit\"\nname = \"Kittenfluff-the-Magnificent\"\n"
	h := newHarness(t, map[string]string{"fm-with-a-long-name": long})
	h.openAll()
	sw := h.m.layout().sw
	if got := sideRow(h, 0); xansi.StringWidth(got) != sw || !strings.HasSuffix(strings.TrimSpace(got), "…") {
		t.Errorf("world row = %q, want cut with … at %d cells", got, sw)
	}
	cs := h.m.chars["fm-with-a-long-name/kit"]
	cs.Unread, cs.Attention = 12, true
	got := sideRow(h, 1)
	if xansi.StringWidth(got) != sw || !strings.HasSuffix(got, " ● 12") || !strings.Contains(got, "…") {
		t.Errorf("char row = %q, want the name cut with … and the activity whole", got)
	}
}

func TestClickXClosesAndNameDoesnt(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	click := func(x, y int) { h.m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}) }
	click(badgeX, 1) // kit is connected: no × to hit
	if h.m.chars["fm/kit"] == nil {
		t.Fatal("clicking a connected character's badge cell closed it")
	}
	click(6, 2) // rook's name: switch, not close
	click(6, 2) // double-click: reconnect, not close
	if h.m.chars["fm/rook"] == nil || h.m.chars["fm/rook"].Sess == nil {
		t.Fatal("double-clicking the name should connect rook")
	}
	h.m.close("fm/rook")
	h.open("fm/rook")
	click(badgeX, 2)
	if h.m.chars["fm/rook"] != nil {
		t.Errorf("clicking × didn't close rook:\n%s", h.screen())
	}
}

func TestCloseCommand(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.typeText("/close")
	h.enter()
	if h.m.chars["fm/kit"] != nil || h.m.a.Active() != "fm/rook" {
		t.Errorf("active = %q; kit should be closed", h.m.a.Active())
	}
}

func TestOpeningShowsConnectingNotX(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.connect(h.m.chars["fm/kit"])
	if got := strings.TrimSpace(sideRow(h, 1)); got != "… Kit" {
		t.Errorf("row = %q right after connecting", got)
	}
}

func TestEmptyState(t *testing.T) {
	h := newHarness(t, map[string]string{"sp": spWorld}) // nothing autoconnects
	if s := h.screen(); !strings.Contains(s, "│"+str.ViewNothingOpen()) {
		t.Fatalf("no empty-state prompt:\n%s", s)
	}
	for _, k := range []rune{tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown, tea.KeyEscape} {
		h.press(k, 0) // must not panic with nothing open
	}
	h.typeText("hello")
	h.enter()
	if !strings.Contains(h.screen(), str.StatusNothingOpen()) {
		t.Errorf("screen:\n%s", h.screen())
	}
	h.press('c', tea.ModCtrl) // clear "hello"
	h.typeText("/log")
	h.enter()
	if !strings.Contains(h.screen(), str.StatusNeedsCharacter("/log")) {
		t.Errorf("screen:\n%s", h.screen())
	}
	h.enter() // empty input: open the picker
	if h.m.picker == nil {
		t.Fatal("Enter on the empty state should open the picker")
	}
	h.press(tea.KeyEscape, 0)
	h.typeText("/quit")
	if cmd := h.enter(); cmd == nil {
		t.Fatal("/quit returned no command")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("/quit didn't quit")
	}
}

func TestClosingLastCharacterShowsEmptyState(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/close")
	h.enter()
	if s := h.screen(); !strings.Contains(s, "│"+str.ViewNothingOpen()) || strings.Contains(s, "Kit") {
		t.Errorf("screen:\n%s", s)
	}
}

// Clicking a world's row shows its overview; even a double-click on the
// badge column closes or connects nothing.
func TestClickingWorldHeaderShowsOverview(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	for _, x := range []int{badgeX, 6, badgeX, 6} { // badge column and name column, twice (a double-click)
		h.m.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	}
	if h.m.a.Active() != worldSel("fm") || h.m.chars["fm/kit"] == nil {
		t.Errorf("active = %q after clicking the world header", h.m.a.Active())
	}
	h.m.Update(tea.MouseClickMsg{X: 6, Y: 1, Button: tea.MouseLeft})
	if h.m.a.Active() != "fm/kit" {
		t.Errorf("active = %q after clicking Kit", h.m.a.Active())
	}
}

// addLine puts a line in cs's scrollback, the core's and the view's,
// drawn as text.
func addLine(cs *charState, text string, l *app.Line) {
	cs.Lines = append(cs.Lines, l)
	cs.sb.AppendLine(lineOf(text, l))
}

// A world's overview shows each open character there: when its last line
// came, and its last few lines (#99).
func TestWorldOverview(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	kit := h.m.chars["fm/kit"]
	for i := range overviewLines + 2 {
		at := h.now.Add(time.Duration(i-10) * time.Minute)
		addLine(kit, fmt.Sprintf("kit line %d", i), &app.Line{Entry: logstore.Entry{Time: at, Text: "x"}})
	}
	h.m.switchTo(worldSel("fm"))
	s := h.screen()
	if !strings.Contains(s, "Kit"+str.Separator()+h.now.Add(-4*time.Minute).Format("15:04")) {
		t.Errorf("Kit's last activity missing:\n%s", s)
	}
	if strings.Contains(s, "kit line 1\n") || !strings.Contains(s, "kit line 2") || !strings.Contains(s, "kit line 6") {
		t.Errorf("want Kit's last %d lines:\n%s", overviewLines, s)
	}
	if !strings.Contains(s, "Rook"+str.Separator()+str.ViewOverviewQuiet()) {
		t.Errorf("Rook should have no activity yet:\n%s", s)
	}
	addLine(h.m.chars["fm/kit"], "old", &app.Line{Entry: logstore.Entry{Time: h.now.AddDate(0, 0, -2)}})
	if s := h.screen(); !strings.Contains(s, h.now.AddDate(0, 0, -2).Format(str.DateDayTime())) {
		t.Errorf("an older last line should say the day:\n%s", s)
	}
}

// bigWorld is a world with more characters than an overview shows at once.
func bigWorld(n int) string {
	w := "host = \"big.test\"\nport = 1\n"
	for i := range n {
		w += fmt.Sprintf("\n[[characters]]\nid = \"c%d\"\nname = \"Char%d\"\n", i, i)
	}
	return w
}

// A world's overview has no input box, draws a rule between characters,
// opens a character on a click in its part, and scrolls when the
// characters don't all fit. Keys still do what they do.
func TestWorldOverviewPane(t *testing.T) {
	h := newHarness(t, map[string]string{"big": bigWorld(6)})
	h.openAll()
	for _, k := range h.m.a.Order() {
		for i := range overviewLines {
			addLine(h.m.chars[k], fmt.Sprintf("%s line %d", k, i), &app.Line{Entry: logstore.Entry{Time: h.now}})
		}
	}
	h.m.switchTo(worldSel("big"))
	s := h.screen()
	if strings.Contains(s, str.ViewNothingOpen()) || h.m.View().Cursor != nil {
		t.Errorf("the overview shouldn't have an input box:\n%s", s)
	}
	rows := strings.Split(s, "\n")
	body := func(y int) string { return strings.SplitN(rows[topH+y], "│", 2)[1] }
	if !strings.HasPrefix(body(overviewLines+1), "───") || !strings.Contains(body(overviewLines+2), "Char1") {
		t.Errorf("want a rule, then the next character:\n%s", s)
	}
	if !strings.Contains(rows[len(rows)-2], "───") {
		t.Errorf("the overview should run down to the statusline's rule:\n%s", s)
	}

	h.typeText("hello") // nowhere to type
	if !h.m.idle.Empty() {
		t.Errorf("typing went to the hidden input: %q", h.m.idle.Value())
	}

	l := h.m.layout()
	h.press(tea.KeyPgDown, 0)
	if h.m.ovTop == 0 {
		t.Fatal("PgDn should scroll the overview")
	}
	s = h.screen()
	var pane []string
	for _, r := range strings.Split(s, "\n") {
		pane = append(pane, strings.SplitN(r, "│", 2)[1])
	}
	if p := strings.Join(pane, "\n"); strings.Contains(p, "Char0") || !strings.Contains(p, "Char5") {
		t.Errorf("scrolled down, the last character should show:\n%s", s)
	}
	rows = strings.Split(s, "\n")
	y := slices.IndexFunc(rows[topH:], func(r string) bool { return strings.Contains(strings.SplitN(r, "│", 2)[1], "Char5") })
	y += 2 // a line under its name
	h.m.Update(tea.MouseClickMsg{X: l.sw + 5, Y: l.top + y, Button: tea.MouseLeft})
	if h.m.a.Active() != "big/c5" {
		t.Fatalf("clicking Char5's lines should open it, active %q", h.m.a.Active())
	}

	h.m.switchTo(worldSel("big"))
	if h.m.ovTop != 0 {
		t.Error("an overview should open at its top")
	}
	for range 3 {
		h.m.handleWheel(tea.MouseWheelMsg{X: l.sw + 5, Y: l.top + 1, Button: tea.MouseWheelDown})
	}
	if h.m.ovTop != 3*h.m.scrollLines() {
		t.Errorf("the wheel should scroll scroll_lines rows a notch: top %d", h.m.ovTop)
	}

	h.enter()
	if h.m.picker == nil {
		t.Error("Enter on an overview should open the picker, as with nothing open")
	}
}

// Ctrl+↑/↓ steps through the worlds' rows as well as the characters'.
func TestSwitchByIncludesWorlds(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.openAll()
	h.m.switchTo(worldSel("fm"))
	var got []string
	for range len(h.m.a.Stops()) {
		h.press(tea.KeyDown, tea.ModCtrl)
		got = append(got, h.m.a.Active())
	}
	want := []string{"fm/kit", "fm/rook", worldSel("sp"), "sp/ash", worldSel("fm")}
	if !slices.Equal(got, want) {
		t.Errorf("Ctrl+Down went %v, want %v", got, want)
	}
	h.press(tea.KeyUp, tea.ModCtrl)
	if h.m.a.Active() != "sp/ash" {
		t.Errorf("Ctrl+Up went to %q", h.m.a.Active())
	}
}

// Ctrl+T on a world's overview edits the world; Esc goes back to it.
func TestEditWorldFromOverview(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.switchTo(worldSel("fm"))
	h.press('t', tea.ModCtrl)
	if p := h.m.picker; p == nil || p.edit == nil || p.edit.kind != editWorld || p.edit.world != "fm" {
		t.Fatalf("Ctrl+T should edit the world:\n%s", h.screen())
	}
	h.press(tea.KeyEscape, 0)
	if h.m.picker != nil || h.m.a.Active() != worldSel("fm") {
		t.Errorf("Esc should go back to the overview: active %q\n%s", h.m.a.Active(), h.screen())
	}
}

// Closing a world's last character takes its overview with it.
func TestClosingWorldsLastCharacterLeavesOverview(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.open("sp/ash")
	h.m.switchTo(worldSel("fm"))
	h.m.close("fm/kit")
	if h.m.a.Active() != "sp/ash" {
		t.Errorf("active = %q", h.m.a.Active())
	}
	// Tab goes back to the last character, not to a world.
	h.m.switchTo(worldSel("sp"))
	h.press(tea.KeyTab, 0)
	if h.m.a.Active() != "sp/ash" {
		t.Errorf("Tab went to %q", h.m.a.Active())
	}
}

func TestSidebarIndent(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook") // disconnected, so it has a ×
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.m.switchTo("fm/rook")
	h.m.switchTo("fm/kit")
	if got := sideRow(h, 1); !strings.HasPrefix(got, " Kit ") {
		t.Errorf("connected row = %q, want the name one space in", got)
	}
	if got := sideRow(h, 2); !strings.HasPrefix(got, " × Rook ") {
		t.Errorf("disconnected row = %q, want the × one space in, pushing the name over", got)
	}
	h.press('o', tea.ModCtrl)
	if got := sideRow(h, 1); !strings.HasPrefix(got, "  "+addCharLabel+" ") {
		t.Errorf("picker row = %q, want its chip one space in too", got)
	}
}

// At the narrowest screen the add chips still fit whole.
func TestAddChipsFitNarrowSidebar(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(tea.WindowSizeMsg{Width: MinWidth, Height: 24})
	if s := h.screen(); !strings.Contains(s, " "+addLabel+" ") {
		t.Errorf("the %s chip is cut:\n%s", addLabel, s)
	}
	h.press('o', tea.ModCtrl)
	if s := h.screen(); !strings.Contains(s, "  "+addCharLabel+" ") {
		t.Errorf("the %s chip is cut:\n%s", addCharLabel, s)
	}
}
