package scene

import (
	"slices"
	"strings"
)

// Item is a row in log mode's filter panel: a tag, or a term a line's
// text must contain.
type Item struct {
	Name string // the tag, or the term as typed
	Text bool
}

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

// under reports whether b is a descendant of a (tags only).
func under(a, b Item) bool {
	return !a.Text && !b.Text && strings.HasPrefix(b.Name, a.Name+"/")
}

// related reports whether a and b are the same item, or one is under
// the other.
func related(a, b Item) bool { return a == b || under(a, b) || under(b, a) }

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
}

func (f *Filter) init() {
	if f.hidden == nil {
		f.hidden = map[Item]bool{}
	}
	if f.known == nil {
		f.known = map[Item]bool{}
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

// Active reports whether anything is hidden or has Only.
func (f *Filter) Active() bool { return len(f.hidden) > 0 || f.hasOnly }

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
		if !it.Text && f.hidden[Item{Name: Parent(it.Name)}] {
			f.hidden[it] = true
		} else if f.hasOnly && f.only.Text == it.Text && !related(f.only, it) {
			f.hidden[it] = true
		}
	}
}

// hide sets Hide on it and its descendants.
func (f *Filter) hide(it Item, items []Item) {
	for _, o := range items {
		if o == it || under(it, o) {
			f.hidden[o] = true
		}
	}
	f.hidden[it] = true
}

// clearKind drops the Only and every Hide of text's kind.
func (f *Filter) clearKind(text bool) {
	f.hasOnly = false
	for it := range f.hidden {
		if it.Text == text {
			delete(f.hidden, it)
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
		f.clearKind(it.Text)
		f.hide(it, items)
	case f.hidden[it]:
		f.hasOnly = false
		for o := range f.hidden { // loaded or not
			if under(it, o) {
				delete(f.hidden, o)
			}
		}
		delete(f.hidden, it)
		if !it.Text {
			for p := Parent(it.Name); p != ""; p = Parent(p) {
				delete(f.hidden, Item{Name: p})
			}
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
		f.clearKind(it.Text)
		return
	}
	f.clearKind(it.Text)
	f.only, f.hasOnly = it, true
	for _, o := range items {
		if o.Text == it.Text && !related(it, o) {
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
// ancestors is hidden, it matches no hidden text row, and, with an Only
// set, it carries that tag (or one under it) or matches that text. lower
// is the line's plain text, lowercased.
func (f *Filter) Visible(tags []string, lower string) bool {
	for _, t := range tags {
		for n := t; n != ""; n = Parent(n) {
			if f.hidden[Item{Name: n}] {
				return false
			}
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
	for _, t := range tags {
		if t == f.only.Name || strings.HasPrefix(t, f.only.Name+"/") {
			return true
		}
	}
	return false
}
