package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/rules"
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
	if l := m.notifyOverrides[cs.key]; l != "" {
		return l
	}
	return cs.ch.Notify
}

// here records that you're at the terminal: a focus-in, key, paste,
// click or wheel. It re-arms "first" for every character.
// You've seen anything held, so it's dropped.
func (m *Model) here() {
	m.lastHere = m.d.Now()
	m.hereGen++
	for _, cs := range m.chars {
		cs.held, cs.heldMore = "", 0
	}
}

// notifyIdle is the notify_idle setting.
func (m *Model) notifyIdle() time.Duration {
	if m.cfg != nil {
		return m.cfg.NotifyIdle
	}
	return config.DefaultNotifyIdle
}

// away reports whether you've switched away, or been idle past
// notify_idle.
func (m *Model) away() bool {
	idle := m.notifyIdle()
	return !m.focused || idle > 0 && m.d.Now().Sub(m.lastHere) > idle
}

// encode turns a message into the bytes to write.
func (m *Model) encode(msg string) tea.Cmd {
	method := notify.OSC
	if m.cfg != nil {
		method = m.cfg.NotifyMethod
	}
	return m.d.Raw(notify.Encode(msg, method, m.d.Tmux))
}

// flushHeld sends what's been held since you were last here: per
// character, the first line and how many more there were.
func (m *Model) flushHeld() tea.Cmd {
	var cmds []tea.Cmd
	for _, k := range m.order {
		cs := m.chars[k]
		if cs.held == "" {
			continue
		}
		msg := cs.held
		if cs.heldMore > 0 {
			msg = str.NotifyHeldMore(msg, cs.heldMore)
		}
		cmds = append(cmds, m.encode(msg))
		cs.held, cs.heldMore = "", 0
	}
	return tea.Batch(cmds...)
}

// notifyName is the character's name, with its world when another
// world has a character of the same name.
func (m *Model) notifyName(cs *charState) string {
	for _, ch := range m.allChars() {
		if ch.World != cs.ch.World && strings.EqualFold(ch.Name, cs.ch.Name) {
			return cs.ch.Name + "@" + cs.ch.World
		}
	}
	return cs.ch.Name
}

// notifyCmd writes a notification for an incoming line if you're away
// and cs's level wants one. If you still count as here, it holds the
// line instead, to send once notify_idle passes without you coming back
// (so nothing that arrives just after you look away is missed).
func (m *Model) notifyCmd(cs *charState, e logstore.Entry, res rules.Result) tea.Cmd {
	away := m.away()
	if e.Dir != logstore.In || res.Quiet || !away && m.notifyIdle() == 0 {
		return nil
	}
	now := m.d.Now()
	if !res.Attention && (now.Sub(cs.connectedAt) < connectGrace || now.Sub(cs.lastSent) < burstGap) {
		return nil
	}
	switch m.notifyLevel(cs) {
	case notify.All:
	case notify.First:
		if !res.Attention && cs.sentGen == m.hereGen {
			return nil
		}
	case notify.Attention:
		if !res.Attention {
			return nil
		}
	default:
		return nil
	}
	cs.sentGen, cs.lastSent = m.hereGen, now
	msg := notify.Message(m.notifyName(cs), e.Text)
	if away {
		return m.encode(msg)
	}
	if cs.held != "" {
		cs.heldMore++
		return nil
	}
	cs.held = msg
	gen := m.hereGen
	return tea.Tick(m.lastHere.Add(m.notifyIdle()).Sub(now), func(time.Time) tea.Msg { return notifyDueMsg(gen) })
}

// notifyCommand is /notify: show the level, set an override until Kiln
// quits, or drop it with "default".
func (m *Model) notifyCommand(cs *charState, args []string) {
	if len(args) == 0 {
		shown := string(m.notifyLevel(cs))
		if l := m.notifyOverrides[cs.key]; l != "" {
			shown = str.NotifyLevelOverride(l, cs.ch.Notify)
		}
		m.setStatus(false, str.NotifyLevel(shown))
		return
	}
	if args[0] == "default" {
		delete(m.notifyOverrides, cs.key)
		m.setStatus(false, str.NotifyLevel(cs.ch.Notify))
		return
	}
	l, err := notify.ParseLevel(args[0])
	if err != nil {
		m.setStatus(true, err.Error())
		return
	}
	m.notifyOverrides[cs.key] = l
	m.setStatus(false, str.NotifyLevelUntilQuit(cs.ch.Name, l))
}
