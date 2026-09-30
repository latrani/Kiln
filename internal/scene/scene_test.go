package scene

import (
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/theme"
)

func TestChipCycle(t *testing.T) {
	if Neutral.Next() != Only || Only.Next() != Hide || Hide.Next() != Neutral {
		t.Error("cycle wrong")
	}
}

func TestExportable(t *testing.T) {
	if !Exportable(logstore.Entry{Dir: logstore.In}) || Exportable(logstore.Entry{Dir: logstore.Out}) || Exportable(logstore.Entry{Dir: logstore.Sys}) {
		t.Error("only received lines are exportable")
	}
}

var sample = []logstore.Entry{
	{Dir: logstore.In, Text: "\x1b[1mSable\x1b[0m waves a paw."},
	{Dir: logstore.In, Text: "Kit says, \"<3 & hi\"\x1b]0;evil\x07"},
}

func TestPlain(t *testing.T) {
	want := "Sable waves a paw.\nKit says, \"<3 & hi\"\n"
	if got := Plain(sample); got != want {
		t.Errorf("Plain = %q, want %q", got, want)
	}
}

func TestANSI(t *testing.T) {
	got := ANSI(sample)
	if !strings.Contains(got, "\x1b[1mSable\x1b[0m waves a paw.\x1b[0m\n") || strings.Contains(got, "evil") {
		t.Errorf("ANSI = %q", got)
	}
}

func TestHTML(t *testing.T) {
	got := HTML(sample, "Tavern <scene>")
	for _, want := range []string{
		"<title>Tavern &lt;scene&gt;</title>",
		`<span style="font-weight:bold">Sable</span> waves a paw.` + "\n",
		"Kit says, &#34;&lt;3 &amp; hi&#34;\n",
		"</pre>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "evil") {
		t.Error("OSC leaked into HTML")
	}
}

func TestFileName(t *testing.T) {
	ts := time.Date(2026, 9, 24, 21, 14, 0, 0, time.UTC)
	got := FileName("/x/Kiln Scenes", "", ts, "furrymuck", "Kit", "html")
	if got != "/x/Kiln Scenes/2026-09-24 2114 furrymuck Kit.html" {
		t.Errorf("FileName = %q", got)
	}
	got = FileName("/x", "{world}/{name} {date}", ts, "furrymuck", "Kit", "plain")
	if got != "/x/furrymuck/Kit 2026-09-24.txt" {
		t.Errorf("FileName with a template = %q", got)
	}
	if Ext("ansi") != "ans" || Ext("plain") != "txt" || Ext("bogus") != "txt" {
		t.Error("Ext wrong")
	}
	if Render("plain", sample, "") != Plain(sample) || Render("html", sample, "t") != HTML(sample, "t") {
		t.Error("Render dispatch wrong")
	}
}

func TestHTMLUsesTheExportRole(t *testing.T) {
	th, err := theme.FromTOML("[ui]\nexport = { fg = \"#0a0b0c\", bg = \"#0d0e0f\" }\n")
	if err != nil {
		t.Fatal(err)
	}
	theme.SetActive(th)
	defer theme.SetActive(theme.Builtin())
	page := HTML(nil, "t")
	if !strings.Contains(page, "background: #0d0e0f") || !strings.Contains(page, "color: #0a0b0c") {
		t.Errorf("page:\n%s", page)
	}
}

func TestHTMLDefaultColorsUnchanged(t *testing.T) {
	page := HTML(nil, "t")
	if !strings.Contains(page, "background: #1b1b1f; color: #d8d8d8;") {
		t.Errorf("the built-in export colors changed:\n%s", page)
	}
}
