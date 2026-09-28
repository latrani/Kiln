package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/rules"
)

// notifyLevel is the /notify override, or the configured level.
func (m *Model) notifyLevel(cs *charState) notify.Level {
	if l := m.notifyOverrides[cs.key]; l != "" {
		return l
	}
	return cs.ch.Notify
}

// here records that you're at the terminal: a focus-in, key, paste,
// click or wheel. It re-arms "first" for every character.
func (m *Model) here() { m.lastHere = m.d.Now() }

// away reports whether you've switched away, or been idle past
// notify_idle.
func (m *Model) away() bool {
	if !m.focused {
		return true
	}
	idle := config.DefaultNotifyIdle
	if m.cfg != nil {
		idle = m.cfg.NotifyIdle
	}
	return idle > 0 && m.d.Now().Sub(m.lastHere) > idle
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
// and cs's level wants one; otherwise it returns nil.
func (m *Model) notifyCmd(cs *charState, e logstore.Entry, res rules.Result) tea.Cmd {
	if e.Dir != logstore.In || res.Quiet || !m.away() {
		return nil
	}
	switch m.notifyLevel(cs) {
	case notify.All:
	case notify.First:
		if !res.Attention && !cs.firstSent.Before(m.lastHere) {
			return nil
		}
	case notify.Attention:
		if !res.Attention {
			return nil
		}
	default:
		return nil
	}
	cs.firstSent = m.d.Now()
	method := notify.OSC
	if m.cfg != nil {
		method = m.cfg.NotifyMethod
	}
	return m.d.Raw(notify.Encode(notify.Message(m.notifyName(cs), e.Text), method, m.d.Tmux))
}

// notifyCommand is /notify: show the level, set an override until Kiln
// quits, or drop it with "default".
func (m *Model) notifyCommand(cs *charState, args []string) {
	if len(args) == 0 {
		shown := string(m.notifyLevel(cs))
		if l := m.notifyOverrides[cs.key]; l != "" {
			shown = fmt.Sprintf("%s (override; config says %s)", l, cs.ch.Notify)
		}
		m.setStatus(false, "%s notify: %s", cs.ch.Name, shown)
		return
	}
	if args[0] == "default" {
		delete(m.notifyOverrides, cs.key)
		m.setStatus(false, "%s notify: %s", cs.ch.Name, cs.ch.Notify)
		return
	}
	l, err := notify.ParseLevel(args[0])
	if err != nil {
		m.setStatus(true, "%v", err)
		return
	}
	m.notifyOverrides[cs.key] = l
	m.setStatus(false, "%s notify: %s until Kiln quits", cs.ch.Name, l)
}
