package ui

import (
	"cmp"
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/config"
)

// compareChars orders characters alphabetically, ignoring case: by world
// id, then by name. The sidebar and the picker both use it.
func compareChars(a, b config.Character) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(a.World), strings.ToLower(b.World)),
		cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		cmp.Compare(key(a.World, a.ID), key(b.World, b.ID)),
	)
}

// allChars is every configured character, in sidebar order.
func (m *Model) allChars() []config.Character {
	var all []config.Character
	if m.cfg != nil {
		for _, w := range m.cfg.Worlds {
			all = append(all, w.Characters...)
		}
	}
	slices.SortFunc(all, compareChars)
	return all
}

// find looks up a configured character by key.
func (m *Model) find(k string) (config.Character, bool) {
	if m.cfg != nil {
		for _, w := range m.cfg.Worlds {
			for _, ch := range w.Characters {
				if key(ch.World, ch.ID) == k {
					return ch, true
				}
			}
		}
	}
	return config.Character{}, false
}

// sortOrder puts the open characters in sidebar order.
func (m *Model) sortOrder() {
	slices.SortFunc(m.order, func(a, b string) int { return compareChars(m.chars[a].ch, m.chars[b].ch) })
}

// open adds configured character k to the sidebar, preloading its
// history, and returns it; an open character is returned as is. It
// returns nil if k isn't configured. It doesn't connect. With nothing
// active, k becomes active.
func (m *Model) open(k string) *charState {
	if cs := m.chars[k]; cs != nil {
		return cs
	}
	ch, ok := m.find(k)
	if !ok {
		return nil
	}
	cs := &charState{key: k, ch: ch, in: NewInput()}
	if err := cs.compile(); err != nil {
		m.setStatus(true, "%s: %v", k, err)
	}
	m.chars[k] = cs
	m.order = append(m.order, k)
	m.sortOrder()
	m.preload(cs)
	if m.active == "" {
		m.active = k
	}
	return cs
}

// close stops an open character's session and removes it from the
// sidebar. If it was active, the next one down becomes active, or the one
// above if it was last.
func (m *Model) close(k string) {
	i := slices.Index(m.order, k)
	if i < 0 {
		return
	}
	if cs := m.chars[k]; cs.cancel != nil {
		cs.cancel()
	}
	delete(m.chars, k)
	m.order = slices.Delete(m.order, i, i+1)
	if m.active != k {
		return
	}
	m.active = ""
	if len(m.order) > 0 {
		m.switchTo(m.order[min(i, len(m.order)-1)])
	}
}
