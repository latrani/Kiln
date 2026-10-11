package ui

import (
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

// /backup and /restore were the xterm web build's; the terminal doesn't
// know them.
func TestBackupCommandsUnknown(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	for _, c := range []string{"/backup", "/restore"} {
		h.typeText(c)
		h.enter()
		if h.m.a.Status().Text != str.StatusUnknownCommand(c) {
			t.Errorf("%s: status = %q", c, h.m.a.Status().Text)
		}
	}
}

func TestStatusMsg(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(StatusMsg{Text: str.WebNotSaving(), Err: true})
	if h.m.a.Status().Text != str.WebNotSaving() || !h.m.a.Status().Err {
		t.Errorf("status = %q, err %v", h.m.a.Status().Text, h.m.a.Status().Err)
	}
}
