package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/str"
)

func TestStatusTimesOut(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.typeText("/bogus")
	h.enter()
	want := str.StatusUnknownCommand("/bogus")
	if !strings.Contains(h.screen(), want) {
		t.Fatalf("no status:\n%s", h.screen())
	}
	h.m.Update(statusExpiredMsg(h.m.statusGen - 1)) // an earlier status's timer
	if !strings.Contains(h.screen(), want) {
		t.Error("a stale timer cleared the status")
	}
	h.m.Update(statusExpiredMsg(h.m.statusGen))
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
	if want := str.ViewConnectedSince(cs.connectedAt.Local().Format("15:04")); strings.TrimSpace(got) != want { //str:ok
		t.Errorf("bottom bar = %q, want %q", got, want)
	}
	cs.connectedAt = cs.connectedAt.Add(-48 * time.Hour)
	got = ansi.Strip(h.m.statusLine(60))
	day := cs.connectedAt.Local().Format(str.DateDay())
	if want := str.ViewConnectedSinceDay(day, cs.connectedAt.Local().Format("15:04")); strings.TrimSpace(got) != want { //str:ok
		t.Errorf("bottom bar = %q, want %q", got, want)
	}
}

func TestBottomBarHasNoClockOrVersion(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.d.Version = "v0.2.1"
	if s := h.screen(); strings.Contains(s, "v0.2.1") || strings.Contains(s, "21:14") {
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
