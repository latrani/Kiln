package scene

import (
	"slices"
	"strings"
	"testing"
)

func tag(n string) Item  { return Item{Name: n} }
func text(n string) Item { return Item{Name: n, Text: true} }

func TestTagItemsAddsParentsInTreeOrder(t *testing.T) {
	got := TagItems([]string{"rp", "page/out", "page-x", "page/in", "page/in"})
	want := []Item{tag("page"), tag("page/in"), tag("page/out"), tag("page-x"), tag("rp")}
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
	return append(TagItems([]string{"page/in", "page/out", "rp"}), text("lighthouse"))
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
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page/out", "rp", "lighthouse"}) {
		t.Errorf("hidden = %v (page is page/in's parent; lighthouse is the other kind)", got)
	}
	if !f.Only(tag("page/in")) {
		t.Error("page/in has no Only")
	}
}

func TestOnlyAgainClearsItsKind(t *testing.T) {
	var f Filter
	its := items()
	f.ToggleHide(text("lighthouse"), its)
	f.PressOnly(tag("rp"), its)
	f.PressOnly(tag("rp"), its)
	if f.Only(tag("rp")) {
		t.Error("Only still set")
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"lighthouse"}) {
		t.Errorf("hidden = %v, want the text hide kept", got)
	}
}

func TestOnlyAddsASecondItem(t *testing.T) {
	var f Filter
	its := items()
	f.PressOnly(tag("rp"), its)
	f.PressOnly(tag("page"), its)
	if !f.Only(tag("rp")) || !f.Only(tag("page")) {
		t.Errorf("Only: rp %v, page %v; want both", f.Only(tag("rp")), f.Only(tag("page")))
	}
	if got := hiddenSet(&f, its); got != nil {
		t.Errorf("hidden = %v", got)
	}
	if f.Visible(nil, "") || !f.Visible([]string{"page/in"}, "") || !f.Visible([]string{"rp"}, "") {
		t.Error("want lines with either, and none with neither")
	}
}

// Only pressed again on one of several takes it out, hiding it; on the
// last, the kind's filter clears.
func TestOnlyAgainLeavesTheSet(t *testing.T) {
	var f Filter
	its := items()
	f.PressOnly(tag("rp"), its)
	f.PressOnly(tag("page"), its)
	f.PressOnly(tag("page"), its)
	if f.Only(tag("page")) || !f.Only(tag("rp")) {
		t.Errorf("Only: page %v, rp %v", f.Only(tag("page")), f.Only(tag("rp")))
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page", "page/in", "page/out"}) {
		t.Errorf("hidden = %v", got)
	}
	f.PressOnly(tag("rp"), its)
	if f.Only(tag("rp")) || hiddenSet(&f, its) != nil || !f.Visible(nil, "") {
		t.Errorf("the last Only out should clear: hidden %v", hiddenSet(&f, its))
	}

	var g Filter // an ancestor out, its soloed child stays
	g.PressOnly(tag("page/in"), its)
	g.PressOnly(tag("page"), its)
	g.PressOnly(tag("page"), its)
	if got := hiddenSet(&g, its); !slices.Equal(got, []string{"page/out", "rp"}) {
		t.Errorf("page out, page/in left: hidden = %v", got)
	}
}

func TestHideOnTheOnlyItemFlipsIt(t *testing.T) {
	var f Filter
	its := items()
	f.PressOnly(tag("page"), its)
	f.ToggleHide(tag("page"), its)
	if f.Only(tag("page")) {
		t.Error("Only still set")
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page", "page/in", "page/out"}) {
		t.Errorf("hidden = %v", got)
	}

	var g Filter // one of two: the other keeps its Only
	g.PressOnly(tag("page"), its)
	g.PressOnly(tag("rp"), its)
	g.ToggleHide(tag("page"), its)
	if g.Only(tag("page")) || !g.Only(tag("rp")) || !g.Hidden(tag("page")) {
		t.Errorf("Only: page %v, rp %v; page hidden %v", g.Only(tag("page")), g.Only(tag("rp")), g.Hidden(tag("page")))
	}
}

// Unhiding while its kind has Only gives it Only too, so a line with
// none of them stays hidden.
func TestUnhidingJoinsOnly(t *testing.T) {
	var f Filter
	its := items()
	f.PressOnly(tag("rp"), its)
	f.ToggleHide(tag("page/in"), its)
	if !f.Only(tag("rp")) || !f.Only(tag("page/in")) {
		t.Errorf("Only: rp %v, page/in %v; want both", f.Only(tag("rp")), f.Only(tag("page/in")))
	}
	if got := hiddenSet(&f, its); !slices.Equal(got, []string{"page/out"}) {
		t.Errorf("hidden = %v", got)
	}
	if f.Visible(nil, "") || !f.Visible([]string{"page", "page/in"}, "") {
		t.Error("an untagged line should stay hidden, and page/in show")
	}
}

func TestSyncHidesNewcomers(t *testing.T) {
	var f Filter
	its := items()
	f.Sync(its)
	f.ToggleHide(tag("page"), its)
	more := append(TagItems([]string{"page/in", "page/out", "page/x", "say", "rp"}), text("lighthouse"))
	f.Sync(more)
	if !f.Hidden(tag("page/x")) || f.Hidden(tag("say")) {
		t.Errorf("under a hidden parent: page/x %v, say %v", f.Hidden(tag("page/x")), f.Hidden(tag("say")))
	}

	var g Filter
	g.Sync(its)
	g.PressOnly(tag("rp"), its)
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
	if !f.Only(text("Rook")) || !f.Only(text("lighthouse")) {
		t.Errorf("a new text row joins Only: Rook %v, lighthouse %v", f.Only(text("Rook")), f.Only(text("lighthouse")))
	}
	f.RemoveText("Rook")
	if f.Only(text("Rook")) || !f.Only(text("lighthouse")) || len(f.Texts()) != 1 {
		t.Error("RemoveText: Rook's Only gone, lighthouse's kept")
	}

	var g Filter
	g.AddText("lighthouse", its)
	g.ToggleHide(text("lighthouse"), its)
	g.AddText("Rook", its)
	if !g.Only(text("Rook")) || !g.Hidden(text("lighthouse")) {
		t.Errorf("the first text Only: Rook %v, lighthouse hidden %v", g.Only(text("Rook")), g.Hidden(text("lighthouse")))
	}
	g.RemoveText("Rook")
	if !g.Hidden(text("lighthouse")) {
		t.Error("RemoveText: the Hide its Only set stays")
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
		{"hidden tag", func(f *Filter) { f.ToggleHide(tag("rp"), its) }, []string{"rp"}, "", false},
		{"uncategorized survives hide", func(f *Filter) { f.ToggleHide(tag("rp"), its) }, nil, "", true},
		{"hidden ancestor, tag not synced", func(f *Filter) { f.ToggleHide(tag("page"), its) }, []string{"page/new"}, "", false},
		{"only: has it", func(f *Filter) { f.PressOnly(tag("page/in"), its) }, []string{"page", "page/in"}, "", true},
		{"only: parent covers child", func(f *Filter) { f.PressOnly(tag("page"), its) }, []string{"page/out"}, "", true},
		{"only: uncategorized hidden", func(f *Filter) { f.PressOnly(tag("rp"), its) }, nil, "", false},
		{"hide beats only", func(f *Filter) {
			f.PressOnly(tag("page"), its)
			f.ToggleHide(text("lighthouse"), its)
		}, []string{"page"}, "the lighthouse", false},
		{"hidden text", func(f *Filter) { f.ToggleHide(text("lighthouse"), its) }, nil, "the lighthouse is dark", false},
		{"text match ignores case", func(f *Filter) { f.AddText("LightHouse", its[:4]) }, []string{"rp"}, "the lighthouse is dark", true},
		{"only text keeps tag hides separate", func(f *Filter) {
			f.ToggleHide(tag("rp"), its)
			f.PressOnly(text("lighthouse"), its)
		}, []string{"page"}, "the lighthouse", true},
		{"only text, tag hidden", func(f *Filter) {
			f.ToggleHide(tag("rp"), its)
			f.PressOnly(text("lighthouse"), its)
		}, []string{"rp"}, "the lighthouse", false},
		{"only text: no match", func(f *Filter) { f.PressOnly(text("lighthouse"), its) }, nil, "evening", false},
		{"two onlys: the other", func(f *Filter) {
			f.PressOnly(tag("rp"), its)
			f.PressOnly(tag("page"), its)
		}, []string{"rp"}, "", true},
		{"two onlys: uncategorized hidden", func(f *Filter) {
			f.PressOnly(tag("rp"), its)
			f.PressOnly(tag("page"), its)
		}, nil, "", false},
		{"only tag and only text: both", func(f *Filter) {
			f.PressOnly(tag("rp"), its)
			f.PressOnly(text("lighthouse"), its)
		}, []string{"rp"}, "the lighthouse", true},
		{"only tag and only text: just the tag", func(f *Filter) {
			f.PressOnly(tag("rp"), its)
			f.PressOnly(text("lighthouse"), its)
		}, []string{"rp"}, "evening", false},
	}
	for _, c := range cases {
		var f Filter
		c.set(&f)
		if got := f.Visible(c.tags, c.lower); got != c.want {
			t.Errorf("%s: Visible = %v, want %v", c.name, got, c.want)
		}
	}
}

// A new log-mode session sees its items afresh: one known from before
// takes the filter set since, so its button lights.
func TestSyncAfterResetSeenRelights(t *testing.T) {
	var f Filter
	withWhisper := append(TagItems([]string{"rp", "whisper"}), text("lighthouse"))
	f.Sync(withWhisper)
	f.PressOnly(tag("rp"), TagItems([]string{"rp"})) // whisper not loaded now
	f.ResetSeen()
	f.Sync(withWhisper)
	if !f.Hidden(tag("whisper")) {
		t.Error("whisper should light under Only self once seen again")
	}
}

func TestUnhideParentClearsUnloadedChildren(t *testing.T) {
	var f Filter
	f.ToggleHide(tag("page"), TagItems([]string{"page/in", "page/x"}))
	f.ToggleHide(tag("page"), TagItems([]string{"page/in"})) // page/x not loaded now
	if f.Hidden(tag("page/x")) {
		t.Error("unhiding page should clear page/x too")
	}
}

var categories, flags = Item{Group: CategoryGroup}, Item{Group: FlagGroup}

// Categories and Flags are their kinds' top-level parents: Hide and Only
// on one reach every tag of its kind.
func TestGroups(t *testing.T) {
	its := append([]Item{categories}, items()...)
	cases := []struct {
		name string
		set  func(f *Filter)
		tags []string
		want bool
	}{
		{"hide categories: line with none", func(f *Filter) { f.ToggleHide(categories, its) }, nil, true},
		{"hide categories: just a flag", func(f *Filter) { f.ToggleHide(categories, its) }, []string{"self"}, true},
		{"hide categories: tagged line", func(f *Filter) { f.ToggleHide(categories, its) }, []string{"rp"}, false},
		{"only categories: line with none", func(f *Filter) { f.PressOnly(categories, its) }, nil, false},
		{"only categories: tagged line", func(f *Filter) { f.PressOnly(categories, its) }, []string{"page/in"}, true},
		{"only flags: just a category", func(f *Filter) { f.PressOnly(flags, its) }, []string{"page"}, false},
		{"only flags: a flag", func(f *Filter) { f.PressOnly(flags, its) }, []string{"page", "self"}, true},
		{"hide flags: a flag", func(f *Filter) { f.ToggleHide(flags, its) }, []string{"self"}, false},
	}
	for _, c := range cases {
		var f Filter
		c.set(&f)
		if got := f.Visible(c.tags, "x"); got != c.want {
			t.Errorf("%s: Visible = %v, want %v", c.name, got, c.want)
		}
	}
	var f Filter
	f.PressOnly(tag("rp"), its)
	if f.Hidden(categories) || !f.Hidden(tag("page")) {
		t.Errorf("Only on a tag keeps its group lit as an ancestor, and lights its siblings")
	}
	var g Filter
	g.ToggleHide(categories, its)
	if !g.Hidden(tag("page")) || !g.Hidden(tag("rp")) || g.Hidden(tag("self")) {
		t.Errorf("Hide on Categories hides every category, and no flag")
	}
	g.ToggleHide(tag("rp"), its)
	if g.Hidden(categories) || g.Hidden(tag("rp")) || !g.Hidden(tag("page")) {
		t.Errorf("unhiding a category clears its group, leaving its siblings hidden")
	}
}

// A top-level tag seen after its group is hidden arrives hidden; one of
// the other kind doesn't.
func TestHiddenGroupHidesLateTags(t *testing.T) {
	var f Filter
	f.Sync([]Item{categories})
	f.ToggleHide(categories, []Item{categories})
	f.Sync([]Item{categories, tag("say"), tag("self")})
	if !f.Hidden(tag("say")) || f.Hidden(tag("self")) {
		t.Errorf("say hidden %v, self hidden %v; want say hidden under Categories, self not", f.Hidden(tag("say")), f.Hidden(tag("self")))
	}
}

func TestTagsListsFilteredTags(t *testing.T) {
	var f Filter
	its := append([]Item{categories}, items()...)
	f.ToggleHide(tag("page"), its)
	f.PressOnly(tag("rp"), its)
	f.PressOnly(tag("rp"), its) // clears; now only whisper is filtered
	f.ToggleHide(tag("whisper/in"), its)
	if got := f.Tags(); !slices.Equal(got, []string{"whisper/in"}) {
		t.Errorf("Tags = %v", got)
	}
	f.ToggleHide(tag("whisper/in"), its)
	f.PressOnly(categories, its)
	if got := f.Tags(); got != nil {
		t.Errorf("Tags lists only tags, not groups: %v", got)
	}
}

// Hide is an ordinary Hide: clearing its kind's filter clears it.
func TestHideIsOrdinary(t *testing.T) {
	var f Filter
	sent, its := tag("sent"), append(items(), tag("sent"))
	f.Hide(sent)
	if !f.Hidden(sent) {
		t.Fatal("Hide didn't hide sent")
	}
	f.PressOnly(sent, its)
	f.PressOnly(sent, its)
	if got := hiddenSet(&f, its); got != nil {
		t.Errorf("Only on sent and off again: hidden %q, want nothing", got)
	}
}

// Flags (self here) are a kind of their own: Only and Hide on a category
// never touch them, and a line with only flags has no category.
func TestFlagsAreTheirOwnKind(t *testing.T) {
	var f Filter
	self, page := tag("self"), tag("page")
	its := append([]Item{categories}, append(TagItems([]string{"page/in"}), append([]Item{flags}, append(TagItems([]string{"self", "self/x"}), text("lighthouse"))...)...)...)
	f.ToggleHide(categories, its)
	if f.Hidden(self) || f.Hidden(flags) || !f.Hidden(page) {
		t.Errorf("Hide on Categories: self hidden %v, page hidden %v; want just the categories lit", f.Hidden(self), f.Hidden(page))
	}
	if !f.Visible([]string{"self"}, "") || f.Visible([]string{"page", "page/in", "self"}, "") {
		t.Error("a room line mentioning you should show under Hide Categories, and a page mentioning you shouldn't")
	}
	f.ToggleHide(categories, its)
	f.PressOnly(self, its)
	if f.Hidden(page) || f.Hidden(tag("self/x")) || f.Visible([]string{"page"}, "") || !f.Visible([]string{"page", "self"}, "") {
		t.Error("Only on self should show just lines mentioning you, lighting no category")
	}
	f.PressOnly(self, its)
	f.SetFlags(func(t string) bool { return t == "page" || strings.HasPrefix(t, "page/") })
	if !f.Visible([]string{"page/in"}, "") {
		t.Fatal("nothing filtered")
	}
	f.ToggleHide(categories, its)
	if f.Hidden(page) || !f.Visible([]string{"page/in"}, "") || !f.Hidden(self) {
		t.Error("with page a flag and self not, Hide on Categories should light self and leave page")
	}
}
