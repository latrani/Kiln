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

func pagerHarness(t *testing.T) *harness {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.advance(time.Minute) // well after connecting
	return h
}

func TestSlowChatDoesNotPause(t *testing.T) {
	h := pagerHarness(t)
	h.feed(40, 2*pageGap)
	if h.m.chars["fm/kit"].sb.Scrolled() {
		t.Errorf("lines trickling in paused the view:\n%s", h.screen())
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

func TestAwayHoldsOutput(t *testing.T) {
	h := pagerHarness(t)
	h.feed(3, 2*pageGap)
	h.typeText("/away")
	h.enter()
	if !h.m.away() || !strings.Contains(h.screen(), str.StatusAway()) {
		t.Fatalf("/away didn't take:\n%s", h.screen())
	}
	h.feed(40, 2*pageGap) // slow, but you're away
	if !h.m.chars["fm/kit"].sb.Scrolled() || !strings.Contains(strings.Split(h.screen(), "\n")[topH], "line 1") {
		t.Errorf("output while away should stop at the first line since:\n%s", h.screen())
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
	h.feed(3, 2*pageGap)
	h.open("fm/rook")
	h.m.switchTo("fm/rook")
	h.feed(40, 2*pageGap) // Kit, in the background
	h.m.switchTo("fm/kit")
	if !h.m.chars["fm/kit"].sb.Scrolled() || !strings.Contains(strings.Split(h.screen(), "\n")[topH], "line 1") {
		t.Errorf("switching back should open at the first line missed:\n%s", h.screen())
	}
}
