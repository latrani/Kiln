package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/str"
)

// connectGrace is how long after connecting only attention lines
// notify, so the login banner doesn't.
const connectGrace = 5 * time.Second

// burstGap is how soon after a notification another line counts as part
// of the same burst (a multi-line description) and doesn't notify.
const burstGap = 250 * time.Millisecond

// notifyLevel is the /notify override, or the configured level.
func (m *Model) notifyLevel(cs *charState) notify.Level {
	if l := m.notifyOverrides[cs.Key]; l != "" {
		return l
	}
	return cs.Ch.Notify
}

// here records that you're at the terminal: a focus-in, key, paste,
// click or wheel. It re-arms "first" for every character.
func (m *Model) here() {
	m.lastHere, m.awayNow = m.d.Now(), false
	m.hereGen++
}

// notifyIdle is the notify_idle setting.
func (m *Model) notifyIdle() time.Duration {
	if m.a.Config() != nil {
		return m.a.Config().NotifyIdle
	}
	return config.DefaultNotifyIdle
}

// presenceState is what the top bar's presence chip shows.
type presenceState int

const (
	presenceUnknown presenceState = iota // not away, and no focus event has ever arrived
	presenceHere                         // not away, and the terminal reports focus
	presenceAway
)

// presence is whether you're away, here, or Kiln can't tell: a terminal
// only reports focus when it changes, so until the first event a
// terminal that reports it looks the same as one that never will.
func (m *Model) presence() presenceState {
	switch {
	case m.away():
		return presenceAway
	case !m.focusSeen:
		return presenceUnknown
	}
	return presenceHere
}

// away reports whether you've switched away, said /away, or been idle
// past notify_idle.
func (m *Model) away() bool {
	idle := m.notifyIdle()
	return !m.focused || m.awayNow || idle > 0 && m.d.Now().Sub(m.lastHere) > idle
}

// encode turns a message into the bytes to write.
func (m *Model) encode(msg string) tea.Cmd {
	method := notify.OSC
	if m.a.Config() != nil {
		method = m.a.Config().NotifyMethod
	}
	return m.d.Raw(notify.Encode(msg, method, m.d.Tmux))
}

// notifyName is the character's name, with its world when another
// world has a character of the same name.
func (m *Model) notifyName(cs *charState) string {
	for _, ch := range m.a.AllChars() {
		if ch.World != cs.Ch.World && strings.EqualFold(ch.Name, cs.Ch.Name) {
			return cs.Ch.Name + "@" + cs.Ch.World
		}
	}
	return cs.Ch.Name
}

// notifyCmd writes a notification for an incoming line if you're away
// and cs's level wants one. Only what arrives while you're away notifies:
// what came while you were here, you saw.
func (m *Model) notifyCmd(cs *charState, l app.Line) tea.Cmd {
	e := l.Entry
	if e.Dir != logstore.In || l.Quiet || !m.away() {
		return nil
	}
	now := m.d.Now()
	if !l.Attention && (now.Sub(cs.ConnectedAt) < connectGrace || now.Sub(cs.lastSent) < burstGap) {
		return nil
	}
	switch m.notifyLevel(cs) {
	case notify.All:
	case notify.First:
		if !l.Attention && cs.sentGen == m.hereGen {
			return nil
		}
	case notify.Attention:
		if !l.Attention {
			return nil
		}
	default:
		return nil
	}
	cs.sentGen, cs.lastSent = m.hereGen, now
	return m.encode(notify.Message(m.notifyName(cs), e.Text))
}

// notifyCommand is /notify: show the level, set an override until Kiln
// quits, or drop it with "default".
func (m *Model) notifyCommand(cs *charState, args []string) {
	if len(args) == 0 {
		shown := string(m.notifyLevel(cs))
		if l := m.notifyOverrides[cs.Key]; l != "" {
			shown = str.NotifyLevelOverride(l, cs.Ch.Notify)
		}
		m.setStatus(false, str.NotifyLevel(shown))
		return
	}
	if args[0] == "default" {
		delete(m.notifyOverrides, cs.Key)
		m.setStatus(false, str.NotifyLevel(cs.Ch.Notify))
		return
	}
	l, err := notify.ParseLevel(args[0])
	if err != nil {
		m.setStatus(true, err.Error())
		return
	}
	m.notifyOverrides[cs.Key] = l
	m.setStatus(false, str.NotifyLevelUntilQuit(cs.Ch.Name, l))
}
