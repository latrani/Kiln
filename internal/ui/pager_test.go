package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/str"
)

// feed sends n numbered lines to Kit, waiting for each, with gap between
// them on the harness clock.
func (h *harness) feed(n int, gap time.Duration) {
	h.t.Helper()
	for i := 1; i <= n; i++ {
		h.advance(gap)
		h.line(fmt.Sprintf("line %d", i))
	}
}

// settleOn waits until the newest scrollback line is text; feed counts
// lines, which can leave the last one still queued behind an event that
// added none.
func (h *harness) settleOn(text string) {
	h.t.Helper()
	sb := &h.m.chars["fm/kit"].sb
	h.settle("fm/kit", func() bool { return sb.Len() > 0 && strings.Contains(sb.lines[sb.Len()-1].text, text) })
}

func pagerHarness(t *testing.T) *harness {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.advance(time.Minute)            // well after connecting
	h.m.chars["fm/kit"].sb.MarkSeen() // the login is read
	return h
}

func TestShortChatScrollsNormally(t *testing.T) {
	h := pagerHarness(t)
	h.feed(10, 2*time.Second) // less than a screen since you last sent
	if h.m.chars["fm/kit"].sb.Scrolled() {
		t.Errorf("a little chat paused the view:\n%s", h.screen())
	}
}

// What piles up since you last sent holds at its start, however slowly
// it came: the pager goes by what you've done, not by how long lines were
// apart or whether you counted as away.
func TestSlowChatHoldsSinceLastSend(t *testing.T) {
	h := pagerHarness(t)
	h.feed(40, 2*time.Minute)
	sb := &h.m.chars["fm/kit"].sb
	if !sb.Scrolled() || !strings.Contains(strings.Split(h.screen(), "\n")[topH], "line 1") {
		t.Errorf("unread chat should stop at its first line:\n%s", h.screen())
	}
}

func TestBurstPausesAtItsStart(t *testing.T) {
	h := pagerHarness(t)
	h.feed(40, 0)
	sb := &h.m.chars["fm/kit"].sb
	if !sb.Scrolled() || !strings.Contains(strings.Split(h.screen(), "\n")[topH], "line 1") {
		t.Errorf("a burst taller than the screen should stop at its first line:\n%s", h.screen())
	}
	for i := 0; sb.Scrolled() && i < 20; i++ {
		h.press(tea.KeyPgDown, 0)
	}
	if !strings.Contains(h.screen(), "line 40") {
		t.Errorf("paging down should reach the end:\n%s", h.screen())
	}
}

// Sending anchors the pager on your line: with local_echo on it stays at
// the top of the held view, with the new lines piled below it.
func TestSentLineStaysAtTheTopOfHeldOutput(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": echoWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.feed(5, 0) // before you send
	h.settleOn("line 5")
	h.typeText("look")
	h.enter()
	h.feed(40, 2*time.Second)
	h.settleOn("line 40")
	sb := &h.m.chars["fm/kit"].sb
	row := strings.Split(h.screen(), "\n")[topH]
	if !sb.Scrolled() || !strings.Contains(row, "look") {
		t.Errorf("the sent line should stay at the top of what piled up after it:\n%s", h.screen())
	}
	if !strings.Contains(strings.Split(h.screen(), "\n")[topH+1], "line 1") {
		t.Errorf("the new lines should start right below it:\n%s", h.screen())
	}
	for i := 0; sb.Scrolled() && i < 20; i++ {
		h.press(tea.KeyPgDown, 0)
	}
	if !strings.Contains(h.screen(), "line 40") {
		t.Errorf("paging down should reach the end:\n%s", h.screen())
	}
}

// Without an echo there's no line of yours to show, so the view stops at
// the first line after it.
func TestHeldOutputStartsAfterAnUnechoedSend(t *testing.T) {
	h := pagerHarness(t)
	h.feed(30, 0)
	h.settleOn("line 30")
	h.typeText("look")
	h.enter()
	h.feed(40, 0)
	h.settleOn("line 40")
	if row := strings.Split(h.screen(), "\n")[topH]; !strings.Contains(row, "line 1") || strings.Contains(row, "look") {
		t.Errorf("top row = %q, want the first new line:\n%s", row, h.screen())
	}
}

func TestAwayDoesNotChangePaging(t *testing.T) {
	h := pagerHarness(t)
	h.typeText("/away")
	h.enter()
	if !h.m.away() || !strings.Contains(h.screen(), str.StatusAway()) {
		t.Fatalf("/away didn't take:\n%s", h.screen())
	}
	h.typeText("x")
	if h.m.away() {
		t.Error("a key should end /away")
	}
}

func TestAwayWithNothingOpen(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("/away")
	h.enter()
	if !h.m.away() {
		t.Errorf("/away needs no character:\n%s", h.screen())
	}
}

func TestSwitchOpensAtFirstUnread(t *testing.T) {
	h := pagerHarness(t)
	h.feed(3, 2*time.Second)
	h.open("fm/rook")
	h.m.switchTo("fm/rook")
	h.feed(40, 2*time.Second) // Kit, in the background
	h.m.switchTo("fm/kit")
	if !h.m.chars["fm/kit"].sb.Scrolled() || !strings.Contains(strings.Split(h.screen(), "\n")[topH], "line 1") {
		t.Errorf("switching back should open at the first line missed:\n%s", h.screen())
	}
}
