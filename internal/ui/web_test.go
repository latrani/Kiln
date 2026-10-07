package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/str"
)

// webCalls records what the web build's hooks were given.
type webCalls struct {
	saved      map[string]string
	backups    int
	restores   int
	restoreN   int
	restoreErr error
}

// webHarness is a harness with the web build's hooks set.
func webHarness(t *testing.T) (*harness, *webCalls) {
	t.Setenv("HOME", t.TempDir()) // "~/" paths never reach the real home
	h := newHarness(t, map[string]string{"fm": fmWorld})
	c := &webCalls{saved: map[string]string{}}
	h.deps.SaveFile = func(name string, data []byte) error { c.saved[name] = string(data); return nil }
	h.deps.Backup = func() (string, error) { c.backups++; return "kiln-backup.zip", nil }
	h.deps.Restore = func() (int, error) { c.restores++; return c.restoreN, c.restoreErr }
	h.m = New(h.deps, h.cfg)
	h.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return h, c
}

// run runs cmd and feeds its message back, as Bubble Tea would.
func (h *harness) run(cmd tea.Cmd) {
	if cmd != nil {
		h.m.Update(cmd())
	}
}

func TestSidebarFooterOnlyWithHooks(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	if strings.Contains(h.screen(), str.SidebarBackUp()) {
		t.Error("desktop sidebar shows Back up")
	}
	w, _ := webHarness(t)
	lines := strings.Split(w.screen(), "\n")
	if !strings.Contains(lines[22], str.SidebarBackUp()) || !strings.Contains(lines[23], str.SidebarRestore()) {
		t.Errorf("footer rows = %q / %q", lines[22], lines[23])
	}
}

func TestBackupClickAndCommand(t *testing.T) {
	h, c := webHarness(t)
	_, cmd := h.m.Update(tea.MouseClickMsg{X: 2, Y: 22, Button: tea.MouseLeft})
	h.run(cmd)
	if c.backups != 1 || h.m.status != str.StatusDownloaded("kiln-backup.zip") {
		t.Errorf("click: backups = %d, status %q", c.backups, h.m.status)
	}
	h.typeText("/backup")
	h.run(h.enter())
	if c.backups != 2 {
		t.Errorf("/backup: backups = %d", c.backups)
	}
}

func TestRestoreReloadsAndCounts(t *testing.T) {
	h, c := webHarness(t)
	c.restoreN = 3
	_, cmd := h.m.Update(tea.MouseClickMsg{X: 2, Y: 23, Button: tea.MouseLeft})
	h.run(cmd)
	if c.restores != 1 || h.m.status != str.StatusRestored(3) {
		t.Errorf("restores = %d, status %q", c.restores, h.m.status)
	}
	c.restoreN, c.restoreErr = 0, nil // cancelled: says nothing
	h.m.status = ""
	h.typeText("/restore")
	h.run(h.enter())
	if h.m.status != "" {
		t.Errorf("cancelled restore said %q", h.m.status)
	}
	c.restoreErr = errors.New("bad zip")
	h.typeText("/restore")
	h.run(h.enter())
	if h.m.status != str.StatusRestoreFailed(c.restoreErr) {
		t.Errorf("failed restore said %q", h.m.status)
	}
}

func TestBackupCommandsUnknownOnDesktop(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	for _, c := range []string{"/backup", "/restore"} {
		h.typeText(c)
		h.enter()
		if h.m.status != str.StatusUnknownCommand(c) {
			t.Errorf("%s: status = %q", c, h.m.status)
		}
	}
}

func TestStatusMsg(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	h.m.Update(StatusMsg{Text: str.WebNotSaving(), Err: true})
	if h.m.status != str.WebNotSaving() || !h.m.statusErr {
		t.Errorf("status = %q, err %v", h.m.status, h.m.statusErr)
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
