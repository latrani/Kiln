package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/str"
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
	h.advance(app.ConnectGrace) // past the login banner
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
	h.advance(time.Second) // past the burst gap
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
	h.m.a.Config().NotifyIdle = 0
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
	quiet := strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nnotify = \"all\"\nquiet = [\"chatter\"]", 1) +
		"\n[[classify]]\ntag = \"chatter\"\npattern = '^Rook'\n"
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
	h.m.a.Config().NotifyMethod = "both"
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
	if !strings.Contains(h.screen(), str.NotifyLevel("first")) {
		t.Errorf("status:\n%s", h.screen())
	}
	h.typeText("/notify none")
	h.enter()
	if lvl := h.m.a.NotifyLevel("fm/kit"); lvl != "none" {
		t.Errorf("level = %q", lvl)
	}
	h.typeText("/notify")
	h.enter()
	if !strings.Contains(h.screen(), str.NotifyLevel(str.NotifyLevelOverride("none", "first"))) {
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
	if lvl := h.m.a.NotifyLevel("fm/kit"); lvl != "first" {
		t.Errorf("after default, level = %q", lvl)
	}
	h.typeText("/notify loud")
	h.enter()
	if !strings.Contains(h.screen(), `notify must be "all", "first"`) {
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
	if lvl := h.m.a.NotifyLevel("fm/kit"); lvl != "all" {
		t.Errorf("after reload, level = %q", lvl)
	}
}

func TestInputMeansFocused(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.m.Update(tea.BlurMsg{}) // and the focus-in never arrives (mosh, tmux reattach)
	h.advance(time.Second)
	h.typeText("x")
	h.line("Rook says, \"hi\"")
	if got := h.notified(); len(got) != 0 {
		t.Errorf("notified while typing: %q", got)
	}
}

func TestNotifyOverrideSurvivesClose(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.typeText("/notify none")
	h.enter()
	h.typeText("/close")
	h.enter()
	h.open("fm/kit")
	if lvl := h.m.a.NotifyLevel("fm/kit"); lvl != "none" {
		t.Errorf("after close and reopen, level = %q", lvl)
	}
}

func TestWheelWhileBlurredIsNotPresence(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \"one\"")
	h.advance(time.Second)
	h.m.Update(tea.MouseWheelMsg{X: 40, Y: 5, Button: tea.MouseWheelUp}) // macOS scrolls background windows
	h.line("Rook says, \"two\"")
	if got := h.notified(); len(got) != 1 {
		t.Errorf("scrolling an unfocused window re-armed first: %q", got)
	}
}

func TestComingBackRearmsAtTheSameInstant(t *testing.T) {
	h := notifyHarness(t, "first", nil)
	h.m.Update(tea.BlurMsg{})
	h.line("Rook says, \"one\"")
	h.m.Update(tea.FocusMsg{}) // same clock reading as the notification
	h.m.Update(tea.BlurMsg{})
	h.advance(app.BurstGap) // so the next line isn't part of a burst
	h.line("Rook says, \"two\"")
	if got := h.notified(); len(got) != 2 {
		t.Errorf("got %q, want two notifications", got)
	}
}

func TestLoginBannerDoesntNotify(t *testing.T) {
	w := strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nnotify = \"all\"", 1)
	h := newHarness(t, map[string]string{"fm": w})
	h.m.Update(tea.BlurMsg{})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.line("Welcome to FurryMUCK!")
	h.line("Mira pages: welcome back")
	h.advance(app.ConnectGrace)
	h.line("Rook says, \"hi\"")
	want := []string{osc("Kit: Mira pages: welcome back"), osc("Kit: Rook says, \"hi\"")}
	if got := h.notified(); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBurstNotifiesOnce(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.m.Update(tea.BlurMsg{})
	h.line("Rook's Den")
	h.advance(10 * time.Millisecond)
	h.line("A cozy room full of cushions.")
	h.line("Mira pages: nice den") // attention gets through a burst
	h.advance(app.BurstGap)
	h.line("Rook says, \"hi\"")
	want := []string{osc("Kit: Rook's Den"), osc("Kit: Mira pages: nice den"), osc("Kit: Rook says, \"hi\"")}
	if got := h.notified(); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Only what arrives while you're away notifies. A line you saw while
// here is never sent later, however long you then stay away.
func TestLineSeenWhileHereNeverNotifiesLater(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.line("Rook says, \"you there?\"") // here: focused, just typed
	h.m.Update(tea.BlurMsg{})
	h.advance(10 * time.Minute)
	h.m.Update(tea.FocusMsg{})
	h.m.Update(tea.BlurMsg{})
	h.advance(10 * time.Minute)
	if got := h.notified(); len(got) != 0 {
		t.Errorf("notified about a line you saw: %q", got)
	}
	h.line("Rook says, \"hello?\"") // now you're away
	if got := h.notified(); !slices.Equal(got, []string{osc("Kit: Rook says, \"hello?\"")}) {
		t.Errorf("got %q", got)
	}
}

// Idle counts as away for a line that arrives after notify_idle, which
// is the way to tell when focus events don't reach Kiln.
func TestIdleNotifiesOnlyWhatArrivesAfterIt(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.line("Rook says, \"early\"") // here: input within notify_idle
	h.advance(6 * time.Minute)     // no focus events, and no input since
	h.line("Rook says, \"late\"")
	if got := h.notified(); !slices.Equal(got, []string{osc("Kit: Rook says, \"late\"")}) {
		t.Errorf("got %q", got)
	}
}

func TestPresenceStates(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	m := h.m
	if m.a.Presence() != app.PresenceUnknown {
		t.Errorf("before any focus event: %v, want can't tell", m.a.Presence())
	}
	m.Update(tea.FocusMsg{})
	if m.a.Presence() != app.PresenceHere {
		t.Errorf("after focus-in: %v", m.a.Presence())
	}
	m.Update(tea.BlurMsg{})
	if m.a.Presence() != app.PresenceAway {
		t.Errorf("after blur: %v", m.a.Presence())
	}
	h.typeText("x") // input implies focus, and events have been seen
	if m.a.Presence() != app.PresenceHere {
		t.Errorf("typing after a blur: %v", m.a.Presence())
	}
	h.key("backspace")
	h.typeText("/away")
	h.enter()
	if m.a.Presence() != app.PresenceAway {
		t.Errorf("/away: %v", m.a.Presence())
	}
}

// Switching away draws nothing: in tmux any output flags the window,
// so the chip changing to Away would mark it every time you left. It
// catches up on the next thing that does draw.
func TestBlurRepaintsNothing(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.m.Update(tea.FocusMsg{})
	before := h.screen()
	h.m.Update(tea.BlurMsg{})
	if after := h.screen(); after != before {
		t.Errorf("blur changed the screen:\n%s\nwant\n%s", after, before)
	}
	h.line("Rook says, \"hi\"")
	if top := rightRow(h, 0); !strings.Contains(top, str.ViewPresenceAway()) {
		t.Errorf("after a line while away: %q", top)
	}
}

// With no focus events, idle still counts as away, and shows it.
func TestPresenceIdleWithoutFocusEvents(t *testing.T) {
	h := notifyHarness(t, "all", nil)
	h.advance(6 * time.Minute)
	if h.m.a.Presence() != app.PresenceAway {
		t.Errorf("idle past notify_idle: %v", h.m.a.Presence())
	}
}
