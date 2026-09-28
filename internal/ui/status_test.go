package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestStatusPinsVersionAndTimeRight(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.d.Version = "v0.2.1"
	got := ansi.Strip(h.m.statusLine(60))
	if ansi.StringWidth(got) != 60 {
		t.Errorf("width %d, want 60: %q", ansi.StringWidth(got), got)
	}
	if !strings.HasPrefix(got, "fm/Kit · disconnected ") || !strings.HasSuffix(got, " v0.2.1 · 21:14") {
		t.Errorf("statusline = %q", got)
	}
}

func TestStatusMessageKeepsClock(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.d.Version = "v0.2.1"
	h.m.setStatus(false, "config reloaded")
	got := ansi.Strip(h.m.statusLine(60))
	if !strings.HasPrefix(got, "fm/Kit · config reloaded ") || !strings.HasSuffix(got, " v0.2.1 · 21:14") {
		t.Errorf("statusline = %q", got)
	}
}

func TestNarrowStatusCutsLeftFirst(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.d.Version = "v0.2.1"
	h.m.setStatus(true, "a very long status message that cannot possibly fit")
	got := ansi.Strip(h.m.statusLine(30))
	if ansi.StringWidth(got) != 30 || !strings.HasSuffix(got, " v0.2.1 · 21:14") || !strings.HasPrefix(got, "fm/Kit · a ver") || !strings.Contains(got, "… v0.2.1") {
		t.Errorf("statusline = %q", got)
	}
	if tiny := ansi.Strip(h.m.statusLine(10)); ansi.StringWidth(tiny) != 10 {
		t.Errorf("tiny statusline = %q", tiny)
	}
}
