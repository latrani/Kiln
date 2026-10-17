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

// p is the tag p, the parent in tagWorld.
var p = scene.Item{Name: "p"}

func panelLog(t *testing.T) (*App, *Log) {
	t.Helper()
	a := sessionApp(t, map[string]string{"fm": tagWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, logDay, "A one", "B two", "Q three")
	return a, a.OpenLog("fm/kit")
}

// entryNames lists entries as names, a group as "[category]" or
// "[flag]", a text row as "text:…", and one folded away in parentheses.
func entryNames(es []PanelEntry) []string {
	var out []string
	for _, e := range es {
		n := e.Item.Name
		if e.Item.Group != "" {
			n = "[" + e.Item.Group + "]"
		}
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
	if got := entryNames(es); !slices.Equal(got, []string{"[category]", "p", "(p/a)", "(p/b)", "q", "sent"}) || !es[0].Parent || es[0].Depth != 0 || !es[1].Parent || es[1].Depth != 1 || es[2].Depth != 2 {
		t.Fatalf("entries %q, %+v; want Categories open, p folded, its children listed folded away", got, es[:3])
	}
	g.Fold(p, false)
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"[category]", "p", "p/a", "p/b", "q", "sent"}) {
		t.Errorf("unfolded: %q", got)
	}
	g.Fold(scene.Item{Name: "q"}, true) // no children: nothing to fold
	if g.c.Collapsed[scene.Item{Name: "q"}] {
		t.Error("folded a tag with no children")
	}
	g.Fold(scene.Item{Group: scene.CategoryGroup}, true)
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"[category]", "(p)", "(p/a)", "(p/b)", "(q)", "(sent)"}) {
		t.Errorf("Categories folded: %q", got)
	}
}

func TestFoldsOutliveLogMode(t *testing.T) {
	a, g := panelLog(t)
	g.Entries()
	g.Fold(p, false)
	a.CloseLog("fm/kit")
	g = a.OpenLog("fm/kit")
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"[category]", "p", "p/a", "p/b", "q", "sent"}) {
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
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"[category]", "p", "(p/a)", "(p/b)", "q", "sent", "text:three"}) {
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
	g.Fold(p, true)
	g.PressOnly(scene.Item{Name: "p/a"})
	if !g.FilteredUnder(p, g.Items()) || !g.FilteredUnder(scene.Item{Group: scene.CategoryGroup}, g.Items()) {
		t.Error("p's folded children don't show as filtered")
	}
}

// Flags come after the categories: self always, and a tag the world's
// flags list names, with everything under it.
func TestPanelListsFlagsAfterCategories(t *testing.T) {
	a := sessionApp(t, map[string]string{"fm": "flags = [\"p\"]\n" + tagWorld})
	openAll(t, a, "fm/kit")
	l, _ := a.LogLayout(a.Char("fm/kit").Ch)
	kilntest.WriteLog(t, l, logDay, "A one", "B two", "Q three", "Rook waves to Kit.")
	g := a.OpenLog("fm/kit")
	g.Entries()
	g.Fold(p, false)
	if got := entryNames(g.Entries()); !slices.Equal(got, []string{"[category]", "q", "sent", "[flag]", "p", "p/a", "p/b", "self"}) {
		t.Errorf("entries %q; want Categories and the categories, then Flags and the flags", got)
	}
}
