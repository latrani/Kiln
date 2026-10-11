package app

import (
	"slices"
	"testing"

	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/scene"
)

// tagWorld classifies "A …" as p/a, "B …" as p/b and "Q …" as q.
const tagWorld = fmWorld + `
[[classify]]
tag = "p/a"
pattern = '^A '

[[classify]]
tag = "p/b"
pattern = '^B '

[[classify]]
tag = "q"
pattern = '^Q '
`

func panelLog(t *testing.T) (*App, *Log) {
	t.Helper()
	a := sessionApp(t, map[string]string{"fm": tagWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, logDay, "A one", "B two", "Q three")
	return a, a.OpenLog("fm/kit")
}

// entryNames lists entries as names, a text row as "text:…", and one
// folded away in brackets.
func entryNames(es []PanelEntry) []string {
	var out []string
	for _, e := range es {
		n := e.Item.Name
		if e.Item.Text {
			n = "text:" + n
		}
		if e.FoldedAway {
			n = "(" + n + ")"
		}
		out = append(out, n)
	}
	return out
}

func TestEntriesListFoldedAwayChildren(t *testing.T) {
	_, g := panelLog(t)
	es := g.Entries()
	if got := entryNames(es); !slices.Equal(got, []string{"p", "(p/a)", "(p/b)", "q"}) || !es[0].Parent || es[1].Depth != 1 {
		t.Fatalf("entries %q, %+v; want p folded, its children listed folded away", got, es[:2])
	}
	g.Fold("p", false)
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "p/a", "p/b", "q"}) {
		t.Errorf("unfolded: %q", got)
	}
	g.Fold("q", true) // no children: nothing to fold
	if g.c.Collapsed["q"] {
		t.Error("folded a tag with no children")
	}
}

func TestFoldsOutliveLogMode(t *testing.T) {
	a, g := panelLog(t)
	g.Entries()
	g.Fold("p", false)
	a.CloseLog("fm/kit")
	g = a.OpenLog("fm/kit")
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "p/a", "p/b", "q"}) {
		t.Errorf("after reopening: %q; p should stay unfolded", got)
	}
}

func TestTextRowsAddAndRemove(t *testing.T) {
	_, g := panelLog(t)
	if !g.AddText("one") || g.AddText("one") || !g.AddText("three") {
		t.Fatal("AddText should report only a new term")
	}
	if g.RemoveText("q") {
		t.Error("removed a tag")
	}
	if !g.RemoveText("one") || g.RemoveText("one") {
		t.Fatal("RemoveText should report only a term it removed")
	}
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "(p/a)", "(p/b)", "q", "text:three"}) {
		t.Errorf("entries after removing one: %q", got)
	}
}

func TestHideAndOnlyThroughTheLog(t *testing.T) {
	_, g := panelLog(t)
	q := scene.Item{Name: "q"}
	g.ToggleHide(q)
	if !g.c.Filter.Hidden(q) {
		t.Error("q not hidden")
	}
	g.Fold("p", true)
	g.PressOnly(scene.Item{Name: "p/a"})
	if !g.FilteredUnder("p", g.Items()) {
		t.Error("p's folded children don't show as filtered")
	}
}
