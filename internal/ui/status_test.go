package ui

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/str"
)

// kitChip is the statusline's start for fm/Kit: the name and the Log chip.
func kitChip() string { return "fm/Kit  " + str.ViewLogButton() + " " }

func TestStatusPinsVersionAndTimeRight(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.d.Version = "v0.2.1"
	got := ansi.Strip(h.m.statusLine(60))
	if ansi.StringWidth(got) != 60 {
		t.Errorf("width %d, want 60: %q", ansi.StringWidth(got), got)
	}
	if !strings.HasPrefix(got, kitChip()+str.Separator()+str.StateDisconnected()+" ") || !strings.HasSuffix(got, " v0.2.1"+str.Separator()+"21:14") {
		t.Errorf("statusline = %q", got)
	}
}

func TestStatusMessageKeepsClock(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.d.Version = "v0.2.1"
	h.m.setStatus(false, str.StatusConfigReloaded())
	got := ansi.Strip(h.m.statusLine(60))
	if !strings.HasPrefix(got, kitChip()+str.Separator()+str.StatusConfigReloaded()+" ") || !strings.HasSuffix(got, " v0.2.1"+str.Separator()+"21:14") {
		t.Errorf("statusline = %q", got)
	}
}

func TestNarrowStatusCutsLeftFirst(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.d.Version = "v0.2.1"
	h.m.setStatus(true, "a very long status message that cannot possibly fit")
	got := ansi.Strip(h.m.statusLine(40))
	if ansi.StringWidth(got) != 40 || !strings.HasSuffix(got, " v0.2.1"+str.Separator()+"21:14") || !strings.HasPrefix(got, kitChip()+str.Separator()+"a ver") || !strings.Contains(got, "… v0.2.1") {
		t.Errorf("statusline = %q", got)
	}
	if tiny := ansi.Strip(h.m.statusLine(10)); ansi.StringWidth(tiny) != 10 {
		t.Errorf("tiny statusline = %q", tiny)
	}
}

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
