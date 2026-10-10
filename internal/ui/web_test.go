package ui

import (
	"errors"
	"fmt"
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
	backupErr  error
}

// webHarness is a harness with the web build's hooks set.
func webHarness(t *testing.T) (*harness, *webCalls) {
	return webHarnessWith(t, map[string]string{"fm": fmWorld})
}

// webHarnessWith is webHarness with these worlds.
func webHarnessWith(t *testing.T, worlds map[string]string) (*harness, *webCalls) {
	t.Setenv("HOME", t.TempDir()) // "~/" paths never reach the real home
	h := newHarness(t, worlds)
	c := &webCalls{saved: map[string]string{}}
	h.deps.SaveFile = func(name string, data []byte) error { c.saved[name] = string(data); return nil }
	h.deps.Backup = func() (string, error) { c.backups++; return "kiln-backup.zip", c.backupErr }
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
	if c.backups != 1 || h.m.a.Status().Text != str.StatusDownloaded("kiln-backup.zip") {
		t.Errorf("click: backups = %d, status %q", c.backups, h.m.a.Status().Text)
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
	if c.restores != 1 || h.m.a.Status().Text != str.StatusRestored(3) {
		t.Errorf("restores = %d, status %q", c.restores, h.m.a.Status().Text)
	}
	c.restoreN, c.restoreErr = 0, nil // cancelled: says nothing
	h.m.a.ClearStatus()
	h.typeText("/restore")
	h.run(h.enter())
	if h.m.a.Status().Text != "" {
		t.Errorf("cancelled restore said %q", h.m.a.Status().Text)
	}
	c.restoreErr = errors.New("bad zip")
	h.typeText("/restore")
	h.run(h.enter())
	if h.m.a.Status().Text != str.StatusRestoreFailed(c.restoreErr) {
		t.Errorf("failed restore said %q", h.m.a.Status().Text)
	}
}

func TestBackupCommandsUnknownOnDesktop(t *testing.T) {
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

func TestBackupFailed(t *testing.T) {
	h, c := webHarness(t)
	c.backupErr = errors.New("no space")
	h.typeText("/backup")
	h.run(h.enter())
	if h.m.a.Status().Text != str.StatusBackupFailed(c.backupErr) || !h.m.a.Status().Err {
		t.Errorf("status = %q, err %v", h.m.a.Status().Text, h.m.a.Status().Err)
	}
}

// /backup and /restore don't need a character.
func TestBackupCommandsWithNothingOpen(t *testing.T) {
	h, c := webHarnessWith(t, map[string]string{"sp": spWorld}) // nothing autoconnects
	c.restoreN = 2
	h.typeText("/backup")
	h.run(h.enter())
	h.typeText("/restore")
	h.run(h.enter())
	if c.backups != 1 || c.restores != 1 {
		t.Errorf("backups = %d, restores = %d; status %q", c.backups, c.restores, h.m.a.Status().Text)
	}
}

// A second Back up while one runs is ignored: a double-click would
// otherwise download two zips.
func TestBackupIgnoredWhileRunning(t *testing.T) {
	h, c := webHarness(t)
	click := func() tea.Cmd {
		_, cmd := h.m.Update(tea.MouseClickMsg{X: 2, Y: 22, Button: tea.MouseLeft})
		return cmd
	}
	first := click()
	if again := click(); again != nil {
		t.Error("second click while backing up returned a command")
	}
	h.typeText("/backup")
	if cmd := h.enter(); cmd != nil {
		t.Error("/backup while backing up returned a command")
	}
	h.run(first)
	h.run(click())
	if c.backups != 2 {
		t.Errorf("backups = %d, want 2", c.backups)
	}
}

func TestFooterHiddenOnShortScreen(t *testing.T) {
	h, c := webHarness(t)
	h.m.Update(tea.WindowSizeMsg{Width: 80, Height: 7})
	if f := h.m.footerH(); f != 0 {
		t.Errorf("footerH at height 7 = %d", f)
	}
	if strings.Contains(h.screen(), str.SidebarBackUp()) {
		t.Errorf("footer on a short screen:\n%s", h.screen())
	}
	_, cmd := h.m.Update(tea.MouseClickMsg{X: 2, Y: 6, Button: tea.MouseLeft})
	h.run(cmd)
	if c.backups != 0 || c.restores != 0 {
		t.Errorf("bottom-row click ran backups = %d, restores = %d", c.backups, c.restores)
	}
	h.m.Update(tea.WindowSizeMsg{Width: 80, Height: 8})
	if f := h.m.footerH(); f != 2 {
		t.Errorf("footerH at height 8 = %d", f)
	}
}

// crowdWorld has enough open characters to overflow a short sidebar.
func crowdWorld(n int) string {
	s := "host = \"crowd.test\"\nport = 1\n"
	for i := range n {
		s += fmt.Sprintf("\n[[characters]]\nid = \"c%d\"\nname = \"C%d\"\nautoconnect = true\n", i, i)
	}
	return s
}

// sideCol is the sidebar column of each screen row, trimmed.
func sideCol(h *harness) []string {
	var out []string
	for _, row := range strings.Split(h.screen(), "\n") {
		out = append(out, strings.TrimSpace(strings.SplitN(row, "│", 2)[0]))
	}
	return out
}

func TestFooterBelowOverflowHints(t *testing.T) {
	h, c := webHarnessWith(t, map[string]string{"crowd": crowdWorld(8)})
	h.m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	side, sv := sideCol(h), h.m.sidebarView()
	if !sv.below {
		t.Fatalf("sidebar doesn't overflow:\n%s", h.screen())
	}
	if want := str.ViewMoreBelow(len(sv.rows) - sv.top - sv.avail); side[7] != want {
		t.Errorf("row 7 = %q, want %q:\n%s", side[7], want, h.screen())
	}
	if side[8] != str.SidebarBackUp() || side[9] != str.SidebarRestore() {
		t.Errorf("footer rows = %q / %q", side[8], side[9])
	}
	_, cmd := h.m.Update(tea.MouseClickMsg{X: 2, Y: 7, Button: tea.MouseLeft}) // the ▼ hint
	h.run(cmd)
	if c.backups != 0 || h.m.sideTop == 0 {
		t.Errorf("hint click: backups = %d, sideTop = %d", c.backups, h.m.sideTop)
	}
	h.m.scrollSidebar(100) // to the end: only the ▲ hint
	side, sv = sideCol(h), h.m.sidebarView()
	if want := str.ViewMoreAbove(sv.top); side[0] != want || sv.below {
		t.Errorf("row 0 = %q, want %q; below %v", side[0], want, sv.below)
	}
	if side[7] != str.SidebarOpenConnection() || side[8] != str.SidebarBackUp() || side[9] != str.SidebarRestore() {
		t.Errorf("bottom rows = %q", side[7:])
	}
}

func TestFooterHiddenInPicker(t *testing.T) {
	h, c := webHarness(t)
	h.m.openPicker()
	if !h.m.listing() {
		t.Fatal("picker didn't open")
	}
	if s := h.screen(); strings.Contains(s, str.SidebarBackUp()) || strings.Contains(s, str.SidebarRestore()) {
		t.Errorf("footer in the picker:\n%s", s)
	}
	_, cmd := h.m.Update(tea.MouseClickMsg{X: 2, Y: 23, Button: tea.MouseLeft})
	if cmd != nil {
		t.Error("click on the picker's empty bottom row returned a command")
	}
	if c.restores != 0 {
		t.Errorf("restores = %d", c.restores)
	}
}

func TestFooterHiddenUnderFilterPanel(t *testing.T) {
	h, c := webHarness(t)
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	if h.br().Panel == nil {
		t.Fatal("panel didn't open")
	}
	if s := h.screen(); strings.Contains(s, str.SidebarBackUp()) || strings.Contains(s, str.SidebarRestore()) {
		t.Errorf("footer under the panel:\n%s", s)
	}
	for _, y := range []int{22, 23} {
		_, cmd := h.m.Update(tea.MouseClickMsg{X: 2, Y: y, Button: tea.MouseLeft})
		h.run(cmd)
	}
	if c.backups != 0 || c.restores != 0 {
		t.Errorf("panel clicks ran backups = %d, restores = %d", c.backups, c.restores)
	}
}
