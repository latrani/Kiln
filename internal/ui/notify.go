package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/notify"
)

// encode turns a message into the bytes to write.
func (m *Model) encode(msg string) tea.Cmd {
	method := notify.OSC
	if m.a.Config() != nil {
		method = m.a.Config().NotifyMethod
	}
	return m.d.Raw(notify.Encode(msg, method, m.d.Tmux))
}
