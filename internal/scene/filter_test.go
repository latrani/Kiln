package scene

import (
	"slices"
	"testing"
)

func tag(n string) Item  { return Item{Name: n} }
func text(n string) Item { return Item{Name: n, Text: true} }

func TestTagItemsAddsParentsInTreeOrder(t *testing.T) {
	got := TagItems([]string{"self", "page/out", "page-x", "page/in", "page/in"})
	want := []Item{tag("page"), tag("page/in"), tag("page/out"), tag("page-x"), tag("self")}
	if !slices.Equal(got, want) {
		t.Errorf("TagItems = %v, want %v", got, want)
	}
}

func TestParent(t *testing.T) {
	for in, want := range map[string]string{"page/in": "page", "a/b/c": "a/b", "page": ""} {
		if got := Parent(in); got != want {
			t.Errorf("Parent(%q) = %q, want %q", in, got, want)
		}
	}
}

// items is the panel's list for these tests: page, page/in, page/out,
// self, and the text row "lighthouse".
func items() []Item {
	return append(TagItems([]string{"page/in", "page/out", "self"}), text("lighthouse"))
}

func hiddenSet(f *Filter, its []Item) []string {
	var out []string
	for _, it := range its {
		if f.Hidden(it) {
			out = append(out, it.Name)
		}
	}
	return out
}

func TestHideParentHidesChildren(t *testing.T) {
	var f Filter
	its := items()
	f.ToggleHide(tag("page"), its)
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page", "page/in", "page/out"}) {
		t.Errorf("hidden = %v", got)
	}
	f.ToggleHide(tag("page/in"), its) // unhide a child: parent clears, sibling stays
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page/out"}) {
		t.Errorf("after unhiding page/in, hidden = %v", got)
	}
	f.ToggleHide(tag("page"), its) // hide the parent again, then unhide it
	f.ToggleHide(tag("page"), its)
	if got := hiddenSet(&f, its); len(got) != 0 {
		t.Errorf("unhiding the parent should unhide its children, hidden = %v", got)
	}
}

func TestOnlyHidesOthersOfItsKind(t *testing.T) {
	var f Filter
	its := items()
	f.ToggleHide(text("lighthouse"), its)
	f.PressOnly(tag("page/in"), its)
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page/out", "self", "lighthouse"}) {
		t.Errorf("hidden = %v (page is page/in's parent; lighthouse is the other kind)", got)
	}
	if o, ok := f.Only(); !ok || o != tag("page/in") {
		t.Errorf("Only = %v, %v", o, ok)
	}
}

func TestOnlyAgainClearsItsKind(t *testing.T) {
	var f Filter
	its := items()
	f.ToggleHide(text("lighthouse"), its)
	f.PressOnly(tag("self"), its)
	f.PressOnly(tag("self"), its)
	if _, ok := f.Only(); ok {
		t.Error("Only still set")
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"lighthouse"}) {
		t.Errorf("hidden = %v, want the text hide kept", got)
	}
}

func TestOnlyMovesToASecondItem(t *testing.T) {
	var f Filter
	its := items()
	f.PressOnly(tag("self"), its)
	f.PressOnly(tag("page"), its)
	if o, _ := f.Only(); o != tag("page") {
		t.Errorf("Only = %v", o)
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"self"}) {
		t.Errorf("hidden = %v", got)
	}
}

func TestHideOnTheOnlyItemFlipsIt(t *testing.T) {
	var f Filter
	its := items()
	f.PressOnly(tag("page"), its)
	f.ToggleHide(tag("page"), its)
	if _, ok := f.Only(); ok {
		t.Error("Only still set")
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page", "page/in", "page/out"}) {
		t.Errorf("hidden = %v", got)
	}
}

func TestUnhidingClearsOnly(t *testing.T) {
	var f Filter
	its := items()
	f.PressOnly(tag("self"), its)
	f.ToggleHide(tag("page/in"), its)
	if _, ok := f.Only(); ok {
		t.Error("Only still set after unhiding page/in")
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page/out"}) {
		t.Errorf("hidden = %v", got)
	}
}

func TestSyncHidesNewcomers(t *testing.T) {
	var f Filter
	its := items()
	f.Sync(its)
	f.ToggleHide(tag("page"), its)
	more := append(TagItems([]string{"page/in", "page/out", "page/x", "say", "self"}), text("lighthouse"))
	f.Sync(more)
	if !f.Hidden(tag("page/x")) || f.Hidden(tag("say")) {
		t.Errorf("under a hidden parent: page/x %v, say %v", f.Hidden(tag("page/x")), f.Hidden(tag("say")))
	}

	var g Filter
	g.Sync(its)
	g.PressOnly(tag("self"), its)
	g.Sync(more)
	if !g.Hidden(tag("say")) || !g.Hidden(tag("page/x")) {
		t.Error("tags arriving under an Only should arrive hidden")
	}
}

func TestTexts(t *testing.T) {
	var f Filter
	its := items()[:4] // tags only
	if !f.AddText("lighthouse", its) || f.AddText("lighthouse", its) || f.AddText("  ", its) {
		t.Fatal("AddText: want true, then false for a duplicate and a blank")
	}
	f.AddText("Rook", its)
	if got := f.Texts(); !slices.Equal(got, []Item{text("lighthouse"), text("Rook")}) {
		t.Errorf("Texts = %v", got)
	}
	if o, _ := f.Only(); o != text("Rook") || !f.Hidden(text("lighthouse")) {
		t.Errorf("a new text row takes Only: Only %v, lighthouse hidden %v", o, f.Hidden(text("lighthouse")))
	}
	f.RemoveText("Rook")
	if _, ok := f.Only(); ok || len(f.Texts()) != 1 || !f.Hidden(text("lighthouse")) {
		t.Errorf("RemoveText: Only cleared, the Hide it set kept")
	}
}

func TestVisible(t *testing.T) {
	its := items()
	cases := []struct {
		name  string
		set   func(f *Filter)
		tags  []string
		lower string
		want  bool
	}{
		{"nothing set", func(*Filter) {}, []string{"page/in"}, "", true},
		{"hidden tag", func(f *Filter) { f.ToggleHide(tag("self"), its) }, []string{"self"}, "", false},
		{"untagged survives hide", func(f *Filter) { f.ToggleHide(tag("self"), its) }, nil, "", true},
		{"hidden ancestor, tag not synced", func(f *Filter) { f.ToggleHide(tag("page"), its) }, []string{"page/new"}, "", false},
		{"only: has it", func(f *Filter) { f.PressOnly(tag("page/in"), its) }, []string{"page", "page/in"}, "", true},
		{"only: parent covers child", func(f *Filter) { f.PressOnly(tag("page"), its) }, []string{"page/out"}, "", true},
		{"only: untagged hidden", func(f *Filter) { f.PressOnly(tag("self"), its) }, nil, "", false},
		{"hide beats only", func(f *Filter) {
			f.PressOnly(tag("page"), its)
			f.ToggleHide(text("lighthouse"), its)
		}, []string{"page"}, "the lighthouse", false},
		{"hidden text", func(f *Filter) { f.ToggleHide(text("lighthouse"), its) }, nil, "the lighthouse is dark", false},
		{"text match ignores case", func(f *Filter) { f.AddText("LightHouse", its[:4]) }, []string{"self"}, "the lighthouse is dark", true},
		{"only text keeps tag hides separate", func(f *Filter) {
			f.ToggleHide(tag("self"), its)
			f.PressOnly(text("lighthouse"), its)
		}, []string{"page"}, "the lighthouse", true},
		{"only text, tag hidden", func(f *Filter) {
			f.ToggleHide(tag("self"), its)
			f.PressOnly(text("lighthouse"), its)
		}, []string{"self"}, "the lighthouse", false},
		{"only text: no match", func(f *Filter) { f.PressOnly(text("lighthouse"), its) }, nil, "evening", false},
	}
	for _, c := range cases {
		var f Filter
		c.set(&f)
		if got := f.Visible(c.tags, c.lower); got != c.want {
			t.Errorf("%s: Visible = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestActive(t *testing.T) {
	var f Filter
	if f.Active() {
		t.Error("zero filter active")
	}
	f.ToggleHide(tag("self"), items())
	if !f.Active() {
		t.Error("a hide should make it active")
	}
}
