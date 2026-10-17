package scene

import (
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/config"
)

// Item is a row in log mode's filter panel: a tag, a term a line's text
// must contain, or a group, Categories or Flags, the parent of every
// top-level tag of its kind.
type Item struct {
	Name  string // the tag, or the term as typed; "" for a group
	Text  bool
	Group string // CategoryGroup or FlagGroup on a group; "" otherwise
}

// The groups' Item.Group.
const (
	CategoryGroup = "category" //str:ok a wire value
	FlagGroup     = "flag"     //str:ok a wire value
)

// kind is which of the filter's kinds an item is: Hide and Only on one
// never touch another's. A tag is a category, what a line is, unless
// it's a flag, a fact about it (see config.Rules.IsFlag).
type kind int

const (
	categoryKind kind = iota
	flagKind
	textKind
)

// Parent is tag's parent up its slashes, or "" at the top.
func Parent(tag string) string {
	i := strings.LastIndex(tag, "/")
	if i < 0 {
		return ""
	}
	return tag[:i]
}

// TagItems is every tag in tags and every parent up their slashes, once
// each, parents before children and siblings alphabetically.
func TagItems(tags []string) []Item {
	seen := map[string]bool{}
	var names []string
	for _, t := range tags {
		for n := t; n != "" && !seen[n]; n = Parent(n) {
			seen[n] = true
			names = append(names, n)
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		return slices.Compare(strings.Split(a, "/"), strings.Split(b, "/"))
	})
	out := make([]Item, len(names))
	for i, n := range names {
		out[i] = Item{Name: n}
	}
	return out
}

// Filter is log mode's filter: a Hide flag per item, at most one item
// with Only, and the text terms. Its zero value filters nothing. Hiding
// a tag hides its descendants; flags are always the item's own, so the
// panel shows exactly what is hidden.
type Filter struct {
	hidden  map[Item]bool // only true entries
	only    Item
	hasOnly bool
	texts   []string
	known   map[Item]bool // items Sync has seen
	dflt    map[Item]bool // items hidden by default; see SetDefault
	isFlag  func(tag string) bool
}

// SetFlags sets which tags are flags; until it's called, only
// config.DefaultFlags are.
func (f *Filter) SetFlags(isFlag func(tag string) bool) { f.isFlag = isFlag }

// flag reports whether tag is a flag.
func (f *Filter) flag(tag string) bool {
	if f.isFlag == nil {
		return config.Rules{}.IsFlag(tag)
	}
	return f.isFlag(tag)
}

// kind is it's kind.
func (f *Filter) kind(it Item) kind {
	switch {
	case it.Text:
		return textKind
	case it.Group == FlagGroup, it.Group == "" && f.flag(it.Name):
		return flagKind
	}
	return categoryKind
}

// ParentOf is it's parent: the tag up its slashes, or for a top-level
// tag its kind's group. Groups and texts have none.
func (f *Filter) ParentOf(it Item) (Item, bool) {
	switch {
	case it.Text || it.Group != "":
		return Item{}, false
	case Parent(it.Name) != "":
		return Item{Name: Parent(it.Name)}, true
	case f.kind(it) == flagKind:
		return Item{Group: FlagGroup}, true
	}
	return Item{Group: CategoryGroup}, true
}

// Under reports whether b is a descendant of a.
func (f *Filter) Under(a, b Item) bool {
	for p, ok := f.ParentOf(b); ok; p, ok = f.ParentOf(p) {
		if p == a {
			return true
		}
	}
	return false
}

// related reports whether a and b are the same item, or one is under
// the other.
func (f *Filter) related(a, b Item) bool { return a == b || f.Under(a, b) || f.Under(b, a) }

// hiddenUp reports whether it or any ancestor is hidden.
func (f *Filter) hiddenUp(it Item) bool {
	for ok := true; ok; it, ok = f.ParentOf(it) {
		if f.hidden[it] {
			return true
		}
	}
	return false
}

func (f *Filter) init() {
	if f.hidden == nil {
		f.hidden = map[Item]bool{}
	}
	if f.known == nil {
		f.known = map[Item]bool{}
	}
	if f.dflt == nil {
		f.dflt = map[Item]bool{}
	}
}

// SetDefault sets whether it is hidden by default, and hides or shows it
// to match, unless an Only of its kind it isn't related to keeps it
// hidden. Clearing its kind's filter puts it back to its default.
func (f *Filter) SetDefault(it Item, hide bool) {
	f.init()
	if hide {
		f.dflt[it], f.hidden[it] = true, true
		return
	}
	delete(f.dflt, it)
	if !f.hasOnly || f.kind(f.only) != f.kind(it) || f.related(f.only, it) {
		delete(f.hidden, it)
	}
}

// Hidden reports it's Hide flag.
func (f *Filter) Hidden(it Item) bool { return f.hidden[it] }

// Only is the item with Only, if any.
func (f *Filter) Only() (Item, bool) { return f.only, f.hasOnly }

// Texts is the text rows, in the order added.
func (f *Filter) Texts() []Item {
	out := make([]Item, len(f.texts))
	for i, t := range f.texts {
		out[i] = Item{Name: t, Text: true}
	}
	return out
}

// Tags is every tag with Hide or Only, sorted, so the panel can
// list them even when none of their lines is loaded.
func (f *Filter) Tags() []string {
	var out []string
	for it := range f.hidden {
		if !it.Text && it.Group == "" {
			out = append(out, it.Name)
		}
	}
	if f.hasOnly && !f.only.Text && f.only.Group == "" && !f.hidden[f.only] {
		out = append(out, f.only.Name)
	}
	slices.Sort(out)
	return out
}

// Active reports whether anything has Only, or is hidden or shown other
// than by default.
func (f *Filter) Active() bool {
	if f.hasOnly {
		return true
	}
	for it := range f.hidden {
		if !f.dflt[it] {
			return true
		}
	}
	for it := range f.dflt {
		if !f.hidden[it] {
			return true
		}
	}
	return false
}

// ResetSeen forgets which items Sync has seen, so a new log-mode
// session, which loads its lines afresh, lights each item by the filter
// as it stands now.
func (f *Filter) ResetSeen() { f.known = nil }

// Sync takes the current items. One it hasn't seen before is hidden when
// an ancestor is, or when an Only of its kind is set that it isn't
// related to, so a tag arriving late obeys the filter already set.
func (f *Filter) Sync(items []Item) {
	f.init()
	for _, it := range items {
		if f.known[it] {
			continue
		}
		f.known[it] = true
		if p, ok := f.ParentOf(it); ok && f.hidden[p] {
			f.hidden[it] = true
		} else if f.hasOnly && f.kind(f.only) == f.kind(it) && !f.related(f.only, it) {
			f.hidden[it] = true
		}
	}
}

// hide sets Hide on it and its descendants.
func (f *Filter) hide(it Item, items []Item) {
	for _, o := range items {
		if o == it || f.Under(it, o) {
			f.hidden[o] = true
		}
	}
	f.hidden[it] = true
}

// clearKind drops the Only and every Hide of kind k, then hides that
// kind's items hidden by default.
func (f *Filter) clearKind(k kind) {
	f.hasOnly = false
	for it := range f.hidden {
		if f.kind(it) == k {
			delete(f.hidden, it)
		}
	}
	for it := range f.dflt {
		if f.kind(it) == k {
			f.hidden[it] = true
		}
	}
}

// ToggleHide is the Hide button. On the item with Only it clears that
// kind's filter and hides just it. Unhiding clears the Only, the item's
// descendants and its ancestors.
func (f *Filter) ToggleHide(it Item, items []Item) {
	f.init()
	switch {
	case f.hasOnly && f.only == it:
		f.clearKind(f.kind(it))
		f.hide(it, items)
	case f.hidden[it]:
		f.hasOnly = false
		for o := range f.hidden { // loaded or not
			if f.Under(it, o) {
				delete(f.hidden, o)
			}
		}
		delete(f.hidden, it)
		for p, ok := f.ParentOf(it); ok; p, ok = f.ParentOf(p) {
			delete(f.hidden, p)
		}
	default:
		f.hide(it, items)
	}
}

// PressOnly is the Only button: it gives it Only and hides every other
// item of its kind except its ancestors and descendants. Pressed on the
// item that has Only, it clears that kind's filter.
func (f *Filter) PressOnly(it Item, items []Item) {
	f.init()
	if f.hasOnly && f.only == it {
		f.clearKind(f.kind(it))
		return
	}
	f.clearKind(f.kind(it))
	f.only, f.hasOnly = it, true
	for o := range f.hidden { // hidden by default, but Only shows it
		if f.related(it, o) {
			delete(f.hidden, o)
		}
	}
	for _, o := range items {
		if f.kind(o) == f.kind(it) && !f.related(it, o) {
			f.hidden[o] = true
		}
	}
}

// AddText adds a text row with Only. A blank term or one already listed
// adds nothing and reports false.
func (f *Filter) AddText(term string, items []Item) bool {
	f.init()
	term = strings.TrimSpace(term)
	if term == "" || slices.Contains(f.texts, term) {
		return false
	}
	f.texts = append(f.texts, term)
	it := Item{Name: term, Text: true}
	f.known[it] = true
	f.PressOnly(it, append(slices.Clone(items), f.Texts()...))
	return true
}

// RemoveText drops a text row. If it had Only, the Only goes; the Hide
// flags it set stay.
func (f *Filter) RemoveText(term string) {
	f.init()
	it := Item{Name: term, Text: true}
	f.texts = slices.DeleteFunc(f.texts, func(t string) bool { return t == term })
	delete(f.hidden, it)
	delete(f.known, it)
	if f.hasOnly && f.only == it {
		f.hasOnly = false
	}
}

// Visible reports whether a line shows: none of its tags or their
// ancestors, their group included, is hidden, it matches no hidden text
// row, and, with an Only set, it carries that tag (or one under it, so
// for a group any tag of its kind) or matches that text. lower is the
// line's plain text, lowercased.
func (f *Filter) Visible(tags []string, lower string) bool {
	for _, t := range tags {
		if f.hiddenUp(Item{Name: t}) {
			return false
		}
	}
	for it := range f.hidden {
		if it.Text && strings.Contains(lower, strings.ToLower(it.Name)) {
			return false
		}
	}
	if !f.hasOnly {
		return true
	}
	if f.only.Text {
		return strings.Contains(lower, strings.ToLower(f.only.Name))
	}
	return slices.ContainsFunc(tags, func(t string) bool {
		return f.only == Item{Name: t} || f.Under(f.only, Item{Name: t})
	})
}
