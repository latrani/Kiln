package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

func TestStartupOpensOnlyAutoconnect(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if got := strings.Join(h.m.order, " "); got != "fm/kit" {
		t.Errorf("order = %q, want only the autoconnect character", got)
	}
	if h.m.active != "fm/kit" {
		t.Errorf("active = %q", h.m.active)
	}
}

func TestOpenKeepsAlphabeticalOrder(t *testing.T) {
	// Case-sensitively these would sort Sp < fm and Kit < Rook < bo.
	fm := fmWorld + "\n[[characters]]\nid = \"bo\"\nname = \"bo\"\n"
	sp := "host = \"sp.test\"\nport = 1\n\n[[characters]]\nid = \"ash\"\nname = \"Ash\"\n"
	h := newHarness(t, map[string]string{"fm": fm, "Sp": sp})
	h.open("Sp/ash", "fm/rook", "fm/bo")
	if got, want := strings.Join(h.m.order, " "), "fm/bo fm/kit fm/rook Sp/ash"; got != want {
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
	if h.m.active != "sp/ash" || h.m.chars["fm/rook"] != nil {
		t.Fatalf("active = %q after closing rook", h.m.active)
	}
	h.m.close("sp/ash") // the last one: the one above
	if h.m.active != "fm/kit" {
		t.Fatalf("active = %q after closing ash", h.m.active)
	}
	h.m.close("fm/kit")
	if h.m.active != "" || len(h.m.order) != 0 || h.m.cur() != nil {
		t.Errorf("active = %q, order = %v; want nothing open", h.m.active, h.m.order)
	}
}

func TestLateEventFromClosedCharacterIgnored(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	sess := h.m.chars["fm/kit"].sess
	h.m.close("fm/kit")
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-sess.Events():
			h.m.Update(eventMsg{key: "fm/kit", sess: sess, ev: ev, ok: ok})
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
	h.m.chars["fm/rook"].state = session.Connecting
	if got := strings.TrimSpace(sideRow(h, 2)); got != "… Rook" {
		t.Errorf("connecting row = %q", got)
	}
	kit := h.m.chars["fm/kit"]
	h.m.switchTo("fm/rook")
	kit.unread, kit.attention = 3, true
	if got := sideRow(h, 1); !strings.HasPrefix(strings.TrimSpace(got), "Kit") || !strings.HasSuffix(got, " ● 3") {
		t.Errorf("attention row = %q, want the ● with the count on the right", got)
	}
	kit.attention = false
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
	cs.unread, cs.attention = 12, true
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
	if h.m.chars["fm/rook"] == nil || h.m.chars["fm/rook"].sess == nil {
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
	if h.m.chars["fm/kit"] != nil || h.m.active != "fm/rook" {
		t.Errorf("active = %q; kit should be closed", h.m.active)
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
	if h.m.active != worldSel("fm") || h.m.chars["fm/kit"] == nil {
		t.Errorf("active = %q after clicking the world header", h.m.active)
	}
	h.m.Update(tea.MouseClickMsg{X: 6, Y: 1, Button: tea.MouseLeft})
	if h.m.active != "fm/kit" {
		t.Errorf("active = %q after clicking Kit", h.m.active)
	}
}

// A world's overview shows each open character there: when its last line
// came, and its last few lines (#99).
func TestWorldOverview(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.open("fm/rook")
	kit := h.m.chars["fm/kit"]
	for i := range overviewLines + 2 {
		at := h.now.Add(time.Duration(i-10) * time.Minute)
		kit.sb.AppendLine(entryLine(fmt.Sprintf("kit line %d", i), logstore.Entry{Time: at, Text: "x"}))
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
	h.m.chars["fm/kit"].sb.AppendLine(entryLine("old", logstore.Entry{Time: h.now.AddDate(0, 0, -2)}))
	if s := h.screen(); !strings.Contains(s, h.now.AddDate(0, 0, -2).Format(str.DateDayTime())) {
		t.Errorf("an older last line should say the day:\n%s", s)
	}
}

// Ctrl+↑/↓ steps through the worlds' rows as well as the characters'.
func TestSwitchByIncludesWorlds(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.openAll()
	h.m.switchTo(worldSel("fm"))
	var got []string
	for range len(h.m.stops()) {
		h.press(tea.KeyDown, tea.ModCtrl)
		got = append(got, h.m.active)
	}
	want := []string{"fm/kit", "fm/rook", worldSel("sp"), "sp/ash", worldSel("fm")}
	if !slices.Equal(got, want) {
		t.Errorf("Ctrl+Down went %v, want %v", got, want)
	}
	h.press(tea.KeyUp, tea.ModCtrl)
	if h.m.active != "sp/ash" {
		t.Errorf("Ctrl+Up went to %q", h.m.active)
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
	if h.m.picker != nil || h.m.active != worldSel("fm") {
		t.Errorf("Esc should go back to the overview: active %q\n%s", h.m.active, h.screen())
	}
}

// Closing a world's last character takes its overview with it.
func TestClosingWorldsLastCharacterLeavesOverview(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld, "sp": spWorld})
	h.open("sp/ash")
	h.m.switchTo(worldSel("fm"))
	h.m.close("fm/kit")
	if h.m.active != "sp/ash" {
		t.Errorf("active = %q", h.m.active)
	}
	// Tab goes back to the last character, not to a world.
	h.m.switchTo(worldSel("sp"))
	h.press(tea.KeyTab, 0)
	if h.m.active != "sp/ash" {
		t.Errorf("Tab went to %q", h.m.active)
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
