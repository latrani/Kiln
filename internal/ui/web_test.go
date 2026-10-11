package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/str"
)

// webCalls records what the web build's hooks were given.
type webCalls struct {
	saved map[string]string
}

// webHarness is a harness with the web build's SaveFile hook set.
func webHarness(t *testing.T) (*harness, *webCalls) {
	t.Setenv("HOME", t.TempDir()) // "~/" paths never reach the real home
	h := newHarness(t, map[string]string{"fm": fmWorld})
	c := &webCalls{saved: map[string]string{}}
	h.deps.SaveFile = func(name string, data []byte) error { c.saved[name] = string(data); return nil }
	h.m = New(h.deps, h.cfg)
	h.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return h, c
}

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

func TestBrowseSaveDownloads(t *testing.T) {
	h, c := webHarness(t)
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("up", "m")
	h.keys("up", "up", "up", "up", "m")
	h.keys("e", "h")
	if v := h.br().pin.Value(); strings.Contains(v, "/") {
		t.Errorf("web filename prompt has a folder: %q", v)
	}
	h.br().pin.SetValue("~/scenes/x.txt")
	h.key("enter")
	if _, ok := c.saved["x.txt"]; !ok || len(c.saved) != 1 {
		t.Errorf("saved = %v", c.saved)
	}
	if !strings.Contains(h.screen(), str.StatusDownloaded("x.txt")) {
		t.Errorf("status missing:\n%s", h.screen())
	}
}
