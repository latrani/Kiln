package scene

import (
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/find"
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

// Filter is log mode's filter: a Hide flag per item, the items with
// Only, and the text terms. Its zero value filters nothing. Hiding a tag
// hides its descendants; flags are always the item's own, so the panel
// shows exactly what is hidden.
type Filter struct {
	hidden map[Item]bool // only true entries
	only   map[Item]bool // only true entries
	texts  []string
	known  map[Item]bool // items Sync has seen
	isFlag func(tag string) bool
	terms  map[string]find.Term // text rows, ready to match, by name
}

// term is the text row named name, ready to match.
func (f *Filter) term(name string) find.Term {
	t, ok := f.terms[name]
	if !ok {
		if f.terms == nil {
			f.terms = map[string]find.Term{}
		}
		t = find.New(name)
		f.terms[name] = t
	}
	return t
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
	if f.only == nil {
		f.only = map[Item]bool{}
	}
}

// Hide sets it's Hide flag, as the Hide button does on an item with
// nothing under it loaded. A character's filter starts with sent hidden
// this way; from then on it's an ordinary Hide.
func (f *Filter) Hide(it Item) {
	f.init()
	f.hidden[it] = true
}

// Hidden reports it's Hide flag.
func (f *Filter) Hidden(it Item) bool { return f.hidden[it] }

// Only reports whether it has Only.
func (f *Filter) Only(it Item) bool { return f.only[it] }

// onlyOf is the items of kind k with Only.
func (f *Filter) onlyOf(k kind) []Item {
	var out []Item
	for it := range f.only {
		if f.kind(it) == k {
			out = append(out, it)
		}
	}
	return out
}

// soloed reports whether kind k has any Only, and it is unrelated to all
// of them, so Only keeps it hidden.
func (f *Filter) soloed(it Item) bool {
	os := f.onlyOf(f.kind(it))
	return len(os) > 0 && !slices.ContainsFunc(os, func(o Item) bool { return f.related(o, it) })
}

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
	for it := range f.only {
		if !it.Text && it.Group == "" && !f.hidden[it] {
			out = append(out, it.Name)
		}
	}
	slices.Sort(out)
	return out
}

// ResetSeen forgets which items Sync has seen, so a new log-mode
// session, which loads its lines afresh, lights each item by the filter
// as it stands now.
func (f *Filter) ResetSeen() { f.known = nil }

// Sync takes the current items. One it hasn't seen before is hidden when
// an ancestor is, or when its kind has Only on items it isn't related
// to, so a tag arriving late obeys the filter already set.
func (f *Filter) Sync(items []Item) {
	f.init()
	for _, it := range items {
		if f.known[it] {
			continue
		}
		f.known[it] = true
		if p, ok := f.ParentOf(it); ok && f.hidden[p] {
			f.hidden[it] = true
		} else if f.soloed(it) {
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

// clearKind drops every Only and Hide of kind k.
func (f *Filter) clearKind(k kind) {
	for it := range f.only {
		if f.kind(it) == k {
			delete(f.only, it)
		}
	}
	for it := range f.hidden {
		if f.kind(it) == k {
			delete(f.hidden, it)
		}
	}
}

// unsolo takes it's Only away. With no Only left in its kind, that
// kind's filter clears; otherwise every item of the kind no Only left is
// related to is hidden, it among them.
func (f *Filter) unsolo(it Item, items []Item) {
	delete(f.only, it)
	if len(f.onlyOf(f.kind(it))) == 0 {
		f.clearKind(f.kind(it))
		return
	}
	for _, o := range items {
		if f.soloed(o) {
			f.hidden[o] = true
		}
	}
	if f.soloed(it) {
		f.hidden[it] = true
	}
}

// solo gives it Only and shows it, its descendants and its ancestors.
// The first Only in a kind clears the kind's filter, then hides every
// other item of it.
func (f *Filter) solo(it Item, items []Item) {
	if len(f.onlyOf(f.kind(it))) == 0 {
		f.clearKind(f.kind(it))
		for _, o := range items {
			if f.kind(o) == f.kind(it) && !f.related(it, o) {
				f.hidden[o] = true
			}
		}
	}
	f.only[it] = true
	for o := range f.hidden { // loaded or not
		if f.related(it, o) {
			delete(f.hidden, o)
		}
	}
}

// ToggleHide is the Hide button. On an item with Only it takes the Only
// away and hides just it. Unhiding shows the item, its descendants and
// its ancestors, and while its kind has Only, gives it Only too.
func (f *Filter) ToggleHide(it Item, items []Item) {
	f.init()
	switch {
	case f.only[it]:
		f.unsolo(it, items)
		f.hide(it, items)
	case f.hidden[it]:
		if len(f.onlyOf(f.kind(it))) > 0 {
			f.solo(it, items)
			return
		}
		for o := range f.hidden { // loaded or not
			if f.related(it, o) {
				delete(f.hidden, o)
			}
		}
	default:
		f.hide(it, items)
	}
}

// PressOnly is the Only button: it gives it Only, alongside any others
// of its kind, and shows it. The first Only in a kind hides every other
// item of the kind except its ancestors and descendants. Pressed on an
// item with Only, it takes the Only away, hiding the item while others
// of its kind have Only and clearing the kind's filter when none do.
func (f *Filter) PressOnly(it Item, items []Item) {
	f.init()
	if f.only[it] {
		f.unsolo(it, items)
		return
	}
	f.solo(it, items)
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
	delete(f.only, it)
}

// Visible reports whether a line shows: none of its tags or their
// ancestors, their group included, is hidden, it matches no hidden text
// row (as find matches: see package find), and for each kind with Only,
// it carries a tag with Only (or one under it, so for a group any tag of
// its kind) or matches a text with Only. lower is the line's plain text,
// lowercased.
func (f *Filter) Visible(tags []string, lower string) bool {
	for _, t := range tags {
		if f.hiddenUp(Item{Name: t}) {
			return false
		}
	}
	var folded find.Folded // the line, folded once there's a text row to match
	text := func() find.Folded {
		if folded == nil {
			folded = find.Fold(lower)
		}
		return folded
	}
	for it := range f.hidden {
		if it.Text && f.term(it.Name).In(text()) {
			return false
		}
	}
	for _, k := range []kind{categoryKind, flagKind, textKind} {
		os := f.onlyOf(k)
		if len(os) > 0 && !slices.ContainsFunc(os, func(o Item) bool { return f.matches(o, tags, text) }) {
			return false
		}
	}
	return true
}

// matches reports whether a line with tags and text carries o (or a tag
// under it), or for a text row, matches it.
func (f *Filter) matches(o Item, tags []string, text func() find.Folded) bool {
	if o.Text {
		return f.term(o.Name).In(text())
	}
	return slices.ContainsFunc(tags, func(t string) bool {
		return o == Item{Name: t} || f.Under(o, Item{Name: t})
	})
}
