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

func entryNames(es []PanelEntry) []string {
	var out []string
	for _, e := range es {
		switch {
		case e.Add:
			out = append(out, "+")
		case e.Item.Text:
			out = append(out, "text:"+e.Item.Name)
		default:
			out = append(out, e.Item.Name)
		}
	}
	return out
}

func TestEntriesFoldNewParents(t *testing.T) {
	_, g := panelLog(t)
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "q", "+"}) {
		t.Fatalf("entries %q; want p folded", got)
	}
	g.SetCollapsed(FilterSel{Item: scene.Item{Name: "p"}}, false)
	es := g.Entries()
	if got := entryNames(es); !slices.Equal(got, []string{"p", "p/a", "p/b", "q", "+"}) || !es[0].Parent || es[1].Depth != 1 {
		t.Errorf("unfolded: %q, %+v", got, es[:2])
	}
	g.SetCollapsed(FilterSel{Item: scene.Item{Name: "q"}}, true) // no children: nothing to fold
	if g.c.Collapsed["q"] {
		t.Error("folded a tag with no children")
	}
}

func TestFoldsOutliveLogMode(t *testing.T) {
	a, g := panelLog(t)
	g.Entries()
	g.SetCollapsed(FilterSel{Item: scene.Item{Name: "p"}}, false)
	a.CloseLog("fm/kit")
	g = a.OpenLog("fm/kit")
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "p/a", "p/b", "q", "+"}) {
		t.Errorf("after reopening: %q; p should stay unfolded", got)
	}
}

func TestPanelOpensOnTheFirstEntryAndMovesWithinIt(t *testing.T) {
	_, g := panelLog(t)
	g.TogglePanel()
	if g.Panel == nil || g.Panel.Sel != (FilterSel{Item: scene.Item{Name: "p"}}) {
		t.Fatalf("opened on %+v", g.Panel)
	}
	g.MovePanel(-1)
	if g.Panel.Sel.Item.Name != "p" {
		t.Error("moved past the top")
	}
	g.MovePanel(100)
	if !g.Panel.Sel.Add {
		t.Errorf("moved to %+v; want + Text", g.Panel.Sel)
	}
	g.TogglePanel()
	if g.Panel != nil {
		t.Error("didn't close")
	}
}

func TestTextRowsAddAndRemove(t *testing.T) {
	_, g := panelLog(t)
	if g.AddText("three") {
		t.Error("selected a row with the panel closed")
	}
	g.TogglePanel()
	if !g.AddText("one") || g.Panel.Sel != (FilterSel{Item: scene.Item{Name: "one", Text: true}}) {
		t.Fatalf("add: sel %+v", g.Panel.Sel)
	}
	if g.RemoveText(FilterSel{Item: scene.Item{Name: "q"}}) {
		t.Error("removed a tag row")
	}
	sel := g.Panel.Sel
	if !g.RemoveText(sel) {
		t.Fatal("didn't remove the text row")
	}
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"p", "q", "text:three", "+"}) {
		t.Errorf("entries after removing one: %q", got)
	}
	g.Select(FilterSel{Item: scene.Item{Name: "three", Text: true}})
	g.RemoveText(g.Panel.Sel) // the last text row
	if !g.Panel.Sel.Add {
		t.Errorf("highlight after removing the last text row: %+v; want + Text", g.Panel.Sel)
	}
}

func TestHideAndOnlyThroughTheLog(t *testing.T) {
	_, g := panelLog(t)
	q := scene.Item{Name: "q"}
	g.ToggleHide(q)
	if !g.c.Filter.Hidden(q) {
		t.Error("q not hidden")
	}
	g.SetCollapsed(FilterSel{Item: scene.Item{Name: "p"}}, true)
	g.PressOnly(scene.Item{Name: "p/a"})
	if !g.FilteredUnder("p", g.Items()) {
		t.Error("p's folded children don't show as filtered")
	}
}
