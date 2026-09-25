package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/style"
)

func TestTrimURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://x.test/a.":           "https://x.test/a",
		"https://x.test/a),":          "https://x.test/a",
		"https://en.wiki/Foo_(bar)":   "https://en.wiki/Foo_(bar)",
		"https://en.wiki/Foo_(bar)).": "https://en.wiki/Foo_(bar)",
		"http://x.test/?q=1&r=2!":     "http://x.test/?q=1&r=2",
		"https://x.test/[1]]":         "https://x.test/[1]",
	} {
		if got := trimURL(in); got != want {
			t.Errorf("trimURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// sbWith is a scrollback of lines, width w, drawn h rows tall.
func sbWith(w, h int, lines ...string) *Scrollback {
	var s Scrollback
	for _, l := range lines {
		s.Append(l)
	}
	s.SetWidth(w)
	s.View(h)
	return &s
}

func TestScrollbackPositionsAcrossWraps(t *testing.T) {
	// Wraps to "see the", "https://ex", "ample.com/", "x now" at width 10;
	// the spaces at the breaks are dropped.
	s := sbWith(10, 4, "see the https://example.com/x now")
	p, ok := s.At(1, 3) // the second "t" of https
	if !ok || p.off != 11 {
		t.Fatalf("At(1, 3) = %+v, %v; want offset 11", p, ok)
	}
	if u := s.URLAt(p); u != "https://example.com/x" {
		t.Errorf("URLAt = %q", u)
	}
	if p, _ := s.At(3, 0); s.URLAt(p) != "https://example.com/x" {
		t.Error("the link's wrapped tail should be clickable too")
	}
	if p, _ := s.At(3, 3); s.URLAt(p) != "" {
		t.Error(`"now" isn't part of the link`)
	}
	if p, _ := s.At(0, 99); p.off != len("see the") {
		t.Errorf("past the row's end: offset %d", p.off)
	}
}

func TestScrollbackSelectionCopiesLogicalLines(t *testing.T) {
	s := sbWith(10, 3, "one two three four", "\x1b[31mred\x1b[0m line")
	// Rows: "one two", "three four", "red line".
	a, _ := s.At(0, 4)
	s.StartSelect(a)
	b, _ := s.At(2, 2)
	s.DragTo(b)
	text, _, clicked := s.EndSelect()
	if clicked || text != "two three four\nred" {
		t.Errorf("selection = %q; soft wraps must not become newlines", text)
	}
	rows := s.View(3)
	if !strings.Contains(rows[0], reverse+"two") || !strings.Contains(rows[2], reverse+"red") {
		t.Errorf("selection not highlighted: %q", rows)
	}
	s.StartSelect(a) // a click without moving selects nothing
	if text, p, clicked := s.EndSelect(); !clicked || text != "" || p != a {
		t.Errorf("click: %q %+v %v", text, p, clicked)
	}
}

func TestScrollbackSelectionSurvivesPrepend(t *testing.T) {
	s := sbWith(20, 3, "alpha", "beta")
	a, _ := s.At(1, 0)
	s.StartSelect(a)
	b, _ := s.At(2, 3)
	s.DragTo(b)
	s.Prepend([]string{"older"}, false)
	if text, _, _ := s.EndSelect(); text != "alpha\nbeta" {
		t.Errorf("after prepend: %q", text)
	}
}

func TestInputSelectionIgnoresSoftWraps(t *testing.T) {
	in := NewInput()
	in.SetValue("abcdefgh\nxyz")
	in.Render(5, 0, false, false) // 4 cells a row: "abcd", "efgh", "xyz"
	in.StartSelect(2, 0)
	in.DragTo(1, 2)
	rows, _, _ := in.Render(5, 0, false, false)
	if !strings.Contains(rows[0], reverse+"c") || strings.Contains(rows[2], reverse+"z") {
		t.Errorf("highlight wrong: %q", rows)
	}
	if got := in.EndSelect(); got != "cdefgh\nxy" {
		t.Errorf("copied %q; the wrap between abcd and efgh isn't a newline", got)
	}
	if in.Value() != "abcdefgh\nxyz" {
		t.Error("selecting changed the text")
	}
	in.Render(5, 0, false, true) // masked: a password is never selectable
	in.StartSelect(0, 0)
	in.DragTo(3, 0)
	if got := in.EndSelect(); got != "" {
		t.Errorf("masked input copied %q", got)
	}
}

// clipboard is what cmd puts on the clipboard, or "".
func clipboard(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	msg := cmd()
	if fmt.Sprintf("%T", msg) != "tea.setClipboardMsg" {
		return ""
	}
	return fmt.Sprint(msg)
}

func TestMouseSelectAndLinks(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	var opened []string
	h.m.d.OpenURL = func(u string) error { opened = append(opened, u); return nil }
	cs := h.m.chars["fm/kit"]
	cs.sb.Append("Mira pages: look at https://kiln.test/map please")
	h.screen()
	l := h.m.layout()
	x0 := l.sw + 1
	y := l.sbH - 1 // the newest line sits at the bottom
	mouse := func(msg tea.Msg) tea.Cmd { _, cmd := h.m.Update(msg); return cmd }

	col := strings.Index("Mira pages: look at https://kiln.test/map please", "kiln")
	mouse(tea.MouseClickMsg{X: x0 + col, Y: y, Button: tea.MouseLeft})
	mouse(tea.MouseReleaseMsg{X: x0 + col, Y: y, Button: tea.MouseLeft})
	if len(opened) != 1 || opened[0] != "https://kiln.test/map" {
		t.Errorf("opened %q", opened)
	}
	mouse(tea.MouseClickMsg{X: x0 + 2, Y: y, Button: tea.MouseLeft}) // not a link
	mouse(tea.MouseReleaseMsg{X: x0 + 2, Y: y, Button: tea.MouseLeft})
	if len(opened) != 1 {
		t.Errorf("a click off the link opened %q", opened)
	}

	mouse(tea.MouseClickMsg{X: x0, Y: y, Button: tea.MouseLeft})
	mouse(tea.MouseMotionMsg{X: x0 + 3, Y: y, Button: tea.MouseLeft})
	if got := clipboard(mouse(tea.MouseReleaseMsg{X: x0 + 3, Y: y, Button: tea.MouseLeft})); got != "Mira" {
		t.Errorf("clipboard = %q", got)
	}
	if !strings.Contains(h.screen(), "copied to clipboard") {
		t.Errorf("no status:\n%s", h.screen())
	}
	h.typeText("x") // a key drops the highlight
	if cs.sb.sel != nil {
		t.Error("selection outlived a key press")
	}

	cs.in.SetValue("hello there")
	h.screen()
	iy := l.sbH + 1
	mouse(tea.MouseClickMsg{X: x0 + gutterWidth, Y: iy, Button: tea.MouseLeft})
	mouse(tea.MouseMotionMsg{X: x0 + gutterWidth + 4, Y: iy, Button: tea.MouseLeft})
	if got := clipboard(mouse(tea.MouseReleaseMsg{X: x0 + gutterWidth + 4, Y: iy, Button: tea.MouseLeft})); got != "hello" {
		t.Errorf("input clipboard = %q", got)
	}
}

func TestLinksUnderlinedAndLitOnHover(t *testing.T) {
	// Red text with a link that wraps: "\x1b[31mgo", "https://ex", "ample.com", "now".
	s := sbWith(10, 4, "\x1b[31mgo https://example.com now\x1b[0m")
	rows := s.View(4)
	if !strings.Contains(rows[1], style.Reset+linkSGR+"https://ex"+style.Reset) ||
		!strings.Contains(rows[2], style.Reset+linkSGR+"ample.com"+style.Reset) {
		t.Fatalf("link not underlined on both rows (over the line's red): %q", rows)
	}
	if strings.Contains(rows[0], linkSGR) || strings.Contains(rows[3], linkSGR) {
		t.Errorf("text outside the link underlined: %q", rows)
	}
	p, _ := s.At(2, 2)
	s.Hover(&p)
	rows = s.View(4)
	if !strings.Contains(rows[1], hoverSGR+"https://ex") || !strings.Contains(rows[2], hoverSGR+"ample.com") {
		t.Errorf("hovered link not blue on every row: %q", rows)
	}
	q, _ := s.At(3, 1)
	s.Hover(&q) // off the link
	if rows = s.View(4); strings.Contains(strings.Join(rows, ""), hoverSGR) {
		t.Errorf("link still lit after the pointer left: %q", rows)
	}
}

func TestHoverTracksPointer(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	cs := h.m.chars["fm/kit"]
	line := "see https://kiln.test/map"
	cs.sb.Append(line)
	h.screen()
	l := h.m.layout()
	col := strings.Index(line, "kiln")
	h.m.Update(tea.MouseMotionMsg{X: l.sw + 1 + col, Y: l.sbH - 1})
	if !strings.Contains(h.m.View().Content, hoverSGR+"https://kiln.test/map") {
		t.Error("link under the pointer isn't lit")
	}
	h.m.Update(tea.MouseMotionMsg{X: 2, Y: 0}) // over the sidebar
	if strings.Contains(h.m.View().Content, hoverSGR) {
		t.Error("link stayed lit after the pointer left the scrollback")
	}
}
