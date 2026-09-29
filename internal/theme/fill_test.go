package theme

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestFillUnstyledIsPlainFit(t *testing.T) {
	b := mustBuild(t, "")
	if got := b.Fill(Sidebar, "abc", 6); got != "abc   " {
		t.Errorf("Fill = %q", got)
	}
	if got := b.Fill(Sidebar, "abcdef", 3); got != "abc" {
		t.Errorf("Fill = %q", got)
	}
}

func TestFillReassertsAfterReset(t *testing.T) {
	th := mustBuild(t, "[ui]\nsidebar = { bg = \"#010203\" }\n\"status.error\" = { fg = \"red\" }\n")
	base := th.SGR(Sidebar)
	row := "a" + th.Paint(StatusError, "b") + "c" // "b" ends in a reset
	got := th.Fill(Sidebar, row, 5)
	if !strings.HasPrefix(got, base) || !strings.Contains(got, Reset+base+"c") || !strings.HasSuffix(got, "  "+Reset) {
		t.Errorf("Fill = %q: want the area's background after every reset and under the padding", got)
	}
	if xansi.StringWidth(got) != 5 {
		t.Errorf("width = %d", xansi.StringWidth(got))
	}
}

func TestFillNarrow(t *testing.T) {
	th := mustBuild(t, "[ui]\nsidebar = { bg = \"#010203\" }\n")
	if got := th.Fill(Sidebar, "abc", 0); got != "" {
		t.Errorf("width 0 = %q", got)
	}
	if got := th.Fill(Sidebar, "abcdef", 2); xansi.StringWidth(got) != 2 {
		t.Errorf("cut = %q", got)
	}
	if got := th.Fill(Sidebar, "abc", -1); got != "" {
		t.Errorf("negative width = %q", got)
	}
}
