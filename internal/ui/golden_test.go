package ui

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
)

var update = flag.Bool("update", false, "rewrite golden screens in testdata/golden")

// sgrState is what a terminal would draw a cell with.
type sgrState struct {
	bold, faint, italic, underline, reverse bool
	fg, bg                                  string
}

func (s sgrState) key() string {
	var parts []string
	for _, f := range []struct {
		on   bool
		name string
	}{{s.bold, "bold"}, {s.faint, "faint"}, {s.italic, "italic"}, {s.underline, "underline"}, {s.reverse, "reverse"}} {
		if f.on {
			parts = append(parts, f.name)
		}
	}
	if s.fg != "" {
		parts = append(parts, "fg="+s.fg)
	}
	if s.bg != "" {
		parts = append(parts, "bg="+s.bg)
	}
	return strings.Join(parts, ",")
}

// apply folds one SGR parameter list into s.
func (s *sgrState) apply(params string) {
	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		n, _ := strconv.Atoi(ps[i])
		switch {
		case ps[i] == "" || n == 0:
			*s = sgrState{}
		case n == 1:
			s.bold = true
		case n == 2:
			s.faint = true
		case n == 3:
			s.italic = true
		case n == 4:
			s.underline = true
		case n == 7:
			s.reverse = true
		case n == 22:
			s.bold, s.faint = false, false
		case n == 23:
			s.italic = false
		case n == 24:
			s.underline = false
		case n == 27:
			s.reverse = false
		case n >= 30 && n <= 37:
			s.fg = fmt.Sprintf("ansi%d", n-30)
		case n >= 90 && n <= 97:
			s.fg = fmt.Sprintf("ansi%d", n-90+8)
		case n == 39:
			s.fg = ""
		case n >= 40 && n <= 47:
			s.bg = fmt.Sprintf("ansi%d", n-40)
		case n >= 100 && n <= 107:
			s.bg = fmt.Sprintf("ansi%d", n-100+8)
		case n == 49:
			s.bg = ""
		case (n == 38 || n == 48) && i+1 < len(ps):
			var c string
			if ps[i+1] == "2" && i+4 < len(ps) {
				r, _ := strconv.Atoi(ps[i+2])
				g, _ := strconv.Atoi(ps[i+3])
				b, _ := strconv.Atoi(ps[i+4])
				c, i = fmt.Sprintf("#%02x%02x%02x", r, g, b), i+4
			} else if ps[i+1] == "5" && i+2 < len(ps) {
				c, i = "idx"+ps[i+2], i+2
			}
			if n == 38 {
				s.fg = c
			} else {
				s.bg = c
			}
		}
	}
}

// cells describes content as rows of text, each followed by its styled
// runs ("  @3-8 bold,reverse"). Two contents that draw the same are equal.
func cells(content string) string {
	var out strings.Builder
	var st sgrState
	for _, row := range strings.Split(content, "\n") {
		var text strings.Builder
		var runs []string
		col, runStart, runKey := 0, 0, ""
		flush := func() {
			if runKey != "" && col > runStart {
				runs = append(runs, fmt.Sprintf("@%d-%d %s", runStart, col-1, runKey))
			}
		}
		for i := 0; i < len(row); {
			if row[i] == 0x1b {
				j := ansi.EscapeEnd(row, i)
				seq := row[i:j]
				if len(seq) >= 3 && seq[1] == '[' && seq[len(seq)-1] == 'm' {
					st.apply(seq[2 : len(seq)-1])
				}
				i = j
				continue
			}
			r, size := []rune(row[i:])[0], len(string([]rune(row[i:])[0]))
			if k := st.key(); k != runKey {
				flush()
				runStart, runKey = col, k
			}
			text.WriteRune(r)
			col++
			i += size
		}
		flush()
		out.WriteString(text.String() + "\n")
		if len(runs) > 0 {
			out.WriteString("  " + strings.Join(runs, "; ") + "\n")
		}
	}
	return out.String()
}

// assertGolden compares content, as cells, with testdata/golden/<name>.txt.
func assertGolden(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".txt")
	got := cells(content)
	if *update {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("screen %s drew differently.\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

func (h *harness) drawn() string { return h.m.View().Content }

func TestGoldenCellsSeeThroughBytes(t *testing.T) {
	a := cells("\x1b[7m\x1b[1mhi\x1b[0m")
	b := cells("\x1b[1;7mhi\x1b[m")
	if a != b {
		t.Errorf("same cells, different result:\n%s\n%s", a, b)
	}
}

// goldenWorld has local echo on, so sent lines show in the scrollback.
var goldenWorld = strings.Replace(fmWorld, "max_line_bytes = 20", "max_line_bytes = 20\nlocal_echo = true", 1)

// show feeds Kit a line and waits until it's in the scrollback.
func (h *harness) show(line string) {
	h.t.Helper()
	h.conn("fm/kit").Feed(line)
	h.settle("fm/kit", func() bool {
		for _, l := range h.m.chars["fm/kit"].sb.lines {
			if strings.Contains(ansi.Strip(l.text), line) {
				return true
			}
		}
		return false
	})
}

func goldenHarness(t *testing.T) *harness {
	h := newHarness(t, map[string]string{"fm": goldenWorld})
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.advance(time.Minute)
	return h
}

func TestGoldenMain(t *testing.T) {
	h := goldenHarness(t)
	h.open("fm/rook")
	h.show("Rook says, \"Evening, Kit.\"")
	h.show("See https://kiln.test/map for the way")
	h.typeText(":waves.")
	h.enter()
	h.m.chars["fm/rook"].Unread, h.m.chars["fm/rook"].Attention = 3, true
	assertGolden(t, "main", h.drawn())
}

func TestGoldenWebSidebar(t *testing.T) {
	h, _ := webHarness(t)
	h.init()
	h.settle("fm/kit", h.connected("fm/kit"))
	h.advance(time.Minute)
	assertGolden(t, "web-sidebar", h.drawn())
}

func TestGoldenPicker(t *testing.T) {
	h := goldenHarness(t)
	h.show("Rook says, \"Evening, Kit.\"") // bold (self), so the backdrop shows
	h.press('o', tea.ModCtrl)
	assertGolden(t, "picker", h.drawn())
}

func TestGoldenEditor(t *testing.T) {
	h := goldenHarness(t)
	h.typeText("/edit world")
	h.enter()
	assertGolden(t, "editor", h.drawn())
	h.focusOn(portLabel)
	h.typeText("x") // rejected: shows the field's error
	assertGolden(t, "editor-error", h.drawn())
}

func TestGoldenLog(t *testing.T) {
	h := goldenHarness(t)
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.keys("m", "up", "m")                                            // a range
	h.kitFilter().PressOnly(scene.Item{Name: "page"}, h.br().Items()) // the Filter chip on
	h.key("/")
	h.typeText("Mira") // the page the chip shows
	h.key("enter")     // find
	assertGolden(t, "log", h.drawn())
}

func TestGoldenInput(t *testing.T) {
	h := goldenHarness(t)
	h.typeText("this line runs past twenty bytes")
	in := h.m.input()
	in.StartSelect(0, 0) // select the first four cells, as a mouse drag would
	in.DragTo(4, 0)
	h.m.setStatus(true, str.StatusNotSent(errors.New("x")))
	assertGolden(t, "input", h.drawn())
}

func TestGoldenScrolled(t *testing.T) {
	h := goldenHarness(t)
	for i := 0; i < 40; i++ {
		h.advance(2 * time.Second)
		h.show(fmt.Sprintf("line %d https://kiln.test/%d", i, i))
		h.m.cur().sb.MarkSeen() // read as it comes, so the pager holds nothing
	}
	h.press(tea.KeyPgUp, 0)
	h.drawn() // hover reads what the last View put on screen
	l := h.m.layout()
	h.m.Update(tea.MouseMotionMsg{X: l.sw + 1 + len("line 19 h"), Y: l.top + l.sbH - 1})
	assertGolden(t, "scrolled", h.drawn())
}

func TestGoldenPassword(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": goldenWorld})
	delete(h.pw, "fm/kit")
	h.init()
	h.settle("fm/kit", func() bool { return h.m.chars["fm/kit"].NeedPW })
	h.typeText("s3cret")
	assertGolden(t, "password", h.drawn())
}

func TestGoldenLogFilter(t *testing.T) {
	h := goldenHarness(t)
	h.writeLog(day24, scene1...)
	h.key("ctrl+l")
	h.key("f")
	h.keys("down", "down", "h", "down", "o") // past Untagged: hide page/in, then Only on self
	assertGolden(t, "log-filter", h.drawn())
}
