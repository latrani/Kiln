package app

import (
	"strings"
	"time"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/str"
)

// ConnectGrace is how long after connecting only attention lines
// notify, so the login banner doesn't.
const ConnectGrace = 5 * time.Second

// BurstGap is how soon after a notification another line counts as part
// of the same burst (a multi-line description) and doesn't notify.
const BurstGap = 250 * time.Millisecond

// Presence is whether you're here, away, or Kiln can't tell.
type Presence int

const (
	PresenceUnknown Presence = iota // not away, and no focus event has ever arrived
	PresenceHere                    // not away, and the front end reports focus
	PresenceAway
)

// Here records that you're at the keyboard: a key, paste or click. Input
// implies focus, even if the focus-in was lost. It ends /away and
// re-arms "first" for every character.
func (a *App) Here() {
	a.focused = true
	a.lastHere, a.awayNow = a.d.Now(), false
	a.hereGen++
}

// Scrolled records a scroll: Here, but only while focused, since macOS
// scrolls windows in the background.
func (a *App) Scrolled() {
	if a.focused {
		a.Here()
	}
}

// Focus reports the front end gaining (in) or losing focus. Gaining it
// counts as Here.
func (a *App) Focus(in bool) {
	a.focusSeen = true
	if in {
		a.Here()
	} else {
		a.focused = false
	}
}

// Focused reports whether the front end has focus, as far as Kiln knows.
func (a *App) Focused() bool { return a.focused }

// SetAway is /away: away until you're next Here. It says so.
func (a *App) SetAway() {
	a.awayNow = true
	a.SetStatus(false, str.StatusAway())
}

// notifyIdle is the notify_idle setting.
func (a *App) notifyIdle() time.Duration {
	if a.cfg != nil {
		return a.cfg.NotifyIdle
	}
	return config.DefaultNotifyIdle
}

// Away reports whether you've switched away, said /away, or been idle
// past notify_idle.
func (a *App) Away() bool {
	idle := a.notifyIdle()
	return !a.focused || a.awayNow || idle > 0 && a.d.Now().Sub(a.lastHere) > idle
}

// Presence is whether you're away, here, or Kiln can't tell: a terminal
// only reports focus when it changes, so until the first event one that
// reports it looks the same as one that never will.
func (a *App) Presence() Presence {
	switch {
	case a.Away():
		return PresenceAway
	case !a.focusSeen:
		return PresenceUnknown
	}
	return PresenceHere
}

// NotifyLevel is open character k's /notify override, or its configured
// level; "" if k isn't open.
func (a *App) NotifyLevel(k string) notify.Level {
	c := a.chars[k]
	if c == nil {
		return ""
	}
	if l := a.notifyOverrides[k]; l != "" {
		return l
	}
	return c.Ch.Notify
}

// notifyName is c's name, with its world when another world has a
// character of the same name.
func (a *App) notifyName(c *Char) string {
	for _, ch := range a.AllChars() {
		if ch.World != c.Ch.World && strings.EqualFold(ch.Name, c.Ch.Name) {
			return c.Ch.Name + "@" + c.Ch.World
		}
	}
	return c.Ch.Name
}

// notifyFor is the notification for an incoming line of c's, if you're
// away and c's level wants one. Only what arrives while you're away
// notifies: what came while you were here, you saw.
func (a *App) notifyFor(c *Char, l Line) []Effect {
	e := l.Entry
	if e.Dir != logstore.In || l.Quiet || !a.Away() {
		return nil
	}
	now := a.d.Now()
	if !l.Attention && (now.Sub(c.ConnectedAt) < ConnectGrace || now.Sub(c.lastSent) < BurstGap) {
		return nil
	}
	switch a.NotifyLevel(c.Key) {
	case notify.All:
	case notify.First:
		if !l.Attention && c.sentGen == a.hereGen {
			return nil
		}
	case notify.Attention:
		if !l.Attention {
			return nil
		}
	default:
		return nil
	}
	c.sentGen, c.lastSent = a.hereGen, now
	return []Effect{Notify{Title: a.notifyName(c), Body: e.Text}}
}

// notifyCommand is /notify: show the level, set an override until Kiln
// quits, or drop it with "default".
func (a *App) notifyCommand(c *Char, args []string) {
	if len(args) == 0 {
		shown := string(a.NotifyLevel(c.Key))
		if l := a.notifyOverrides[c.Key]; l != "" {
			shown = str.NotifyLevelOverride(l, c.Ch.Notify)
		}
		a.SetStatus(false, str.NotifyLevel(shown))
		return
	}
	if args[0] == "default" {
		delete(a.notifyOverrides, c.Key)
		a.SetStatus(false, str.NotifyLevel(c.Ch.Notify))
		return
	}
	l, err := notify.ParseLevel(args[0])
	if err != nil {
		a.SetStatus(true, err.Error())
		return
	}
	a.notifyOverrides[c.Key] = l
	a.SetStatus(false, str.NotifyLevelUntilQuit(c.Ch.Name, l))
}
