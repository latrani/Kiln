package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/version"
)

func TestStatusTimesOut(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/bogus")
	h.enter()
	want := str.StatusUnknownCommand("/bogus")
	if !strings.Contains(h.screen(), want) {
		t.Fatalf("no status:\n%s", h.screen())
	}
	h.m.Update(statusExpiredMsg(h.m.a.Status().Gen - 1)) // an earlier status's timer
	if !strings.Contains(h.screen(), want) {
		t.Error("a stale timer cleared the status")
	}
	h.m.Update(statusExpiredMsg(h.m.a.Status().Gen))
	if strings.Contains(h.screen(), want) {
		t.Errorf("status outlived its timeout:\n%s", h.screen())
	}
}

func TestTypingClearsStatus(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/bogus")
	h.enter()
	want := str.StatusUnknownCommand("/bogus")
	h.press(tea.KeyPgUp, 0) // not typing
	if !strings.Contains(h.screen(), want) {
		t.Fatalf("a key that doesn't type cleared the status:\n%s", h.screen())
	}
	h.typeText("h")
	if strings.Contains(h.screen(), want) {
		t.Errorf("typing didn't clear the status:\n%s", h.screen())
	}
}

func TestBottomBarConnectedSince(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	cs := h.m.chars["fm/kit"]
	got := ansi.Strip(h.m.statusLine(60))
	if want := str.ViewConnectedSince(cs.ConnectedAt.Local().Format("15:04")); strings.TrimSpace(got) != want { //str:ok
		t.Errorf("bottom bar = %q, want %q", got, want)
	}
	cs.ConnectedAt = cs.ConnectedAt.Add(-48 * time.Hour)
	got = ansi.Strip(h.m.statusLine(60))
	day := cs.ConnectedAt.Local().Format(str.DateDay())
	if want := str.ViewConnectedSinceDay(day, cs.ConnectedAt.Local().Format("15:04")); strings.TrimSpace(got) != want { //str:ok
		t.Errorf("bottom bar = %q, want %q", got, want)
	}
}

func TestBottomBarHasNoClockOrVersion(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if s := h.screen(); strings.Contains(s, version.String()) || strings.Contains(s, "21:14") {
		t.Errorf("clock or version on screen:\n%s", s)
	}
}

// Nothing on screen depends on the minute, so an idle terminal stays
// idle.
func TestScreenDoesNotTick(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	before := h.m.View().Content
	h.advance(time.Minute)
	if after := h.m.View().Content; after != before {
		t.Error("the screen changed after a minute with nothing happening")
	}
}

func TestLogMessagesInBottomBar(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("up", "m", "up", "m") // a range: "2 lines in range"
	rows := strings.Split(h.screen(), "\n")
	bottom := strings.TrimSpace(strings.SplitN(rows[len(rows)-1], "│", 2)[1])
	hints := rows[len(rows)-3]
	if bottom != str.BrowseLinesInRange(2) {
		t.Errorf("bottom bar = %q, want the log message", bottom)
	}
	if !strings.Contains(hints, firstPart(str.BrowseHints())) {
		t.Errorf("hint row lost its hints: %q", hints)
	}
	if !strings.Contains(rows[len(rows)-2], "───") {
		t.Errorf("no rule under the hint row: %q", rows[len(rows)-2])
	}
	h.key("down") // any key clears it
	rows = strings.Split(h.screen(), "\n")
	if got := strings.TrimSpace(strings.SplitN(rows[len(rows)-1], "│", 2)[1]); got != str.ViewSelected(2) {
		t.Errorf("after a key the bottom bar = %q, want the count", got)
	}
}

// The bottom bar shows the newest message: a global one set after a log
// mode one replaces it.
func TestNewestMessageWins(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("up", "m", "up", "m") // "2 lines in range"
	h.m.setStatus(false, str.StatusConfigReloaded())
	if got := strings.TrimSpace(ansi.Strip(h.m.statusLine(60))); got != str.StatusConfigReloaded() {
		t.Errorf("bottom bar = %q, want the newer message", got)
	}
}

// Where daylight saving starts at midnight, the next midnight doesn't
// exist; the tick must still land after now, not spin.
func TestNextMidnightSkipsADSTGap(t *testing.T) {
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Skip("no zoneinfo:", err)
	}
	now := time.Date(2026, 9, 5, 23, 30, 0, 0, loc)
	if next := nextMidnight(now); !next.After(now) || next.Sub(now) > 2*time.Hour {
		t.Errorf("nextMidnight(%v) = %v", now, next)
	}
	now = time.Date(2026, 9, 29, 21, 0, 0, 0, loc)
	if next := nextMidnight(now); next != time.Date(2026, 9, 30, 0, 0, 0, 0, loc) {
		t.Errorf("ordinary day: nextMidnight = %v", next)
	}
}

// Log mode's messages time out like any other, and a click that changes
// the selection clears one, so the count shows again.
func TestLogMessagesExpireAndClear(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.screen()
	b := h.br()
	l := h.m.layout()
	rowOf := func(i int) int {
		for row, bl := range b.rowLines {
			if bl == b.Lines[i] {
				return l.top + row
			}
		}
		t.Fatalf("line %d not on screen", i)
		return 0
	}
	x := l.sw + 11
	bar := func() string { return strings.TrimSpace(ansi.Strip(h.m.statusLine(60))) }
	h.m.Update(tea.MouseClickMsg{X: x, Y: rowOf(0), Button: tea.MouseLeft})
	h.m.Update(tea.MouseClickMsg{X: x, Y: rowOf(4), Button: tea.MouseLeft, Mod: tea.ModShift})
	if got := bar(); got != str.BrowseLinesInRange(5) {
		t.Fatalf("bar = %q", got)
	}
	h.m.Update(tea.MouseClickMsg{X: x, Y: rowOf(2), Button: tea.MouseLeft, Mod: tea.ModShift}) // leave line 2 out
	if got, want := bar(), str.ViewSelected(len(b.Selection())); got != want {
		t.Errorf("after excluding a line the bar = %q, want %q", got, want)
	}
	h.key("c") // "copied N lines"
	if bar() == str.ViewSelected(len(b.Selection())) {
		t.Fatal("copy set no message")
	}
	h.m.Update(statusExpiredMsg(h.m.a.Status().Gen))
	if got, want := bar(), str.ViewSelected(len(b.Selection())); got != want {
		t.Errorf("after the message timed out the bar = %q, want %q", got, want)
	}
}
