package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/str"
)

// clockApp is sessionApp with a clock the test moves.
func clockApp(t *testing.T, worlds map[string]string) (*App, *time.Time) {
	t.Helper()
	a := sessionApp(t, worlds)
	clk := now
	a.d.Now = func() time.Time { return clk }
	a.lastHere = clk
	return a, &clk
}

func withLevel(world, level string) string {
	return strings.Replace(world, "login = \"connect {name} {password}\"\n", "login = \"connect {name} {password}\"\nnotify = \""+level+"\"\n", 1)
}

func notes(effs []Effect) []Notify {
	var n []Notify
	for _, e := range effs {
		if x, ok := e.(Notify); ok {
			n = append(n, x)
		}
	}
	return n
}

func TestPresenceStates(t *testing.T) {
	a, clk := clockApp(t, map[string]string{"fm": fmWorld})
	if a.Presence() != PresenceUnknown {
		t.Errorf("at start: %v", a.Presence())
	}
	a.Here() // input with no focus event yet: still can't tell
	if a.Presence() != PresenceUnknown {
		t.Errorf("typing before any focus event: %v", a.Presence())
	}
	a.Focus(true)
	if a.Presence() != PresenceHere {
		t.Errorf("after focus-in: %v", a.Presence())
	}
	a.Focus(false)
	if a.Presence() != PresenceAway || a.Focused() {
		t.Errorf("after blur: %v, focused %v", a.Presence(), a.Focused())
	}
	a.Here()
	if a.Presence() != PresenceHere || !a.Focused() {
		t.Errorf("input after a blur: %v", a.Presence())
	}
	a.SetAway()
	if a.Presence() != PresenceAway || a.Status().Text != str.StatusAway() {
		t.Errorf("/away: %v, status %q", a.Presence(), a.Status().Text)
	}
	a.Here()
	if a.Away() {
		t.Error("input didn't end /away")
	}
	*clk = clk.Add(a.Config().NotifyIdle + time.Second)
	if a.Presence() != PresenceAway {
		t.Errorf("idle past notify_idle: %v", a.Presence())
	}
}

// A new character's first line while away notifies under "first"
// (hereGen starts at 0, so sentGen must start below it); the next
// doesn't, until you've been here again.
func TestNotifyFirstReArmsOnHere(t *testing.T) {
	a, clk := clockApp(t, map[string]string{"fm": withLevel(fmWorld, "first")})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	a.Focus(false)
	line := func(text string) []Notify {
		*clk = clk.Add(time.Second) // past the burst gap
		_, _, effs := a.Handle(lineMsg("fm/kit", s, text))
		return notes(effs)
	}
	if got := line("one"); !slices.Equal(got, []Notify{{Title: "Kit", Body: "one"}}) {
		t.Errorf("first line: %v", got)
	}
	if got := line("two"); got != nil {
		t.Errorf("second line: %v", got)
	}
	a.Here()
	a.Focus(false)
	if got := line("three"); len(got) != 1 {
		t.Errorf("after being here again: %v", got)
	}
}

func TestNotifyGraceAndBursts(t *testing.T) {
	a, clk := clockApp(t, map[string]string{"fm": withLevel(fmWorld, "all")})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	a.Char("fm/kit").ConnectedAt = *clk
	a.Focus(false)
	line := func(text string) int {
		_, _, effs := a.Handle(lineMsg("fm/kit", s, text))
		return len(notes(effs))
	}
	if n := line("banner"); n != 0 {
		t.Error("notified within the connect grace")
	}
	*clk = clk.Add(ConnectGrace)
	if n := line("hi"); n != 1 {
		t.Error("didn't notify past the grace")
	}
	if n := line("more of it"); n != 0 {
		t.Error("notified within a burst")
	}
	*clk = clk.Add(BurstGap)
	if n := line("again"); n != 1 {
		t.Error("didn't notify after the burst gap")
	}
}

func TestNoNotifyWhileHere(t *testing.T) {
	a, clk := clockApp(t, map[string]string{"fm": withLevel(fmWorld, "all")})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	a.Focus(true)
	*clk = clk.Add(time.Second)
	if _, _, effs := a.Handle(lineMsg("fm/kit", s, "hi")); notes(effs) != nil {
		t.Errorf("notified while here: %v", notes(effs))
	}
}

// The title says which world when another world has a character of the
// same name.
func TestNotifyTitleNamesTheWorldWhenAmbiguous(t *testing.T) {
	kitToo := strings.Replace(zzWorld, "name = \"Ash\"", "name = \"Kit\"", 1)
	a, clk := clockApp(t, map[string]string{"fm": withLevel(fmWorld, "all"), "zz": kitToo})
	openAll(t, a, "fm/kit")
	s := attach(a, "fm/kit")
	a.Focus(false)
	*clk = clk.Add(time.Second)
	_, _, effs := a.Handle(lineMsg("fm/kit", s, "hi"))
	if got := notes(effs); len(got) != 1 || got[0].Title != "Kit@fm" {
		t.Errorf("got %v, want title Kit@fm", got)
	}
}

func TestAwayAndNotifyCommands(t *testing.T) {
	a, _ := clockApp(t, map[string]string{"fm": fmWorld})
	a.SetInput("/away") // with nothing open
	if _, effs := a.Submit(); effs != nil || !a.Away() || a.Status().Text != str.StatusAway() {
		t.Errorf("/away with nothing open: effects %v, away %v, status %q", effs, a.Away(), a.Status().Text)
	}
	openAll(t, a, "fm/kit")
	a.SetInput("/notify none")
	if _, effs := a.Submit(); effs != nil || a.NotifyLevel("fm/kit") != "none" {
		t.Errorf("/notify none: effects %v, level %q", effs, a.NotifyLevel("fm/kit"))
	}
	a.SetInput("/notify bogus")
	if a.Submit(); !a.Status().Err {
		t.Error("/notify bogus wasn't an error")
	}
}

func TestNotifyOverrideOutlivesClose(t *testing.T) {
	a, _ := clockApp(t, map[string]string{"fm": fmWorld})
	openAll(t, a, "fm/kit")
	a.SetInput("/notify all")
	a.Submit()
	a.Close("fm/kit")
	openAll(t, a, "fm/kit")
	if l := a.NotifyLevel("fm/kit"); l != "all" {
		t.Errorf("after reopening: %q, want the override", l)
	}
}
