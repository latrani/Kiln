package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// notifyHarness has Kit connected with the given notify level (and
// optional extra world files), past its login and connect notices.
func notifyHarness(t *testing.T, level string, extra map[string]string) *harness {
	t.Helper()
	w := strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nnotify = \""+level+"\"", 1)
	worlds := map[string]string{"fm": w}
	for k, v := range extra {
		worlds[k] = v
	}
	h := newHarness(t, worlds)
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.notified()
	return h
}

// line feeds one incoming line for Kit and waits until it's shown.
func (h *harness) line(s string) {
	h.t.Helper()
	h.conn("fm/kit").lines <- s
	want := h.m.chars["fm/kit"].sb.Len() + 1
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].sb.Len() >= want })
}

func osc(msg string) string { return "\x1b]9;" + msg + "\x07" }

func TestNoNotifyWhileHere(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.line("Rook says, \"hi\"")
	if got := h.notified(); len(got) != 0 {
		t.Errorf("notified while focused: %q", got)
	}
}

func TestNotifyAllWhileBlurred(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \"hi\"")
	h.line("Rook says, \"again\"")
	want := []string{osc("Kit: Rook says, \"hi\""), osc("Kit: Rook says, \"again\"")}
	if got := h.notified(); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNotifyFirstOnlyOnce(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.m.Update(tea.BlurMsg{})
	h.advance(time.Second)
	h.line("Rook says, \"one\"")
	h.line("Rook says, \"two\"")
	h.line("Mira pages: you around?") // attention still gets through
	want := []string{osc("Kit: Rook says, \"one\""), osc("Kit: Mira pages: you around?")}
	if got := h.notified(); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNotifyFirstRearmsOnFocus(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.m.Update(tea.BlurMsg{})
	h.advance(time.Second)
	h.line("Rook says, \"one\"")
	h.advance(time.Second)
	h.m.Update(tea.FocusMsg{})
	h.advance(time.Second)
	h.m.Update(tea.BlurMsg{})
	h.advance(time.Second)
	h.line("Rook says, \"two\"")
	want := []string{osc("Kit: Rook says, \"one\""), osc("Kit: Rook says, \"two\"")}
	if got := h.notified(); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNotifyFirstRearmsOnKey(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.advance(10 * time.Minute) // idle: away without a blur
	h.line("Rook says, \"one\"")
	h.advance(time.Second) // come back strictly after the notification
	h.typeText("x")
	h.advance(10 * time.Minute)
	h.line("Rook says, \"two\"")
	if got := h.notified(); len(got) != 2 {
		t.Errorf("got %q, want two notifications", got)
	}
}

func TestNotifyAttentionOnly(t *testing.T) {
	h := notifyHarness(t, "attention", nil)
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \"hi\"")
	h.line("Mira pages: you around?")
	if got := h.notified(); !slices.Equal(got, []string{osc("Kit: Mira pages: you around?")}) {
		t.Errorf("got %q", got)
	}
}

func TestNotifyNone(t *testing.T) {
	h := notifyHarness(t, "none", nil)
	h.m.Update(tea.BlurMsg{})
	h.line("Mira pages: you around?")
	if got := h.notified(); len(got) != 0 {
		t.Errorf("got %q", got)
	}
}

func TestNotifyIdleFallback(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.advance(4 * time.Minute)
	h.line("Rook says, \"early\"")
	h.advance(2 * time.Minute) // 6 minutes without input
	h.line("Rook says, \"late\"")
	if got := h.notified(); !slices.Equal(got, []string{osc("Kit: Rook says, \"late\"")}) {
		t.Errorf("got %q", got)
	}
}

func TestNotifyIdleOff(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.m.cfg.NotifyIdle = 0
	h.advance(time.Hour)
	h.line("Rook says, \"hi\"")
	if got := h.notified(); len(got) != 0 {
		t.Errorf("idle 0 should never count as away: %q", got)
	}
}

func TestMouseMotionIsNotPresence(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.advance(10 * time.Minute)
	h.m.Update(tea.MouseMotionMsg{X: 40, Y: 5})
	h.line("Rook says, \"hi\"")
	if got := h.notified(); len(got) != 1 {
		t.Errorf("hover should not count as being here: %q", got)
	}
}

func TestQuietAndSentLinesDontNotify(t *testing.T) {
	quiet := strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nnotify = \"all\"", 1) +
		"\n[[highlight]]\nmatch = { pattern = '^Rook' }\nquiet = true\n"
	h := newHarness(t, map[string]string{"fm": quiet})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.notified()
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \"shh\"")
	if got := h.notified(); len(got) != 0 {
		t.Errorf("quiet line notified: %q", got)
	}
}

func TestNotifyNameDisambiguates(t *testing.T) {
	other := "host = \"h\"\nport = 1\n[[characters]]\nid = \"kit\"\nname = \"kit\"\n"
	h := notifyHarness(t, "all", map[string]string{"sp": other})
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \"hi\"")
	if got := h.notified(); !slices.Equal(got, []string{osc("Kit@fm: Rook says, \"hi\"")}) {
		t.Errorf("got %q", got)
	}
}

func TestNotifySanitizesInjection(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \x1b]0;pwned\x07\"hi\"\x07\u009b")
	got := h.notified()
	if !slices.Equal(got, []string{osc("Kit: Rook says, \"hi\"")}) {
		t.Errorf("got %q", got)
	}
}

func TestNotifyTmuxAndMethod(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.m.d.Tmux = true
	h.m.cfg.NotifyMethod = "both"
	h.m.Update(tea.BlurMsg{})
	h.line("hi")
	want := "\x1bPtmux;\x1b\x1b]9;Kit: hi\x07\x1b\\\x07"
	if got := h.notified(); !slices.Equal(got, []string{want}) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestViewReportsFocus(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	if !h.m.View().ReportFocus {
		t.Error("focus reporting is off")
	}
}

func TestNotifyCommand(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.typeText("/notify")
	h.enter()
	if !strings.Contains(h.screen(), "Kit notify: first") {
		t.Errorf("status:\n%s", h.screen())
	}
	h.typeText("/notify none")
	h.enter()
	if lvl := h.m.chars["fm/kit"].notifyLevel(); lvl != "none" {
		t.Errorf("level = %q", lvl)
	}
	h.typeText("/notify")
	h.enter()
	if !strings.Contains(h.screen(), "Kit notify: none (override; config says first)") {
		t.Errorf("status:\n%s", h.screen())
	}
	h.m.Update(tea.BlurMsg{})
	h.line("Mira pages: you around?")
	if got := h.notified(); len(got) != 0 {
		t.Errorf("override ignored: %q", got)
	}
	h.m.Update(tea.FocusMsg{})
	h.typeText("/notify default")
	h.enter()
	if lvl := h.m.chars["fm/kit"].notifyLevel(); lvl != "first" {
		t.Errorf("after default, level = %q", lvl)
	}
	h.typeText("/notify loud")
	h.enter()
	if !strings.Contains(h.screen(), `notify must be "all", "first", "attention" or "none"`) {
		t.Errorf("status:\n%s", h.screen())
	}
}

func TestNotifyOverrideSurvivesReload(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.typeText("/notify all")
	h.enter()
	if !h.m.reloadNow() {
		t.Fatal("reload failed")
	}
	if lvl := h.m.chars["fm/kit"].notifyLevel(); lvl != "all" {
		t.Errorf("after reload, level = %q", lvl)
	}
}
